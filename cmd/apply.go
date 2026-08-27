package cmd

import (
	"fmt"
	"os"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/profile"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

// profilePath resolves the --profile flag, falling back to the config dir.
func profilePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return profile.DefaultPath()
}

func newApply() *cobra.Command {
	var path string
	var dryRun bool

	c := &cobra.Command{
		Use:   "apply",
		Short: "write the saved profile to the headset",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, resolved, err := loadProfile(path)
			if err != nil {
				return err
			}
			fmt.Printf("%s %s\n", styleTitle.Render("apply"), styleDim.Render(resolved))

			if dryRun {
				printPlan(p)
				fmt.Println(styleDim.Render("\n  dry run, nothing is sent"))
				return nil
			}

			dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
			if err != nil {
				return err
			}
			defer dev.Close()

			n, err := applyProfile(dev, p)
			if err != nil {
				return err
			}
			fmt.Printf("%s %d change(s)\n", styleHit.Render("applied"), n)
			return nil
		},
	}
	c.Flags().StringVar(&path, "profile", "", "profile path, defaults to the config dir")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be written without opening the device")
	return c
}

func loadProfile(override string) (*profile.Profile, string, error) {
	resolved, err := profilePath(override)
	if err != nil {
		return nil, "", err
	}
	p, err := profile.Load(resolved)
	if os.IsNotExist(err) {
		return nil, resolved, fmt.Errorf("no profile at %s, run: occam save", resolved)
	}
	if err != nil {
		return nil, resolved, err
	}
	return p, resolved, nil
}

func printPlan(p *profile.Profile) {
	for _, s := range p.Slots {
		eq, err := s.EQ()
		if err != nil {
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("slot %-4d", s.Index)), err)
			continue
		}
		name := s.Name
		if name != "" {
			name = "  " + styleDim.Render(name)
		}
		fmt.Printf("  %s %s%s\n", styleKey.Render(fmt.Sprintf("slot %-4d", s.Index)), eq, name)
	}
	if p.Active >= 0 {
		fmt.Printf("  %s %d\n", styleKey.Render(fmt.Sprintf("%-9s", "active")), p.Active)
	}
	if p.Sidetone >= 0 {
		fmt.Printf("  %s %d\n", styleKey.Render(fmt.Sprintf("%-9s", "sidetone")), p.Sidetone)
	}
}

// applyProfile writes every slot the profile names, then selects the active
// one. Returns how many device writes it made.
func applyProfile(dev *hid.Device, p *profile.Profile) (int, error) {
	n := 0
	for _, s := range p.Slots {
		eq, err := s.EQ()
		if err != nil {
			return n, err
		}
		if err := writeSlot(dev, byte(s.Index), eq); err != nil {
			return n, err
		}
		n++
	}
	if p.Sidetone >= 0 {
		if err := send(dev, proto.SetSidetone(byte(p.Sidetone))); err != nil {
			return n, err
		}
		n++
	}
	if p.Active >= 0 {
		if err := selectSlot(dev, byte(p.Active)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func newSave() *cobra.Command {
	var path string
	var all bool

	c := &cobra.Command{
		Use:   "save",
		Short: "read the headset's current state into a profile file",
		Long: "By default only the active slot is saved, since that is what most\n" +
			"people want re-applied. --all records every slot.",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved, err := profilePath(path)
			if err != nil {
				return err
			}

			dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
			if err != nil {
				return err
			}
			defer dev.Close()

			// Keep whatever names are already on disk: a save that silently
			// wiped them would be infuriating.
			p := profile.New()
			existing := map[int]string{}
			if old, err := profile.Load(resolved); err == nil {
				for _, s := range old.Slots {
					existing[s.Index] = s.Name
				}
			}

			for pos := byte(0); pos < proto.Slots; pos++ {
				s, err := readSlot(dev, pos)
				if err != nil {
					return err
				}
				if s.Order.Active {
					p.Active = int(pos)
				}
				if all || s.Order.Active {
					name := existing[int(pos)]
					if name == "" {
						name = profile.DefaultNames(int(pos))
					}
					p.Slots = append(p.Slots, profile.FromEQ(int(pos), name, s.EQ))
				}
			}

			if err := profile.Save(resolved, p); err != nil {
				return err
			}
			fmt.Printf("%s %s\n", styleHit.Render("saved"), resolved)
			printPlan(p)
			return nil
		},
	}
	c.Flags().StringVar(&path, "profile", "", "profile path, defaults to the config dir")
	c.Flags().BoolVar(&all, "all", false, "save every slot, not just the active one")
	return c
}
