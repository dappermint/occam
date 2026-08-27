package cmd

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

type reader struct {
	name string
	msg  func(slot byte) *proto.Message
	show func(m *proto.Message) string
}

var readers = map[string]reader{
	"battery": {
		name: "battery",
		msg:  func(byte) *proto.Message { return proto.Battery() },
		show: func(m *proto.Message) string {
			if len(m.Args) < 1 {
				return "no level returned"
			}
			return fmt.Sprintf("%d%%", m.Args[0])
		},
	},
	"charging": {
		name: "charging",
		msg:  func(byte) *proto.Message { return proto.Charging() },
		show: func(m *proto.Message) string {
			if len(m.Args) < 1 {
				return "no status returned"
			}
			if m.Args[0] == 0 {
				return "off"
			}
			return fmt.Sprintf("on (0x%02X)", m.Args[0])
		},
	},
	"serial": {
		name: "serial",
		msg:  func(byte) *proto.Message { return proto.SerialNumber() },
		show: func(m *proto.Message) string { return strings.TrimRight(string(m.Args), "\x00") },
	},
	"firmware": {
		name: "firmware",
		msg:  func(byte) *proto.Message { return proto.FirmwareVersion() },
		show: func(m *proto.Message) string { return fmt.Sprintf("% X", m.Args) },
	},
	"sidetone": {
		name: "sidetone",
		msg:  func(byte) *proto.Message { return proto.New(proto.GetSidetoneVolume, 0x00) },
		show: func(m *proto.Message) string { return fmt.Sprintf("% X", m.Args) },
	},
}

func newGet() *cobra.Command {
	var slot int
	var raw bool

	names := make([]string, 0, len(readers))
	for k := range readers {
		names = append(names, k)
	}

	c := &cobra.Command{
		Use:       "get <" + strings.Join(names, "|") + ">",
		Short:     "read a value back from the headset",
		Args:      cobra.ExactArgs(1),
		ValidArgs: names,
		Long: "Sends a read command then pulls the reply with a synchronous GET_REPORT.\n" +
			"Synapse reads its replies off the interrupt IN endpoint instead, so if\n" +
			"this returns nothing the device only answers asynchronously and occam\n" +
			"needs the CFRunLoop input callback.",
		RunE: func(cmd *cobra.Command, args []string) error {
			r, ok := readers[args[0]]
			if !ok {
				return fmt.Errorf("no reader %q, have %s", args[0], strings.Join(names, ", "))
			}

			dev, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
			if err != nil {
				return err
			}
			defer dev.Close()

			out, err := r.msg(byte(slot)).Encode()
			if err != nil {
				return err
			}
			in, err := dev.Request(proto.ReportID, out, replyTimeout)
			if err != nil {
				return fmt.Errorf("%s: %w", r.name, err)
			}
			if raw {
				fmt.Println(hex.EncodeToString(in))
			}

			m, err := proto.Decode(in)
			if err != nil {
				return fmt.Errorf("%w\n  raw: %s", err, hex.EncodeToString(in))
			}
			if m.Status != proto.StatusSuccess {
				return fmt.Errorf("device replied %s to %s", proto.StatusText(m.Status), r.name)
			}

			fmt.Printf("%s %s\n", styleKey.Render(fmt.Sprintf("%-10s", r.name)), styleHit.Render(r.show(m)))
			return nil
		},
	}
	c.Flags().IntVar(&slot, "slot", 0, "EQ slot, for get eq")
	c.Flags().BoolVar(&raw, "raw", false, "also print the raw reply payload")
	return c
}
