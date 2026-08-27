package cmd

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

func newListen() *cobra.Command {
	var seconds int

	c := &cobra.Command{
		Use:   "listen",
		Short: "print input reports the device pushes, without sending anything",
		Long: "Passive. Useful for telling apart a device that is not answering from\n" +
			"a run loop that is not wired up: press a button on the headset and the\n" +
			"consumer control reports should appear here.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
			if err != nil {
				return err
			}
			defer dev.Close()

			fmt.Printf("%s %s\n\n",
				styleKey.Render(fmt.Sprintf("%-10s", "listening")),
				styleDim.Render(fmt.Sprintf("%d seconds, press buttons on the headset", seconds)))

			n, err := dev.Listen(time.Duration(seconds)*time.Second, func(id byte, data []byte) {
				fmt.Printf("  %s %s\n",
					styleHit.Render(fmt.Sprintf("report 0x%02X (%d)", id, len(data))),
					hex.EncodeToString(data))
				if id != proto.ReportID {
					return
				}
				if m, err := proto.Decode(data); err == nil {
					fmt.Printf("             %s\n", m)
				}
			})
			if err != nil {
				return err
			}
			fmt.Printf("\n%s %d\n", styleKey.Render(fmt.Sprintf("%-10s", "reports")), n)
			fmt.Printf("%s %s\n", styleKey.Render(fmt.Sprintf("%-10s", "run loop")), dev.LastRunResult)
			return nil
		},
	}
	c.Flags().IntVar(&seconds, "seconds", 8, "how long to listen")
	return c
}
