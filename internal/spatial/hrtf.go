package spatial

import "math"

// A parametric head model, not measured HRIRs.
//
// Measured data sounds better, but a dataset is a licensing and size decision
// and none of the good ones are MIT. Everything below is generated from head
// geometry, so it ships with no third-party data at all. Swapping in real
// HRIRs later replaces this file and nothing else: the renderer only asks for
// a per-ear delay and filter, which measured data can also provide.
//
// Three cues carry almost all of the effect:
//
//  1. ITD, the time difference between ears, which dominates below ~1.5 kHz
//  2. ILD, the level difference from head shadowing, which dominates above it
//  3. a pinna notch that moves with elevation, which is what lets you tell
//     above from in front at all
const (
	headRadius = 0.0875 // metres, the usual anthropometric average
	speedSound = 343.0  // m/s at 20C
)

// Ear is which side a filter is for.
type Ear int

const (
	Left Ear = iota
	Right
)

// Direction is where a virtual speaker sits relative to the listener.
type Direction struct {
	Azimuth   float64 // degrees, positive to the right
	Elevation float64 // degrees, positive up
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }

// angleToEar is the angle between a direction and one ear's axis, which is
// what both the delay and the shadowing depend on.
func (d Direction) angleToEar(e Ear) float64 {
	az := rad(d.Azimuth)
	el := rad(d.Elevation)

	// Ear axis: right ear at +90 azimuth, left at -90, both on the horizontal.
	earAz := math.Pi / 2
	if e == Left {
		earAz = -math.Pi / 2
	}

	// Angle between two unit vectors on a sphere.
	cos := math.Cos(el)*math.Cos(az)*math.Cos(0)*math.Cos(earAz) +
		math.Cos(el)*math.Sin(az)*math.Cos(0)*math.Sin(earAz) +
		math.Sin(el)*math.Sin(0)
	return math.Acos(clampUnit(cos))
}

func clampUnit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

// delaySeconds is the Woodworth spherical-head approximation: sound reaching
// the far ear has to travel around the head, not through it.
func (d Direction) delaySeconds(e Ear) float64 {
	theta := d.angleToEar(e)
	if theta < math.Pi/2 {
		// Facing the ear: straight-line shortening.
		return -headRadius * math.Cos(theta) / speedSound
	}
	// Shadowed: around the curve.
	return headRadius * (theta - math.Pi/2) / speedSound
}

// shadowGain is the broadband level at one ear, the ILD. Real head shadowing
// is frequency dependent; the lowpass below carries that part, and this is
// the flat component.
func (d Direction) shadowGain(e Ear) float64 {
	theta := d.angleToEar(e)
	// 1.0 facing the ear down to about 0.45 fully shadowed.
	return 0.45 + 0.55*math.Cos(theta/2)
}

// shadowCutoff is where the far ear starts losing high frequencies. A head is
// only an obstacle to wavelengths shorter than itself, so the more shadowed a
// direction is the lower this sits.
func (d Direction) shadowCutoff() float64 {
	// Near ear keeps everything, far ear rolls off from ~2 kHz.
	return 20000
}

func (d Direction) shadowCutoffFor(e Ear) float64 {
	theta := d.angleToEar(e)
	if theta <= math.Pi/2 {
		return 20000
	}
	// Falls from 20 kHz at 90 degrees to about 2.2 kHz directly opposite.
	t := (theta - math.Pi/2) / (math.Pi / 2)
	return 20000*math.Pow(0.11, t) + 1500
}

// pinnaNotch is the elevation cue. The outer ear reflects sound into itself
// with a delay that changes with elevation, notching one frequency out. It
// sits near 7 kHz in front and climbs as a source rises, and that movement is
// most of what separates "above" from "ahead".
func (d Direction) pinnaNotch() (freq, depth float64) {
	el := d.Elevation
	if el < -40 {
		el = -40
	}
	if el > 90 {
		el = 90
	}
	freq = 6500 + 45*(el+40) // ~6.5 kHz below, ~12.3 kHz overhead
	// The notch is strongest for sources in front and fades behind, where
	// the pinna faces away.
	front := math.Cos(rad(d.Azimuth))
	if front < 0 {
		front = 0
	}
	depth = 0.35 + 0.35*front
	return freq, depth
}

// earFilter is the per-ear, per-direction chain the renderer applies: a
// fractional delay, a gain, a one-pole lowpass for head shadowing, and a
// notch for elevation.
type earFilter struct {
	delay  fracDelay
	gain   float64
	shadow onePole
	notch  biquad
}

func newEarFilter(d Direction, e Ear, rate int) earFilter {
	freq, depth := d.pinnaNotch()
	return earFilter{
		delay:  newFracDelay(d.delaySeconds(e) * float64(rate)),
		gain:   d.shadowGain(e),
		shadow: newOnePole(d.shadowCutoffFor(e), rate),
		notch:  newNotch(freq, depth, rate),
	}
}

func (f *earFilter) reset() {
	f.delay.reset()
	f.shadow.reset()
	f.notch.reset()
}

func (f *earFilter) process(x float64) float64 {
	y := f.delay.process(x) * f.gain
	y = f.shadow.process(y)
	return f.notch.process(y)
}
