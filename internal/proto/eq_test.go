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

// Ambient was 2 for a while. The register stores 2 without complaint and the
// hardware ignores it; polling while the earcup button cycled gave 0x50.
func TestANCModeValues(t *testing.T) {
	want := []struct {
		value byte
		name  string
		level bool
	}{{0x00, "Off", false}, {0x01, "Noise cancelling", true}, {0x50, "Ambient", false}}

	if len(ANCModes) != len(want) {
		t.Fatalf("have %d modes, want %d", len(ANCModes), len(want))
	}
	for i, w := range want {
		if ANCModes[i].Value != w.value || ANCModes[i].Name != w.name {
			t.Errorf("row %d is {0x%02X %q}, want {0x%02X %q}",
				i, ANCModes[i].Value, ANCModes[i].Name, w.value, w.name)
		}
		if ANCLevelApplies(i) != w.level {
			t.Errorf("ANCLevelApplies(%d) = %v, want %v", i, ANCLevelApplies(i), w.level)
		}
		if got := ANCModeRow(w.value); got != i {
			t.Errorf("ANCModeRow(0x%02X) = %d, want %d", w.value, got, i)
		}
		if v, ok := ANCModeValue(i); !ok || v != w.value {
			t.Errorf("ANCModeValue(%d) = 0x%02X %v, want 0x%02X true", i, v, ok, w.value)
		}
	}
	if ANCModeRow(0x02) != -1 {
		t.Error("0x02 is not a mode, it is the byte that looked like one")
	}
}
