package spatial

import (
	"math"
	"path/filepath"
	"testing"
)

const testRate = 48000

// renderWith builds a renderer and renders, so a test reads as one step.
func renderWith(t *testing.T, l Layout, in Audio) (Audio, error) {
	t.Helper()
	r, err := NewRenderer(l, testRate, Measured)
	if err != nil {
		return Audio{}, err
	}
	return r.Render(in)
}

func renderOrFail(t *testing.T, l Layout, in Audio) Audio {
	t.Helper()
	out, err := renderWith(t, l, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// tone builds a mono sine, which is the easiest signal to reason about when
// checking level and delay.
func tone(freq float64, frames int) []float64 {
	out := make([]float64, frames)
	for i := range out {
		out[i] = 0.5 * math.Sin(2*math.Pi*freq*float64(i)/testRate)
	}
	return out
}

func energy(x []float64) float64 {
	sum := 0.0
	for _, v := range x {
		sum += v * v
	}
	return sum
}

// A source hard to one side must reach that ear louder. If this fails the
// head model is not shadowing at all and nothing else will localise.
func TestHardRightIsLouderInTheRightEar(t *testing.T) {
	layout := Surround71
	in := NewAudio(testRate, layout.Channels(), testRate/2)

	right := -1
	for i, s := range layout.Speakers {
		if s.Name == "Rss" { // dead right, 90 degrees
			right = i
		}
	}
	if right < 0 {
		t.Fatal("7.1 has no Rss speaker")
	}
	copy(in.Channels[right], tone(1000, in.Frames()))

	out, err := renderWith(t, layout, in)
	if err != nil {
		t.Fatal(err)
	}

	l, r := energy(out.Channels[0]), energy(out.Channels[1])
	if r <= l {
		t.Fatalf("a source at 90 degrees right gave left %.4f, right %.4f", l, r)
	}
	// It must not be a null either; the far ear still hears the source.
	if l == 0 {
		t.Fatal("the far ear got exactly nothing, which is not head shadowing")
	}
}

// The synthetic model is built from symmetric geometry, so it must mirror
// exactly. A handedness bug would show up here and nowhere else.
func TestSyntheticIsExactlySymmetric(t *testing.T) {
	layout := Surround71
	var li, ri int
	for i, s := range layout.Speakers {
		switch s.Name {
		case "Lss":
			li = i
		case "Rss":
			ri = i
		}
	}

	measure := func(ch int) (float64, float64) {
		in := NewAudio(testRate, layout.Channels(), testRate/2)
		copy(in.Channels[ch], tone(1000, in.Frames()))
		r, err := NewRenderer(layout, testRate, Synthetic)
		if err != nil {
			t.Fatal(err)
		}
		out, err := r.Render(in)
		if err != nil {
			t.Fatal(err)
		}
		return energy(out.Channels[0]), energy(out.Channels[1])
	}

	ll, lr := measure(li)
	rl, rr := measure(ri)

	if math.Abs(ll-rr) > ll*0.01 {
		t.Errorf("near-ear energy differs by side: left source %.4f, right source %.4f", ll, rr)
	}
	if math.Abs(lr-rl) > math.Max(lr, rl)*0.01 {
		t.Errorf("far-ear energy differs by side: %.4f vs %.4f", lr, rl)
	}
}

// Measured data is only roughly symmetric and should be. A KU100 and its
// measurement rig are physical objects, so a few percent of left/right
// difference is the data being honest, not a bug. This checks the asymmetry
// stays small enough to be that rather than a swapped channel.
func TestMeasuredIsRoughlySymmetric(t *testing.T) {
	layout := Surround71
	var li, ri int
	for i, s := range layout.Speakers {
		switch s.Name {
		case "Lss":
			li = i
		case "Rss":
			ri = i
		}
	}

	measure := func(ch int) (float64, float64) {
		in := NewAudio(testRate, layout.Channels(), testRate/2)
		copy(in.Channels[ch], tone(1000, in.Frames()))
		out := renderOrFail(t, layout, in)
		return energy(out.Channels[0]), energy(out.Channels[1])
	}

	ll, lr := measure(li)
	rl, rr := measure(ri)

	const tolerance = 0.20
	if d := math.Abs(ll-rr) / math.Max(ll, rr); d > tolerance {
		t.Errorf("near ear differs by %.1f%% between sides, more than measurement noise", d*100)
	}
	if d := math.Abs(lr-rl) / math.Max(lr, rl); d > tolerance {
		t.Errorf("far ear differs by %.1f%% between sides, more than measurement noise", d*100)
	}
	// The point of the test: near ear must still dominate on both sides.
	if lr > ll || rl > rr {
		t.Error("the far ear is louder than the near ear, so a channel is swapped")
	}
}

// A centre source sits on the median plane, so both ears should get roughly
// the same. Roughly, again: the measured head is not perfectly symmetric.
func TestCentreIsBalanced(t *testing.T) {
	layout := Surround51
	centre := -1
	for i, s := range layout.Speakers {
		if s.Name == "C" {
			centre = i
		}
	}
	in := NewAudio(testRate, layout.Channels(), testRate/2)
	copy(in.Channels[centre], tone(1000, in.Frames()))

	out, err := renderWith(t, layout, in)
	if err != nil {
		t.Fatal(err)
	}
	l, r := energy(out.Channels[0]), energy(out.Channels[1])
	if d := math.Abs(l-r) / math.Max(l, r); d > 0.15 {
		t.Fatalf("centre is %.1f%% off balance: left %.4f right %.4f", d*100, l, r)
	}
}

// Interaural delay is the strongest localisation cue below about 1.5 kHz, so
// it has to be in the plausible range: a head is roughly 0.7 ms wide.
func TestInterauralDelayIsPlausible(t *testing.T) {
	d := Direction{Azimuth: 90}
	near := d.delaySeconds(Right)
	far := d.delaySeconds(Left)
	itd := far - near

	if itd < 0.0005 || itd > 0.0011 {
		t.Fatalf("ITD at 90 degrees is %.1f us, expected roughly 600 to 900", itd*1e6)
	}
	if front := (Direction{Azimuth: 0}); math.Abs(front.delaySeconds(Left)-front.delaySeconds(Right)) > 1e-9 {
		t.Fatal("a source dead ahead must reach both ears together")
	}
}

// The elevation cue has to actually move, or heights are indistinguishable
// from the front speakers they sit above.
func TestPinnaNotchRisesWithElevation(t *testing.T) {
	low, _ := (Direction{Azimuth: 0, Elevation: 0}).pinnaNotch()
	high, _ := (Direction{Azimuth: 0, Elevation: 45}).pinnaNotch()
	if high <= low {
		t.Fatalf("notch did not rise with elevation: %.0f Hz at 0, %.0f Hz at 45", low, high)
	}
}

func TestUpmixFillsEveryChannel(t *testing.T) {
	in := NewAudio(testRate, 2, testRate/4)
	copy(in.Channels[0], tone(440, in.Frames()))
	copy(in.Channels[1], tone(660, in.Frames()))

	out, err := Upmix(in, Surround714)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Channels) != Surround714.Channels() {
		t.Fatalf("upmix gave %d channels, want %d", len(out.Channels), Surround714.Channels())
	}
	for i, ch := range out.Channels {
		if energy(ch) == 0 {
			t.Errorf("channel %d (%s) is silent after upmix",
				i, Surround714.Speakers[i].Name)
		}
	}
}

func TestUpmixRejectsNonStereo(t *testing.T) {
	if _, err := Upmix(NewAudio(testRate, 6, 10), Surround714); err == nil {
		t.Fatal("upmixing six channels should be an error")
	}
}

func TestWAVRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "round.wav")

	in := NewAudio(testRate, 2, 1000)
	copy(in.Channels[0], tone(440, 1000))
	copy(in.Channels[1], tone(880, 1000))

	if err := WriteWAV(path, in); err != nil {
		t.Fatal(err)
	}
	back, err := ReadWAV(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Rate != in.Rate || len(back.Channels) != 2 || back.Frames() != in.Frames() {
		t.Fatalf("shape changed: %d Hz, %d ch, %d frames",
			back.Rate, len(back.Channels), back.Frames())
	}
	// 24-bit quantisation, so exact equality is not the bar.
	for c := range in.Channels {
		for i := range in.Channels[c] {
			if math.Abs(in.Channels[c][i]-back.Channels[c][i]) > 1e-6 {
				t.Fatalf("channel %d sample %d: wrote %.9f, read %.9f",
					c, i, in.Channels[c][i], back.Channels[c][i])
			}
		}
	}
}

func TestLayoutLookup(t *testing.T) {
	if _, err := LayoutByName("7.1.4"); err != nil {
		t.Error(err)
	}
	if _, err := LayoutByName("9.2.6"); err == nil {
		t.Error("an unknown layout should be an error")
	}
	l, err := LayoutForChannels(12)
	if err != nil || l.Name != "7.1.4" {
		t.Errorf("12 channels should be 7.1.4, got %v %v", l.Name, err)
	}
	if _, err := LayoutForChannels(7); err == nil {
		t.Error("7 channels matches no standard layout")
	}
}
