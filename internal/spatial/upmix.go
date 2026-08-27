package spatial

import (
	"fmt"
	"math"
)

// Upmix spreads a stereo pair across a larger layout.
//
// This is the part that makes the thing useful. Almost nothing on macOS emits
// twelve discrete channels: Apple Music decodes Atmos inside the OS and never
// hands it to a device, and games are stereo. Reproducing 7.1.4 would work on
// almost no real source. Synthesising it from what you actually listen to
// works on all of them, and is what THX and Dolby Headphone do too.
//
// The method is passive matrix decoding, the same idea as Dolby Surround:
//
//   - what both channels share is in front, so it feeds centre and fronts
//   - what they disagree on is ambience, so it feeds surrounds and heights
//   - the difference signal is decorrelated per output so the rear pair does
//     not collapse back into a single phantom behind the head
func Upmix(in Audio, target Layout) (Audio, error) {
	if len(in.Channels) != 2 {
		return Audio{}, fmt.Errorf("spatial: upmix takes stereo, got %d channels", len(in.Channels))
	}
	if target.Channels() <= 2 {
		return in, nil
	}

	frames := in.Frames()
	out := NewAudio(in.Rate, target.Channels(), frames)

	l, r := in.Channels[0], in.Channels[1]

	// Mid and side. Mid is what is common to both and reads as frontal;
	// side is what differs and reads as ambience.
	mid := make([]float64, frames)
	side := make([]float64, frames)
	for i := range frames {
		mid[i] = (l[i] + r[i]) * 0.5
		side[i] = (l[i] - r[i]) * 0.5
	}

	// Bass for the LFE, since a subwoofer feed should not carry the whole
	// spectrum. Two poles is enough for a bed channel.
	lp1 := newOnePole(120, in.Rate)
	lp2 := newOnePole(120, in.Rate)

	// Each surround and height output gets its own decorrelator, otherwise
	// four channels carrying the same side signal fuse into one phantom.
	decor := make(map[int]*allpass, target.Channels())
	seed := 0
	for i, s := range target.Speakers {
		if s.LFE || math.Abs(s.Azimuth) <= 90 && s.Elevation == 0 {
			continue
		}
		seed++
		decor[i] = newAllpass(in.Rate, seed)
	}

	for i := range frames {
		for c, s := range target.Speakers {
			var v float64
			switch {
			case s.LFE:
				v = lp2.process(lp1.process(mid[i])) * 0.8

			case s.Name == "C":
				// Centre is the shared content, at -3 dB so it does not
				// double up against the front pair.
				v = mid[i] * 0.707

			case s.Elevation > 0:
				// Heights take ambience, quieter: inventing a ceiling out of
				// a stereo file goes wrong fast if it is loud.
				v = side[i] * 0.35
				if s.Azimuth < 0 {
					v = -v
				}

			case math.Abs(s.Azimuth) > 90:
				// Rear surrounds: ambience at moderate level.
				v = side[i] * 0.5
				if s.Azimuth < 0 {
					v = -v
				}

			case math.Abs(s.Azimuth) == 90:
				// Side surrounds sit between the front and the rear, so they
				// get a bit of both.
				v = side[i]*0.45 + mid[i]*0.15
				if s.Azimuth < 0 {
					v = -v
				}

			default:
				// Front left and right keep the original channel.
				if s.Azimuth < 0 {
					v = l[i]
				} else {
					v = r[i]
				}
			}

			if d, ok := decor[c]; ok {
				v = d.process(v)
			}
			out.Channels[c][i] = v
		}
	}
	return out, nil
}

// allpass smears phase without changing level, which makes two copies of the
// same signal stop fusing into one point source. Different delays per output
// keep them from correlating with each other.
type allpass struct {
	buf  []float64
	at   int
	gain float64
}

func newAllpass(rate, seed int) *allpass {
	// A few milliseconds, stepped per output. Long enough to decorrelate,
	// short enough not to read as an echo.
	ms := 7.0 + float64(seed)*3.5
	n := max(1, int(ms*float64(rate)/1000))
	return &allpass{buf: make([]float64, n), gain: 0.6}
}

func (a *allpass) process(x float64) float64 {
	d := a.buf[a.at]
	y := -a.gain*x + d
	a.buf[a.at] = x + a.gain*d
	a.at = (a.at + 1) % len(a.buf)
	return y
}
