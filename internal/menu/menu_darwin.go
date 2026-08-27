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
void occam_menu_add_separator(void);
void occam_menu_run(void);
void occam_menu_quit(void);
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Item is one row. A Separator ignores every other field.
type Item struct {
	Title     string
	Tag       int
	Checked   bool
	Disabled  bool
	Separator bool
}

// Sep is a separator row.
func Sep() Item { return Item{Separator: true} }

// AppKit gives no way to pass a Go pointer through a menu action, so the one
// live menu lives here. There is only ever one status item.
var (
	mu      sync.Mutex
	build   func() []Item
	onClick func(tag int)
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
		c := C.CString(it.Title)
		enabled, checked := C.int(1), C.int(0)
		if it.Disabled {
			enabled = 0
		}
		if it.Checked {
			checked = 1
		}
		C.occam_menu_add(c, C.int(it.Tag), checked, enabled)
		C.free(unsafe.Pointer(c))
	}
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
