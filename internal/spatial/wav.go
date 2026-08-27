// Package spatial renders multichannel audio down to a binaural stereo pair,
// so a headset with no spatial hardware of its own can still be given one.
//
// The BlackShark V3 Pro has no spatial processing on the device: both its
// CoreAudio endpoints are two channels and THX Spatial is a Windows host-side
// driver. Anything spatial has to be rendered here.
package spatial

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// Audio is deinterleaved float samples, one slice per channel, all the same
// length. Deinterleaved because every filter here works down a single channel.
type Audio struct {
	Rate     int
	Channels [][]float64
}

// Frames is the sample count per channel.
func (a Audio) Frames() int {
	if len(a.Channels) == 0 {
		return 0
	}
	return len(a.Channels[0])
}

// NewAudio allocates silence.
func NewAudio(rate, channels, frames int) Audio {
	a := Audio{Rate: rate, Channels: make([][]float64, channels)}
	for i := range a.Channels {
		a.Channels[i] = make([]float64, frames)
	}
	return a
}

// riff header field sizes, so the offsets below read as intent rather than
// magic numbers.
const (
	fmtPCM       = 1
	fmtFloat     = 3
	fmtExtension = 0xFFFE
)

// ReadWAV loads a PCM or float WAV. Only what a renderer actually meets:
// 8, 16, 24 and 32-bit integer, and 32 and 64-bit float.
func ReadWAV(path string) (Audio, error) {
	f, err := os.Open(path)
	if err != nil {
		return Audio{}, err
	}
	defer f.Close()

	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil {
		return Audio{}, fmt.Errorf("spatial: %s is too short to be a WAV", path)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return Audio{}, fmt.Errorf("spatial: %s is not a RIFF/WAVE file", path)
	}

	var (
		format   uint16
		channels int
		rate     int
		bits     int
		haveFmt  bool
	)

	for {
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err == io.EOF || err == io.ErrUnexpectedEOF {
			return Audio{}, fmt.Errorf("spatial: %s has no data chunk", path)
		} else if err != nil {
			return Audio{}, err
		}
		id := string(hdr[0:4])
		size := int64(binary.LittleEndian.Uint32(hdr[4:8]))

		switch id {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(f, buf); err != nil {
				return Audio{}, err
			}
			if len(buf) < 16 {
				return Audio{}, fmt.Errorf("spatial: fmt chunk is %d bytes, need 16", len(buf))
			}
			format = binary.LittleEndian.Uint16(buf[0:2])
			channels = int(binary.LittleEndian.Uint16(buf[2:4]))
			rate = int(binary.LittleEndian.Uint32(buf[4:8]))
			bits = int(binary.LittleEndian.Uint16(buf[14:16]))
			// WAVE_FORMAT_EXTENSIBLE hides the real format in its GUID's
			// first two bytes.
			if format == fmtExtension && len(buf) >= 26 {
				format = binary.LittleEndian.Uint16(buf[24:26])
			}
			haveFmt = true

		case "data":
			if !haveFmt {
				return Audio{}, fmt.Errorf("spatial: data chunk arrived before fmt")
			}
			raw := make([]byte, size)
			if _, err := io.ReadFull(f, raw); err != nil {
				return Audio{}, err
			}
			return decodeSamples(raw, format, channels, rate, bits)

		default:
			if size%2 == 1 {
				size++ // chunks are word aligned
			}
			if _, err := f.Seek(size, io.SeekCurrent); err != nil {
				return Audio{}, err
			}
		}
	}
}

func decodeSamples(raw []byte, format uint16, channels, rate, bits int) (Audio, error) {
	if channels <= 0 {
		return Audio{}, fmt.Errorf("spatial: %d channels", channels)
	}
	bytesPer := bits / 8
	if bytesPer <= 0 {
		return Audio{}, fmt.Errorf("spatial: %d bits per sample", bits)
	}
	frames := len(raw) / (bytesPer * channels)

	out := NewAudio(rate, channels, frames)
	for i := range frames {
		for c := range channels {
			at := (i*channels + c) * bytesPer
			v, err := decodeOne(raw[at:at+bytesPer], format, bits)
			if err != nil {
				return Audio{}, err
			}
			out.Channels[c][i] = v
		}
	}
	return out, nil
}

func decodeOne(b []byte, format uint16, bits int) (float64, error) {
	switch {
	case format == fmtFloat && bits == 32:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), nil
	case format == fmtFloat && bits == 64:
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case format != fmtPCM:
		return 0, fmt.Errorf("spatial: unsupported WAV format 0x%04X", format)

	case bits == 8:
		// 8-bit PCM is unsigned, every other width is signed.
		return (float64(b[0]) - 128) / 128, nil
	case bits == 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768, nil
	case bits == 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if v&0x800000 != 0 {
			v |= ^0xFFFFFF // sign extend
		}
		return float64(v) / 8388608, nil
	case bits == 32:
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648, nil
	}
	return 0, fmt.Errorf("spatial: unsupported PCM width of %d bits", bits)
}

// WriteWAV writes 24-bit PCM, which is enough headroom for a rendered mix
// without doubling the file size against 32-bit float.
func WriteWAV(path string, a Audio) error {
	if len(a.Channels) == 0 {
		return fmt.Errorf("spatial: nothing to write")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	const bits = 24
	channels := len(a.Channels)
	frames := a.Frames()
	dataLen := frames * channels * (bits / 8)

	put32 := func(v uint32) { binary.Write(f, binary.LittleEndian, v) }
	put16 := func(v uint16) { binary.Write(f, binary.LittleEndian, v) }

	f.WriteString("RIFF")
	put32(uint32(36 + dataLen))
	f.WriteString("WAVEfmt ")
	put32(16)
	put16(fmtPCM)
	put16(uint16(channels))
	put32(uint32(a.Rate))
	put32(uint32(a.Rate * channels * bits / 8)) // byte rate
	put16(uint16(channels * bits / 8))          // block align
	put16(bits)
	f.WriteString("data")
	put32(uint32(dataLen))

	buf := make([]byte, 0, dataLen)
	for i := range frames {
		for c := range channels {
			v := int32(clamp(a.Channels[c][i]) * 8388607)
			buf = append(buf, byte(v), byte(v>>8), byte(v>>16))
		}
	}
	_, err = f.Write(buf)
	return err
}

// clamp keeps a sample inside full scale. Rendering sums several virtual
// speakers into one ear, so overshoot is normal and has to be caught.
func clamp(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
