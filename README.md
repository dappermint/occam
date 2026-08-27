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
  0xFF14/0x01    true
```

status: **working.** the EQ changes audibly, set from macOS, no synapse and
no windows involved. protocol is written up in `docs/protocol.md`.

```
$ occam eq --bands "6,6,0,0,0,0,0,0,0,0" --slot 4
$ occam get firmware
firmware   01 03 03 00
```

## what works

- 10-band EQ across nine onboard slots, `occam eq`
- reading every slot back with its metadata, `occam profile`
- battery, charging, firmware, serial, sidetone, `occam get`
- watching the device talk, `occam listen`

```
$ occam profile
  slot bands (dB)
   0   0,0,0,0,0,0,0,0,0,0
   1   2,2,5,5,1,-1,2,3,3,3 cloud=1
  *3   -2,-1,-1,-2,3,3,0,1,2,1 custom cloud=22
   ...
```

still to come: a saved profile and a launchd agent that re-applies it when the
dongle reconnects.

there is no DSP volume or enhancement command. on the V3 Pro those are THX
host-side processing, not device settings, so they do not exist over HID.

game/chat balance is already free: the dongle presents two CoreAudio output
devices, so that is two volume sliders, not a protocol feature.

firmware flashing is a hard non-goal. no DFU, no bootloader, ever.

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

## protocol

the dongle exposes one HID interface with four top-level collections. the
audio DSP lives on vendor usage page `0xFF14`, report ID `0x02`, 63 payload
bytes. frame layout is inherited from
[Ashesh3/razer-device-control](https://github.com/Ashesh3/razer-device-control)
(MIT), which solved the BlackShark **V2** Pro on windows. the V3 Pro moved
from usage page `0xFF00` to `0xFF14` and may have shifted command ids; that
is what `docs/design.md` phases 2 and 3 are for.

## license

MIT.
