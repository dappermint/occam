// Compiles the Objective-C shim with clang directly, rather than pulling in
// the cc crate. This crate has no dependencies and that is worth keeping: the
// whole thing is a few frameworks and one .m file.

use std::{env, path::PathBuf, process::Command};

fn main() {
    let out = PathBuf::from(env::var("OUT_DIR").unwrap());
    let obj = out.join("shim.o");
    let lib = out.join("libshim.a");

    // CATapDescription needs 14.2; the SDK default deployment target is older
    // and the call would be an error rather than a warning.
    let status = Command::new("clang")
        .args(["-c", "-fobjc-arc", "-O2", "-Wall", "-mmacosx-version-min=14.2"])
        .arg("src/shim.m")
        .arg("-o")
        .arg(&obj)
        .status()
        .expect("clang not found; it comes with the command line tools");
    assert!(status.success(), "compiling src/shim.m failed");

    let status = Command::new("ar")
        .arg("crs")
        .arg(&lib)
        .arg(&obj)
        .status()
        .expect("ar not found");
    assert!(status.success(), "archiving shim.o failed");

    println!("cargo:rustc-link-search=native={}", out.display());
    println!("cargo:rustc-link-lib=static=shim");
    println!("cargo:rustc-link-lib=framework=CoreAudio");
    println!("cargo:rustc-link-lib=framework=AudioToolbox");
    println!("cargo:rustc-link-lib=framework=CoreFoundation");
    println!("cargo:rustc-link-lib=framework=Foundation");
    println!("cargo:rustc-link-lib=objc");
    println!("cargo:rerun-if-changed=src/shim.m");
    println!("cargo:rerun-if-changed=build.rs");
}
