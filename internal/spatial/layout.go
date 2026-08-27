package spatial

import (
	"fmt"
	"strings"
)

// Speaker is one virtual source, placed the way ITU and Dolby place them.
//
// Azimuth is degrees clockwise from straight ahead, so +30 is front right and
// -30 is front left. Elevation is degrees above the horizontal.
type Speaker struct {
	Name      string
	Azimuth   float64
	Elevation float64

	// LFE is summed rather than placed: it has no useful direction and
	// running it through a head model only smears it.
	LFE bool
}

// Layout is an ordered set of speakers matching a file's channel order.
type Layout struct {
	Name     string
	Speakers []Speaker
}

// Channels is how many channels a file in this layout carries.
func (l Layout) Channels() int { return len(l.Speakers) }

// Standard channel orders. These follow the WAV channel mask order, which is
// what every multichannel WAV a player emits will use.
var (
	Stereo = Layout{"stereo", []Speaker{
		{Name: "L", Azimuth: -30},
		{Name: "R", Azimuth: 30},
	}}

	Surround51 = Layout{"5.1", []Speaker{
		{Name: "L", Azimuth: -30},
		{Name: "R", Azimuth: 30},
		{Name: "C", Azimuth: 0},
		{Name: "LFE", LFE: true},
		{Name: "Ls", Azimuth: -110},
		{Name: "Rs", Azimuth: 110},
	}}

	Surround71 = Layout{"7.1", []Speaker{
		{Name: "L", Azimuth: -30},
		{Name: "R", Azimuth: 30},
		{Name: "C", Azimuth: 0},
		{Name: "LFE", LFE: true},
		{Name: "Lrs", Azimuth: -150},
		{Name: "Rrs", Azimuth: 150},
		{Name: "Lss", Azimuth: -90},
		{Name: "Rss", Azimuth: 90},
	}}

	// Atmos bed. The four height speakers sit at 45 degrees up, which is
	// where Dolby puts top-front and top-rear in a 7.1.4 room.
	//
	// Not the default, and the reason is a listening test rather than a
	// preference: with no head tracking the height channels were
	// indistinguishable from the front pair, so they were spending eight
	// convolutions a sample on a cue the listener cannot confirm. Kept
	// because it costs nothing to keep and head tracking would change it.
	Surround714 = Layout{"7.1.4", []Speaker{
		{Name: "L", Azimuth: -30},
		{Name: "R", Azimuth: 30},
		{Name: "C", Azimuth: 0},
		{Name: "LFE", LFE: true},
		{Name: "Lrs", Azimuth: -150},
		{Name: "Rrs", Azimuth: 150},
		{Name: "Lss", Azimuth: -90},
		{Name: "Rss", Azimuth: 90},
		{Name: "Ltf", Azimuth: -45, Elevation: 45},
		{Name: "Rtf", Azimuth: 45, Elevation: 45},
		{Name: "Ltr", Azimuth: -135, Elevation: 45},
		{Name: "Rtr", Azimuth: 135, Elevation: 45},
	}}
)

// Layouts is every layout by name, for the CLI.
var Layouts = map[string]Layout{
	"stereo": Stereo,
	"5.1":    Surround51,
	"7.1":    Surround71,
	"7.1.4":  Surround714,
}

// LayoutByName looks one up, listing the alternatives when it misses.
func LayoutByName(name string) (Layout, error) {
	if l, ok := Layouts[strings.ToLower(name)]; ok {
		return l, nil
	}
	names := make([]string, 0, len(Layouts))
	for k := range Layouts {
		names = append(names, k)
	}
	return Layout{}, fmt.Errorf("spatial: no layout %q, have %s", name, strings.Join(names, ", "))
}

// LayoutForChannels guesses a layout from a channel count, which is all a
// plain WAV tells you unless it carries a channel mask.
func LayoutForChannels(n int) (Layout, error) {
	switch n {
	case 2:
		return Stereo, nil
	case 6:
		return Surround51, nil
	case 8:
		return Surround71, nil
	case 12:
		return Surround714, nil
	}
	return Layout{}, fmt.Errorf("spatial: no standard layout for %d channels, pass --layout", n)
}
