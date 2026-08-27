// Ported from internal/spatial. Runs on the render thread, so nothing here
// allocates after construction.

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

#[derive(Clone, Copy, Default)]
struct OnePole {
    a: f32,
    prev: f32,
}

impl OnePole {
    fn new(cutoff: f32, rate: f32) -> Self {
        if cutoff >= rate / 2.0 {
            return Self { a: 1.0, prev: 0.0 };
        }
        Self { a: 1.0 - (-2.0 * std::f32::consts::PI * cutoff / rate).exp(), prev: 0.0 }
    }

    #[inline]
    fn process(&mut self, x: f32) -> f32 {
        self.prev += self.a * (x - self.prev);
        self.prev
    }
}

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
    lfe1: OnePole,
    lfe2: OnePole,
    norm: f32,
    lfe_gain: f32,
    gain: f32,
}

const LFE_CUTOFF: f32 = 120.0;

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
        let mut seed = 0usize;

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
                seed += 1;
                Some(Allpass::new(rate, seed))
            });
        }

        let placed = speakers.iter().filter(|s| !s.lfe).count().max(1);
        Ok(Self {
            speakers: speakers.to_vec(),
            firs,
            decor,
            lfe1: OnePole::new(LFE_CUTOFF, rate),
            lfe2: OnePole::new(LFE_CUTOFF, rate),
            norm: 1.0 / (placed as f32).sqrt(),
            lfe_gain: 0.7,
            gain,
        })
    }

    #[inline]
    pub fn frame(&mut self, l: f32, r: f32) -> (f32, f32) {
        let mid = (l + r) * 0.5;
        let side = (l - r) * 0.5;
        let lfe = self.lfe2.process(self.lfe1.process(mid));

        let mut out_l = 0.0f32;
        let mut out_r = 0.0f32;

        for (i, s) in self.speakers.iter().enumerate() {
            if s.lfe {
                let v = lfe * 0.8 * self.lfe_gain;
                out_l += v;
                out_r += v;
                continue;
            }

            let mut v = if s.azimuth == 0.0 && s.elevation == 0.0 {
                mid * 0.707
            } else if s.elevation > 0.0 {
                signed(side * 0.35, s.azimuth)
            } else if s.azimuth.abs() > 90.0 {
                signed(side * 0.5, s.azimuth)
            } else if s.azimuth.abs() == 90.0 {
                signed(side * 0.45 + mid * 0.15, s.azimuth)
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
