# list recipes
default:
    @just --list

# build the binary
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

# remove build outputs
clean:
    rm -f occam
    rm -rf result
