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
//
// Upmixer holds the filter state a stereo-to-surround matrix needs across
// blocks: the LFE lowpass and one decorrelator per ambience output.
type Upmixer struct {
	target   Layout
	lp1, lp2 onePole
	decor    []*allpass // nil where a channel is left correlated
}

// NewUpmixer prepares one for a target layout.
func NewUpmixer(target Layout, rate int) *Upmixer {
	u := &Upmixer{
		target: target,
		lp1:    newOnePole(lfeCutoff, rate),
		lp2:    newOnePole(lfeCutoff, rate),
		decor:  make([]*allpass, target.Channels()),
	}
	seed := 0
	for i, s := range target.Speakers {
		if s.LFE || isFrontal(s) {
			continue
		}
		seed++
		u.decor[i] = newAllpass(rate, seed)
	}
	return u
}

// isFrontal reports whether a speaker keeps the original channel rather than
// carrying synthesised ambience.
func isFrontal(s Speaker) bool {
	return math.Abs(s.Azimuth) <= 90 && s.Elevation == 0
}

// lfeCutoff is where the synthesised low frequency feed is filtered. A
// subwoofer bed should not carry the whole spectrum.
const lfeCutoff = 120

// Block expands n frames of stereo into the target layout, writing into out.
func (u *Upmixer) Block(l, r []float64, out [][]float64, n int) {
	for i := range n {
		mid := (l[i] + r[i]) * 0.5
		side := (l[i] - r[i]) * 0.5
		lfe := u.lp2.process(u.lp1.process(mid))

		for c, s := range u.target.Speakers {
			var v float64
			switch {
			case s.LFE:
				v = lfe * 0.8

			case s.Name == "C":
				// Shared content, at -3 dB so it does not double up against
				// the front pair.
				v = mid * 0.707

			case s.Elevation > 0:
				// Heights take ambience, quietly: inventing a ceiling from a
				// stereo file goes wrong fast if it is loud.
				v = side * 0.35
				if s.Azimuth < 0 {
					v = -v
				}

			case math.Abs(s.Azimuth) > 90:
				v = side * 0.5
				if s.Azimuth < 0 {
					v = -v
				}

			case math.Abs(s.Azimuth) == 90:
				// Side surrounds sit between front and rear, so they get some
				// of both.
				v = side*0.45 + mid*0.15
				if s.Azimuth < 0 {
					v = -v
				}

			default:
				if s.Azimuth < 0 {
					v = l[i]
				} else {
					v = r[i]
				}
			}

			if d := u.decor[c]; d != nil {
				v = d.process(v)
			}
			out[c][i] = v
		}
	}
}

// Upmix expands a whole stereo buffer in one go. Convenience over Upmixer for
// callers that already hold the entire signal.
func Upmix(in Audio, target Layout) (Audio, error) {
	if len(in.Channels) != 2 {
		return Audio{}, fmt.Errorf("spatial: upmix takes stereo, got %d channels", len(in.Channels))
	}
	if target.Channels() <= 2 {
		return in, nil
	}
	out := NewAudio(in.Rate, target.Channels(), in.Frames())
	NewUpmixer(target, in.Rate).Block(in.Channels[0], in.Channels[1], out.Channels, in.Frames())
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
