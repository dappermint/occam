# occam

razer's razor. native macOS control of a BlackShark V3 Pro, without synapse.

synapse for mac does not support headsets. this talks to the dongle's
vendor HID interface directly:

```
$ occam probe --open
BlackShark V3 Pro  1532:0577
  manufacturer   Razer Inc
  location       0x03130000
  version        0x0100
  report sizes   in 64  out 64  feature 1
  primary        0x000C/0x01
  usages         0x000C/0x01  0xFF13/0x01  0x000B/0x05  0xFF14/0x01

open ok
  interface      1532:0577 at 0x03130000
  transport      2.4GHz dongle
  0xFF14/0x01    true
```

no synapse and no windows involved.

```
$ occam eq --bands "6,6,0,0,0,0,0,0,0,0" --slot 4
$ occam get battery
battery    87%
```

## what works

- 10-band speaker EQ across nine onboard slots, `occam eq`
- 10-band mic EQ, mic presets, sidetone, and mic mute
- active noise cancellation (off, anc, ambient) and anc level
- game/chat balance, dongle indicator light, auto power off, ultra-low latency
- reading every slot back with its metadata, `occam profile`
- saving state to TOML and writing it back, `occam save` / `occam apply`
- menu bar status item and three-pane AppKit settings window, `occam menu`
- re-applying profile whenever the dongle reconnects, `occam agent install`
- battery, charging state, firmware version, serial, `occam get`
- 41 razer cloud presets baked in, `occam presets`
- offline multichannel and stereo-upmix binaural rendering, `occam-spatial`
- realtime system audio binaural rendering via CoreAudio tap, `occmixer`
- watching raw HID traffic, `occam listen`
- interactive REPL console for probing the device, `occam console`

there is no DSP volume or enhancement command. on the V3 Pro those are THX
host-side processing, not device settings, so they do not exist over HID.

game/chat balance is already free: the dongle presents two CoreAudio output
devices, so that is two volume sliders, not a protocol feature.

firmware flashing is a hard non-goal. no DFU, no bootloader, ever.

## install

```
brew install dappermint/tap/occam
brew services start occam      # menu bar app at login
```

three binaries:
- `occam`: headset control, menu bar app, settings window, launchd agent
- `occam-spatial`: renders audio files to binaural with stereo upmix
- `occmixer`: renders live system audio to binaural as it plays

## menu bar and window

```
occam menu
```

two pieces, split by what you are doing. the menu bar is for glancing and
switching; editing happens in a window.

```
Battery 87%
── Equaliser ──────
   EQ 1
 ✓ Game
   Music
── Noise ──────────
  [Off] (ANC) [Ambient]
  Level ────●────── 7
───────────────────
Settings…
Quit occam
```

**Settings…** opens an AppKit window with three tabs:

- **Headset**: preset picker, ten sliders labelled with razer's band frequencies
  (31Hz through 16kHz, -6dB to +6dB), noise cancelling mode and level,
  game/chat balance, dongle indicator light, auto power off timer, and
  ultra-low latency toggle
- **Microphone**: mic mute toggle, monitoring (sidetone) slider, mic preset
  picker, and ten mic EQ sliders
- **Spatial**: toggle realtime binaural rendering of system audio, select layout
  (7.1 or 7.1.4), and check mixer status
- bottom bar: status text, **Reload from Headset**, and **Save to Profile**

slider drags are coalesced and written 250ms after you stop moving, since each
write is three bracketed frames at 30ms apiece.

no `.app` bundle. `NSApplicationActivationPolicyAccessory` keeps it out of the
dock, `NSMenuItem.sectionHeaderWithTitle` draws section headers the way the
system does, and the icon is the `headphones` SF Symbol as a template image so
it tracks light, dark and highlight states.

it does everything `occam watch` does, so `occam agent install` (or
`brew services start occam`) runs this and starts it at login. `KeepAlive` is
deliberately off: quitting from the menu should quit.

slot names come from your profile, so rename them there and both the menu and
the picker follow:

```toml
[[slot]]
  index = 3
  name = "footsteps"
```

razer's own names live in a cloud eq library keyed by `cloudEqId` and are not
available offline, so a fresh `occam save` seeds `EQ 1` through `EQ 9` for you
to edit. `save` never overwrites a name you have set.

## cli

```
occam profile                         # inspect all 9 onboard slots
occam eq --bands "2,2,0,0,1,-1,-1,3,3,3" --slot 3
occam eq --preset "Valorant" --slot 1
occam get battery                     # 87%
occam get anc                         # 01 07
occam presets valorant                # search built-in cloud curves
occam probe --open                    # inspect HID interfaces and open device
occam listen                          # stream raw HID input reports
```

readers available for `occam get`: `battery`, `charging`, `serial`, `firmware`,
`anc`, `mic`, `balance`, `led`, `poweroff`, `micpreset`, `hyperspeed`, `sidetone`.

## persistence

```
occam save --all              # ~/.config/occam/profile.toml
occam apply                   # write it back
occam agent install           # launchd fires it on dongle attach
```

the agent is one long-lived `occam menu` under launchd, asleep on an in-process
IOKit attach notification. 0.0% cpu, no polling, and a replug is caught however
fast it happens.

launchd's own IOKit matching looks like the right tool here and is not: a job
with `IOMatchLaunchStream` has to drain an XPC event stream, and one that does
not gets relaunched forever.

```toml
# occam profile
active = 3

[[slot]]
  index = 3
  name = "game"
  bands = [-2, -1, -1, -2, 3, 3, 0, 1, 2, 1]
```

## eq library

razer's whole preset library is baked in, lifted from the synapse capture:

```
$ occam presets valorant
   10  Valorant                              1,1,-1,0,2,0,4,4,4,-3
   17  Valorant · Mako · DRX                 -1,0,1,1,-1,-1,0,0,-1,-2
   23  Valorant · Zellsis · Sentinels        1,4,-1,4,2,3,4,4,4,-3

$ occam eq --preset "CS2 · NiKo" --slot 4
```

41 presets with names, band curves and footstep scaling. the headset records
which library entry each slot came from as `cloudEqId`, so slots get their real
names automatically rather than being numbered.

## spatial audio

```
occam-spatial --upmix 7.1.4 track.wav
```

a second binary, sharing no code with the hid tool. it renders multichannel
audio to a binaural stereo pair, and upmixes stereo first when there is
nothing multichannel to start from.

that upmix is the point. the v3 pro has no spatial hardware at all, both its
endpoints are stereo and thx is a windows host-side driver, and almost nothing
on macos emits twelve discrete channels anyway. so a layout gets synthesised
from what you actually listen to, then rendered through a head.

the head is real: **SADIE II subject D1**, a neumann ku100 dummy head, 48 khz,
256 taps, CC BY 4.0. only the thirteen directions the layouts use are embedded,
26 kb, each one an exact measured position rather than an interpolation. see
`internal/spatial/hrir/LICENSE-SADIE.md`.

it streams, so a whole album costs about 5 mb of memory rather than gigabytes,
and runs around 9x realtime.

input has to be a 48 khz wav, since that is what the impulses were measured at
and resampling a 256-tap impulse smears the timing that carries localisation.
`just listen <file>` converts anything to that with soxr and renders it, using
nixpkgs `ffmpeg-full` because the homebrew build ships without soxr.

`--model synthetic` swaps in a parametric head built from geometry alone, which
needs no data and works at any sample rate. it sounds worse. it exists so there
is always something that runs.

## realtime

```
just mix             # or: occmixer --frames 128
```

a third binary, in rust, rendering system audio to binaural as it plays. can
also be switched on and off from the Spatial tab in the settings window.

no virtual audio device and no blackhole: a coreaudio process tap captures
system output, and one aggregate device carries that tap as its input and the
headset as its output. a single IOProc then hands both sides of the same time
slice, so there is no ring buffer between capture and playback and no drift
between two clocks.

rust rather than go because the dsp runs inside the render callback. go would
need a ring buffer to keep its garbage collector off that thread, and the ring
costs a block of latency. 128 frames is 2.7 ms a cycle and runs clean.

the tap excludes occmixer's own process. that is load-bearing: a global tap
mutes what it captures, so one that excludes nothing mutes our own output too.

output level is measured, not assumed. dividing by the square root of the
speaker count is the obvious guess and it is 7 db too quiet, because the upmix
derives all seven feeds from one mid/side pair and they sum coherently. so a
known signal goes through the chain at startup and the scale comes out of what
it measures. loudness parity leaves peaks about 2x over full scale, which a
soft knee rounds off rather than the clamp that used to catch them.

## the console

reverse engineering a HID protocol wants a REPL that holds the device open
between pokes. that is [glass](https://github.com/dappermint/glass):

```
$ occam console &
console up
  socket         /tmp/occam.sock
  device         1532:0577
  attach: nc -U /tmp/occam.sock

$ nc -U /tmp/occam.sock
>> f := proto.Frame(0x0D, 0x95, 6,6,0,0,0,0,0,0,0,0)
>> proto.Send(f)
>> proto.Last
```

`dev` is the open `hid.Device`. `proto` builds, mutates and sends frames.
nothing is cached between calls, so the device state is always the real one.

## build

```
just build
just test
```

or with nix:

```
nix build
```

darwin only. it talks to IOHIDManager through cgo, no hidapi, no vendored C.

no permissions needed, including from the launchd agent. that is deliberate:
occam never calls `IOHIDManagerOpen`, which is the call that asks for the HID
event stream and trips Input Monitoring. enumeration and device open do not
need it.

## protocol

four of the setters (anc, mic status, game/chat balance, auto power off) are
not in the synapse capture. they were derived from the `set = get | 0x80` rule
that every captured pair follows, then verified by writing each one the value
the device already held and checking it replied success rather than not
supported.

the dongle exposes one HID interface with four top-level collections. the
audio DSP lives on vendor usage page `0xFF14`, report ID `0x02`, 63 payload
bytes. frame layout is inherited from
[Ashesh3/razer-device-control](https://github.com/Ashesh3/razer-device-control)
(MIT), which solved the BlackShark **V2** Pro on windows. the V3 Pro moved
from usage page `0xFF00` to `0xFF14` and shifted command ids.

## license

MIT.
