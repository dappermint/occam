//go:build darwin

// Package menu is a minimal NSStatusItem wrapper: one menu bar item whose
// contents are rebuilt each time it opens.
//
// No .app bundle. NSApplicationActivationPolicyAccessory keeps it out of the
// Dock and the app switcher, which is all a bundle would have bought here.
package menu

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

void occam_menu_start(const char *title);
int occam_menu_set_symbol(const char *name, const char *fallback);
void occam_menu_set_title(const char *title);
void occam_menu_clear(void);
void occam_menu_add(const char *title, int tag, int checked, int enabled);
void occam_menu_add_section(const char *title);
void occam_menu_add_slider(const char *label, int tag, double lo, double hi,
                           double value, int enabled);
void occam_menu_add_segments(const char **labels, const char **symbols, int count,
                             int tag, int selected);
void occam_menu_add_separator(void);
void occam_menu_set_row_hidden(int tag, int hidden);
void occam_menu_run(void);
void occam_menu_quit(void);
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Item is one row. Separator and Section ignore the other fields.
type Item struct {
	Title     string
	Tag       int
	Checked   bool
	Disabled  bool
	Separator bool
	Section   bool
	// Slider turns the row into a labelled control, using Min, Max and Value.
	Slider bool
	Min    int
	Max    int
	Value  int
	// Segments turns the row into a picker, with Value as the chosen index.
	// Symbols, when present, replace the labels with SF Symbols.
	Segments []string
	Symbols  []string
}

// Sep is a separator row.
func Sep() Item { return Item{Separator: true} }

// Section is a system-drawn section header.
func Section(title string) Item { return Item{Title: title, Section: true} }

// Segments is a segmented picker row, the shape Apple uses for noise control.
// Symbols may be nil, which falls back to the labels.
func Segments(tag int, labels, symbols []string, selected int) Item {
	return Item{Tag: tag, Segments: labels, Symbols: symbols, Value: selected}
}

// Slider is a labelled slider row. Tag identifies it to the slide callback.
func Slider(title string, tag, min, max, value int, enabled bool) Item {
	return Item{
		Title: title, Tag: tag, Slider: true,
		Min: min, Max: max, Value: value, Disabled: !enabled,
	}
}

// AppKit gives no way to pass a Go pointer through a menu action, so the one
// live menu lives here. There is only ever one status item.
var (
	mu      sync.Mutex
	build   func() []Item
	onClick func(tag int)
	onValue func(tag, value int)
	running bool
)

// ErrRunning means Run was called twice.
var ErrRunning = errors.New("menu: already running")

// Run installs the status item and blocks in the AppKit event loop.
//
// It MUST be called from the main OS thread. Package main locks it.
//
// build is called on the main thread every time the menu opens, so it has to
// render from cached state; doing device I/O there stalls the menu.
func Run(symbol, fallback string, buildFn func() []Item, clickFn func(tag int)) error {
	mu.Lock()
	if running {
		mu.Unlock()
		return ErrRunning
	}
	running, build, onClick = true, buildFn, clickFn
	mu.Unlock()

	cTitle := C.CString(fallback)
	C.occam_menu_start(cTitle)
	C.free(unsafe.Pointer(cTitle))

	SetSymbol(symbol, fallback)
	render()
	C.occam_menu_run()
	return nil
}

// SetRowHidden shows or hides a slider or segmented row without rebuilding the
// menu, for when a click changes whether a later row applies.
func SetRowHidden(tag int, hidden bool) {
	C.occam_menu_set_row_hidden(C.int(tag), cbool(hidden))
}

// OnValue registers the handler for slider and segmented rows. Separate from
// Run's click handler because those carry a value the tag cannot.
func OnValue(fn func(tag, value int)) {
	mu.Lock()
	onValue = fn
	mu.Unlock()
}

// SetSymbol puts an SF Symbol in the menu bar as a template image, so it
// tracks the bar's light, dark and highlighted states. Falls back to text when
// the symbol name is not available, and reports which it used.
func SetSymbol(name, fallback string) bool {
	n, f := C.CString(name), C.CString(fallback)
	defer C.free(unsafe.Pointer(n))
	defer C.free(unsafe.Pointer(f))
	return C.occam_menu_set_symbol(n, f) != 0
}

// SetTitle changes the menu bar text. Safe from any goroutine.
func SetTitle(s string) {
	c := C.CString(s)
	defer C.free(unsafe.Pointer(c))
	C.occam_menu_set_title(c)
}

// Quit stops the event loop and exits the process.
func Quit() { C.occam_menu_quit() }

func render() {
	mu.Lock()
	fn := build
	mu.Unlock()
	if fn == nil {
		return
	}

	C.occam_menu_clear()
	for _, it := range fn() {
		if it.Separator {
			C.occam_menu_add_separator()
			continue
		}
		if it.Section {
			h := C.CString(it.Title)
			C.occam_menu_add_section(h)
			C.free(unsafe.Pointer(h))
			continue
		}
		if len(it.Segments) > 0 {
			labels, freeLabels := cStrings(it.Segments)
			symbols, freeSymbols := cStrings(it.Symbols)
			if len(it.Symbols) != len(it.Segments) {
				symbols = nil
			}
			C.occam_menu_add_segments(labels, symbols, C.int(len(it.Segments)),
				C.int(it.Tag), C.int(it.Value))
			freeSymbols()
			freeLabels()
			continue
		}

		c := C.CString(it.Title)
		enabled, checked := C.int(1), C.int(0)
		if it.Disabled {
			enabled = 0
		}
		if it.Checked {
			checked = 1
		}
		if it.Slider {
			C.occam_menu_add_slider(c, C.int(it.Tag), C.double(it.Min), C.double(it.Max),
				C.double(it.Value), enabled)
		} else {
			C.occam_menu_add(c, C.int(it.Tag), checked, enabled)
		}
		C.free(unsafe.Pointer(c))
	}
}

//export occamMenuValue
func occamMenuValue(tag, value C.int) {
	mu.Lock()
	fn := onValue
	mu.Unlock()
	if fn == nil {
		return
	}
	go fn(int(tag), int(value))
}

//export occamMenuWillOpen
func occamMenuWillOpen() { render() }

//export occamMenuSelect
func occamMenuSelect(tag C.int) {
	mu.Lock()
	fn := onClick
	mu.Unlock()
	if fn == nil {
		return
	}
	// Off the main thread: a click usually means device I/O, and blocking here
	// freezes the menu bar.
	go fn(int(tag))
}
