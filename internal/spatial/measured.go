package spatial

import (
	"fmt"

	"github.com/dappermint/occam/internal/spatial/hrir"
)

// Model selects how a direction is turned into what each ear hears.
type Model int

const (
	// Measured convolves against SADIE II KU100 impulse responses. Default,
	// and the reason this renderer is worth using.
	Measured Model = iota

	// Synthetic uses the parametric head model in hrtf.go. No third-party
	// data, works at any sample rate, and audibly worse. Kept as a fallback
	// for rates the measured set does not cover.
	Synthetic
)

func (m Model) String() string {
	if m == Synthetic {
		return "synthetic"
	}
	return "measured (SADIE II KU100)"
}

// ModelByName parses the CLI flag.
func ModelByName(s string) (Model, error) {
	switch s {
	case "", "measured":
		return Measured, nil
	case "synthetic":
		return Synthetic, nil
	}
	return 0, fmt.Errorf("spatial: no model %q, have measured, synthetic", s)
}

// binaural turns one mono sample into a contribution for each ear.
type binaural interface {
	process(x float64) (left, right float64)
	// reset clears the delay lines and filter memory, so a calibration pass
	// does not leave its tail in the first block of real audio.
	reset()
}

// synthPair is the parametric model behind the same interface.
type synthPair struct {
	left, right earFilter
}

func (p *synthPair) reset() {
	p.left.reset()
	p.right.reset()
}

func (p *synthPair) process(x float64) (float64, float64) {
	return p.left.process(x), p.right.process(x)
}

// firPair convolves against a measured impulse for each ear.
//
// Direct convolution, not FFT. At 256 taps and twelve speakers this is about
// 6k multiply-accumulates per output sample, which is a few percent of one
// core at 48 kHz. Partitioned FFT would win only for much longer impulses and
// would cost latency and a lot of code.
type firPair struct {
	left, right []float64
	hist        []float64
	at          int
}

func newFIRPair(r hrir.Response) *firPair {
	n := len(r.Left)
	return &firPair{
		left:  r.Left,
		right: r.Right,
		hist:  make([]float64, n),
	}
}

func (p *firPair) reset() {
	clear(p.hist)
	p.at = 0
}

func (p *firPair) process(x float64) (float64, float64) {
	n := len(p.hist)
	p.hist[p.at] = x

	var l, r float64
	// Walk the history backwards from the newest sample, which lines up
	// history[at-k] with tap k.
	i := p.at
	for k := range n {
		v := p.hist[i]
		l += v * p.left[k]
		r += v * p.right[k]
		i--
		if i < 0 {
			i = n - 1
		}
	}

	p.at++
	if p.at == n {
		p.at = 0
	}
	return l, r
}

// loadMeasured builds one convolver per speaker, refusing anything the
// embedded set cannot cover exactly.
func loadMeasured(layout Layout, rate int) ([]binaural, error) {
	set, err := hrir.Load()
	if err != nil {
		return nil, err
	}
	if set.Rate != rate {
		return nil, fmt.Errorf(
			"spatial: measured responses are %d Hz but the audio is %d Hz; "+
				"resample the input or pass --model synthetic", set.Rate, rate)
	}

	out := make([]binaural, layout.Channels())
	for i, s := range layout.Speakers {
		if s.LFE {
			continue // summed flat, never convolved
		}
		r, off := set.Nearest(s.Azimuth, s.Elevation)
		if off > 1 {
			return nil, fmt.Errorf(
				"spatial: no measured response for %s at az %+.0f el %+.0f, "+
					"nearest is %.1f degrees away; regenerate the blob with `just hrir`",
				s.Name, s.Azimuth, s.Elevation, off)
		}
		out[i] = newFIRPair(r)
	}
	return out, nil
}

func loadSynthetic(layout Layout, rate int) []binaural {
	out := make([]binaural, layout.Channels())
	for i, s := range layout.Speakers {
		if s.LFE {
			continue
		}
		d := Direction{Azimuth: s.Azimuth, Elevation: s.Elevation}
		out[i] = &synthPair{
			left:  newEarFilter(d, Left, rate),
			right: newEarFilter(d, Right, rate),
		}
	}
	return out
}
