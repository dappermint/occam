// Package hid talks to USB HID devices through IOHIDManager.
package hid

import (
	"fmt"
	"strings"
)

// UsagePair is one top-level collection exposed by a HID interface.
type UsagePair struct {
	Page  uint32
	Usage uint32
}

func (u UsagePair) String() string {
	return fmt.Sprintf("0x%04X/0x%02X", u.Page, u.Usage)
}

// Vendor reports whether the usage page is in the vendor-defined range.
func (u UsagePair) Vendor() bool {
	return u.Page >= 0xFF00
}

// Info describes one HID interface as IOHIDManager sees it.
type Info struct {
	VendorID   uint16
	ProductID  uint16
	Product    string
	Manufact   string
	Serial     string
	Version    uint16
	LocationID uint32
	Primary    UsagePair
	Usages     []UsagePair
	MaxIn      int
	MaxOut     int
	MaxFeature int
	Descriptor []byte
}

// HasUsage reports whether the interface exposes the given page and usage.
func (i Info) HasUsage(page, usage uint32) bool {
	for _, u := range i.Usages {
		if u.Page == page && u.Usage == usage {
			return true
		}
	}
	return false
}

// RunResult is what CFRunLoopRunInMode returned on the last turn. It separates
// a real wait from a run loop that had no sources to wait on.
type RunResult int

const (
	RunFinished      RunResult = 1 // no sources installed, the wait was a spin
	RunStopped       RunResult = 2
	RunTimedOut      RunResult = 3 // waited properly, nothing arrived
	RunHandledSource RunResult = 4
)

func (r RunResult) String() string {
	switch r {
	case RunFinished:
		return "finished (no sources: the callback was never armed)"
	case RunStopped:
		return "stopped"
	case RunTimedOut:
		return "timed out (armed and waiting, device said nothing)"
	case RunHandledSource:
		return "handled a source"
	default:
		return fmt.Sprintf("unknown (%d)", int(r))
	}
}

// Razer is the USB vendor ID shared by every Razer device.
const Razer uint16 = 0x1532

// The V3 Pro reaches the host two ways, as two different USB products. Both
// expose the same report descriptor and the same 0xFF14 vendor collection, so
// the protocol does not care which one is attached.
const (
	V3ProDongle uint16 = 0x0577 // HyperSpeed 2.4 GHz dongle
	V3ProWired  uint16 = 0x0576 // headset over USB-C, "BlackShark V3 Pro USB"
)

// BlackSharkV3Pro lists both products in preference order: the dongle first,
// since a wired headset is usually only plugged in to charge.
var BlackSharkV3Pro = []uint16{V3ProDongle, V3ProWired}

func hexList(pids []uint16) string {
	if len(pids) == 0 {
		return "(no product ids)"
	}
	out := make([]string, len(pids))
	for i, p := range pids {
		out[i] = fmt.Sprintf("%04x", p)
	}
	return strings.Join(out, "/")
}

// Transport names how a product reaches the host.
func Transport(pid uint16) string {
	switch pid {
	case V3ProDongle:
		return "dongle"
	case V3ProWired:
		return "wired"
	default:
		return "unknown"
	}
}
