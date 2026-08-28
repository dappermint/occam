// Runs on the render thread, so nothing here allocates after construction.

use crate::hrir::{Response, Set};

#[derive(Clone, Copy)]
pub struct Speaker {
    pub azimuth: f32,
    pub elevation: f32,
    pub lfe: bool,
}

const fn spk(azimuth: f32) -> Speaker {
    Speaker { azimuth, elevation: 0.0, lfe: false }
}
const fn spk_up(azimuth: f32, elevation: f32) -> Speaker {
    Speaker { azimuth, elevation, lfe: false }
}
const LFE: Speaker = Speaker { azimuth: 0.0, elevation: 0.0, lfe: true };

// No height: a listening test could not distinguish it without head tracking,
// and this headset has no IMU.
pub const SURROUND_71: [Speaker; 8] = [
    spk(-30.0), spk(30.0), spk(0.0), LFE,
    spk(-150.0), spk(150.0), spk(-90.0), spk(90.0),
];

pub const SURROUND_714: [Speaker; 12] = [
    spk(-30.0), spk(30.0), spk(0.0), LFE,
    spk(-150.0), spk(150.0), spk(-90.0), spk(90.0),
    spk_up(-45.0, 45.0), spk_up(45.0, 45.0),
    spk_up(-135.0, 45.0), spk_up(135.0, 45.0),
];

// Decorrelates, so several outputs carrying the same signal stop fusing into
// one phantom source.
struct Allpass {
    buf: Vec<f32>,
    at: usize,
    gain: f32,
}

impl Allpass {
    fn new(rate: f32, seed: usize) -> Self {
        let ms = 7.0 + seed as f32 * 3.5;
        let n = ((ms * rate / 1000.0) as usize).max(1);
        Self { buf: vec![0.0; n], at: 0, gain: 0.6 }
    }

    fn reset(&mut self) {
        self.buf.fill(0.0);
        self.at = 0;
    }

    #[inline]
    fn process(&mut self, x: f32) -> f32 {
        let d = self.buf[self.at];
        let y = -self.gain * x + d;
        self.buf[self.at] = x + self.gain * d;
        self.at = (self.at + 1) % self.buf.len();
        y
    }
}

// Direct convolution, not FFT: 256 taps over seven speakers is ~3.6k MACs a
// frame, and partitioned FFT would add latency to save nothing here.
struct Fir {
    left: Vec<f32>,
    right: Vec<f32>,
    hist: Vec<f32>,
    at: usize,
}

impl Fir {
    fn new(r: &Response) -> Self {
        Self {
            left: r.left.clone(),
            right: r.right.clone(),
            hist: vec![0.0; r.left.len()],
            at: 0,
        }
    }

    fn reset(&mut self) {
        self.hist.fill(0.0);
        self.at = 0;
    }

    #[inline]
    fn process(&mut self, x: f32) -> (f32, f32) {
        let n = self.hist.len();
        self.hist[self.at] = x;

        let mut l = 0.0f32;
        let mut r = 0.0f32;
        let mut i = self.at;
        for k in 0..n {
            let v = self.hist[i];
            l += v * self.left[k];
            r += v * self.right[k];
            i = if i == 0 { n - 1 } else { i - 1 };
        }

        self.at += 1;
        if self.at == n {
            self.at = 0;
        }
        (l, r)
    }
}

pub struct Pipeline {
    speakers: Vec<Speaker>,
    firs: Vec<Option<Fir>>,
    decor: Vec<Option<Allpass>>,
    norm: f32,
    peak: f32,
    gain: f32,
}

impl Pipeline {
    pub fn new(speakers: &[Speaker], set: &Set, rate: f32, gain: f32) -> Result<Self, String> {
        if (set.rate as f32 - rate).abs() > 0.5 {
            return Err(format!(
                "measured responses are {} Hz but the device is running at {} Hz",
                set.rate, rate as u32
            ));
        }

        let mut firs = Vec::with_capacity(speakers.len());
        let mut decor = Vec::with_capacity(speakers.len());
        // Keyed by mirrored pair, so Lrs and Rrs share a delay. Different
        // delays across the median plane drag the image sideways.
        let mut pairs: Vec<(i32, i32)> = Vec::new();

        for s in speakers {
            if s.lfe {
                firs.push(None);
                decor.push(None);
                continue;
            }
            let (r, off) = set.nearest(s.azimuth, s.elevation);
            if off > 1.0 {
                return Err(format!(
                    "no measured response within 1 degree of az {:+.0} el {:+.0}, nearest is {:.1} away",
                    s.azimuth, s.elevation, off
                ));
            }
            firs.push(Some(Fir::new(r)));
            decor.push(if is_frontal(s) {
                None
            } else {
                let key = (s.azimuth.abs() as i32, s.elevation as i32);
                let seed = match pairs.iter().position(|k| *k == key) {
                    Some(i) => i + 1,
                    None => {
                        pairs.push(key);
                        pairs.len()
                    }
                };
                Some(Allpass::new(rate, seed))
            });
        }

        let mut pipe = Self {
            speakers: speakers.to_vec(),
            firs,
            decor,
            norm: 1.0,
            peak: 1.0,
            gain,
        };
        let (norm, peak) = pipe.measure();
        pipe.norm = norm;
        pipe.peak = peak;
        Ok(pipe)
    }

    // 1/sqrt(speaker count) is 8 dB too quiet: the front pair carries the whole
    // signal while the surrounds carry only the side component, which is near
    // zero for correlated material. Correlated noise for the same reason.
    fn measure(&mut self) -> (f32, f32) {
        let user = std::mem::replace(&mut self.gain, 1.0);
        let mut seed = 0x2545_F491_4F6C_DD1Du64;
        let mut sum_in = 0.0f64;
        let mut sum_out = 0.0f64;
        let mut peak_in = 0.0f32;
        let mut peak_out = 0.0f32;

        const WARMUP: usize = 4096;
        const MEASURE: usize = 16384;

        for i in 0..(WARMUP + MEASURE) {
            seed = seed.wrapping_mul(6364136223846793005).wrapping_add(1442695040888963407);
            let x = ((seed >> 40) as i32 - 8388608) as f32 / 8388608.0 * 0.5;

            let (l, r) = self.frame(x, x);
            if i >= WARMUP {
                sum_in += (x * x) as f64 * 2.0;
                sum_out += (l * l + r * r) as f64;
                peak_in = peak_in.max(x.abs());
                peak_out = peak_out.max(l.abs()).max(r.abs());
            }
        }

        self.gain = user;

        // The measurement's tail would otherwise leak into real audio.
        for f in self.firs.iter_mut().flatten() {
            f.reset();
        }
        for d in self.decor.iter_mut().flatten() {
            d.reset();
        }

        if sum_out <= 0.0 || peak_in <= 0.0 {
            return (1.0, 1.0);
        }
        // A wild number here means the measurement went wrong, and being quiet
        // beats the alternative.
        let norm = ((sum_in / sum_out).sqrt() as f32).clamp(0.25, 4.0);
        (norm, peak_out / peak_in * norm)
    }

    pub fn norm(&self) -> f32 {
        self.norm
    }

    // Peak amplification after normalising, measured on noise. Above 1.0 means
    // a track mastered to full scale will reach soft_clip's knee.
    pub fn peak(&self) -> f32 {
        self.peak
    }

    #[inline]
    pub fn frame(&mut self, l: f32, r: f32) -> (f32, f32) {
        let side = (l - r) * 0.5;

        let mut out_l = 0.0f32;
        let mut out_r = 0.0f32;

        for (i, s) in self.speakers.iter().enumerate() {
            // Centre and LFE take nothing. Anything the front pair already
            // carries would reach each ear a second time from another
            // direction: in phase below the head shadow, combing above it.
            if s.lfe || (s.azimuth == 0.0 && s.elevation == 0.0) {
                continue;
            }

            let mut v = if s.elevation > 0.0 {
                signed(side * 0.35, s.azimuth)
            } else if s.azimuth.abs() > 90.0 {
                signed(side * 0.5, s.azimuth)
            } else if s.azimuth.abs() == 90.0 {
                signed(side * 0.45, s.azimuth)
            } else if s.azimuth < 0.0 {
                l
            } else {
                r
            };

            if let Some(d) = self.decor[i].as_mut() {
                v = d.process(v);
            }
            if let Some(f) = self.firs[i].as_mut() {
                let (el, er) = f.process(v);
                out_l += el;
                out_r += er;
            }
        }

        let g = self.norm * self.gain;
        (out_l * g, out_r * g)
    }
}

#[inline]
fn signed(v: f32, azimuth: f32) -> f32 {
    if azimuth < 0.0 { -v } else { v }
}

fn is_frontal(s: &Speaker) -> bool {
    s.azimuth.abs() <= 90.0 && s.elevation == 0.0
}

// Loudness parity leaves peaks above full scale, because summing seven
// decorrelated HRIR copies raises crest factor. Rounding those off beats the
// hard clamp that would otherwise catch them. Continuous in value and slope at
// the knee, asymptotic to 1.0, so it never needs a clamp behind it.
#[inline]
pub fn soft_clip(x: f32) -> f32 {
    const KNEE: f32 = 0.7;
    const ROOM: f32 = 1.0 - KNEE;

    let mag = x.abs();
    if mag <= KNEE {
        return x;
    }
    let over = mag - KNEE;
    (KNEE + ROOM * over / (over + ROOM)) * x.signum()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn soft_clip_is_transparent_below_the_knee() {
        for i in 0..=70 {
            let x = i as f32 / 100.0;
            assert_eq!(soft_clip(x), x);
            assert_eq!(soft_clip(-x), -x);
        }
    }

    #[test]
    fn soft_clip_never_reaches_full_scale() {
        for x in [0.71, 1.0, 1.5, 2.12, 10.0, 1e6] {
            assert!(soft_clip(x) < 1.0, "{x} mapped to {}", soft_clip(x));
            assert!(soft_clip(-x) > -1.0);
        }
    }

    #[test]
    fn soft_clip_is_monotonic_and_continuous() {
        let mut prev = soft_clip(0.0);
        let mut x = 0.0f32;
        while x < 4.0 {
            x += 0.001;
            let y = soft_clip(x);
            assert!(y >= prev, "not monotonic at {x}");
            assert!(y - prev < 0.002, "jump at {x}: {prev} -> {y}");
            prev = y;
        }
    }

    const RATE: f32 = 48000.0;
    const N: usize = 8192;

    fn pipeline(speakers: &[Speaker]) -> Pipeline {
        let set = Set::load().expect("hrir blob");
        Pipeline::new(speakers, &set, RATE, 1.0).expect("pipeline")
    }

    fn tone(n: usize, f: f32) -> Vec<f32> {
        (0..n)
            .map(|i| 0.5 * (2.0 * std::f32::consts::PI * f * i as f32 / RATE).sin())
            .collect()
    }

    #[test]
    fn centre_stays_centred() {
        let mono = tone(N, 440.0);
        for speakers in [&SURROUND_71[..], &SURROUND_714[..]] {
            let mut p = pipeline(speakers);
            for i in 0..N {
                let (l, r) = p.frame(mono[i], mono[i]);
                assert!(i < 1024 || (l - r).abs() < 1e-6, "sample {i}: {l} left, {r} right");
            }
        }
    }

    #[test]
    fn swapping_inputs_swaps_outputs() {
        let a = tone(N, 440.0);
        let b = tone(N, 661.0);
        for speakers in [&SURROUND_71[..], &SURROUND_714[..]] {
            let mut p = pipeline(speakers);
            let mut q = pipeline(speakers);
            for i in 0..N {
                let (l, _) = p.frame(a[i], b[i]);
                let (_, r) = q.frame(b[i], a[i]);
                assert!(i < 1024 || (l - r).abs() < 1e-6, "sample {i}: {l} vs mirrored {r}");
            }
        }
    }

    #[test]
    fn centred_content_is_not_coloured() {
        let mut bare = pipeline(&[spk(-30.0), spk(30.0)]);
        let mut full = pipeline(&SURROUND_71[..]);
        let (bn, fnorm) = (bare.norm(), full.norm());

        let mono = tone(N, 440.0);
        for i in 0..N {
            let (bl, _) = bare.frame(mono[i], mono[i]);
            let (fl, _) = full.frame(mono[i], mono[i]);
            assert!(
                i < 1024 || (bl / bn - fl / fnorm).abs() < 1e-6,
                "sample {i}: front pair {} vs 7.1 {}", bl / bn, fl / fnorm);
        }
    }
}
