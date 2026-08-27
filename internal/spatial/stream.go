package spatial

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Streaming WAV, so a whole track never has to be resident.
//
// The buffered version wants Frames x Channels x 8 bytes. A 23 minute track
// upmixed to twelve channels is 6.4 GB, which is not a tuning problem, it is
// a design one. Everything here works a block at a time instead.

// Reader pulls deinterleaved blocks out of a WAV file.
type Reader struct {
	f        *os.File
	Rate     int
	Channels int

	format uint16
	bits   int
	stride int   // bytes per frame
	left   int64 // bytes of data still unread
	raw    []byte
}

// OpenWAV reads the headers and stops at the start of the samples.
func OpenWAV(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil {
		f.Close()
		return nil, fmt.Errorf("spatial: %s is too short to be a WAV", path)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		f.Close()
		return nil, fmt.Errorf("spatial: %s is not a RIFF/WAVE file", path)
	}

	r := &Reader{f: f}
	haveFmt := false
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			f.Close()
			return nil, fmt.Errorf("spatial: %s has no data chunk", path)
		}
		id := string(hdr[0:4])
		size := int64(binary.LittleEndian.Uint32(hdr[4:8]))

		switch id {
		case "fmt ":
			buf := make([]byte, size)
			if _, err := io.ReadFull(f, buf); err != nil {
				f.Close()
				return nil, err
			}
			if len(buf) < 16 {
				f.Close()
				return nil, fmt.Errorf("spatial: fmt chunk is %d bytes, need 16", len(buf))
			}
			r.format = binary.LittleEndian.Uint16(buf[0:2])
			r.Channels = int(binary.LittleEndian.Uint16(buf[2:4]))
			r.Rate = int(binary.LittleEndian.Uint32(buf[4:8]))
			r.bits = int(binary.LittleEndian.Uint16(buf[14:16]))
			if r.format == fmtExtension && len(buf) >= 26 {
				r.format = binary.LittleEndian.Uint16(buf[24:26])
			}
			haveFmt = true

		case "data":
			if !haveFmt {
				f.Close()
				return nil, fmt.Errorf("spatial: data chunk arrived before fmt")
			}
			if r.Channels <= 0 || r.bits < 8 {
				f.Close()
				return nil, fmt.Errorf("spatial: %d channels at %d bits", r.Channels, r.bits)
			}
			r.stride = r.Channels * r.bits / 8
			r.left = size
			return r, nil

		default:
			if size%2 == 1 {
				size++
			}
			if _, err := f.Seek(size, io.SeekCurrent); err != nil {
				f.Close()
				return nil, err
			}
		}
	}
}

// Frames is the total length, for progress reporting.
func (r *Reader) Frames() int64 { return r.left / int64(r.stride) }

// Read fills up to len(out[0]) frames and reports how many it got. It returns
// 0, io.EOF at the end.
func (r *Reader) Read(out [][]float64) (int, error) {
	if len(out) != r.Channels {
		return 0, fmt.Errorf("spatial: reading into %d channels, file has %d", len(out), r.Channels)
	}
	if r.left <= 0 {
		return 0, io.EOF
	}

	want := int64(len(out[0]) * r.stride)
	if want > r.left {
		want = r.left - r.left%int64(r.stride)
	}
	if want == 0 {
		return 0, io.EOF
	}
	if int64(cap(r.raw)) < want {
		r.raw = make([]byte, want)
	}
	buf := r.raw[:want]

	if _, err := io.ReadFull(r.f, buf); err != nil {
		return 0, err
	}
	r.left -= want

	bytesPer := r.bits / 8
	frames := int(want) / r.stride
	for i := range frames {
		for c := range r.Channels {
			at := i*r.stride + c*bytesPer
			v, err := decodeOne(buf[at:at+bytesPer], r.format, r.bits)
			if err != nil {
				return 0, err
			}
			out[c][i] = v
		}
	}
	return frames, nil
}

// Close releases the file.
func (r *Reader) Close() error { return r.f.Close() }

// Writer appends deinterleaved blocks as 24-bit PCM, patching the header
// sizes on Close since the length is not known up front.
type Writer struct {
	f        *os.File
	channels int
	frames   int64
	raw      []byte
}

// CreateWAV opens a file and reserves space for the header.
func CreateWAV(path string, rate, channels int) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := &Writer{f: f, channels: channels}

	const bits = 24
	put32 := func(v uint32) { binary.Write(f, binary.LittleEndian, v) }
	put16 := func(v uint16) { binary.Write(f, binary.LittleEndian, v) }

	f.WriteString("RIFF")
	put32(0) // patched on Close
	f.WriteString("WAVEfmt ")
	put32(16)
	put16(fmtPCM)
	put16(uint16(channels))
	put32(uint32(rate))
	put32(uint32(rate * channels * bits / 8))
	put16(uint16(channels * bits / 8))
	put16(bits)
	f.WriteString("data")
	put32(0) // patched on Close
	return w, nil
}

// Write appends n frames from the given channels.
func (w *Writer) Write(in [][]float64, n int) error {
	if len(in) != w.channels {
		return fmt.Errorf("spatial: writing %d channels into a %d channel file", len(in), w.channels)
	}
	need := n * w.channels * 3
	if cap(w.raw) < need {
		w.raw = make([]byte, need)
	}
	buf := w.raw[:0]

	for i := range n {
		for c := range w.channels {
			v := int32(clamp(in[c][i]) * 8388607)
			buf = append(buf, byte(v), byte(v>>8), byte(v>>16))
		}
	}
	if _, err := w.f.Write(buf); err != nil {
		return err
	}
	w.frames += int64(n)
	return nil
}

// Close patches the two length fields and closes the file.
func (w *Writer) Close() error {
	dataLen := w.frames * int64(w.channels) * 3

	patch := func(at int64, v uint32) error {
		if _, err := w.f.Seek(at, io.SeekStart); err != nil {
			return err
		}
		return binary.Write(w.f, binary.LittleEndian, v)
	}
	if err := patch(4, uint32(36+dataLen)); err != nil {
		w.f.Close()
		return err
	}
	if err := patch(40, uint32(dataLen)); err != nil {
		w.f.Close()
		return err
	}
	return w.f.Close()
}
