package proto

import "testing"

// Guards the index-to-curve pairing, which was wrong once: Esports and Flat
// had their indices swapped, so picking Esports applied a flat curve.
func TestMicPresetCurves(t *testing.T) {
	want := map[byte]EQ{
		0x20: {-5, -4, -4, -3, -2, 1, 2, 3, 3, 3},
		0x21: {0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		0x22: {5, 4, 3, 1, -1, 0, 2, 3, 4, 4},
		0x23: {-6, -5, -5, -4, 0, 1, 1, 1, 1, 1},
	}
	if len(MicPresets) != len(want) {
		t.Fatalf("have %d presets, want %d", len(MicPresets), len(want))
	}
	for _, p := range MicPresets {
		if p.Bands != want[p.Index] {
			t.Errorf("0x%02X %q has %v, device reports %v", p.Index, p.Name, p.Bands, want[p.Index])
		}
	}
}
