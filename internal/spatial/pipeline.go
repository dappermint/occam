package spatial

import "fmt"

// Pipeline is the whole chain held open across blocks: upmix a stereo pair to
// a layout, render that layout to two ears.
//
// Everything in here carries state between blocks. Delay lines, biquads and
// the FIR history all have to survive a block boundary or the seams click, so
// a pipeline is built once and fed forever.
type Pipeline struct {
	layout   Layout
	rate     int
	upmixer  *Upmixer
	renderer *Renderer

	// bed is the intermediate layout block, reused every call. Allocating it
	// per block would hand the garbage collector work in the one place a
	// realtime path cannot afford it.
	bed  [][]float64
	size int
}

// NewPipeline builds one for a fixed block size. Passing stereo as the target
// layout skips the upmix and renders the input directly.
func NewPipeline(target Layout, rate, blockSize int, model Model) (*Pipeline, error) {
	if blockSize <= 0 {
		return nil, fmt.Errorf("spatial: block size is %d", blockSize)
	}
	renderer, err := NewRenderer(target, rate, model)
	if err != nil {
		return nil, err
	}

	p := &Pipeline{
		layout:   target,
		rate:     rate,
		renderer: renderer,
		size:     blockSize,
		bed:      make([][]float64, target.Channels()),
	}
	for i := range p.bed {
		p.bed[i] = make([]float64, blockSize)
	}
	if target.Channels() > 2 {
		p.upmixer = NewUpmixer(target, rate)

		// The renderer calibrated itself against bare channels, but an upmix
		// splits one stereo pair across them: the front carries the whole
		// signal and the surrounds only the side component. Re-measure through
		// both stages so the level matches the stereo that went in.
		one := make([]float64, 1)
		p.renderer.calibrate(func(noise float64, bed [][]float64, i int) {
			one[0] = noise
			p.upmixer.Block(one, one, bed, 1)
			for c := range bed {
				bed[c][i] = bed[c][0]
			}
		})
		p.upmixer.Reset()
	}
	return p, nil
}

// BlockSize is the number of frames per call.
func (p *Pipeline) BlockSize() int { return p.size }

// Layout reports what the input is expanded to before rendering.
func (p *Pipeline) Layout() Layout { return p.layout }

// Model reports which head model is in use.
func (p *Pipeline) Model() Model { return p.renderer.Model() }

// Process turns one block of stereo into one block of binaural stereo.
// Slices must all be the same length and no longer than the block size.
// outL and outR may alias inL and inR.
func (p *Pipeline) Process(inL, inR, outL, outR []float64) error {
	n := len(inL)
	if len(inR) != n || len(outL) != n || len(outR) != n {
		return fmt.Errorf("spatial: block lengths differ")
	}
	if n > p.size {
		return fmt.Errorf("spatial: block of %d frames exceeds the %d it was built for", n, p.size)
	}

	if p.upmixer != nil {
		p.upmixer.Block(inL, inR, p.bed, n)
	} else {
		copy(p.bed[0][:n], inL)
		copy(p.bed[1][:n], inR)
	}
	p.renderer.Block(p.bed, outL, outR, n)
	return nil
}
