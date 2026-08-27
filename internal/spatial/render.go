package spatial

import (
	"fmt"
	"math"
	"math/rand/v2"
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

	norm float64
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

	r.norm = 1
	r.norm = r.measure(func(noise float64, bed [][]float64, i int) {
		for c, s := range r.layout.Speakers {
			if s.LFE {
				bed[c][i] = 0
				continue
			}
			bed[c][i] = noise
		}
	})
	return r, nil
}

// calibrate re-measures against a caller-supplied drive, for a chain that
// feeds the renderer something other than bare channels.
func (r *Renderer) calibrate(drive func(noise float64, bed [][]float64, i int)) {
	r.norm = 1
	r.norm = r.measure(drive)
}

// measure pushes correlated noise through the chain and returns the scale that
// brings the output back to the input's level.
//
// Dividing by the square root of the speaker count was the obvious guess and
// it is about 7 dB too quiet. That assumes the feeds sum incoherently, but an
// upmix derives them all from one mid/side pair and real multichannel content
// is correlated across channels too, so they add far closer to coherently.
//
// Correlated noise, since that is what the assumption got wrong.
func (r *Renderer) measure(drive func(noise float64, bed [][]float64, i int)) float64 {
	const (
		warmup = 4096
		block  = 1024
		blocks = 16
	)

	bed := make([][]float64, r.layout.Channels())
	for i := range bed {
		bed[i] = make([]float64, block)
	}
	outL := make([]float64, block)
	outR := make([]float64, block)

	// Fixed seed, so a render is reproducible.
	rng := rand.New(rand.NewPCG(0x2545F491, 0x4F6CDD1D))
	var sumIn, sumOut float64

	for b := range warmup/block + blocks {
		for i := range block {
			noise := (rng.Float64()*2 - 1) * 0.5
			drive(noise, bed, i)
			if b*block >= warmup {
				sumIn += noise * noise * 2
			}
		}
		r.Block(bed, outL, outR, block)
		if b*block < warmup {
			continue
		}
		for i := range block {
			sumOut += outL[i]*outL[i] + outR[i]*outR[i]
		}
	}

	r.reset()

	if sumOut <= 0 {
		return 1
	}
	// A wild number here means the measurement went wrong, and being quiet
	// beats the alternative.
	return min(max(math.Sqrt(sumIn/sumOut), 0.25), 4)
}

func (r *Renderer) reset() {
	for _, e := range r.ears {
		if e != nil {
			e.reset()
		}
	}
}

// Model reports which head model this renderer was built with.
func (r *Renderer) Model() Model { return r.model }

// Block renders n frames from a layout buffer into two ear buffers. State
// carries across calls, so a stream can be fed indefinitely.
func (r *Renderer) Block(in [][]float64, outL, outR []float64, n int) {
	norm := r.norm
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
