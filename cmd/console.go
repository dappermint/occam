package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dappermint/glass/console"
	"github.com/dappermint/glass/glass"
	"github.com/dappermint/glass/gs"
	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
	"github.com/spf13/cobra"
)

const defaultSocket = "/tmp/occam.sock"

func newConsole() *cobra.Command {
	var socket string

	c := &cobra.Command{
		Use:   "console",
		Short: "hold the dongle open and expose it on a glass console",
		Long: "Opens the 0xFF14 interface and keeps it open while a glass console\n" +
			"serves on a unix socket. Attach with: nc -U " + defaultSocket + "\n\n" +
			"This is the reverse-engineering loop. The device stays open between\n" +
			"pokes, so frames can be built, mutated and resent without reattaching.",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := hid.Open(hid.Razer, hid.BlackSharkV3Pro...)
			if err != nil {
				return err
			}
			defer d.Close()

			// Every type the console can reach has to be registered, the
			// facade included, or method dispatch on it fails at the prompt.
			gs.Register[proto.Message]()
			gs.Register[hid.Info]()
			gs.Register[hid.Device]()
			gs.Register[protoFacade]()

			in := glass.New()
			in.Define("dev", d)
			in.Define("proto", newProtoFacade(d))

			srv, err := console.Serve(socket, in)
			if err != nil {
				return err
			}
			defer srv.Close()

			fmt.Println(styleTitle.Render("console up"))
			fmt.Printf("  %s %s\n", styleKey.Render(fmt.Sprintf("%-14s", "socket")), socket)
			fmt.Printf("  %s %04x:%04x\n", styleKey.Render(fmt.Sprintf("%-14s", "device")), d.Info.VendorID, d.Info.ProductID)
			fmt.Println(styleDim.Render("  attach: nc -U " + socket))

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig
			fmt.Println()
			return nil
		},
	}
	c.Flags().StringVar(&socket, "socket", defaultSocket, "unix socket path for the glass console")
	return c
}

// protoFacade is the surface the console pokes at. Methods stay small and
// string-free so they read well from the interpreter.
type protoFacade struct {
	dev *hid.Device

	// Last holds the bytes of the most recent send, Reply the most recent
	// input report, both for inspection after the fact.
	Last    []byte
	Reply   []byte
	ReplyID byte
}

func newProtoFacade(d *hid.Device) *protoFacade {
	return &protoFacade{dev: d}
}

// Send encodes a message and writes it to report 0x02 on page 0xFF14.
func (p *protoFacade) Send(m *proto.Message) error {
	buf, err := m.Encode()
	if err != nil {
		return err
	}
	p.Last = buf
	return p.dev.SetReport(proto.ReportID, buf)
}

// Raw writes payload bytes verbatim, padded to the declared report count.
func (p *protoFacade) Raw(payload []byte) error {
	buf := make([]byte, proto.PayloadLen)
	copy(buf, payload)
	p.Last = buf
	return p.dev.SetReport(proto.ReportID, buf)
}

// Msg builds a message without sending it, so the console can mutate it first.
func (p *protoFacade) Msg(command, sub byte, args ...byte) *proto.Message {
	return proto.New(command, sub, args...)
}

// Decode reads a payload back, checksum verified.
func (p *protoFacade) Decode(payload []byte) (*proto.Message, error) {
	return proto.Decode(payload)
}

// Commands lists every command id read out of the Synapse logs.
func (p *protoFacade) Commands() map[byte]string { return proto.Commands() }

// Ask sends a message and waits for the reply on the interrupt IN endpoint.
// This is the one to reach for: GetReport does not work for this protocol.
func (p *protoFacade) Ask(m *proto.Message) (*proto.Message, error) {
	buf, err := m.Encode()
	if err != nil {
		return nil, err
	}
	p.Last = buf
	in, err := p.dev.Request(proto.ReportID, buf, 2*time.Second)
	if err != nil {
		return nil, err
	}
	p.Reply = in
	return proto.Decode(in)
}

// AskRaw is Ask without the encoder, for trying a payload byte by byte.
func (p *protoFacade) AskRaw(payload []byte) ([]byte, error) {
	buf := make([]byte, proto.PayloadLen)
	copy(buf, payload)
	p.Last = buf
	in, err := p.dev.Request(proto.ReportID, buf, 2*time.Second)
	p.Reply = in
	return in, err
}

// Listen waits for unprompted input reports and returns how many arrived.
// RunResult afterwards says whether the run loop was actually armed.
func (p *protoFacade) Listen(seconds int) (int, error) {
	return p.dev.Listen(time.Duration(seconds)*time.Second, func(id byte, data []byte) {
		p.Reply = data
		p.ReplyID = id
	})
}

// RunResult reports what the CFRunLoop did on the last wait.
func (p *protoFacade) RunResult() string { return p.dev.LastRunResult.String() }

// CRC exposes the checksum so a hand-built payload can be fixed up.
func (p *protoFacade) CRC(payload []byte) byte { return proto.CRC(payload) }

// Read pulls one input report synchronously. Kept only to demonstrate that it
// returns stale element data that fails its own checksum.
func (p *protoFacade) Read() ([]byte, error) {
	return p.dev.GetReport(proto.ReportID, proto.PayloadLen)
}
