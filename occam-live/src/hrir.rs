// Shares the blob with the Go renderer, so a difference between the two is
// never the data. SADIE II D1, CC BY 4.0, attribution in
// ../../internal/spatial/hrir/LICENSE-SADIE.md.

const BLOB: &[u8] = include_bytes!("../../internal/spatial/hrir/sadie48.bin");
const MAGIC: &[u8; 4] = b"OCHR";

pub struct Response {
    pub azimuth: f32,
    pub elevation: f32,
    pub left: Vec<f32>,
    pub right: Vec<f32>,
}

pub struct Set {
    pub rate: u32,
    pub taps: usize,
    pub responses: Vec<Response>,
}

impl Set {
    pub fn load() -> Result<Self, String> {
        if BLOB.len() < 20 || &BLOB[0..4] != MAGIC {
            return Err("embedded hrir data is not an OCHR blob".into());
        }
        let u32_at = |at: usize| {
            u32::from_le_bytes([BLOB[at], BLOB[at + 1], BLOB[at + 2], BLOB[at + 3]])
        };
        let f32_at = |at: usize| f32::from_bits(u32_at(at));

        if u32_at(4) != 1 {
            return Err(format!("hrir blob version {}, this build reads 1", u32_at(4)));
        }
        let rate = u32_at(8);
        let taps = u32_at(12) as usize;
        let count = u32_at(16) as usize;

        let stride = 8 + taps * 4 * 2;
        if BLOB.len() != 20 + count * stride {
            return Err(format!(
                "hrir blob is {} bytes, expected {}",
                BLOB.len(),
                20 + count * stride
            ));
        }

        let mut responses = Vec::with_capacity(count);
        let mut at = 20;
        for _ in 0..count {
            let azimuth = f32_at(at);
            let elevation = f32_at(at + 4);
            let mut p = at + 8;
            let read = |p: &mut usize| {
                let mut v = Vec::with_capacity(taps);
                for _ in 0..taps {
                    v.push(f32_at(*p));
                    *p += 4;
                }
                v
            };
            let left = read(&mut p);
            let right = read(&mut p);
            responses.push(Response { azimuth, elevation, left, right });
            at += stride;
        }
        Ok(Set { rate, taps, responses })
    }

    // A non-zero result means a layout gained an angle the blob lacks.
    pub fn nearest(&self, azimuth: f32, elevation: f32) -> (&Response, f32) {
        let target = unit(azimuth, elevation);
        let mut best = &self.responses[0];
        let mut best_dot = -2.0f32;
        for r in &self.responses {
            let d = dot(target, unit(r.azimuth, r.elevation));
            if d > best_dot {
                best = r;
                best_dot = d;
            }
        }
        (best, best_dot.clamp(-1.0, 1.0).acos().to_degrees())
    }
}

fn unit(az: f32, el: f32) -> [f32; 3] {
    let (a, e) = (az.to_radians(), el.to_radians());
    [e.cos() * a.cos(), e.cos() * a.sin(), e.sin()]
}

fn dot(a: [f32; 3], b: [f32; 3]) -> f32 {
    a[0] * b[0] + a[1] * b[1] + a[2] * b[2]
}
