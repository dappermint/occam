//! Renders macOS system audio to binaural in realtime, via a CoreAudio
//! process tap rather than a loopback device.

mod dsp;
mod hrir;
mod sys;

use std::ffi::{c_void, CString};
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};

use dsp::{Pipeline, Speaker, SURROUND_71, SURROUND_714};

struct Render {
    pipe: Pipeline,
    // Recorded rather than judged, so a mismatch reports what it actually saw.
    saw_in: AtomicU32,
    saw_out: AtomicU32,
    cycles: AtomicU32,
    // Peaks as millionths, since atomics cannot hold floats.
    in_peak: AtomicU32,
    out_peak: AtomicU32,
}

fn bump_peak(cell: &AtomicU32, v: f32) {
    let scaled = (v.abs() * 1_000_000.0) as u32;
    if scaled > cell.load(Ordering::Relaxed) {
        cell.store(scaled, Ordering::Relaxed);
    }
}

// Render thread: no allocation, no locks, no logging, no panics.
extern "C" fn render(
    input: *const f32,
    in_channels: u32,
    output: *mut f32,
    out_channels: u32,
    frames: u32,
    ctx: *mut c_void,
) {
    if output.is_null() || ctx.is_null() || out_channels == 0 {
        return;
    }
    let state = unsafe { &mut *(ctx as *mut Render) };
    let n = frames as usize;
    let out = unsafe { std::slice::from_raw_parts_mut(output, n * out_channels as usize) };

    // Silence, not whatever was left in the buffer.
    if input.is_null() || in_channels == 0 {
        out.fill(0.0);
        return;
    }
    state.saw_in.store(in_channels, Ordering::Relaxed);
    state.saw_out.store(out_channels, Ordering::Relaxed);
    state.cycles.fetch_add(1, Ordering::Relaxed);

    if out_channels < 2 || in_channels < 2 {
        out.fill(0.0);
        return;
    }

    let inp = unsafe { std::slice::from_raw_parts(input, n * in_channels as usize) };
    let stride = in_channels as usize;

    // Write the pair into the first two channels and leave any others silent,
    // rather than refusing a layout wider than stereo.
    let out_stride = out_channels as usize;
    let mut in_peak = 0.0f32;
    let mut out_peak = 0.0f32;

    for i in 0..n {
        let (sl, sr) = (inp[i * stride], inp[i * stride + 1]);
        in_peak = in_peak.max(sl.abs()).max(sr.abs());

        let (l, r) = state.pipe.frame(sl, sr);
        out_peak = out_peak.max(l.abs()).max(r.abs());
        let at = i * out_stride;
        out[at] = l.clamp(-1.0, 1.0);
        out[at + 1] = r.clamp(-1.0, 1.0);
        for c in 2..out_stride {
            out[at + c] = 0.0;
        }
    }

    bump_peak(&state.in_peak, in_peak);
    bump_peak(&state.out_peak, out_peak);
}

fn main() {
    if let Err(e) = run() {
        eprintln!("occam-live: {e}");
        std::process::exit(1);
    }
}

fn run() -> Result<(), String> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.iter().any(|a| a == "-h" || a == "--help") {
        usage();
        return Ok(());
    }
    if args.iter().any(|a| a == "--list") {
        list_outputs();
        return Ok(());
    }
    let dry = args.iter().any(|a| a == "--dry-run");

    // The full name, not "BlackShark": this headset publishes both a Game and
    // a Chat endpoint and a substring match picked Chat.
    let device = arg(&args, "--device").unwrap_or_else(|| "BlackShark V3 Pro - Game".into());
    let layout_name = arg(&args, "--layout").unwrap_or_else(|| "7.1".into());
    let frames: u32 = arg(&args, "--frames")
        .unwrap_or_else(|| "256".into())
        .parse()
        .map_err(|_| "--frames wants a number".to_string())?;
    let gain_db: f32 = arg(&args, "--gain")
        .unwrap_or_else(|| "-3".into())
        .parse()
        .map_err(|_| "--gain wants a number in dB".to_string())?;

    let speakers: &[Speaker] = match layout_name.as_str() {
        "7.1" => &SURROUND_71,
        "7.1.4" => &SURROUND_714,
        other => return Err(format!("no layout {other:?}, have 7.1 and 7.1.4")),
    };

    let needle = CString::new(device.clone()).map_err(|_| "device name has a nul byte")?;
    let out_device = unsafe { sys::occam_find_output_exact(needle.as_ptr()) };
    if out_device == sys::AUDIO_OBJECT_UNKNOWN {
        return Err(format!("no output device whose name contains {device:?}"));
    }

    let rate = unsafe { sys::occam_device_rate(out_device) };
    if rate <= 0.0 {
        return Err("could not read the device sample rate".into());
    }

    let set = hrir::Set::load()?;
    let pipe = Pipeline::new(speakers, &set, rate as f32, 10f32.powf(gain_db / 20.0))?;

    println!("  device   {device}, {} Hz", rate as u32);
    println!("  layout   {layout_name}, {} speakers", speakers.len());
    println!("  model    SADIE II KU100, {} taps", set.taps);
    println!("  gain     {gain_db:+} dB");
    describe("device out", out_device, false);

    // Leaked: freeing this while CoreAudio might still call back would be a
    // use-after-free on the render thread.
    let state = Box::leak(Box::new(Render {
        pipe,
        saw_in: AtomicU32::new(0),
        saw_out: AtomicU32::new(0),
        cycles: AtomicU32::new(0),
        in_peak: AtomicU32::new(0),
        out_peak: AtomicU32::new(0),
    }));
    let ctx = state as *mut Render as *mut c_void;

    let status = unsafe { sys::occam_live_start(out_device, frames, render, ctx) };
    if status != 0 {
        return Err(format!(
            "starting the tap failed: {status} ({})",
            sys::status_name(status)
        ));
    }

    let agg = unsafe { sys::occam_aggregate_id() };
    describe("agg in ", agg, true);
    describe("agg out", agg, false);
    if dry {
        unsafe { sys::occam_live_stop() };
        println!("\n  dry run, stopped before any audio was muted");
        return Ok(());
    }

    let actual = unsafe { sys::occam_buffer_frames(agg) };
    let ms = actual as f64 / rate * 1000.0;
    println!("  buffer   {actual} frames, {ms:.1} ms per cycle");
    println!("\n  running. system audio is muted at the tap and re-rendered here.");
    println!("  ctrl-c to stop.\n");

    // The tap mutes system output, so failing to stop cleanly leaves the
    // machine silent.
    wait_for_interrupt();

    unsafe { sys::occam_live_stop() };
    let (si, so, cy) = (
        state.saw_in.load(Ordering::Relaxed),
        state.saw_out.load(Ordering::Relaxed),
        state.cycles.load(Ordering::Relaxed),
    );
    let ip = state.in_peak.load(Ordering::Relaxed) as f64 / 1_000_000.0;
    let op = state.out_peak.load(Ordering::Relaxed) as f64 / 1_000_000.0;
    println!("  cycles   {cy}, tap gave {si} ch, device took {so} ch");
    println!("  peaks    in {ip:.4}, out {op:.4}");
    if ip == 0.0 && cy > 0 {
        eprintln!("occam-live: the tap captured pure silence, so it is muted at capture");
    } else if op == 0.0 && ip > 0.0 {
        eprintln!("occam-live: audio came in but the render produced nothing");
    }
    if cy == 0 {
        eprintln!("occam-live: the render callback never ran");
    } else if si < 2 {
        eprintln!("occam-live: the tap only ever delivered {si} channel(s)");
    }
    println!("stopped, audio restored");
    Ok(())
}

fn describe(label: &str, device: sys::AudioObjectID, input: bool) {
    let mut counts = [0u32; 16];
    let n = unsafe {
        sys::occam_describe(device, input as i32, counts.as_mut_ptr(), counts.len() as u32)
    };
    let shown: Vec<String> = counts[..(n as usize).min(counts.len())]
        .iter()
        .map(|c| c.to_string())
        .collect();
    println!("  {label}  {n} buffer(s), channels [{}]", shown.join(", "));
}

fn list_outputs() {
    let mut buf = vec![0i8; 8192];
    let n = unsafe { sys::occam_list_outputs(buf.as_mut_ptr(), buf.len() as u32) };
    let bytes: Vec<u8> = buf[..n as usize].iter().map(|c| *c as u8).collect();
    print!("{}", String::from_utf8_lossy(&bytes));
}

fn arg(args: &[String], name: &str) -> Option<String> {
    let at = args.iter().position(|a| a == name)?;
    args.get(at + 1).cloned()
}

fn wait_for_interrupt() {
    static STOP: AtomicBool = AtomicBool::new(false);

    extern "C" fn on_signal(_: i32) {
        STOP.store(true, Ordering::SeqCst);
    }
    unsafe {
        libc_signal(2, on_signal as *const () as usize); // SIGINT
        libc_signal(15, on_signal as *const () as usize); // SIGTERM
    }
    while !STOP.load(Ordering::SeqCst) {
        std::thread::sleep(std::time::Duration::from_millis(100));
    }
}

unsafe extern "C" {
    #[link_name = "signal"]
    fn libc_signal(sig: i32, handler: usize) -> usize;
}

fn usage() {
    println!(
        "occam-live renders macOS system audio to binaural, in realtime.

  occam-live [--device NAME] [--layout 7.1|7.1.4] [--frames N] [--gain DB]

It taps system output, upmixes what it captures, renders it through measured
head impulses, and plays the result to the headset. The tap mutes the original
output so nothing is heard twice.

  --device NAME   substring of the output device name, default BlackShark
  --layout NAME   7.1 (default) or 7.1.4
  --frames N      buffer size per cycle, default 256
  --gain DB       output gain, default -3
  --list          print every output device name and exit
  --dry-run       build the tap, report the layout, stop before muting

7.1 is the default because a listening test could not distinguish 7.1.4's
height channels without head tracking, and this headset has no IMU."
    );
}
