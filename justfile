# Newest macOS SDK installed, not the one xcode-select points at.
#
# The macOS 26+ look, Liquid Glass included, is gated on the SDK version a
# binary is linked against. nix-darwin points xcode-select at apple-sdk-14.4,
# which puts every AppKit control into the old appearance no matter what the
# code does.
sdk := ```
    newest=$(ls -d /Library/Developer/CommandLineTools/SDKs/MacOSX*.sdk 2>/dev/null \
        | grep -E 'MacOSX[0-9]+(\.[0-9]+)?\.sdk$' | sort -V | tail -1)
    echo "${newest:-$(xcrun --show-sdk-path)}"
```
export CGO_CFLAGS := "-isysroot " + sdk + " -mmacosx-version-min=14.0"
export CGO_LDFLAGS := "-isysroot " + sdk + " -mmacosx-version-min=14.0"

# list recipes
default:
    @just --list

# report which SDK the go builds will link against
sdk:
    @echo "{{ sdk }}"

# build the menu bar app
build:
    go build -o occam .

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

# go and rust tests
test:
    go test ./...
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c "cd occmixer && cargo test --release"

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

# stamp the version everywhere it is recorded, then tag
release version:
    @git diff --quiet || { echo "working tree is dirty"; exit 1; }
    sd '^version = ".*"' 'version = "{{ version }}"' occmixer/Cargo.toml
    sd 'version = "[0-9.]+";' 'version = "{{ version }}";' flake.nix
    sd 'cmd.version=[0-9.]+' 'cmd.version={{ version }}' flake.nix
    nix shell nixpkgs#cargo -c bash -c "cd occmixer && cargo update -p occmixer"
    git commit -am "chore: stamp v{{ version }}"
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

# realtime binaural: tap system audio, render it, play it to the headset
mix frames='128':
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c \
        "cd occmixer && cargo run --release -- --frames {{ frames }}"

# list output devices as occmixer sees them
mix-list:
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c \
        "cd occmixer && cargo run --release -- --list"

# build the tap, report its layout, stop before muting anything
mix-dry:
    nix shell nixpkgs#cargo nixpkgs#rustc -c bash -c \
        "cd occmixer && cargo run --release -- --dry-run"

# regenerate the embedded hrir blob from a downloaded sofa file
hrir:
    @echo "get D1_HRIR_SOFA.zip from https://zenodo.org/records/12092466"
    @echo "then unzip it and run:"
    @echo "  uv run --with h5py python3 hack/extract-hrir.py <path-to>/D1_48K_24bit_256tap_FIR_SOFA.sofa"

# remove build outputs
clean:
    rm -f occam
    rm -rf result
