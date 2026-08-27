package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dappermint/occam/internal/proto"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.toml")

	want := New()
	want.Active = 3
	want.Sidetone = 40
	want.Slots = []Slot{
		FromEQ(3, "game", proto.Presets["game"]),
		FromEQ(7, "", proto.Presets["flat"]),
	}

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.Active != want.Active || got.Sidetone != want.Sidetone {
		t.Fatalf("got active=%d sidetone=%d, want active=%d sidetone=%d",
			got.Active, got.Sidetone, want.Active, want.Sidetone)
	}
	if len(got.Slots) != len(want.Slots) {
		t.Fatalf("got %d slots, want %d", len(got.Slots), len(want.Slots))
	}

	eq, err := got.Slots[0].EQ()
	if err != nil {
		t.Fatal(err)
	}
	if eq != proto.Presets["game"] {
		t.Errorf("slot 3 came back as %s, want %s", eq, proto.Presets["game"])
	}
	if got.Slots[0].Name != "game" {
		t.Errorf("slot 3 name is %q, want %q", got.Slots[0].Name, "game")
	}
}

// An absent key must not read as zero: an unspecified active slot silently
// selecting slot 0 on every apply would be a nasty surprise.
func TestAbsentKeysStayUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.toml")
	body := "[[slot]]\nindex = 2\nbands = [0,0,0,0,0,0,0,0,0,0]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Active != -1 {
		t.Errorf("active is %d with no key present, want -1", p.Active)
	}
	if p.Sidetone != -1 {
		t.Errorf("sidetone is %d with no key present, want -1", p.Sidetone)
	}
}

func TestValidateRejects(t *testing.T) {
	ten := []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	cases := map[string]*Profile{
		"active out of range": {Active: proto.Slots, Sidetone: -1},
		"sidetone too large":  {Active: -1, Sidetone: 300},
		"slot index negative": {Active: -1, Sidetone: -1, Slots: []Slot{{Index: -1, Bands: ten}}},
		"slot index too high": {Active: -1, Sidetone: -1, Slots: []Slot{{Index: proto.Slots, Bands: ten}}},
		"duplicate slot": {Active: -1, Sidetone: -1, Slots: []Slot{
			{Index: 1, Bands: ten}, {Index: 1, Bands: ten}}},
		"wrong band count": {Active: -1, Sidetone: -1, Slots: []Slot{
			{Index: 1, Bands: []int{0, 0, 0}}}},
		"band out of range": {Active: -1, Sidetone: -1, Slots: []Slot{
			{Index: 1, Bands: []int{200, 0, 0, 0, 0, 0, 0, 0, 0, 0}}}},
	}
	for name, p := range cases {
		if err := p.Validate(); err == nil {
			t.Errorf("%s: validated without error", name)
		}
	}
}

func TestValidateAcceptsEmpty(t *testing.T) {
	if err := New().Validate(); err != nil {
		t.Fatalf("an empty profile should be valid: %v", err)
	}
}

func TestDefaultPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/xdg/occam/profile.toml"; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
