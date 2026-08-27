# list recipes
default:
    @just --list

# build both binaries
build:
    go build -o occam .
    go build -o occam-spatial ./cmd/occam-spatial

# run without building, args pass through: just run probe --open
run *args:
    go run . {{ args }}

# enumerate HID interfaces and try opening the dongle
probe:
    go run . probe --open

# hold the device open on a glass console
console:
    go run . console

# read every eq slot off the headset
profile:
    go run . profile

# save the headset's current state to ~/.config/occam/profile.toml
save:
    go run . save --all

# write the saved profile back to the headset
apply:
    go run . apply

# install to GOBIN then point the launchd agent at it, survives `just clean`
agent: install
    occam agent install

# preview the phase 4 frames without touching the device
dry preset='game':
    go run . eq --preset {{ preset }} --dry-run

# attach to a running console
attach:
    nc -U /tmp/occam.sock

# go tests
test:
    go test ./...

# gofmt + nixfmt everything
fmt:
    gofmt -w .
    nixfmt flake.nix

# vet + nix lints
lint:
    go vet ./...
    statix check .
    deadnix .

# tests and lints together
check: test lint

# install to GOBIN
install:
    go install .

# build through nix
nix-build:
    nix build

# dump the HID report descriptor, baseline is sha256 794de1634fdd0777
descriptor:
    @go run . probe --descriptor

# tag a release and print the sha256 the brew formula needs
release version:
    @git diff --quiet || { echo "working tree is dirty"; exit 1; }
    git tag -a v{{ version }} -m "v{{ version }}"
    @echo
    @echo "now: git push && git push --tags"
    @echo "then set url and sha256 in Formula/occam.rb from the github tarball:"
    @echo "  curl -sL https://github.com/dappermint/occam/archive/refs/tags/v{{ version }}.tar.gz | shasum -a 256"

# check the formula the way homebrew will
formula:
    cp Formula/occam.rb $(brew --repository)/Library/Taps/dappermint/homebrew-tap/Formula/occam.rb
    brew style --formula dappermint/tap/occam
    brew audit --formula --strict dappermint/tap/occam

# render a wav to binaural, upmixing stereo first
spatial file layout='7.1.4':
    go run ./cmd/occam-spatial --upmix {{ layout }} {{ file }}

# convert any audio to the 48k wav the measured hrirs need, then render it.
# nixpkgs ffmpeg-full because the homebrew build has no soxr, and resampling
# 44.1 to 48 badly undoes the point of using measured impulses.
listen file start='0' length='180':
    nix shell nixpkgs#ffmpeg-full -c ffmpeg -v error -y \
        -ss {{ start }} -t {{ length }} -i {{ file }} \
        -af "aresample=48000:resampler=soxr:precision=28:dither_method=triangular" \
        -c:a pcm_s24le /tmp/occam-listen-src.wav
    go run ./cmd/occam-spatial --upmix 7.1.4 -o /tmp/occam-listen-binaural.wav /tmp/occam-listen-src.wav
    @echo
    @echo "reference: /tmp/occam-listen-src.wav"
    @echo "binaural:  /tmp/occam-listen-binaural.wav"

# realtime binaural: tap system audio, render it, play it to the headset
live frames='128':
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c \
        "cd occam-live && cargo run --release -- --frames {{ frames }}"

# list output devices as occam-live sees them
live-devices:
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c \
        "cd occam-live && cargo run --release -- --list"

# build the tap, report its layout, stop before muting anything
live-dry:
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c \
        "cd occam-live && cargo run --release -- --dry-run"

# regenerate the embedded hrir blob from a downloaded sofa file
hrir:
    @echo "get D1_HRIR_SOFA.zip from https://zenodo.org/records/12092466"
    @echo "then unzip it and run:"
    @echo "  uv run --with h5py python3 hack/extract-hrir.py <path-to>/D1_48K_24bit_256tap_FIR_SOFA.sofa"

# remove build outputs
clean:
    rm -f occam occam-spatial
    rm -rf result
