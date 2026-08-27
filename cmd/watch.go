package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/profile"
	"github.com/spf13/cobra"
)

// present reports whether either V3 Pro product is on the bus. This is a plain
// enumeration rather than an IOKit match notification: it costs microseconds
// and there is no callback lifetime to get wrong.
func present() bool {
	devices, err := hid.List()
	if err != nil && len(devices) == 0 {
		return false
	}
	for _, d := range devices {
		if d.VendorID != hid.Razer {
			continue
		}
		for _, pid := range hid.BlackSharkV3Pro {
			if d.ProductID == pid {
				return true
			}
		}
	}
	return false
}

func newWatch() *cobra.Command {
	var path string
	var interval time.Duration
	var once bool

	c := &cobra.Command{
		Use:   "watch",
		Short: "re-apply the profile whenever the dongle reconnects",
		Long: "Polls for the device and applies the profile on every absent to\n" +
			"present transition. Runs until interrupted; see `occam agent install`\n" +
			"to have launchd keep it running.",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, resolved, err := loadProfile(path)
			if err != nil {
				return err
			}

			fmt.Printf("%s %s\n", styleTitle.Render("watch"), styleDim.Render(resolved))
			if !once {
				fmt.Printf("  %s every %s\n", styleKey.Render(fmt.Sprintf("%-9s", "polling")), interval)
			}
			fmt.Println()

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

			// Starting "present" means a device already attached does not
			// trigger a spurious apply on launch. --once overrides that, which
			// is what the launchd agent wants.
			was := present() && !once

			apply := func() {
				n, err := applyOnce(p)
				stamp := time.Now().Format("15:04:05")
				if err != nil {
					fmt.Printf("  %s %s\n", styleDim.Render(stamp), err)
					return
				}
				fmt.Printf("  %s %s %d change(s)\n", styleDim.Render(stamp), styleHit.Render("applied"), n)
			}

			if once {
				if !present() {
					return errors.New("no BlackShark V3 Pro on the bus")
				}
				apply()
				return nil
			}

			tick := time.NewTicker(interval)
			defer tick.Stop()
			for {
				select {
				case <-sig:
					fmt.Println()
					return nil
				case <-tick.C:
					now := present()
					if now && !was {
						apply()
					}
					was = now
				}
			}
		},
	}
	c.Flags().StringVar(&path, "profile", "", "profile path, defaults to the config dir")
	c.Flags().DurationVar(&interval, "interval", 3*time.Second, "how often to check the bus")
	c.Flags().BoolVar(&once, "once", false, "apply immediately and exit, for launchd")
	return c
}

// applyOnce opens the device, applies, and closes. The handle is not held
// between events so occam never blocks anything else that wants the device.
func applyOnce(p *profile.Profile) (int, error) {
	dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		return 0, err
	}
	defer dev.Close()
	return applyProfile(dev, p)
}
