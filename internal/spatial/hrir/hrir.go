// Package hrir carries the measured head-related impulse responses occam
// renders with, and nothing else.
//
// Source: the SADIE II database, subject D1, a Neumann KU100 dummy head,
// measured at the University of York. 48 kHz, 256 taps, CC BY 4.0. See
// LICENSE-SADIE.md for the attribution that licence requires.
//
// Only the directions occam's speaker layouts actually use are embedded,
// thirteen of the 8802 measured. Each one landed on an exact measured
// position, so none of them are interpolated.
package hrir

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
)

//go:embed sadie48.bin
var blob []byte

// Response is one measured direction: the impulse arriving at each ear.
type Response struct {
	Azimuth   float64
	Elevation float64
	Left      []float64
	Right     []float64
}

// Set is everything embedded, at one sample rate.
type Set struct {
	Rate      int
	Taps      int
	Responses []Response
}

const magic = "OCHR"

// Load parses the embedded blob. It is cheap enough to call once at startup
// and there is nothing to fail at runtime, so errors here mean a corrupt build.
func Load() (Set, error) {
	if len(blob) < 20 || string(blob[0:4]) != magic {
		return Set{}, fmt.Errorf("hrir: embedded data is not an %s blob", magic)
	}
	version := binary.LittleEndian.Uint32(blob[4:8])
	if version != 1 {
		return Set{}, fmt.Errorf("hrir: blob version %d, this build reads 1", version)
	}

	set := Set{
		Rate: int(binary.LittleEndian.Uint32(blob[8:12])),
		Taps: int(binary.LittleEndian.Uint32(blob[12:16])),
	}
	count := int(binary.LittleEndian.Uint32(blob[16:20]))

	at := 20
	stride := 8 + set.Taps*4*2
	if want := 20 + count*stride; len(blob) != want {
		return Set{}, fmt.Errorf("hrir: blob is %d bytes, expected %d", len(blob), want)
	}

	set.Responses = make([]Response, count)
	for i := range set.Responses {
		r := Response{
			Azimuth:   float64(math.Float32frombits(binary.LittleEndian.Uint32(blob[at : at+4]))),
			Elevation: float64(math.Float32frombits(binary.LittleEndian.Uint32(blob[at+4 : at+8]))),
			Left:      make([]float64, set.Taps),
			Right:     make([]float64, set.Taps),
		}
		p := at + 8
		for t := range set.Taps {
			r.Left[t] = float64(math.Float32frombits(binary.LittleEndian.Uint32(blob[p : p+4])))
			p += 4
		}
		for t := range set.Taps {
			r.Right[t] = float64(math.Float32frombits(binary.LittleEndian.Uint32(blob[p : p+4])))
			p += 4
		}
		set.Responses[i] = r
		at += stride
	}
	return set, nil
}

// Nearest returns the closest embedded direction and the angle it is off by,
// in degrees. Every direction occam's layouts use is exact, so a non-zero
// error means a layout gained a speaker the blob was not generated for.
func (s Set) Nearest(azimuth, elevation float64) (Response, float64) {
	best, bestDot := Response{}, -2.0
	target := unit(azimuth, elevation)
	for _, r := range s.Responses {
		if d := dot(target, unit(r.Azimuth, r.Elevation)); d > bestDot {
			best, bestDot = r, d
		}
	}
	if bestDot > 1 {
		bestDot = 1
	}
	return best, math.Acos(bestDot) * 180 / math.Pi
}

type vec3 [3]float64

func unit(az, el float64) vec3 {
	a, e := az*math.Pi/180, el*math.Pi/180
	return vec3{math.Cos(e) * math.Cos(a), math.Cos(e) * math.Sin(a), math.Sin(e)}
}

func dot(a, b vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
