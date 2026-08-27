package cmd

import (
	"fmt"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

// Slot is one EQ slot as the headset holds it.
type Slot struct {
	Order proto.Order
	EQ    proto.EQ
}

// readSlot performs the two-call read Synapse uses. OrderInfo positions the
// device's cursor; GetBands then returns the entry it is sitting on. Calling
// GetBands alone gets a stale buffer back, with flags 0x00 rather than 0x80.
func readSlot(dev *hid.Device, position byte) (Slot, error) {
	var s Slot

	order, err := ask(dev, proto.OrderInfo(position))
	if err != nil {
		return s, fmt.Errorf("order info for slot %d: %w", position, err)
	}
	if s.Order, err = proto.ParseOrder(order.Args); err != nil {
		return s, err
	}

	bands, err := ask(dev, proto.GetBands(position))
	if err != nil {
		return s, fmt.Errorf("bands for slot %d: %w", position, err)
	}
	if len(bands.Args) < 1+proto.Bands {
		return s, fmt.Errorf("slot %d returned %d argument bytes, need %d",
			position, len(bands.Args), 1+proto.Bands)
	}
	if s.EQ, err = proto.ParseBands(bands.Args[1 : 1+proto.Bands]); err != nil {
		return s, err
	}
	return s, nil
}

func ask(dev *hid.Device, m *proto.Message) (*proto.Message, error) {
	out, err := m.Encode()
	if err != nil {
		return nil, err
	}
	in, err := dev.Request(proto.ReportID, out, replyTimeout)
	if err != nil {
		return nil, err
	}
	reply, err := proto.Decode(in)
	if err != nil {
		return nil, err
	}
	if reply.Status != proto.StatusSuccess {
		return nil, fmt.Errorf("device replied %s", proto.StatusText(reply.Status))
	}
	return reply, nil
}

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
				if err != nil {
					fmt.Printf("  %-4d %s\n", pos, styleDim.Render(err.Error()))
					continue
				}

				mark := " "
				if s.Order.Active {
					mark = "*"
				}
				line := fmt.Sprintf("%s%-3d %s", mark, pos, s.EQ)
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
