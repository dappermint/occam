package spatial

import "math"

// fracDelay is a delay line with linear interpolation between samples.
//
// Whole-sample delay is not enough here. At 48 kHz one sample is 7.1 mm of
// path difference, and the ear resolves interaural delays down to about a
// tenth of that, so rounding would quantise the image into a handful of
// positions instead of a continuous arc.
type fracDelay struct {
	buf   []float64
	at    int
	whole int
	frac  float64
}

func newFracDelay(samples float64) fracDelay {
	if samples < 0 {
		samples = 0
	}
	whole := int(samples)
	frac := samples - float64(whole)
	// Two extra so the interpolation can always read at whole+1.
	return fracDelay{buf: make([]float64, whole+2), whole: whole, frac: frac}
}

func (d *fracDelay) reset() {
	clear(d.buf)
	d.at = 0
}

func (d *fracDelay) process(x float64) float64 {
	d.buf[d.at] = x
	n := len(d.buf)

	a := d.buf[(d.at-d.whole+n*2)%n]
	b := d.buf[(d.at-d.whole-1+n*2)%n]

	d.at = (d.at + 1) % n
	return a*(1-d.frac) + b*d.frac
}

// onePole is a first-order lowpass, which is the right shape for head
// shadowing: a gentle 6 dB/octave roll rather than anything resonant.
type onePole struct {
	a    float64
	prev float64
}

func (f *onePole) reset() { f.prev = 0 }

func newOnePole(cutoff float64, rate int) onePole {
	if cutoff >= float64(rate)/2 {
		return onePole{a: 1} // pass through
	}
	x := math.Exp(-2 * math.Pi * cutoff / float64(rate))
	return onePole{a: 1 - x}
}

func (f *onePole) process(x float64) float64 {
	f.prev += f.a * (x - f.prev)
	return f.prev
}

// biquad is a direct form I second-order section.
type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func (f *biquad) reset() { f.x1, f.x2, f.y1, f.y2 = 0, 0, 0, 0 }

// newNotch builds a band-reject at freq. depth is 0 for no cut and 1 for a
// full null; the pinna notch is deep but never total.
func newNotch(freq, depth float64, rate int) biquad {
	nyquist := float64(rate) / 2
	if freq >= nyquist || depth <= 0 {
		return biquad{b0: 1}
	}

	const q = 3.0 // narrow enough to be a cue, wide enough not to ring
	w := 2 * math.Pi * freq / float64(rate)
	alpha := math.Sin(w) / (2 * q)
	cosw := math.Cos(w)

	// A peaking EQ with negative gain, which is a controllable notch. A true
	// notch nulls the band completely and sounds like a defect.
	A := math.Pow(10, -depth*12/40) // depth 1 is about -12 dB

	b0 := 1 + alpha*A
	b1 := -2 * cosw
	b2 := 1 - alpha*A
	a0 := 1 + alpha/A
	a1 := -2 * cosw
	a2 := 1 - alpha/A

	return biquad{
		b0: b0 / a0, b1: b1 / a0, b2: b2 / a0,
		a1: a1 / a0, a2: a2 / a0,
	}
}

func (f *biquad) process(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}
