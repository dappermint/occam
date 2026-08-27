//go:build darwin

package hid

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/hid/IOHIDManager.h>

static IOHIDManagerRef occam_manager_open(void) {
	IOHIDManagerRef mgr = IOHIDManagerCreate(kCFAllocatorDefault, kIOHIDOptionsTypeNone);
	if (!mgr) return NULL;
	IOHIDManagerSetDeviceMatching(mgr, NULL);
	if (IOHIDManagerOpen(mgr, kIOHIDOptionsTypeNone) != kIOReturnSuccess) {
		CFRelease(mgr);
		return NULL;
	}
	return mgr;
}

static void occam_manager_close(IOHIDManagerRef mgr) {
	if (!mgr) return;
	IOHIDManagerClose(mgr, kIOHIDOptionsTypeNone);
	CFRelease(mgr);
}

// Fills out with retained device refs. Returns the true device count, which
// may exceed max; the caller checks for truncation.
static CFIndex occam_device_list(IOHIDManagerRef mgr, IOHIDDeviceRef *out, CFIndex max) {
	CFSetRef set = IOHIDManagerCopyDevices(mgr);
	if (!set) return 0;
	CFIndex total = CFSetGetCount(set);
	if (total > 0) {
		const void **all = (const void **)calloc((size_t)total, sizeof(void *));
		CFSetGetValues(set, all);
		CFIndex n = total < max ? total : max;
		for (CFIndex i = 0; i < n; i++) {
			out[i] = (IOHIDDeviceRef)all[i];
			CFRetain(out[i]);
		}
		free(all);
	}
	CFRelease(set);
	return total;
}

static int occam_prop_int(IOHIDDeviceRef dev, CFStringRef key, int32_t *out) {
	CFTypeRef v = IOHIDDeviceGetProperty(dev, key);
	if (!v || CFGetTypeID(v) != CFNumberGetTypeID()) return 0;
	return CFNumberGetValue((CFNumberRef)v, kCFNumberSInt32Type, out) ? 1 : 0;
}

static int occam_prop_str(IOHIDDeviceRef dev, CFStringRef key, char *buf, int len) {
	CFTypeRef v = IOHIDDeviceGetProperty(dev, key);
	if (!v || CFGetTypeID(v) != CFStringGetTypeID()) return 0;
	return CFStringGetCString((CFStringRef)v, buf, len, kCFStringEncodingUTF8) ? 1 : 0;
}

static int occam_vendor_id(IOHIDDeviceRef d, int32_t *o)   { return occam_prop_int(d, CFSTR(kIOHIDVendorIDKey), o); }
static int occam_product_id(IOHIDDeviceRef d, int32_t *o)  { return occam_prop_int(d, CFSTR(kIOHIDProductIDKey), o); }
static int occam_version(IOHIDDeviceRef d, int32_t *o)     { return occam_prop_int(d, CFSTR(kIOHIDVersionNumberKey), o); }
static int occam_location(IOHIDDeviceRef d, int32_t *o)    { return occam_prop_int(d, CFSTR(kIOHIDLocationIDKey), o); }
static int occam_primary_page(IOHIDDeviceRef d, int32_t *o){ return occam_prop_int(d, CFSTR(kIOHIDPrimaryUsagePageKey), o); }
static int occam_primary_use(IOHIDDeviceRef d, int32_t *o) { return occam_prop_int(d, CFSTR(kIOHIDPrimaryUsageKey), o); }
static int occam_max_in(IOHIDDeviceRef d, int32_t *o)      { return occam_prop_int(d, CFSTR(kIOHIDMaxInputReportSizeKey), o); }
static int occam_max_out(IOHIDDeviceRef d, int32_t *o)     { return occam_prop_int(d, CFSTR(kIOHIDMaxOutputReportSizeKey), o); }
static int occam_max_feat(IOHIDDeviceRef d, int32_t *o)    { return occam_prop_int(d, CFSTR(kIOHIDMaxFeatureReportSizeKey), o); }

#define OCCAM_STR_PRODUCT  0
#define OCCAM_STR_MANUFACT 1
#define OCCAM_STR_SERIAL   2

static int occam_str(IOHIDDeviceRef d, int kind, char *b, int n) {
	switch (kind) {
	case OCCAM_STR_PRODUCT:  return occam_prop_str(d, CFSTR(kIOHIDProductKey), b, n);
	case OCCAM_STR_MANUFACT: return occam_prop_str(d, CFSTR(kIOHIDManufacturerKey), b, n);
	case OCCAM_STR_SERIAL:   return occam_prop_str(d, CFSTR(kIOHIDSerialNumberKey), b, n);
	}
	return 0;
}

static CFIndex occam_descriptor(IOHIDDeviceRef dev, uint8_t *buf, CFIndex max) {
	CFTypeRef v = IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDReportDescriptorKey));
	if (!v || CFGetTypeID(v) != CFDataGetTypeID()) return 0;
	CFDataRef data = (CFDataRef)v;
	CFIndex n = CFDataGetLength(data);
	if (n > max) n = max;
	CFDataGetBytes(data, CFRangeMake(0, n), buf);
	return n;
}

static CFIndex occam_usage_pairs(IOHIDDeviceRef dev, uint32_t *pages, uint32_t *usages, CFIndex max) {
	CFTypeRef v = IOHIDDeviceGetProperty(dev, CFSTR(kIOHIDDeviceUsagePairsKey));
	if (!v || CFGetTypeID(v) != CFArrayGetTypeID()) return 0;
	CFArrayRef arr = (CFArrayRef)v;
	CFIndex n = CFArrayGetCount(arr);
	if (n > max) n = max;
	CFIndex k = 0;
	for (CFIndex i = 0; i < n; i++) {
		CFDictionaryRef d = (CFDictionaryRef)CFArrayGetValueAtIndex(arr, i);
		if (!d || CFGetTypeID(d) != CFDictionaryGetTypeID()) continue;
		int32_t pv = 0, uv = 0;
		CFNumberRef p = (CFNumberRef)CFDictionaryGetValue(d, CFSTR(kIOHIDDeviceUsagePageKey));
		CFNumberRef u = (CFNumberRef)CFDictionaryGetValue(d, CFSTR(kIOHIDDeviceUsageKey));
		if (p) CFNumberGetValue(p, kCFNumberSInt32Type, &pv);
		if (u) CFNumberGetValue(u, kCFNumberSInt32Type, &uv);
		pages[k] = (uint32_t)pv;
		usages[k] = (uint32_t)uv;
		k++;
	}
	return k;
}

static int occam_open(IOHIDDeviceRef d)  { return (int)IOHIDDeviceOpen(d, kIOHIDOptionsTypeNone); }
static int occam_close(IOHIDDeviceRef d) { return (int)IOHIDDeviceClose(d, kIOHIDOptionsTypeNone); }

static int occam_set_report(IOHIDDeviceRef d, int32_t id, uint8_t *data, CFIndex len) {
	return (int)IOHIDDeviceSetReport(d, kIOHIDReportTypeOutput, (CFIndex)id, data, len);
}

static int occam_get_report(IOHIDDeviceRef d, int32_t id, uint8_t *data, CFIndex *len) {
	return (int)IOHIDDeviceGetReport(d, kIOHIDReportTypeInput, (CFIndex)id, data, len);
}

static void occam_release(IOHIDDeviceRef d) { if (d) CFRelease(d); }

// The device answers on the interrupt IN endpoint, not through GET_REPORT, so
// replies arrive through an input-report callback on a CFRunLoop. The inbox
// lives in C memory: cgo may not hand a Go pointer to a callback context, and
// the scratch buffer has to stay valid for as long as the callback is armed.
typedef struct {
	uint8_t  buf[64];
	uint8_t  scratch[64];
	CFIndex  len;
	uint32_t id;
	int      got;
} occam_inbox;

static occam_inbox *occam_inbox_new(void)  { return (occam_inbox *)calloc(1, sizeof(occam_inbox)); }
static void occam_inbox_free(occam_inbox *b) { free(b); }
static int occam_inbox_got(occam_inbox *b)   { return b->got; }
static CFIndex occam_inbox_len(occam_inbox *b) { return b->len; }
static uint32_t occam_inbox_id(occam_inbox *b) { return b->id; }
static void occam_inbox_reset(occam_inbox *b) { b->got = 0; b->len = 0; b->id = 0; }
static void occam_inbox_copy(occam_inbox *b, uint8_t *out) { memcpy(out, b->buf, (size_t)b->len); }

static void occam_report_cb(void *context, IOReturn result, void *sender,
                            IOHIDReportType type, uint32_t reportID,
                            uint8_t *report, CFIndex reportLength) {
	occam_inbox *b = (occam_inbox *)context;
	if (!b || b->got || result != kIOReturnSuccess) return;
	CFIndex n = reportLength > (CFIndex)sizeof(b->buf) ? (CFIndex)sizeof(b->buf) : reportLength;
	memcpy(b->buf, report, (size_t)n);
	b->len = n;
	b->id = reportID;
	b->got = 1;
}

// The registration must outlive every wait: it hands the device a pointer to
// b->scratch, and the device keeps writing there. Registering per call and
// freeing the inbox afterwards leaves the device writing into freed memory,
// which crashes inside CoreFoundation a few calls later.
static void occam_listen_register(IOHIDDeviceRef d, occam_inbox *b) {
	IOHIDDeviceRegisterInputReportCallback(d, b->scratch, (CFIndex)sizeof(b->scratch), occam_report_cb, b);
}

static void occam_listen_start(IOHIDDeviceRef d) {
	IOHIDDeviceScheduleWithRunLoop(d, CFRunLoopGetCurrent(), kCFRunLoopDefaultMode);
}

static void occam_listen_stop(IOHIDDeviceRef d) {
	IOHIDDeviceUnscheduleFromRunLoop(d, CFRunLoopGetCurrent(), kCFRunLoopDefaultMode);
}

// Returns the CFRunLoopRunInMode result so callers can tell a genuine wait
// (timed out, 3) from a run loop with no sources installed (finished, 1).
static int occam_run(double seconds) {
	return (int)CFRunLoopRunInMode(kCFRunLoopDefaultMode, seconds, true);
}

// CF_BRIDGED_TYPE hides the pointer-ness of these typedefs from cgo, so null
// handling stays on the C side.
static int occam_mgr_null(IOHIDManagerRef m) { return m == NULL; }
static int occam_ref_null(IOHIDDeviceRef d) { return d == NULL; }
static IOHIDDeviceRef occam_ref_nil(void)    { return NULL; }
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

const (
	maxDevices    = 256
	maxUsagePairs = 32
	maxDescriptor = 4096
	strBuf        = 256
)

// ErrTruncated means more HID interfaces exist than the enumeration buffer holds.
var ErrTruncated = errors.New("hid: device list truncated")

// Device is an open handle to one HID interface.
type Device struct {
	Info Info

	// LastRunResult records what the CFRunLoop did on the last turn, so a
	// silent device can be told apart from a callback that was never armed.
	LastRunResult RunResult

	ref  C.IOHIDDeviceRef
	box  *C.occam_inbox
	open bool
}

type entry struct {
	info Info
	ref  C.IOHIDDeviceRef
}

func enumerate() ([]entry, error) {
	mgr := C.occam_manager_open()
	if C.occam_mgr_null(mgr) != 0 {
		return nil, errors.New("hid: IOHIDManagerOpen failed")
	}
	defer C.occam_manager_close(mgr)

	refs := make([]C.IOHIDDeviceRef, maxDevices)
	total := C.occam_device_list(mgr, &refs[0], C.CFIndex(maxDevices))
	n := min(int(total), maxDevices)

	out := make([]entry, 0, n)
	for i := range n {
		out = append(out, entry{info: readInfo(refs[i]), ref: refs[i]})
	}
	if int(total) > maxDevices {
		return out, ErrTruncated
	}
	return out, nil
}

func readInfo(ref C.IOHIDDeviceRef) Info {
	var v C.int32_t
	var info Info

	if C.occam_vendor_id(ref, &v) != 0 {
		info.VendorID = uint16(v)
	}
	if C.occam_product_id(ref, &v) != 0 {
		info.ProductID = uint16(v)
	}
	if C.occam_version(ref, &v) != 0 {
		info.Version = uint16(v)
	}
	if C.occam_location(ref, &v) != 0 {
		info.LocationID = uint32(v)
	}
	if C.occam_primary_page(ref, &v) != 0 {
		info.Primary.Page = uint32(v)
	}
	if C.occam_primary_use(ref, &v) != 0 {
		info.Primary.Usage = uint32(v)
	}
	if C.occam_max_in(ref, &v) != 0 {
		info.MaxIn = int(v)
	}
	if C.occam_max_out(ref, &v) != 0 {
		info.MaxOut = int(v)
	}
	if C.occam_max_feat(ref, &v) != 0 {
		info.MaxFeature = int(v)
	}

	info.Product = readStr(ref, C.OCCAM_STR_PRODUCT)
	info.Manufact = readStr(ref, C.OCCAM_STR_MANUFACT)
	info.Serial = readStr(ref, C.OCCAM_STR_SERIAL)

	desc := make([]byte, maxDescriptor)
	if n := int(C.occam_descriptor(ref, (*C.uint8_t)(unsafe.Pointer(&desc[0])), C.CFIndex(maxDescriptor))); n > 0 {
		info.Descriptor = desc[:n:n]
	}

	pages := make([]C.uint32_t, maxUsagePairs)
	usages := make([]C.uint32_t, maxUsagePairs)
	k := int(C.occam_usage_pairs(ref, &pages[0], &usages[0], C.CFIndex(maxUsagePairs)))
	for i := range k {
		info.Usages = append(info.Usages, UsagePair{Page: uint32(pages[i]), Usage: uint32(usages[i])})
	}
	return info
}

func readStr(ref C.IOHIDDeviceRef, kind C.int) string {
	buf := make([]byte, strBuf)
	if C.occam_str(ref, kind, (*C.char)(unsafe.Pointer(&buf[0])), C.int(strBuf)) == 0 {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// List returns every HID interface IOHIDManager can see.
func List() ([]Info, error) {
	entries, err := enumerate()
	out := make([]Info, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.info)
		C.occam_release(e.ref)
	}
	return out, err
}

// Open finds an interface matching vid and one of pids, taking the first pid
// that is present rather than the first device enumerated. The interface is
// opened shared, not seized, so the system HID driver keeps working.
func Open(vid uint16, pids ...uint16) (*Device, error) {
	entries, err := enumerate()
	if err != nil && len(entries) == 0 {
		return nil, err
	}

	pick := -1
	for _, pid := range pids {
		for i, e := range entries {
			if e.info.VendorID == vid && e.info.ProductID == pid {
				pick = i
				break
			}
		}
		if pick >= 0 {
			break
		}
	}

	var found *Device
	for i, e := range entries {
		if i == pick {
			found = &Device{Info: e.info, ref: e.ref}
			continue
		}
		C.occam_release(e.ref)
	}
	if found == nil {
		return nil, fmt.Errorf("hid: no device %04x:%s on the bus", vid, hexList(pids))
	}

	if rc := C.occam_open(found.ref); rc != 0 {
		C.occam_release(found.ref)
		return nil, fmt.Errorf("hid: IOHIDDeviceOpen: %s", ioReturn(int(rc)))
	}
	found.open = true

	found.box = C.occam_inbox_new()
	if found.box == nil {
		found.Close()
		return nil, errors.New("hid: could not allocate the input buffer")
	}
	C.occam_listen_register(found.ref, found.box)
	return found, nil
}

// Close releases the handle. Safe to call twice.
func (d *Device) Close() error {
	if d == nil || C.occam_ref_null(d.ref) != 0 {
		return nil
	}
	if d.open {
		C.occam_close(d.ref)
		d.open = false
	}
	C.occam_release(d.ref)
	d.ref = C.occam_ref_nil()

	// Only safe once the handle is closed: until then the device may still be
	// writing input reports into the scratch buffer.
	if d.box != nil {
		C.occam_inbox_free(d.box)
		d.box = nil
	}
	return nil
}

// SetReport sends an output report. buf is the payload without the report ID.
//
// The ID is prefixed to the bytes on the wire as well as passed separately.
// This looks redundant and is not: for a numbered report the device expects it
// in the data, which is how input reports arrive too. Sending the payload
// alone completes the USB transfer and is then silently discarded, which is a
// miserable failure to debug.
func (d *Device) SetReport(reportID byte, buf []byte) error {
	if d == nil || C.occam_ref_null(d.ref) != 0 {
		return errors.New("hid: device not open")
	}
	if len(buf) == 0 {
		return errors.New("hid: empty report")
	}

	framed := make([]byte, 0, len(buf)+1)
	framed = append(framed, reportID)
	framed = append(framed, buf...)

	rc := C.occam_set_report(d.ref, C.int32_t(reportID), (*C.uint8_t)(unsafe.Pointer(&framed[0])), C.CFIndex(len(framed)))
	if rc != 0 {
		return fmt.Errorf("hid: IOHIDDeviceSetReport: %s", ioReturn(int(rc)))
	}
	return nil
}

// GetReport pulls an input report synchronously. Interrupt-IN traffic that the
// device pushes on its own is not visible here; that needs an input callback,
// which lands with the phase 3 command decoding.
func (d *Device) GetReport(reportID byte, size int) ([]byte, error) {
	if d == nil || C.occam_ref_null(d.ref) != 0 {
		return nil, errors.New("hid: device not open")
	}
	buf := make([]byte, size)
	length := C.CFIndex(size)
	rc := C.occam_get_report(d.ref, C.int32_t(reportID), (*C.uint8_t)(unsafe.Pointer(&buf[0])), &length)
	if rc != 0 {
		return nil, fmt.Errorf("hid: IOHIDDeviceGetReport: %s", ioReturn(int(rc)))
	}
	return buf[:int(length)], nil
}

// ErrTimeout means no input report arrived before the deadline.
var ErrTimeout = errors.New("hid: no reply before the timeout")

// Request writes a report and waits for the reply the device pushes on the
// interrupt IN endpoint. GetReport does not work for this protocol: the device
// answers asynchronously, and a synchronous GET_REPORT returns stale element
// data that fails its own checksum.
//
// Registration and the run loop have to share a thread, so the goroutine is
// pinned for the duration.
func (d *Device) Request(reportID byte, out []byte, timeout time.Duration) ([]byte, error) {
	if d == nil || C.occam_ref_null(d.ref) != 0 || d.box == nil {
		return nil, errors.New("hid: device not open")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	box := d.box
	C.occam_inbox_reset(box)

	C.occam_listen_start(d.ref)
	defer C.occam_listen_stop(d.ref)

	if err := d.SetReport(reportID, out); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		d.LastRunResult = RunResult(C.occam_run(C.double(pollSlice.Seconds())))
		if C.occam_inbox_got(box) == 0 {
			continue
		}
		n := int(C.occam_inbox_len(box))
		if n == 0 {
			return nil, errors.New("hid: empty reply")
		}
		buf := make([]byte, n)
		C.occam_inbox_copy(box, (*C.uint8_t)(unsafe.Pointer(&buf[0])))
		return buf, nil
	}
	return nil, ErrTimeout
}

// pollSlice is how long each CFRunLoop turn blocks for. Short enough that the
// deadline stays honest, long enough not to spin.
const pollSlice = 50 * time.Millisecond

// Listen waits for input reports without sending anything, for working out
// whether the device talks unprompted and whether the run loop is wired up at
// all. Returns every distinct report seen before the timeout.
func (d *Device) Listen(timeout time.Duration, onReport func(reportID byte, data []byte)) (int, error) {
	if d == nil || C.occam_ref_null(d.ref) != 0 || d.box == nil {
		return 0, errors.New("hid: device not open")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	box := d.box
	C.occam_inbox_reset(box)

	C.occam_listen_start(d.ref)
	defer C.occam_listen_stop(d.ref)

	seen := 0
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		d.LastRunResult = RunResult(C.occam_run(C.double(pollSlice.Seconds())))
		if C.occam_inbox_got(box) == 0 {
			continue
		}
		n := int(C.occam_inbox_len(box))
		buf := make([]byte, n)
		if n > 0 {
			C.occam_inbox_copy(box, (*C.uint8_t)(unsafe.Pointer(&buf[0])))
		}
		seen++
		if onReport != nil {
			onReport(byte(C.occam_inbox_id(box)), buf)
		}
		C.occam_inbox_reset(box)
	}
	return seen, nil
}

// ioReturn names the IOKit error codes this package actually provokes.
func ioReturn(rc int) string {
	switch uint32(rc) {
	case 0xE00002C2:
		return "kIOReturnExclusiveAccess (another process seized the interface)"
	case 0xE00002C1:
		return "kIOReturnNotPrivileged (permission denied)"
	case 0xE00002BC:
		return "kIOReturnError"
	case 0xE00002C7:
		return "kIOReturnUnsupported"
	case 0xE00002D5:
		return "kIOReturnNotOpen"
	case 0xE00002EB:
		return "kIOReturnTimeout"
	case 0xE00002F0:
		return "kIOReturnNoDevice"
	default:
		return fmt.Sprintf("IOReturn 0x%08X", uint32(rc))
	}
}
