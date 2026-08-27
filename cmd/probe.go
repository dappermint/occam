package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/dappermint/occam/internal/hid"
	"github.com/spf13/cobra"
)

func newProbe() *cobra.Command {
	var all bool
	var open bool
	var descriptor bool

	c := &cobra.Command{
		Use:   "probe",
		Short: "enumerate HID interfaces and report what the dongle exposes",
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := hid.List()
			if err != nil && !errors.Is(err, hid.ErrTruncated) {
				return err
			}
			if errors.Is(err, hid.ErrTruncated) {
				fmt.Println(styleDim.Render("more interfaces exist than the buffer holds; list is partial"))
			}

			shown := 0
			for _, d := range devices {
				if !all && d.VendorID != hid.Razer {
					continue
				}
				printDevice(d)
				if descriptor {
					printDescriptor(d)
				}
				shown++
			}
			if shown == 0 {
				return errors.New("no Razer HID interface found; is the dongle plugged in directly?")
			}

			if open {
				return tryOpen()
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "list every HID interface, not just Razer")
	c.Flags().BoolVar(&open, "open", false, "also try opening the BlackShark V3 Pro")
	c.Flags().BoolVar(&descriptor, "descriptor", false, "print the raw HID report descriptor as hex")
	return c
}

// printDescriptor dumps the descriptor as one hex string plus a sha256, so a
// firmware update can be diffed against a recorded value in one command.
func printDescriptor(d hid.Info) {
	if len(d.Descriptor) == 0 {
		fmt.Println(styleDim.Render("  no report descriptor exposed"))
		fmt.Println()
		return
	}
	sum := sha256.Sum256(d.Descriptor)
	fmt.Printf("  %s %d bytes, sha256 %x\n",
		styleKey.Render(fmt.Sprintf("%-14s", "descriptor")), len(d.Descriptor), sum[:8])
	fmt.Printf("  %s\n\n", hex.EncodeToString(d.Descriptor))
}

func printDevice(d hid.Info) {
	name := d.Product
	if name == "" {
		name = "(unnamed)"
	}
	fmt.Println(styleTitle.Render(fmt.Sprintf("%s  %04x:%04x", name, d.VendorID, d.ProductID)))

	row := func(k, v string) {
		fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-14s", k)), v)
	}
	if d.Manufact != "" {
		row("manufacturer", d.Manufact)
	}
	row("location", fmt.Sprintf("0x%08X", d.LocationID))
	row("version", fmt.Sprintf("0x%04X", d.Version))
	row("report sizes", fmt.Sprintf("in %d  out %d  feature %d", d.MaxIn, d.MaxOut, d.MaxFeature))
	row("primary", d.Primary.String())

	pairs := make([]string, 0, len(d.Usages))
	for _, u := range d.Usages {
		s := u.String()
		if u.Vendor() {
			s = styleHit.Render(s)
		}
		pairs = append(pairs, s)
	}
	row("usages", strings.Join(pairs, "  "))
	fmt.Println()
}

func tryOpen() error {
	d, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
	if err != nil {
		return err
	}
	defer d.Close()

	fmt.Println(styleTitle.Render("open ok"))
	fmt.Printf("  %s %s\n",
		styleKey.Render(fmt.Sprintf("%-14s", "interface")),
		fmt.Sprintf("%04x:%04x at 0x%08X", d.Info.VendorID, d.Info.ProductID, d.Info.LocationID))
	fmt.Printf("  %s %s\n",
		styleKey.Render(fmt.Sprintf("%-14s", "transport")),
		hid.Transport(d.Info.ProductID))

	audio := d.Info.HasUsage(0xFF14, 0x01)
	fmt.Printf("  %s %v\n", styleKey.Render(fmt.Sprintf("%-14s", "0xFF14/0x01")), audio)
	if !audio {
		return errors.New("this interface does not expose the 0xFF14 vendor collection")
	}
	return nil
}
