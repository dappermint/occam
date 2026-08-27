package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

func newProfile() *cobra.Command {
	c := &cobra.Command{
		Use:   "profile",
		Short: "read every EQ slot back off the headset",
		Long: "Reads all nine slots the way Synapse does: getEQOrderInfo to position\n" +
			"the cursor, then getCustomerEQBand to read the entry.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
			if err != nil {
				return err
			}
			defer dev.Close()

			fmt.Printf("%s %04x:%04x (%s)\n\n",
				styleTitle.Render("profile"),
				dev.Info.VendorID, dev.Info.ProductID, hid.Transport(dev.Info.ProductID))

			fmt.Printf("  %s %s\n",
				styleKey.Render(fmt.Sprintf("%-4s", "slot")),
				styleKey.Render("bands (dB)"))

			for pos := byte(0); pos < proto.Slots; pos++ {
				s, err := readSlot(dev, pos)
				if errors.Is(err, proto.ErrHeadsetOff) {
					return fmt.Errorf("%w: the dongle is connected but has nothing to report", err)
				}
				if err != nil {
					fmt.Printf("  %-4d %s\n", pos, styleDim.Render(err.Error()))
					continue
				}

				mark := " "
				if s.Order.Active {
					mark = "*"
				}
				name, _ := proto.LibraryName(s.Order.CloudID)
				line := fmt.Sprintf("%s%-3d %-24s %s", mark, pos, truncate(name, 24), s.EQ)
				if s.Order.Active {
					line = styleHit.Render(line)
				}

				tags := ""
				if !s.Order.Enabled {
					tags += " disabled"
				}
				if s.Order.Custom {
					tags += " custom"
				}
				if s.Order.CloudID != 0 {
					tags += fmt.Sprintf(" cloud=%d", s.Order.CloudID)
				}
				fmt.Printf("  %s%s\n", line, styleDim.Render(tags))

				time.Sleep(interFrame)
			}

			fmt.Println()
			fmt.Println(styleDim.Render("  * is the active slot"))
			return nil
		},
	}
	return c
}
