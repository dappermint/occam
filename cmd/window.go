package cmd

import (
	"sync"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/menu"
	"github.com/dappermint/occam/internal/profile"
	"github.com/dappermint/occam/internal/proto"
)

// Band range the sliders offer. Captured curves only ever used -4 to +5, but
// the encoding carries a full signed magnitude and Synapse's own sliders run
// to twelve.
const (
	bandMin = -12
	bandMax = 12
)

// writeDelay coalesces slider drags. Every tick would otherwise be a bracketed
// three-frame write at 30 ms a frame, which the device cannot keep up with.
const writeDelay = 250 * time.Millisecond

// editor owns what the window is showing and pushes it to the device.
type editor struct {
	mu       sync.Mutex
	slot     int
	eq       proto.EQ
	names    map[int]string
	pending  *time.Timer
	profPath string
}

func newEditor(names map[int]string, profPath string) *editor {
	return &editor{slot: -1, names: names, profPath: profPath}
}

// load pulls the device state into the window.
func (e *editor) load(st *state) {
	s := st.snapshot()
	if !s.connected || len(s.slots) == 0 {
		menu.RunOnMain(func() { menu.SetStatus("No headset connected") })
		return
	}

	slot := s.active
	if slot < 0 || slot >= len(s.slots) {
		slot = 0
	}

	e.mu.Lock()
	e.slot, e.eq = slot, s.slots[slot].EQ
	names := make([]string, len(s.slots))
	for i := range s.slots {
		names[i] = slotName(i, e.names)
	}
	eq := e.eq
	e.mu.Unlock()

	menu.RunOnMain(func() {
		menu.SetSlots(names, slot)
		menu.SetBands(bandsOf(eq))
		menu.SetStatus("")
	})
}

func bandsOf(eq proto.EQ) []int {
	out := make([]int, proto.Bands)
	for i, v := range eq {
		out[i] = int(v)
	}
	return out
}

// setBand records a slider move and schedules the write.
func (e *editor) setBand(band, value int) {
	e.mu.Lock()
	if band < 0 || band >= proto.Bands {
		e.mu.Unlock()
		return
	}
	e.eq[band] = int8(value)
	if e.pending != nil {
		e.pending.Stop()
	}
	e.pending = time.AfterFunc(writeDelay, e.flush)
	e.mu.Unlock()

	menu.RunOnMain(func() { menu.SetStatus("…") })
}

// flush writes the current curve to the slot being edited.
func (e *editor) flush() {
	e.mu.Lock()
	slot, eq := e.slot, e.eq
	e.mu.Unlock()
	if slot < 0 {
		return
	}

	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		menu.RunOnMain(func() { menu.SetStatus("Headset not connected") })
		return
	}
	defer dev.Close()

	if err := writeSlot(dev, byte(slot), eq); err != nil {
		msg := truncate(err.Error(), 40)
		menu.RunOnMain(func() { menu.SetStatus(msg) })
		return
	}
	stamp := time.Now().Format("15:04:05")
	menu.RunOnMain(func() { menu.SetStatus("Written " + stamp) })
}

// selectSlotFromWindow switches the active preset and loads its curve.
func (e *editor) selectSlotFromWindow(slot int, st *state) {
	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		menu.RunOnMain(func() { menu.SetStatus("Headset not connected") })
		return
	}
	if err := selectSlot(dev, byte(slot)); err != nil {
		dev.Close()
		msg := truncate(err.Error(), 40)
		menu.RunOnMain(func() { menu.SetStatus(msg) })
		return
	}
	sl, err := readSlot(dev, byte(slot))
	dev.Close()
	if err != nil {
		return
	}

	e.mu.Lock()
	e.slot, e.eq = slot, sl.EQ
	eq := e.eq
	e.mu.Unlock()

	menu.RunOnMain(func() {
		menu.SetBands(bandsOf(eq))
		menu.SetStatus("")
	})
	st.refresh()
}

func (e *editor) setSidetone(value int) {
	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		return
	}
	defer dev.Close()
	_ = send(dev, proto.SetSidetone(byte(value)))
}

// saveToProfile records what the window is showing, keeping existing names.
func (e *editor) saveToProfile(st *state) {
	s := st.snapshot()
	if !s.connected {
		menu.RunOnMain(func() { menu.SetStatus("No headset connected") })
		return
	}

	e.mu.Lock()
	slot, eq := e.slot, e.eq
	e.mu.Unlock()

	existing := map[int]string{}
	if old, err := profile.Load(e.profPath); err == nil {
		for _, sl := range old.Slots {
			existing[sl.Index] = sl.Name
		}
	}

	p := profile.New()
	p.Active = slot
	for i, sl := range s.slots {
		curve := sl.EQ
		if i == slot {
			curve = eq
		}
		name := existing[i]
		if name == "" {
			name = profile.DefaultNames(i)
		}
		p.Slots = append(p.Slots, profile.FromEQ(i, name, curve))
	}

	if err := profile.Save(e.profPath, p); err != nil {
		msg := truncate(err.Error(), 40)
		menu.RunOnMain(func() { menu.SetStatus(msg) })
		return
	}
	menu.RunOnMain(func() { menu.SetStatus("Saved to profile") })
}

func bandLabels() []string {
	out := make([]string, proto.Bands)
	copy(out, proto.BandLabels[:])
	return out
}
