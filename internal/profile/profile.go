// Package profile stores the settings occam re-applies when the dongle comes
// back, as TOML under the user's config directory.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dappermint/occam/internal/proto"
)

// Slot is one EQ slot as the profile records it.
type Slot struct {
	Index int    `toml:"index"`
	Name  string `toml:"name,omitempty"`
	Bands []int  `toml:"bands"`
}

// Profile is the whole saved state.
type Profile struct {
	// Active is the slot to select after writing. Negative leaves it alone.
	Active int `toml:"active"`

	// Sidetone is the mic monitoring level, 0 to 255. Negative leaves it alone.
	Sidetone int `toml:"sidetone"`

	Slots []Slot `toml:"slot"`
}

// New returns an empty profile that changes nothing when applied.
func New() *Profile {
	return &Profile{Active: -1, Sidetone: -1}
}

// DefaultPath is where occam looks when no path is given.
//
// XDG rather than os.UserConfigDir, which points at ~/Library/Application
// Support on macOS. Anything else on this machine lives under ~/.config and a
// dotfile path is easier to keep in a repo.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "occam", "profile.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "occam", "profile.toml"), nil
}

// Load reads a profile. A missing file is an error; callers that want a
// default should check with os.IsNotExist.
func Load(path string) (*Profile, error) {
	p := New()
	md, err := toml.DecodeFile(path, p)
	if err != nil {
		return nil, err
	}
	// Absent keys must not read as "set to zero", or an unspecified Active
	// would silently select slot 0 on every apply.
	if !md.IsDefined("active") {
		p.Active = -1
	}
	if !md.IsDefined("sidetone") {
		p.Sidetone = -1
	}
	return p, p.Validate()
}

// Save writes the profile, creating the directory if needed.
func Save(path string, p *Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# occam profile\n")
	b.WriteString("# re-applied whenever the dongle reconnects, see: occam agent install\n\n")
	if err := toml.NewEncoder(&b).Encode(p); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Validate rejects a profile that would produce a malformed frame.
func (p *Profile) Validate() error {
	if p.Active >= proto.Slots {
		return fmt.Errorf("profile: active slot is %d, the headset has %d", p.Active, proto.Slots)
	}
	if p.Sidetone > 255 {
		return fmt.Errorf("profile: sidetone is %d, must be 0 to 255", p.Sidetone)
	}

	seen := make(map[int]bool, len(p.Slots))
	for _, s := range p.Slots {
		if s.Index < 0 || s.Index >= proto.Slots {
			return fmt.Errorf("profile: slot index %d is outside 0 to %d", s.Index, proto.Slots-1)
		}
		if seen[s.Index] {
			return fmt.Errorf("profile: slot %d appears twice", s.Index)
		}
		seen[s.Index] = true

		if len(s.Bands) != proto.Bands {
			return fmt.Errorf("profile: slot %d has %d bands, need %d", s.Index, len(s.Bands), proto.Bands)
		}
		for i, v := range s.Bands {
			if v < -127 || v > 127 {
				return fmt.Errorf("profile: slot %d band %d is %d, outside the sign-magnitude range",
					s.Index, i+1, v)
			}
		}
	}
	return nil
}

// EQ converts a slot's bands to the wire type.
func (s Slot) EQ() (proto.EQ, error) {
	var eq proto.EQ
	if len(s.Bands) != proto.Bands {
		return eq, fmt.Errorf("profile: slot %d has %d bands, need %d", s.Index, len(s.Bands), proto.Bands)
	}
	for i, v := range s.Bands {
		eq[i] = int8(v)
	}
	return eq, nil
}

// FromEQ builds a slot from a wire curve.
func FromEQ(index int, name string, eq proto.EQ) Slot {
	bands := make([]int, proto.Bands)
	for i, v := range eq {
		bands[i] = int(v)
	}
	return Slot{Index: index, Name: name, Bands: bands}
}
