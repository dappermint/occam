package proto

import (
	"fmt"
	"strings"
)

// Bands is the band count. Every captured bandInfo array is exactly ten long.
const Bands = 10

// Slots is how many EQ slots the headset holds. Captured eqIndex values run
// 0 through 8.
const Slots = 9

// Band values are sign-magnitude, not two's complement: bit 7 carries the sign
// and the low seven bits carry the magnitude in dB. Captured values are 0 to 5
// and 0x81 to 0x84, so +5 and -4 are the extremes actually observed.
const signBit byte = 0x80

// BandGroups are the ranges Synapse prints beneath the frequency axis.
var BandGroups = []struct {
	Name  string
	First int
	Last  int
}{
	{"Sub Bass", 0, 0},
	{"Bass", 1, 2},
	{"Mid Range", 3, 6},
	{"Treble", 7, 9},
}

// BandLabels are the centre frequencies Razer's own UI prints under each
// slider. Extracted from the Synapse product page logs, then confirmed against
// a screenshot of the Audio Equalizer tab. Ten bands, ten labels.
var BandLabels = [Bands]string{
	"31Hz", "63Hz", "125Hz", "250Hz", "500Hz",
	"1kHz", "2kHz", "4kHz", "8kHz", "16kHz",
}

// EQ is one curve, in dB per band.
type EQ [Bands]int8

// Presets are the V2 Pro curves from Ashesh3/razer-device-control, kept as
// starting points. The V3 Pro's own onboard presets live in slots 0 to 8 and
// are read with GetCustomerEQBand rather than shipped here.
var Presets = map[string]EQ{
	"flat":  {0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	"game":  {-3, -3, -4, 0, 5, 5, 4, 1, 0, -1},
	"music": {2, 2, 1, 1, 2, 3, 3, 3, 1, 0},
	"movie": {4, 4, 3, 0, -3, -1, 3, 5, 2, 1},
}

// encodeBand renders one dB value as the sign-magnitude byte the device wants.
func encodeBand(v int8) byte {
	if v < 0 {
		return signBit | byte(-int(v))
	}
	return byte(v)
}

// decodeBand reads a sign-magnitude byte back to dB.
func decodeBand(b byte) int8 {
	if b&signBit != 0 {
		return -int8(b &^ signBit)
	}
	return int8(b)
}

// Bytes renders the curve as the ten sign-magnitude bytes.
func (e EQ) Bytes() []byte {
	out := make([]byte, Bands)
	for i, v := range e {
		out[i] = encodeBand(v)
	}
	return out
}

// ParseBands reads ten sign-magnitude bytes back into a curve.
func ParseBands(b []byte) (EQ, error) {
	var eq EQ
	if len(b) != Bands {
		return eq, fmt.Errorf("proto: got %d band bytes, need %d", len(b), Bands)
	}
	for i, v := range b {
		eq[i] = decodeBand(v)
	}
	return eq, nil
}

// Rows pairs each band with its frequency label, for anything that wants to
// show the curve rather than just the numbers.
func (e EQ) Rows() []struct {
	Label string
	Level int
} {
	out := make([]struct {
		Label string
		Level int
	}, Bands)
	for i, v := range e {
		out[i].Label = BandLabels[i]
		out[i].Level = int(v)
	}
	return out
}

func (e EQ) String() string {
	parts := make([]string, Bands)
	for i, v := range e {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ",")
}

// SetBands writes a curve into one EQ slot. The argument field is the slot
// index followed by the ten bands, eleven bytes, matching the captured
// command tuple [149, 0, 11].
func SetBands(slot byte, eq EQ) *Message {
	args := append([]byte{slot}, eq.Bytes()...)
	return New(SetCustomerEQBand, 0x00, args...)
}

// GetBands reads the EQ slot the cursor is currently on. It is only meaningful
// after OrderInfo has positioned that cursor; on its own the device answers
// with flags 0x00 and a stale buffer. Synapse pairs the two calls for every
// slot it reads, which is why its facade is named getSpeakerEQBandByPosition.
func GetBands(slot byte) *Message { return New(GetCustomerEQBand, 0x00, slot) }

// OrderInfo selects a slot and returns its metadata. Always send this first.
func OrderInfo(position byte) *Message { return New(GetEQOrderInfo, 0x00, position) }

// Order is what OrderInfo returns: six bytes describing one EQ slot.
type Order struct {
	Index       byte
	VoicePrompt byte
	Enabled     bool
	Active      bool
	CloudID     byte
	Custom      bool
}

// ParseOrder reads the six argument bytes of an OrderInfo reply.
func ParseOrder(args []byte) (Order, error) {
	if len(args) < 6 {
		return Order{}, fmt.Errorf("proto: order info is %d bytes, need 6", len(args))
	}
	return Order{
		Index:       args[0],
		VoicePrompt: args[1],
		Enabled:     args[2] != 0,
		Active:      args[3] != 0,
		CloudID:     args[4],
		Custom:      args[5] != 0,
	}, nil
}

// SelectPreset switches the active onboard curve.
func SelectPreset(slot byte) *Message { return New(SetSpeakerPresetEQ, 0x00, slot) }

// EQUpdateStart and EQUpdateStop bracket a curve write. Synapse sends the
// start before touching bands and the stop afterwards; the captured tuple is
// [225, 0, 1], so the argument is a single flag.
func EQUpdateStart() *Message { return New(SetEQOrderUpdate, 0x00, 0x01) }
func EQUpdateStop() *Message  { return New(SetEQOrderUpdate, 0x00, 0x00) }

// Battery, charging and firmware are reads with no argument.
func Battery() *Message         { return New(GetBatteryStatus, 0x00) }
func Charging() *Message        { return New(GetChargingStatus, 0x00) }
func FirmwareVersion() *Message { return New(GetFirmwareVersion, 0x00) }
func SerialNumber() *Message    { return New(GetSerialNumber, 0x00) }

// Sidetone is the mic monitoring level, 0 to 255.
func SetSidetone(level byte) *Message { return New(SetSidetoneVolume, 0x00, level) }

// ANCModes are the noise cancelling states. Synapse shows a master toggle and
// an ANC/Ambient pair; the device reported 1 while ANC was selected, so 0 is
// taken to be off and 2 to be ambient. Only 0 and 1 are confirmed.
var ANCModes = []string{"Off", "Noise cancelling", "Ambient"}

// ANCLevelMin and ANCLevelMax bound the level. Synapse offers 1, 2, 3, 4 with
// no zero, and the device reported 4 while 4 was selected.
const (
	ANCLevelMin = 1
	ANCLevelMax = 4
)

// ANC reads noise cancelling mode and level, two bytes.
func ANC() *Message { return New(GetANCStatusAndLevel, 0x00) }

// SetANC writes mode and level back.
func SetANC(mode, level byte) *Message {
	return New(SetANCStatusAndLevel, 0x00, mode, level)
}

// MicPresetBase is where the mic EQ preset indices start. They are 0x20
// through 0x23, not 0 through 3: getMicPresetEQIndex returned 32, 33, 34 and
// 35 as Synapse's four buttons were clicked in order. Sending a plain 0 would
// have selected nothing and looked like a dead control.
const MicPresetBase byte = 0x20

// MicPreset is one of the four mic EQ buttons, with the curve Synapse writes.
type MicPreset struct {
	Index byte
	Name  string
	Bands EQ
}

// MicPresets are those four. Each curve is what the device reports back from
// getMicCustomerEQBand once the matching index is selected, so the pairing is
// measured rather than assumed.
//
// The names are Synapse's, not the firmware's. getMicPresetEQIndex also
// returns an eqPresetEnum, but it calls 0x21 "MicBoost" while the curve it
// applies is flat, and 0x23 "Conference" for the one Synapse labels esports.
var MicPresets = []MicPreset{
	{MicPresetBase + 0, "Default", EQ{-5, -4, -4, -3, -2, 1, 2, 3, 3, 3}},
	{MicPresetBase + 1, "Flat", EQ{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	{MicPresetBase + 2, "Broadcast", EQ{5, 4, 3, 1, -1, 0, 2, 3, 4, 4}},
	{MicPresetBase + 3, "Esports", EQ{-6, -5, -5, -4, 0, 1, 1, 1, 1, 1}},
}

// MicPresetNames lists them for a picker.
func MicPresetNames() []string {
	out := make([]string, len(MicPresets))
	for i, p := range MicPresets {
		out[i] = p.Name
	}
	return out
}

// MicPresetRow maps a device index onto a row in that list, or -1.
func MicPresetRow(index byte) int {
	for i, p := range MicPresets {
		if p.Index == index {
			return i
		}
	}
	return -1
}

// MicBands reads the mic curve. Unlike the speaker one it carries no slot
// byte: the capture shows setMicCustomerEQBand taking exactly ten arguments.
func MicBands() *Message { return New(GetMicCustomerEQBand, 0x00) }

// SetMicBands writes the mic curve, ten bands and nothing else.
func SetMicBands(eq EQ) *Message {
	return New(SetMicCustomerEQBand, 0x00, eq.Bytes()...)
}

// MicPresetIndex reads which mic EQ preset is selected.
func MicPresetIndex() *Message { return New(GetMicPresetEQIndex, 0x00) }

// SetMicPresetIndex selects a mic EQ preset by its device index, which starts
// at MicPresetBase.
func SetMicPresetIndex(index byte) *Message {
	return New(SetMicPresetEQIndex, 0x00, index)
}

// SetMicPresetRow selects by row in MicPresets, doing the base offset for you.
func SetMicPresetRow(row int) (*Message, bool) {
	if row < 0 || row >= len(MicPresets) {
		return nil, false
	}
	return SetMicPresetIndex(MicPresets[row].Index), true
}

// MicStatus reads whether the mic is muted.
func MicStatus() *Message { return New(GetMicStatus, 0x00) }

// SetMicMuted mutes or unmutes the mic.
func SetMicMuted(muted bool) *Message {
	var v byte
	if muted {
		v = 1
	}
	return New(SetMicStatus, 0x00, v)
}

// DongleLED reads the dongle's indicator state.
func DongleLED() *Message { return New(GetDongleLEDStatus, 0x00) }

// LEDModes are the three values the indicator light accepts. They choose what
// the light reports, not how bright it is.
//
// Synapse's logs carry indicatorLedStatus 0, 1 and 2 and never a label; the
// strings live in Razer's remote UI bundle. These names come from the device
// owner, and the order is their reading of the Synapse list.
// PresetNames are the slot labels Synapse shows, read off the Audio Equalizer
// tab. Only the first six were visible; the headset holds nine and the rest
// scroll off the right of that row.
var PresetNames = []string{
	"Default",
	"Game",
	"Music",
	"Esports 1",
	"Esports 2",
	"Esports 3",
}

// Confirmed against the Power tab: value 0 was selected there while the device
// reported 0, and the three labels are Synapse's own wording.
var LEDModes = []string{
	"Connection status",
	"Battery status",
	"Battery warning only",
}

// SetDongleLED sets the indicator light mode, 0 to 2.
func SetDongleLED(mode byte) *Message { return New(SetDongleLEDStatus, 0x00, mode) }

// SleepMinutes are the idle timeouts Synapse offers. It is a fixed list of
// four, not a free slider; the device reported 15 while 15 was selected.
// Synapse also has a master toggle for the feature whose off value is unknown,
// so occam does not offer one.
var SleepMinutes = []byte{15, 30, 45, 60}

// AutoPowerOff reads the idle timeout. The device reports minutes.
func AutoPowerOff() *Message { return New(GetAutoPowerOffStatus, 0x00) }

// SetAutoPowerOff writes the idle timeout in minutes.
func SetAutoPowerOff(minutes byte) *Message {
	return New(SetAutoPowerOffStatus, 0x00, minutes)
}

// SidetoneMax is the top of Synapse's mic monitoring slider.
const SidetoneMax = 15

// HyperSpeed is Synapse's "Ultra-Low Latency" toggle, the Gen-2 dongle mode.
func HyperSpeed() *Message { return New(GetHyperSpeedMode, 0x00) }

// SetHyperSpeed turns ultra-low latency on or off. The setter id follows the
// set = get | 0x80 rule and is not itself in any capture.
func SetHyperSpeed(on bool) *Message {
	var v byte
	if on {
		v = 1
	}
	return New(SetHyperSpeedMode, 0x00, v)
}

// SetGameChat writes the mix between the Game and Chat endpoints.
func SetGameChat(balance byte) *Message { return New(SetGameChatBalance, 0x00, balance) }

// GameChatBalance reads the mix between the Game and Chat endpoints.
func GameChatBalance() *Message { return New(GetGameChatBalance, 0x00) }
