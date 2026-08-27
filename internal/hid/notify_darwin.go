//go:build darwin

package hid

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>

// Attach notifications, in process. launchd's own IOKit matching is not usable
// for this: it requires IOMatchLaunchStream, which obliges the job to drain an
// XPC event stream or be relaunched forever.
typedef struct {
	int fired;
	int arming;
} occam_watchbox;

static occam_watchbox *occam_watchbox_new(void) {
	return (occam_watchbox *)calloc(1, sizeof(occam_watchbox));
}
static void occam_watchbox_free(occam_watchbox *w) { free(w); }
static int occam_watchbox_fired(occam_watchbox *w) { return w->fired; }
static void occam_watchbox_reset(occam_watchbox *w) { w->fired = 0; }

// The iterator must be drained or the notification never re-arms.
static void occam_match_cb(void *refcon, io_iterator_t iter) {
	occam_watchbox *w = (occam_watchbox *)refcon;
	io_object_t obj;
	int seen = 0;
	while ((obj = IOIteratorNext(iter))) {
		IOObjectRelease(obj);
		seen++;
	}
	if (w && !w->arming) w->fired += seen;
}

typedef struct {
	IONotificationPortRef port;
	io_iterator_t         iter[2];
	int                   count;
} occam_watcher;

static occam_watcher *occam_watcher_new(void) {
	return (occam_watcher *)calloc(1, sizeof(occam_watcher));
}

static int occam_watcher_add(occam_watcher *wt, occam_watchbox *w, uint32_t vid, uint32_t pid) {
	if (wt->count >= 2) return 0;
	if (!wt->port) {
		wt->port = IONotificationPortCreate(kIOMainPortDefault);
		if (!wt->port) return 0;
		CFRunLoopAddSource(CFRunLoopGetCurrent(),
			IONotificationPortGetRunLoopSource(wt->port), kCFRunLoopDefaultMode);
	}

	CFMutableDictionaryRef match = IOServiceMatching("IOUSBHostDevice");
	if (!match) return 0;
	int32_t v = (int32_t)vid, p = (int32_t)pid;
	CFNumberRef vn = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &v);
	CFNumberRef pn = CFNumberCreate(kCFAllocatorDefault, kCFNumberSInt32Type, &p);
	CFDictionarySetValue(match, CFSTR("idVendor"), vn);
	CFDictionarySetValue(match, CFSTR("idProduct"), pn);
	CFRelease(vn);
	CFRelease(pn);

	io_iterator_t iter = 0;
	kern_return_t kr = IOServiceAddMatchingNotification(wt->port,
		kIOFirstMatchNotification, match, occam_match_cb, w, &iter);
	if (kr != KERN_SUCCESS) return 0;

	// Arming drains whatever is already attached. Those are not new arrivals,
	// so they must not count as an attach.
	w->arming = 1;
	occam_match_cb(w, iter);
	w->arming = 0;

	wt->iter[wt->count++] = iter;
	return 1;
}

// Per-file cgo preamble, so this cannot reuse the one in hid_darwin.go.
static int occam_notify_run(double seconds) {
	return (int)CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, true);
}

static void occam_watcher_free(occam_watcher *wt) {
	if (!wt) return;
	for (int i = 0; i < wt->count; i++) {
		if (wt->iter[i]) IOObjectRelease(wt->iter[i]);
	}
	if (wt->port) {
		CFRunLoopRemoveSource(CFRunLoopGetCurrent(),
			IONotificationPortGetRunLoopSource(wt->port), kCFRunLoopDefaultMode);
		IONotificationPortDestroy(wt->port);
	}
	free(wt);
}
*/
import "C"

import (
	"errors"
	"runtime"
	"time"
)

// WatchAttach calls onAttach every time one of the given products appears on
// the USB bus, and blocks until stop is closed.
//
// This is IOKit telling us, not a timer asking. A poll cannot see an unplug and
// replug that completes inside one interval; this does, because the kernel
// posts the arrival either way.
//
// The notification port and the run loop share a thread, so the goroutine is
// pinned for the whole call.
func WatchAttach(vid uint16, pids []uint16, stop <-chan struct{}, onAttach func()) error {
	if len(pids) == 0 {
		return errors.New("hid: no product ids to watch")
	}
	if len(pids) > 2 {
		return errors.New("hid: at most two product ids can be watched")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	box := C.occam_watchbox_new()
	if box == nil {
		return errors.New("hid: could not allocate the watch state")
	}
	defer C.occam_watchbox_free(box)

	wt := C.occam_watcher_new()
	if wt == nil {
		return errors.New("hid: could not allocate the notification port")
	}
	defer C.occam_watcher_free(wt)

	for _, pid := range pids {
		if C.occam_watcher_add(wt, box, C.uint32_t(vid), C.uint32_t(pid)) == 0 {
			return errors.New("hid: IOServiceAddMatchingNotification failed")
		}
	}

	for {
		select {
		case <-stop:
			return nil
		default:
		}

		C.occam_notify_run(C.double(watchSlice.Seconds()))
		if C.occam_watchbox_fired(box) == 0 {
			continue
		}
		C.occam_watchbox_reset(box)

		// The device enumerates before its HID interfaces are matched, so the
		// bus notification arrives slightly ahead of a usable handle.
		time.Sleep(settleDelay)
		onAttach()
	}
}

const (
	// watchSlice is how long each run loop turn blocks. The thread is asleep
	// for all of it; this only bounds how fast a stop is noticed.
	watchSlice = 500 * time.Millisecond

	// settleDelay covers the gap between USB enumeration and IOHIDManager
	// publishing the interfaces.
	settleDelay = 700 * time.Millisecond
)
