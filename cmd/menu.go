package cmd

import (
	"fmt"
	"log"
	"os"
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
	tagReapply = 100 + iota
	tagSave
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

			click := func(tag int) {
				switch {
				case tag >= 0 && tag < proto.Slots:
					if dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...); err == nil {
						_ = selectSlot(dev, byte(tag))
						dev.Close()
					}
				case tag == tagReapply:
					if _, err := applyOnce(p); err != nil {
						st.mu.Lock()
						st.v.lastErr = err
						st.mu.Unlock()
					}
				case tag == tagSave:
					saveFromDevice(path, st)
				case tag == tagRefresh:
					// handled by the refresh below
				case tag == tagQuit:
					menu.Quit()
					return
				}
				st.refresh()
			}

			if err := menu.Run("headphones", "occam", func() []menu.Item { return items(st) }, click); err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().StringVar(&path, "profile", "", "profile path, defaults to the config dir")
	return c
}

// items mirrors Synapse's own layout for this device: a SOUND section holding
// the EQ, then POWER. Section names and the band frequencies are Razer's, read
// out of the product page logs.
func items(st *state) []menu.Item {
	s := st.snapshot()

	if !s.connected {
		return []menu.Item{
			{Title: "BlackShark V3 Pro not connected", Tag: tagInert, Disabled: true},
			menu.Sep(),
			{Title: "Refresh", Tag: tagRefresh},
			menu.Sep(),
			{Title: "Quit occam", Tag: tagQuit},
		}
	}

	out := []menu.Item{
		{Title: "Razer BlackShark V3 Pro", Tag: tagInert, Disabled: true},
		{Title: "  " + powerLine(s), Tag: tagInert, Disabled: true},
		menu.Sep(),
		{Title: "SOUND", Tag: tagInert, Disabled: true},
	}

	for i, sl := range s.slots {
		out = append(out, menu.Item{
			Title:   fmt.Sprintf("  %s", slotName(i, sl)),
			Tag:     i,
			Checked: i == s.active,
		})
	}

	// The active curve, laid out the way the Synapse sliders are labelled.
	if s.active >= 0 && s.active < len(s.slots) {
		out = append(out, menu.Sep(), menu.Item{Title: "EQUALIZER", Tag: tagInert, Disabled: true})
		for _, r := range s.slots[s.active].EQ.Rows() {
			out = append(out, menu.Item{
				Title:    fmt.Sprintf("  %-6s %+d dB", r.Label, r.Level),
				Tag:      tagInert,
				Disabled: true,
			})
		}
	}

	if s.lastErr != nil {
		out = append(out, menu.Sep(),
			menu.Item{Title: truncate(s.lastErr.Error(), 48), Tag: tagInert, Disabled: true})
	}

	out = append(out, menu.Sep())
	if !s.lastApply.IsZero() {
		out = append(out, menu.Item{
			Title:    "Applied at " + s.lastApply.Format("15:04"),
			Tag:      tagInert,
			Disabled: true,
		})
	}
	return append(out,
		menu.Item{Title: "Re-apply Profile", Tag: tagReapply},
		menu.Item{Title: "Save Current to Profile", Tag: tagSave},
		menu.Item{Title: "Refresh", Tag: tagRefresh},
		menu.Sep(),
		menu.Item{Title: "Quit occam", Tag: tagQuit},
	)
}

// slotName is the best label available offline. Razer names its slots from a
// cloud EQ library keyed by cloudEqId, and that mapping is not in the logs, so
// a custom slot gets its index and everything else gets its cloud id.
func slotName(i int, sl Slot) string {
	switch {
	case sl.Order.Custom:
		return fmt.Sprintf("Custom %d", i)
	case sl.Order.CloudID != 0:
		return fmt.Sprintf("Preset %d", sl.Order.CloudID)
	case i == 0:
		return "Default"
	default:
		return fmt.Sprintf("Slot %d", i)
	}
}

func powerLine(s view) string {
	switch {
	case s.battery < 0 || s.battery > 100:
		return s.transport + ", battery unknown"
	case s.charging:
		return fmt.Sprintf("%s, %d%% charging", s.transport, s.battery)
	default:
		return fmt.Sprintf("%s, %d%%", s.transport, s.battery)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func saveFromDevice(override string, st *state) {
	resolved, err := profilePath(override)
	if err != nil {
		return
	}
	s := st.snapshot()
	if !s.connected {
		return
	}

	p := profile.New()
	p.Active = s.active
	for i, sl := range s.slots {
		p.Slots = append(p.Slots, profile.FromEQ(i, "", sl.EQ))
	}
	if err := profile.Save(resolved, p); err != nil {
		fmt.Fprintln(os.Stderr, "save:", err)
	}
}
