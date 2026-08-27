// Bindings to src/shim.m. Hand-written because the surface is six functions.

use std::os::raw::{c_char, c_void};

pub type AudioObjectID = u32;
pub type OSStatus = i32;

pub const AUDIO_OBJECT_UNKNOWN: AudioObjectID = 0;

// input is null whenever the tap has nothing this cycle.
pub type RenderFn = extern "C" fn(
    input: *const f32,
    in_channels: u32,
    output: *mut f32,
    out_channels: u32,
    frames: u32,
    ctx: *mut c_void,
);

unsafe extern "C" {
    pub fn occam_find_output_exact(wanted: *const c_char) -> AudioObjectID;
    pub fn occam_list_outputs(out: *mut c_char, cap: u32) -> u32;
    pub fn occam_describe(device: AudioObjectID, input: i32, counts: *mut u32, max: u32) -> u32;
    pub fn occam_device_rate(device: AudioObjectID) -> f64;
    pub fn occam_buffer_frames(device: AudioObjectID) -> u32;
    pub fn occam_aggregate_id() -> AudioObjectID;
    pub fn occam_live_start(
        output: AudioObjectID,
        want_frames: u32,
        render: RenderFn,
        ctx: *mut c_void,
    ) -> OSStatus;
    pub fn occam_live_stop();
}

// CoreAudio status codes are four-character constants, so the raw numbers are
// unreadable.
pub fn status_name(s: OSStatus) -> &'static str {
    match s {
        0 => "ok",
        x if x == fourcc(b"!obj") => "bad object",
        x if x == fourcc(b"!dev") => "bad device",
        x if x == fourcc(b"!dat") => "illegal operation",
        x if x == fourcc(b"nope") => "not running",
        x if x == fourcc(b"what") => "unspecified error",
        x if x == fourcc(b"unop") => "unsupported operation",
        x if x == fourcc(b"prm?") => "not permitted, likely TCC",
        _ => "unknown",
    }
}

const fn fourcc(c: &[u8; 4]) -> OSStatus {
    (((c[0] as u32) << 24) | ((c[1] as u32) << 16) | ((c[2] as u32) << 8) | (c[3] as u32)) as OSStatus
}
