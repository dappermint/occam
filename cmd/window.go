package cmd

import (
	"fmt"
	"sync"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/menu"
	"github.com/dappermint/occam/internal/mixer"
	"github.com/dappermint/occam/internal/profile"
	"github.com/dappermint/occam/internal/proto"
)

// Band range, read off Synapse's own axis: it is labelled +6dB at the top and
// -6dB at the bottom. The sign-magnitude encoding could carry more, and
// captured curves only ever used -4 to +5, but there is no reason to offer a
// range the vendor's own UI does not.
const (
	bandMin = -6
	bandMax = 6
)

// writeDelay coalesces slider drags. Every tick would otherwise be a bracketed
// three-frame write at 30 ms a frame, which the device cannot keep up with.
const writeDelay = 250 * time.Millisecond

// editor owns what the window is showing and pushes it to the device.
type editor struct {
	mu       sync.Mutex
	slot     int
	eq       proto.EQ
	micEQ    proto.EQ
	micPend  *time.Timer
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
		names[i] = slotLabel(i, s.slots[i], e.names)
	}
	eq := e.eq
	e.mu.Unlock()

	// One handle for the whole read: opening per setting is slow and the
	// device NAKs more when hammered.
	var extras menu.Extras
	if dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...); err == nil {
		extras = readExtras(dev)
		dev.Close()
	}

	e.mu.Lock()
	e.micEQ = extras.MicBands
	e.mu.Unlock()

	menu.RunOnMain(func() {
		menu.SetLEDModes(proto.LEDModes)
		menu.SetANCModes(proto.ANCModes)
		menu.SetSleepOptions(sleepLabels())
		menu.SetMicPresets(proto.MicPresetNames(), extras.MicPreset)
		menu.SetMix(mixState())
		menu.SetSlots(names, slot)
		menu.SetBands(bandsOf(eq))
		menu.SetMicBands(bandsOf(extras.MicBands))
		menu.SetExtras(extras)
		menu.SetSidetone(extras.Sidetone)
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

// setMicBand records a mic slider move and schedules the write.
func (e *editor) setMicBand(band, value int) {
	e.mu.Lock()
	if band < 0 || band >= proto.Bands {
		e.mu.Unlock()
		return
	}
	e.micEQ[band] = int8(value)
	if e.micPend != nil {
		e.micPend.Stop()
	}
	e.micPend = time.AfterFunc(writeDelay, e.flushMic)
	e.mu.Unlock()

	menu.RunOnMain(func() { menu.SetStatus("…") })
}

// flushMic writes the mic curve. It needs no start/stop bracket: the capture
// only ever wraps the speaker bands.
func (e *editor) flushMic() {
	e.mu.Lock()
	eq := e.micEQ
	e.mu.Unlock()
	e.write("mic EQ", proto.SetMicBands(eq))
}

// setMicPreset takes the popup row; the device index starts at 0x20.
func (e *editor) setMicPreset(row int) {
	m, ok := proto.SetMicPresetRow(row)
	if !ok {
		return
	}
	e.write("mic preset", m)

	// Selecting a preset replaces the curve, so pull the new one back.
	eq, err := e.readMicBands()
	if err != nil {
		msg := truncate("mic preset: "+err.Error(), 40)
		menu.RunOnMain(func() { menu.SetStatus(msg) })
		return
	}

	e.mu.Lock()
	e.micEQ = eq
	e.mu.Unlock()
	menu.RunOnMain(func() { menu.SetMicBands(bandsOf(eq)) })
}

func (e *editor) readMicBands() (proto.EQ, error) {
	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		return proto.EQ{}, err
	}
	defer dev.Close()

	r, err := ask(dev, proto.MicBands())
	if err != nil {
		return proto.EQ{}, err
	}
	if len(r.Args) < proto.Bands {
		return proto.EQ{}, fmt.Errorf("reply carries %d bands, want %d", len(r.Args), proto.Bands)
	}
	return proto.ParseBands(r.Args[:proto.Bands])
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
	e.write("sidetone", proto.SetSidetone(byte(value)))
}

// write pushes one setting and reports the outcome in the status line. These
// are all single frames, so unlike the equalizer they need no debouncing
// beyond what the retry loop already does.
func (e *editor) write(what string, m *proto.Message) {
	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		menu.RunOnMain(func() { menu.SetStatus("Headset not connected") })
		return
	}
	defer dev.Close()

	if err := send(dev, m); err != nil {
		msg := truncate(what+": "+err.Error(), 40)
		menu.RunOnMain(func() { menu.SetStatus(msg) })
		return
	}
	menu.RunOnMain(func() { menu.SetStatus(what + " set") })
}

// mixState renders the occmixer agent for the spatial tab.
func mixState() menu.Mix {
	st := mixer.Read()
	m := menu.Mix{
		Layouts:  mixer.Layouts,
		Selected: mixer.LayoutRow(st.Layout),
		On:       st.Running,
		Enabled:  true,
	}
	switch {
	case !st.Installed:
		m.Enabled = false
		m.Status = "occmixer is not installed"
	case st.Running:
		m.Status = fmt.Sprintf("rendering, %d frames", st.Frames)
	default:
		m.Status = "off, system audio is untouched"
	}
	return m
}

func (e *editor) setMixEnabled(on bool) {
	st := mixer.Read()
	var err error
	if on {
		err = mixer.Start(st.Layout, st.Frames)
	} else {
		err = mixer.Stop()
	}
	e.reportMix("spatial", err)
}

func (e *editor) setMixLayout(row int) {
	if row < 0 || row >= len(mixer.Layouts) {
		return
	}
	e.reportMix("layout", mixer.SetLayout(mixer.Layouts[row]))
}

// reportMix refreshes the tab either way, so the checkbox follows launchd
// rather than whatever the click set it to.
func (e *editor) reportMix(what string, err error) {
	msg := what + " set"
	if err != nil {
		msg = truncate(what+": "+err.Error(), 40)
	}
	m := mixState()
	menu.RunOnMain(func() {
		menu.SetMix(m)
		menu.SetStatus(msg)
	})
}

func (e *editor) setANC(mode, level int) {
	e.write("noise cancelling", proto.SetANC(byte(mode), byte(level)))
}

func (e *editor) setMic(muted bool) { e.write("microphone", proto.SetMicMuted(muted)) }
func (e *editor) setBalance(v int)  { e.write("game/chat", proto.SetGameChat(byte(v))) }
func (e *editor) setLED(mode int)   { e.write("dongle light", proto.SetDongleLED(byte(mode))) }

// setPowerOff takes the popup index, since Synapse offers a fixed list rather
// than a free value.
func (e *editor) setPowerOff(index int) {
	if index < 0 || index >= len(proto.SleepMinutes) {
		return
	}
	e.write("sleep timer", proto.SetAutoPowerOff(proto.SleepMinutes[index]))
}

func (e *editor) setLowLatency(on bool) {
	e.write("ultra-low latency", proto.SetHyperSpeed(on))
}

// readExtras pulls everything below the equalizer. A setting the device does
// not answer for keeps its zero value rather than failing the whole read.
func readExtras(dev *hid.Device) menu.Extras {
	var e menu.Extras
	if m, err := ask(dev, proto.ANC()); err == nil && len(m.Args) >= 2 {
		e.ANCMode, e.ANCLevel = int(m.Args[0]), int(m.Args[1])
	}
	if m, err := ask(dev, proto.MicStatus()); err == nil && len(m.Args) >= 1 {
		e.MicMuted = m.Args[0] != 0
	}
	if m, err := ask(dev, proto.GameChatBalance()); err == nil && len(m.Args) >= 1 {
		e.Balance = int(m.Args[0])
	}
	if m, err := ask(dev, proto.DongleLED()); err == nil && len(m.Args) >= 1 {
		e.LEDMode = int(m.Args[0])
	}
	if m, err := ask(dev, proto.AutoPowerOff()); err == nil && len(m.Args) >= 1 {
		e.SleepIndex = sleepIndexOf(m.Args[0])
	}
	if m, err := ask(dev, proto.HyperSpeed()); err == nil && len(m.Args) >= 1 {
		e.LowLatency = m.Args[0] != 0
	}
	if m, err := ask(dev, proto.New(proto.GetSidetoneVolume, 0x00)); err == nil && len(m.Args) >= 1 {
		e.Sidetone = int(m.Args[0])
	}
	if m, err := ask(dev, proto.MicPresetIndex()); err == nil && len(m.Args) >= 1 &&
		!proto.Unavailable(m.Args) {
		e.MicPreset = proto.MicPresetRow(m.Args[0])
	}
	if m, err := ask(dev, proto.MicBands()); err == nil && len(m.Args) >= proto.Bands {
		if eq, err := proto.ParseBands(m.Args[:proto.Bands]); err == nil {
			e.MicBands = eq
		}
	}
	return e
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
		p.Slots = append(p.Slots, profile.FromEQ(i, existing[i], curve))
	}

	if err := profile.Save(e.profPath, p); err != nil {
		msg := truncate(err.Error(), 40)
		menu.RunOnMain(func() { menu.SetStatus(msg) })
		return
	}
	menu.RunOnMain(func() { menu.SetStatus("Saved to profile") })
}

// sleepIndexOf maps the device's minute value back to a popup row.
func sleepIndexOf(minutes byte) int {
	for i, m := range proto.SleepMinutes {
		if m == minutes {
			return i
		}
	}
	return 0
}

func sleepLabels() []string {
	out := make([]string, len(proto.SleepMinutes))
	for i, m := range proto.SleepMinutes {
		out[i] = fmt.Sprintf("%d min", m)
	}
	return out
}

func bandLabels() []string {
	out := make([]string, proto.Bands)
	copy(out, proto.BandLabels[:])
	return out
}
