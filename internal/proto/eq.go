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

// BandLabels are the centre frequencies Razer's own UI prints under each
// slider, lifted from the Synapse product page logs. Ten bands, ten labels.
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

// GameChatBalance reads the mix between the Game and Chat endpoints.
func GameChatBalance() *Message { return New(GetGameChatBalance, 0x00) }
