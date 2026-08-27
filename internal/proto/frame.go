// Package proto builds the frames the dongle's 0xFF14 vendor collection
// accepts.
//
// This is the standard Razer report, the same envelope the mice use, shrunk to
// a 63-byte payload and carrying an audio sub-protocol in its argument field.
// It is NOT the "PA" magic layout from the BlackShark V2 Pro writeup; the V3
// Pro shares command ids with that work but not the envelope.
//
// Derived from Synapse 4 logs, module AudioDevice-0.0.340, device class
// AudioValkyrieDongle. 84 captured response frames agree on every field below.
//
//	frame 0        HID report ID, 0x02, passed out of band on macOS
//	frame 1        status
//	frame 2        transaction id, 0x60
//	frame 3,4      remaining packets, always 0
//	frame 5        protocol type, always 0
//	frame 6        data size, 4 + len(args)
//	frame 7        command class, 0
//	frame 8        command id, 0 for audio
//	frame 9..61    arguments, holding the audio message
//	frame 62       crc, XOR of frames 2 through 61
//	frame 63       reserved, 0
//
// The audio message inside the argument field:
//
//	arg 0          flags, 0x80, mandatory in both directions
//	arg 1          audio command
//	arg 2          sub-index
//	arg 3          argument length
//	arg 4..        arguments
package proto

import (
	"errors"
	"fmt"
)

// ReportID is the HID report ID carrying the protocol on usage page 0xFF14.
const ReportID byte = 0x02

// PayloadLen is the report count declared for 0xFF14, report 0x02. macOS passes
// the report ID separately, so a payload is one byte shorter than the frame.
const PayloadLen = 63

// Payload offsets. Frame offsets are these plus one.
const (
	offStatus    = 0
	offTransID   = 1
	offRemaining = 2
	offProtocol  = 4
	offDataSize  = 5
	offClass     = 6
	offCommandID = 7
	offArgs      = 8
	offCRC       = 61
	offReserved  = 62
)

// Audio message offsets, relative to offArgs.
const (
	offFlags    = 0
	offCommand  = 1
	offSub      = 2
	offArgLen   = 3
	offAudioArg = 4

	audioHeader = 4 // flags, command, sub, length
)

// Status values. Only the two seen in captures are named; the rest follow the
// documented Razer report and are here so an unexpected reply reads clearly.
const (
	StatusNew         byte = 0x00
	StatusBusy        byte = 0x01
	StatusSuccess     byte = 0x02
	StatusFailure     byte = 0x03
	StatusNoResponse  byte = 0x04
	StatusUnsupported byte = 0x05
)

// TransactionID is the value every captured frame carries.
const TransactionID byte = 0x60

// FlagSet is the flags byte, and it is mandatory in both directions.
//
// Sending a request with 0x00 gets a well-formed reply back: right command,
// right argument count, valid checksum, and an argument field holding stale
// buffer content. It reads exactly like an unsupported command, so this cost
// more time than any other single byte in the protocol.
const (
	FlagSet   byte = 0x80
	FlagClear byte = 0x00

	ClassAudio   byte = 0x00
	CommandAudio byte = 0x00
)

// MaxArgs is how many argument bytes fit before the crc.
const MaxArgs = offCRC - offArgs - audioHeader

// Message is one audio command. Status, TransID, Class and CommandID are
// exported so the glass console can override them while decoding is ongoing.
type Message struct {
	Status    byte
	TransID   byte
	Class     byte
	CommandID byte
	Flags     byte
	Command   byte
	Sub       byte
	Args      []byte
}

// New builds a host to device audio command.
func New(command, sub byte, args ...byte) *Message {
	return &Message{
		Status:    StatusNew,
		TransID:   TransactionID,
		Class:     ClassAudio,
		CommandID: CommandAudio,
		Flags:     FlagSet,
		Command:   command,
		Sub:       sub,
		Args:      args,
	}
}

// CRC is the checksum the device expects: a XOR fold over the frame from the
// transaction id up to the byte before the checksum itself. Confirmed against
// every captured frame.
func CRC(payload []byte) byte {
	var x byte
	for _, b := range payload[offTransID:offCRC] {
		x ^= b
	}
	return x
}

// Encode lays the message out as the 63 payload bytes, checksum included. The
// HID report ID is not part of the result; pass it to SetReport separately.
func (m *Message) Encode() ([]byte, error) {
	if len(m.Args) > MaxArgs {
		return nil, fmt.Errorf("proto: %d argument bytes exceed the %d that fit", len(m.Args), MaxArgs)
	}
	p := make([]byte, PayloadLen)
	p[offStatus] = m.Status
	p[offTransID] = m.TransID
	p[offDataSize] = byte(audioHeader + len(m.Args))
	p[offClass] = m.Class
	p[offCommandID] = m.CommandID
	p[offArgs+offFlags] = m.Flags
	p[offArgs+offCommand] = m.Command
	p[offArgs+offSub] = m.Sub
	p[offArgs+offArgLen] = byte(len(m.Args))
	copy(p[offArgs+offAudioArg:], m.Args)
	p[offCRC] = CRC(p)
	return p, nil
}

// ErrBadCRC means the reply's checksum does not match its contents.
var ErrBadCRC = errors.New("proto: checksum mismatch")

// Decode reads a reply back into a message and verifies the checksum.
//
// Accepts either the 63-byte payload or the full 64-byte frame. macOS delivers
// input reports with the report ID prefixed while SetReport takes the payload
// separately, so both shapes turn up depending on which way the bytes came.
func Decode(p []byte) (*Message, error) {
	if len(p) == PayloadLen+1 {
		if p[0] != ReportID {
			return nil, fmt.Errorf("proto: frame is on report 0x%02X, want 0x%02X", p[0], ReportID)
		}
		p = p[1:]
	}
	if len(p) < PayloadLen {
		return nil, fmt.Errorf("proto: payload is %d bytes, need %d", len(p), PayloadLen)
	}
	if got, want := p[offCRC], CRC(p); got != want {
		return nil, fmt.Errorf("%w: frame says 0x%02X, contents give 0x%02X", ErrBadCRC, got, want)
	}

	m := &Message{
		Status:    p[offStatus],
		TransID:   p[offTransID],
		Class:     p[offClass],
		CommandID: p[offCommandID],
		Flags:     p[offArgs+offFlags],
		Command:   p[offArgs+offCommand],
		Sub:       p[offArgs+offSub],
	}
	n := int(p[offArgs+offArgLen])
	if end := offArgs + offAudioArg + n; n >= 0 && end <= offCRC {
		m.Args = append([]byte(nil), p[offArgs+offAudioArg:end]...)
	}
	return m, nil
}

// StatusText names a status byte for error messages.
func StatusText(s byte) string {
	switch s {
	case StatusNew:
		return "new"
	case StatusBusy:
		return "busy"
	case StatusSuccess:
		return "success"
	case StatusFailure:
		return "failure"
	case StatusNoResponse:
		return "no response"
	case StatusUnsupported:
		return "not supported"
	default:
		return fmt.Sprintf("unknown 0x%02X", s)
	}
}

func (m *Message) String() string {
	name := CommandName(m.Command)
	return fmt.Sprintf("%s cmd=0x%02X sub=%d status=%s args=% X",
		name, m.Command, m.Sub, StatusText(m.Status), m.Args)
}
