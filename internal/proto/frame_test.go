package proto

import (
	"encoding/hex"
	"testing"
)

// capturedBattery is a real reply, lifted from a Synapse 4 log: a Get Battery
// Status response reporting level 15. The HID report ID is stripped, so this
// is the 63 payload bytes exactly as macOS hands them over.
const capturedBattery = "026000000005000080210101" + "0f" +
	"000000000000000000000000000000000000000000000000" +
	"000000000000000000000000000000000000000000000000" +
	"cb00"

func TestDecodeCapturedFrame(t *testing.T) {
	p, err := hex.DecodeString(capturedBattery)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != PayloadLen {
		t.Fatalf("fixture is %d bytes, want %d", len(p), PayloadLen)
	}

	m, err := Decode(p)
	if err != nil {
		t.Fatalf("the captured frame does not decode: %v", err)
	}
	if m.Status != StatusSuccess {
		t.Errorf("status is %s, want success", StatusText(m.Status))
	}
	if m.TransID != TransactionID {
		t.Errorf("transaction id is 0x%02X, want 0x%02X", m.TransID, TransactionID)
	}
	if m.Command != GetBatteryStatus {
		t.Errorf("command is 0x%02X, want 0x%02X", m.Command, GetBatteryStatus)
	}
	if m.Flags != FlagSet {
		t.Errorf("flags are 0x%02X, want 0x%02X", m.Flags, FlagSet)
	}
	if len(m.Args) != 1 || m.Args[0] != 15 {
		t.Errorf("args are % X, want a single 0x0F", m.Args)
	}
}

func TestCapturedCRCIsReproducible(t *testing.T) {
	p, _ := hex.DecodeString(capturedBattery)
	if got, want := CRC(p), p[offCRC]; got != want {
		t.Fatalf("computed crc 0x%02X, frame carries 0x%02X", got, want)
	}
}

func TestDecodeRejectsBadCRC(t *testing.T) {
	p, _ := hex.DecodeString(capturedBattery)
	p[offCRC] ^= 0xFF
	if _, err := Decode(p); err == nil {
		t.Fatal("a corrupted checksum decoded without error")
	}
}

func TestEncodeLayout(t *testing.T) {
	p, err := SetBands(4, Presets["game"]).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != PayloadLen {
		t.Fatalf("payload is %d bytes, want %d", len(p), PayloadLen)
	}

	want := map[int]byte{
		offStatus:             StatusNew,
		offTransID:            TransactionID,
		offDataSize:           audioHeader + 1 + Bands,
		offClass:              ClassAudio,
		offCommandID:          CommandAudio,
		offArgs + offFlags:    FlagSet,
		offArgs + offCommand:  SetCustomerEQBand,
		offArgs + offArgLen:   1 + Bands,
		offArgs + offAudioArg: 4, // the slot index
		offReserved:           0,
	}
	for at, b := range want {
		if p[at] != b {
			t.Errorf("byte %d is 0x%02X, want 0x%02X", at, p[at], b)
		}
	}
	if p[offCRC] != CRC(p) {
		t.Errorf("encoder wrote crc 0x%02X, contents give 0x%02X", p[offCRC], CRC(p))
	}
}

func TestRoundTrip(t *testing.T) {
	in := SetBands(2, Presets["movie"])
	p, err := in.Encode()
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decode(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Command != in.Command || out.Sub != in.Sub {
		t.Fatalf("got cmd=0x%02X sub=%d, want cmd=0x%02X sub=%d",
			out.Command, out.Sub, in.Command, in.Sub)
	}
	if len(out.Args) != 1+Bands {
		t.Fatalf("decoded %d args, want %d", len(out.Args), 1+Bands)
	}
	eq, err := ParseBands(out.Args[1:])
	if err != nil {
		t.Fatal(err)
	}
	if eq != Presets["movie"] {
		t.Errorf("round trip gave %s, want %s", eq, Presets["movie"])
	}
}

// capturedBandInfo is a real curve out of the logs. Synapse reported it as
// [1,1,129,0,2,0,4,4,4,131], which is sign-magnitude, not two's complement.
func TestSignMagnitudeMatchesCapture(t *testing.T) {
	raw := []byte{1, 1, 129, 0, 2, 0, 4, 4, 4, 131}
	want := EQ{1, 1, -1, 0, 2, 0, 4, 4, 4, -3}

	got, err := ParseBands(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded %s, want %s", got, want)
	}

	for i, b := range want.Bytes() {
		if b != raw[i] {
			t.Errorf("band %d encodes to 0x%02X, capture has 0x%02X", i, b, raw[i])
		}
	}
}

func TestArgsMustFit(t *testing.T) {
	if _, err := New(SetCustomerEQBand, 0, make([]byte, MaxArgs+1)...).Encode(); err == nil {
		t.Fatal("an oversized argument list encoded without error")
	}
}

func TestEveryPresetIsTenBands(t *testing.T) {
	for name, eq := range Presets {
		if len(eq.Bytes()) != Bands {
			t.Errorf("preset %q has %d bands, want %d", name, len(eq.Bytes()), Bands)
		}
	}
}
