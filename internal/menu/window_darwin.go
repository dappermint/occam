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
void occam_window_set_mic_bands(const int *values, int count);
void occam_window_set_mic_presets(const char **names, int count, int selected);
void occam_window_set_sidetone(int value);
void occam_window_set_led_modes(const char **names, int count);
void occam_window_set_anc_modes(const char **names, int count);
void occam_window_set_sleep_options(const char **names, int count);
void occam_window_set_extras(int ancMode, int ancLevel, int micMuted,
                             int balance, int ledMode, int sleepIndex,
                             int lowLatency);
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
	OnBand       func(band, value int)
	OnMicBand    func(band, value int)
	OnMicPreset  func(index int)
	OnSlot       func(slot int)
	OnSidetone   func(value int)
	OnANC        func(mode, level int)
	OnMic        func(muted bool)
	OnBalance    func(value int)
	OnLED        func(mode int)
	OnPowerOff   func(index int)
	OnLowLatency func(on bool)
	OnAction     func(tag int)
}

// Extras is everything in the window below the equalizer.
type Extras struct {
	ANCMode    int
	ANCLevel   int
	MicMuted   bool
	Balance    int
	LEDMode    int
	SleepIndex int
	LowLatency bool
	Sidetone   int
	MicPreset  int
	MicBands   [10]int8
}

// SetLEDModes fills the indicator light popup. The device takes 0, 1 or 2.
func SetLEDModes(names []string) {
	c, free := cStrings(names)
	defer free()
	C.occam_window_set_led_modes(c, C.int(len(names)))
}

// SetANCModes fills the noise cancelling popup.
func SetANCModes(names []string) {
	c, free := cStrings(names)
	defer free()
	C.occam_window_set_anc_modes(c, C.int(len(names)))
}

// SetSleepOptions fills the idle timeout popup.
func SetSleepOptions(names []string) {
	c, free := cStrings(names)
	defer free()
	C.occam_window_set_sleep_options(c, C.int(len(names)))
}

// SetExtras fills those controls without firing their callbacks.
func SetExtras(e Extras) {
	C.occam_window_set_extras(C.int(e.ANCMode), C.int(e.ANCLevel), cbool(e.MicMuted),
		C.int(e.Balance), C.int(e.LEDMode), C.int(e.SleepIndex), cbool(e.LowLatency))
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
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

// SetMicPresets fills the mic EQ preset picker.
func SetMicPresets(names []string, selected int) {
	c, free := cStrings(names)
	defer free()
	C.occam_window_set_mic_presets(c, C.int(len(names)), C.int(selected))
}

// SetMicBands moves the mic EQ sliders without firing change callbacks.
func SetMicBands(values []int) {
	if len(values) == 0 {
		return
	}
	buf := make([]C.int, len(values))
	for i, v := range values {
		buf[i] = C.int(v)
	}
	C.occam_window_set_mic_bands(&buf[0], C.int(len(buf)))
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

//export occamMicBandChanged
func occamMicBandChanged(band, value C.int) {
	if h := handlers().OnMicBand; h != nil {
		go h(int(band), int(value))
	}
}

//export occamMicPresetChanged
func occamMicPresetChanged(index C.int) {
	if h := handlers().OnMicPreset; h != nil {
		go h(int(index))
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

//export occamANCChanged
func occamANCChanged(mode, level C.int) {
	if h := handlers().OnANC; h != nil {
		go h(int(mode), int(level))
	}
}

//export occamMicChanged
func occamMicChanged(muted C.int) {
	if h := handlers().OnMic; h != nil {
		go h(muted != 0)
	}
}

//export occamBalanceChanged
func occamBalanceChanged(value C.int) {
	if h := handlers().OnBalance; h != nil {
		go h(int(value))
	}
}

//export occamLEDChanged
func occamLEDChanged(mode C.int) {
	if h := handlers().OnLED; h != nil {
		go h(int(mode))
	}
}

//export occamPowerOffChanged
func occamPowerOffChanged(index C.int) {
	if h := handlers().OnPowerOff; h != nil {
		go h(int(index))
	}
}

//export occamLowLatencyChanged
func occamLowLatencyChanged(on C.int) {
	if h := handlers().OnLowLatency; h != nil {
		go h(on != 0)
	}
}

//export occamAction
func occamAction(tag C.int) {
	if h := handlers().OnAction; h != nil {
		go h(int(tag))
	}
}
