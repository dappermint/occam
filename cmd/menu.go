package cmd

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/menu"
	"github.com/dappermint/occam/internal/profile"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

// Menu tags. Slots take 0 through proto.Slots-1, so actions start above that.
const (
	tagWindow = 100 + iota
	tagRefresh
	tagQuit
	tagInert
)

// view is one immutable read of the device state. The menu is built on
// AppKit's main thread and must not touch the device, so it renders one of
// these instead.
type view struct {
	connected bool
	transport string
	battery   int // -1 when unknown
	charging  bool
	slots     []Slot
	active    int
	lastErr   error
	lastApply time.Time
}

// state is the mutable version behind a lock. It is deliberately a different
// type from view: returning the locked struct by value copies the mutex.
type state struct {
	mu sync.RWMutex
	v  view
}

func (s *state) snapshot() view {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.v
}

// refresh reads the device. Slow, so it never runs on the main thread.
func (s *state) refresh() {
	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		s.mu.Lock()
		s.v = view{battery: -1, active: -1}
		s.mu.Unlock()
		return
	}
	defer dev.Close()

	var (
		slots     []Slot
		active    = -1
		battery   = -1
		charging  bool
		firstErr  error
		transport = hid.Transport(dev.Info.ProductID)
	)

	if m, err := ask(dev, proto.Battery()); err == nil && len(m.Args) > 0 {
		battery = int(m.Args[0])
	} else if err != nil {
		firstErr = err
	}
	if m, err := ask(dev, proto.Charging()); err == nil && len(m.Args) > 0 {
		charging = m.Args[0] != 0 && m.Args[0] != 0xFF
	}

	for pos := byte(0); pos < proto.Slots; pos++ {
		sl, err := readSlot(dev, pos)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			break
		}
		if sl.Order.Active {
			active = int(pos)
		}
		slots = append(slots, sl)
	}

	s.mu.Lock()
	s.v.connected, s.v.transport = true, transport
	s.v.battery, s.v.charging = battery, charging
	s.v.slots, s.v.active, s.v.lastErr = slots, active, firstErr
	s.mu.Unlock()
}

func newMenu() *cobra.Command {
	var path string

	c := &cobra.Command{
		Use:   "menu",
		Short: "run the menu bar app",
		Long: "A status item showing battery and the EQ slots, with the active one\n" +
			"ticked. Also does what `occam watch` does: re-applies the profile\n" +
			"whenever the dongle reconnects.\n\n" +
			"Replaces `occam watch` in the launchd agent; it has to be running\n" +
			"anyway, so one process is enough.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, _, err := loadProfile(path)
			if err != nil {
				return err
			}

			st := &state{v: view{battery: -1, active: -1}}

			// Everything device-facing lives off the main thread. AppKit owns
			// the main thread once menu.Run blocks.
			go func() {
				st.refresh()
				stop := make(chan struct{})
				go func() {
					tick := time.NewTicker(2 * time.Minute)
					defer tick.Stop()
					for range tick.C {
						st.refresh()
					}
				}()
				err := hid.WatchAttach(hid.Razer, hid.BlackSharkV3Pro, stop, func() {
					if _, err := applyOnce(p); err != nil {
						st.mu.Lock()
						st.v.lastErr = err
						st.mu.Unlock()
					} else {
						st.mu.Lock()
						st.v.lastApply = time.Now()
						st.mu.Unlock()
					}
					st.refresh()
				})
				if err != nil {
					log.Println("watch:", err)
				}
			}()

			resolved, _ := profilePath(path)
			ed := newEditor(slotNames(p), resolved)

			click := func(tag int) {
				switch {
				case tag == tagWindow:
					st.refresh()
					ed.load(st)
					menu.RunOnMain(menu.ShowWindow)
					return
				case tag >= 0 && tag < proto.Slots:
					if dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...); err == nil {
						_ = selectSlot(dev, byte(tag))
						dev.Close()
					}
				case tag == tagRefresh:
					// the refresh below is the whole job
				case tag == tagQuit:
					menu.Quit()
					return
				}
				st.refresh()
			}

			names := slotNames(p)
			menu.BuildWindow(bandLabels(), bandMin, bandMax, menu.WindowHandlers{
				OnBand:     ed.setBand,
				OnSlot:     func(slot int) { ed.selectSlotFromWindow(slot, st) },
				OnSidetone: ed.setSidetone,
				OnAction: func(tag int) {
					switch tag {
					case menu.ActionSave:
						ed.saveToProfile(st)
					case menu.ActionReload:
						st.refresh()
						ed.load(st)
					}
				},
			})

			if err := menu.Run("headphones", "occam", func() []menu.Item { return items(st, names) }, click); err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().StringVar(&path, "profile", "", "profile path, defaults to the config dir")
	return c
}

// items is deliberately small: battery at a glance, and switching preset.
// Everything editable lives in the window, opened from here.
func items(st *state, names map[int]string) []menu.Item {
	s := st.snapshot()

	if !s.connected {
		return []menu.Item{
			{Title: "No headset connected", Tag: tagInert, Disabled: true},
			menu.Sep(),
			{Title: "Refresh", Tag: tagRefresh},
			{Title: "Quit occam", Tag: tagQuit},
		}
	}

	out := []menu.Item{
		{Title: powerLine(s), Tag: tagInert, Disabled: true},
		menu.Section("Equalizer"),
	}
	for i := range s.slots {
		out = append(out, menu.Item{
			Title:   slotName(i, names),
			Tag:     i,
			Checked: i == s.active,
		})
	}

	if s.lastErr != nil {
		out = append(out, menu.Sep(),
			menu.Item{Title: truncate(s.lastErr.Error(), 44), Tag: tagInert, Disabled: true})
	}

	return append(out,
		menu.Sep(),
		menu.Item{Title: "Settings…", Tag: tagWindow},
		menu.Item{Title: "Quit occam", Tag: tagQuit},
	)
}

// slotName prefers the name from the profile. Razer resolves its own names
// from a cloud library keyed by cloudEqId and that mapping is not available
// offline, so the fallback is a plain one-based number rather than a guess.
func slotName(i int, names map[int]string) string {
	if n := names[i]; n != "" {
		return n
	}
	return fmt.Sprintf("EQ %d", i+1)
}

// powerLine is the one status row the menu keeps. 0xFF is what the device
// reports before it has a reading.
func powerLine(s view) string {
	switch {
	case s.battery < 0 || s.battery > 100:
		return "Battery unknown"
	case s.charging:
		return fmt.Sprintf("Battery %d%%, charging", s.battery)
	default:
		return fmt.Sprintf("Battery %d%%", s.battery)
	}
}

func slotNames(p *profile.Profile) map[int]string {
	names := make(map[int]string, len(p.Slots))
	for _, sl := range p.Slots {
		names[sl.Index] = sl.Name
	}
	return names
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
