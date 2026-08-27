package cmd

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

type step struct {
	name string
	msg  *proto.Message
}

func newEQ() *cobra.Command {
	var (
		preset   string
		bandSpec string
		slot     int
		activate bool
		dryRun   bool
	)

	c := &cobra.Command{
		Use:   "eq",
		Short: "write an EQ curve into one of the headset's slots",
		Long: "Brackets the write with setEQOrderUpdateStartStop the way Synapse does,\n" +
			"then writes the curve with setCustomerEQBand.\n\n" +
			"Band values are dB and are encoded sign-magnitude, matching the captured\n" +
			"frames. Start with --dry-run to see the exact bytes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			eq, err := resolveEQ(preset, bandSpec)
			if err != nil {
				return err
			}
			if slot < 0 || slot >= proto.Slots {
				return fmt.Errorf("slot is %d, must be 0 to %d", slot, proto.Slots-1)
			}

			steps := []step{
				{"eqUpdateStart", proto.EQUpdateStart()},
				{"setCustomerEQBand", proto.SetBands(byte(slot), eq)},
				{"eqUpdateStop", proto.EQUpdateStop()},
			}
			if activate {
				steps = append(steps, step{"setSpeakerPresetEQ", proto.SelectPreset(byte(slot))})
			}

			fmt.Println(styleTitle.Render(fmt.Sprintf("slot %d  %s", slot, eq)))

			var dev *hid.Device
			if dryRun {
				fmt.Println(styleDim.Render("  dry run, nothing is sent"))
			} else {
				dev, err = hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
				if err != nil {
					return err
				}
				defer dev.Close()
				fmt.Printf("  %s %04x:%04x (%s)\n",
					styleKey.Render(fmt.Sprintf("%-18s", "device")),
					dev.Info.VendorID, dev.Info.ProductID, hid.Transport(dev.Info.ProductID))
			}
			fmt.Println()

			for _, s := range steps {
				payload, err := s.msg.Encode()
				if err != nil {
					return err
				}
				fmt.Printf("  %s %s\n",
					styleKey.Render(fmt.Sprintf("%-18s", s.name)),
					hex.EncodeToString(payload[:24]))
				if dryRun {
					continue
				}
				if err := dev.SetReport(proto.ReportID, payload); err != nil {
					return fmt.Errorf("%s: %w", s.name, err)
				}
				time.Sleep(interFrame)
			}
			fmt.Println()

			if dryRun {
				return nil
			}
			fmt.Println(styleHit.Render("sent"))
			fmt.Println(styleDim.Render("  every report was accepted. that is not the same as the device"))
			fmt.Println(styleDim.Render("  acting on them, so listen before believing it."))
			return nil
		},
	}

	c.Flags().StringVar(&preset, "preset", "flat", "one of flat, game, music, movie")
	c.Flags().StringVar(&bandSpec, "bands", "", "ten comma-separated dB values, overrides --preset")
	c.Flags().IntVar(&slot, "slot", 0, fmt.Sprintf("EQ slot to write, 0 to %d", proto.Slots-1))
	c.Flags().BoolVar(&activate, "activate", true, "also select the slot with setSpeakerPresetEQ")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print the frames without opening or writing to the device")
	return c
}

func resolveEQ(preset, bandSpec string) (proto.EQ, error) {
	if bandSpec == "" {
		eq, ok := proto.Presets[preset]
		if !ok {
			return proto.EQ{}, fmt.Errorf("no preset %q, have flat, game, music, movie", preset)
		}
		return eq, nil
	}

	parts := strings.Split(bandSpec, ",")
	if len(parts) != proto.Bands {
		return proto.EQ{}, fmt.Errorf("got %d bands, need exactly %d", len(parts), proto.Bands)
	}
	var eq proto.EQ
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return proto.EQ{}, fmt.Errorf("band %d: %w", i+1, err)
		}
		if v < -127 || v > 127 {
			return proto.EQ{}, fmt.Errorf("band %d is %d, outside the sign-magnitude range", i+1, v)
		}
		eq[i] = int8(v)
	}
	return eq, nil
}
