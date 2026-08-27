//go:build darwin

package menu

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

void occam_main_async(void);
void occam_window_build(const char **bandLabels, int minDB, int maxDB);
void occam_window_show(void);
void occam_window_set_slots(const char **names, int count, int selected);
void occam_window_set_bands(const int *values, int count);
void occam_window_set_sidetone(int value);
void occam_window_set_status(const char *text);
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Window actions, matching the button tags in window_darwin.m.
const (
	ActionSave   = 1
	ActionReload = 2
)

// WindowHandlers is what the window calls back into. Every one runs off the
// main thread, so they may talk to the device.
type WindowHandlers struct {
	OnBand     func(band, value int)
	OnSlot     func(slot int)
	OnSidetone func(value int)
	OnAction   func(tag int)
}

var (
	winMu sync.RWMutex
	win   WindowHandlers
	built bool
)

// BuildWindow creates the settings window. Call once, from the main thread,
// before Run blocks. bandLabels must have one entry per slider.
func BuildWindow(bandLabels []string, minDB, maxDB int, h WindowHandlers) {
	winMu.Lock()
	win, built = h, true
	winMu.Unlock()

	c, free := cStrings(bandLabels)
	defer free()
	C.occam_window_build(c, C.int(minDB), C.int(maxDB))
}

// ShowWindow brings the window to the front. Must be called on the main
// thread, so from a click handler use RunOnMain(menu.ShowWindow).
func ShowWindow() { C.occam_window_show() }

// mainQueue holds work waiting for the main thread. Buffered so RunOnMain does
// not block a click handler.
var mainQueue = make(chan func(), 64)

// RunOnMain schedules fn on AppKit's main thread. Every menu and window
// callback runs on a goroutine, so anything touching AppKit goes through here.
func RunOnMain(fn func()) {
	select {
	case mainQueue <- fn:
		C.occam_main_async()
	default:
		// Dropping is better than deadlocking a click handler; the queue only
		// fills if the main thread is already wedged.
	}
}

//export occamMainCallback
func occamMainCallback() {
	select {
	case fn := <-mainQueue:
		fn()
	default:
	}
}

// SetSlots fills the preset picker.
func SetSlots(names []string, selected int) {
	c, free := cStrings(names)
	defer free()
	C.occam_window_set_slots(c, C.int(len(names)), C.int(selected))
}

// SetBands moves the sliders without firing change callbacks.
func SetBands(values []int) {
	if len(values) == 0 {
		return
	}
	buf := make([]C.int, len(values))
	for i, v := range values {
		buf[i] = C.int(v)
	}
	C.occam_window_set_bands(&buf[0], C.int(len(buf)))
}

// SetSidetone moves the sidetone slider without firing a callback.
func SetSidetone(value int) { C.occam_window_set_sidetone(C.int(value)) }

// SetStatus writes the line beside the buttons.
func SetStatus(s string) {
	c := C.CString(s)
	defer C.free(unsafe.Pointer(c))
	C.occam_window_set_status(c)
}

// cStrings builds a NULL-free char** for AppKit and returns its cleanup.
func cStrings(in []string) (**C.char, func()) {
	if len(in) == 0 {
		return nil, func() {}
	}
	ptrs := make([]*C.char, len(in))
	for i, s := range in {
		ptrs[i] = C.CString(s)
	}
	return &ptrs[0], func() {
		for _, p := range ptrs {
			C.free(unsafe.Pointer(p))
		}
	}
}

func handlers() WindowHandlers {
	winMu.RLock()
	defer winMu.RUnlock()
	return win
}

//export occamBandChanged
func occamBandChanged(band, value C.int) {
	if h := handlers().OnBand; h != nil {
		go h(int(band), int(value))
	}
}

//export occamSlotChanged
func occamSlotChanged(slot C.int) {
	if h := handlers().OnSlot; h != nil {
		go h(int(slot))
	}
}

//export occamSidetoneChanged
func occamSidetoneChanged(value C.int) {
	if h := handlers().OnSidetone; h != nil {
		go h(int(value))
	}
}

//export occamAction
func occamAction(tag C.int) {
	if h := handlers().OnAction; h != nil {
		go h(int(tag))
	}
}
