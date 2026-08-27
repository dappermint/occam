package spatial

import (
	"fmt"
	"math"
)

// Renderer folds a layout down to a binaural stereo pair.
//
// Every speaker gets one filter chain per ear, held across the whole render so
// the delay lines and biquads keep their state. Rebuilding them per block
// would click at every boundary.
type Renderer struct {
	layout Layout
	rate   int
	model  Model

	ears []binaural

	// LFE has no direction worth rendering, so it is summed flat into both
	// ears at a fixed level instead.
	lfeGain float64
}

// NewRenderer prepares a renderer for one layout at one sample rate. The
// measured model refuses a sample rate its impulses were not measured at,
// since resampling a 256-tap impulse smears exactly the fine timing that
// carries localisation.
func NewRenderer(layout Layout, rate int, model Model) (*Renderer, error) {
	r := &Renderer{layout: layout, rate: rate, model: model, lfeGain: 0.7}

	var err error
	switch model {
	case Synthetic:
		r.ears = loadSynthetic(layout, rate)
	default:
		if r.ears, err = loadMeasured(layout, rate); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Model reports which head model this renderer was built with.
func (r *Renderer) Model() Model { return r.model }

// Block renders n frames from a layout buffer into two ear buffers. State
// carries across calls, so a stream can be fed indefinitely.
func (r *Renderer) Block(in [][]float64, outL, outR []float64, n int) {
	norm := r.norm()
	for i := range n {
		var l, rr float64
		for c, s := range r.layout.Speakers {
			x := in[c][i]
			if s.LFE {
				l += x * r.lfeGain
				rr += x * r.lfeGain
				continue
			}
			el, er := r.ears[c].process(x)
			l += el
			rr += er
		}
		outL[i] = l * norm
		outR[i] = rr * norm
	}
}

// norm scales for how many speakers actually sum into one ear, so a full
// layout does not clip where a sparse one would be quiet.
func (r *Renderer) norm() float64 {
	return 1.0 / math.Sqrt(float64(max(1, r.layout.Channels()-1)))
}

// Render turns multichannel audio into two channels.
func (r *Renderer) Render(in Audio) (Audio, error) {
	if len(in.Channels) != r.layout.Channels() {
		return Audio{}, fmt.Errorf("spatial: input has %d channels, %s expects %d",
			len(in.Channels), r.layout.Name, r.layout.Channels())
	}
	if in.Rate != r.rate {
		return Audio{}, fmt.Errorf("spatial: input is %d Hz, renderer built for %d",
			in.Rate, r.rate)
	}

	out := NewAudio(in.Rate, 2, in.Frames())
	r.Block(in.Channels, out.Channels[0], out.Channels[1], in.Frames())
	return out, nil
}

// Peak is the largest absolute sample, for reporting headroom.
func Peak(a Audio) float64 {
	peak := 0.0
	for _, ch := range a.Channels {
		for _, v := range ch {
			if m := math.Abs(v); m > peak {
				peak = m
			}
		}
	}
	return peak
}

// Normalize scales so the peak lands at target, leaving silence alone.
func Normalize(a Audio, target float64) {
	peak := Peak(a)
	if peak == 0 || target <= 0 {
		return
	}
	g := target / peak
	if g >= 1 {
		return // only ever pull down, never make it up
	}
	for _, ch := range a.Channels {
		for i := range ch {
			ch[i] *= g
		}
	}
}
