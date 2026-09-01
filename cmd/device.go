package cmd

import (
	"fmt"
	"time"

	"github.com/dappermint/occam/internal/hid"
	"github.com/dappermint/occam/internal/proto"
)

// interFrame is the gap Synapse leaves between transfers. Its device layer
// logs it as sleepTimeBetweenOut: 30, so this is measured, not guessed.
const interFrame = 30 * time.Millisecond

// replyTimeout is generous: Synapse's own device layer retries a read ten times.
const replyTimeout = 2 * time.Second

// The device NAKs occasionally and there is nothing wrong when it does.
// Synapse's device layer logs maxRetryOut: 20 and maxRetryIn: 10, so retrying
// is part of the protocol rather than a workaround.
const (
	maxAttempts  = 10
	retryBackoff = 40 * time.Millisecond
)

// retry runs op until it succeeds or the attempts run out.
func retry(what string, op func() error) error {
	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if last = op(); last == nil {
			return nil
		}
		time.Sleep(retryBackoff)
	}
	return fmt.Errorf("%s failed %d times, last: %w", what, maxAttempts, last)
}

// Slot is one EQ slot as the headset holds it.
type Slot struct {
	Order proto.Order
	EQ    proto.EQ
}

// ask sends a message and returns the decoded reply, refusing anything the
// device did not mark successful.
func ask(dev *hid.Device, m *proto.Message) (*proto.Message, error) {
	out, err := m.Encode()
	if err != nil {
		return nil, err
	}

	var reply *proto.Message
	err = retry(proto.CommandName(m.Command), func() error {
		in, err := dev.Request(proto.ReportID, out, replyTimeout)
		if err != nil {
			return err
		}
		reply, err = proto.Decode(in)
		if err != nil {
			return err
		}
		if reply.Status != proto.StatusSuccess {
			return fmt.Errorf("device replied %s", proto.StatusText(reply.Status))
		}
		if reply.Command != m.Command {
			return fmt.Errorf("reply is for %s, not %s",
				proto.CommandName(reply.Command), proto.CommandName(m.Command))
		}
		// Retrying will not help: the headset has to come back first.
		if proto.Unavailable(reply.Args) {
			return nil
		}
		return nil
	})
	return reply, err
}

// send writes a message and discards the reply.
//
// It has to read that reply even though nothing wants it. The device answers
// every command on one interrupt-IN pipe, so a write whose reply is left
// queued desynchronises the next read: a get would come back holding the set's
// answer. Reading it also means a device-level rejection surfaces here rather
// than being reported as success.
func send(dev *hid.Device, m *proto.Message) error {
	_, err := ask(dev, m)
	if err != nil {
		return err
	}
	time.Sleep(interFrame)
	return nil
}

// readSlot performs the two-call read Synapse uses. OrderInfo positions the
// device's cursor; GetBands then returns the entry it is sitting on. Calling
// GetBands alone gets a stale buffer back.
func readSlot(dev *hid.Device, position byte) (Slot, error) {
	var s Slot

	order, err := ask(dev, proto.OrderInfo(position))
	if err != nil {
		return s, fmt.Errorf("order info for slot %d: %w", position, err)
	}
	if proto.Unavailable(order.Args) {
		return s, proto.ErrHeadsetOff
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

// writeSlot writes one curve. The sequence is: activate the slot (0xE1,
// argc=1, slot index), write the bands (0x95, argc=11, slot + 10 bands),
// then commit (0xEB, argc=11, slot + 10 bands). The commit is what makes
// the write stick; without it the device ACKs and discards.
func writeSlot(dev *hid.Device, position byte, eq proto.EQ) error {
	commit := proto.New(0xEB, 0x00, append([]byte{position}, eq.Bytes()...)...)
	for _, m := range []*proto.Message{
		proto.New(0xE1, 0x00, position),
		proto.SetBands(position, eq),
		commit,
	} {
		if err := send(dev, m); err != nil {
			return fmt.Errorf("slot %d: %w", position, err)
		}
	}
	return nil
}

// selectSlot makes a slot the active one.
func selectSlot(dev *hid.Device, position byte) error {
	return send(dev, proto.SelectPreset(position))
}
