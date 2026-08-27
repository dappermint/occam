//go:build darwin

#import <Cocoa/Cocoa.h>

// Implemented in Go, see window_darwin.go.
extern void occamBandChanged(int band, int value);
extern void occamSlotChanged(int slot);
extern void occamSidetoneChanged(int value);
extern void occamAction(int tag);
extern void occamMainCallback(void);
extern void occamANCChanged(int on, int level);
extern void occamMicChanged(int muted);
extern void occamBalanceChanged(int value);
extern void occamLEDChanged(int on);
extern void occamPowerOffChanged(int index);
extern void occamLowLatencyChanged(int on);

// AppKit calls that must happen on the main thread, invoked from goroutines.
void occam_main_async(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ occamMainCallback(); });
}

#define OCCAM_BANDS 10

@interface OccamWindowTarget : NSObject <NSWindowDelegate>
@end

static NSWindow          *gWindow = nil;
static OccamWindowTarget *gWinTarget = nil;
static NSPopUpButton     *gSlotPicker = nil;
static NSSlider          *gBand[OCCAM_BANDS];
static NSTextField       *gBandValue[OCCAM_BANDS];
static NSSlider          *gSidetone = nil;
static NSTextField       *gSidetoneValue = nil;
static NSSlider          *gANCLevel = nil;
static NSTextField       *gANCValue = nil;
static NSButton          *gMicMute = nil;
static NSSlider          *gBalance = nil;
static NSTextField       *gBalanceValue = nil;
static NSPopUpButton     *gDongleLED = nil;
static NSPopUpButton     *gPowerOff = nil;
static NSButton          *gLowLatency = nil;
static NSPopUpButton     *gANCMode = nil;
static NSTextField       *gStatus = nil;

// Set while Go is pushing values in, so programmatic changes do not echo back
// out as if the user had dragged something.
static BOOL gQuiet = NO;

static NSTextField *occam_label(NSString *text, CGFloat width, NSTextAlignment align) {
	NSTextField *f = [NSTextField labelWithString:text];
	f.alignment = align;
	f.font = [NSFont monospacedDigitSystemFontOfSize:11 weight:NSFontWeightRegular];
	f.textColor = [NSColor secondaryLabelColor];
	[f.widthAnchor constraintEqualToConstant:width].active = YES;
	return f;
}

@implementation OccamWindowTarget

- (void)bandMoved:(id)sender {
	if (gQuiet) return;
	NSSlider *s = (NSSlider *)sender;
	int band = (int)s.tag;
	int value = (int)lround(s.doubleValue);
	gBandValue[band].stringValue = [NSString stringWithFormat:@"%+d", value];
	occamBandChanged(band, value);
}

- (void)slotPicked:(id)sender {
	if (gQuiet) return;
	occamSlotChanged((int)[(NSPopUpButton *)sender indexOfSelectedItem]);
}

- (void)sidetoneMoved:(id)sender {
	if (gQuiet) return;
	int value = (int)lround([(NSSlider *)sender doubleValue]);
	gSidetoneValue.stringValue = [NSString stringWithFormat:@"%d", value];
	occamSidetoneChanged(value);
}

- (void)ancPicked:(id)sender {
	if (gQuiet) return;
	occamANCChanged((int)[gANCMode indexOfSelectedItem], (int)lround(gANCLevel.doubleValue));
}

- (void)ancLevelMoved:(id)sender {
	if (gQuiet) return;
	int level = (int)lround([(NSSlider *)sender doubleValue]);
	gANCValue.stringValue = [NSString stringWithFormat:@"%d", level];
	occamANCChanged((int)[gANCMode indexOfSelectedItem], level);
}

- (void)micToggled:(id)sender {
	if (gQuiet) return;
	occamMicChanged([(NSButton *)sender state] == NSControlStateValueOn ? 1 : 0);
}

- (void)balanceMoved:(id)sender {
	if (gQuiet) return;
	int v = (int)lround([(NSSlider *)sender doubleValue]);
	gBalanceValue.stringValue = [NSString stringWithFormat:@"%d", v];
	occamBalanceChanged(v);
}

- (void)ledPicked:(id)sender {
	if (gQuiet) return;
	occamLEDChanged((int)[(NSPopUpButton *)sender indexOfSelectedItem]);
}

- (void)powerOffPicked:(id)sender {
	if (gQuiet) return;
	occamPowerOffChanged((int)[(NSPopUpButton *)sender indexOfSelectedItem]);
}

- (void)lowLatencyToggled:(id)sender {
	if (gQuiet) return;
	occamLowLatencyChanged([(NSButton *)sender state] == NSControlStateValueOn ? 1 : 0);
}

- (void)actionClicked:(id)sender {
	occamAction((int)[(NSButton *)sender tag]);
}

// Accessory apps have no Dock icon to reopen from, so closing must not destroy
// the window; it is hidden and reused.
- (BOOL)windowShouldClose:(NSWindow *)sender {
	[sender orderOut:nil];
	return NO;
}

@end

static NSView *occam_row(NSString *label, NSView *middle, NSTextField *value) {
	NSStackView *row = [NSStackView stackViewWithViews:@[
		occam_label(label, 46, NSTextAlignmentRight), middle, value
	]];
	row.orientation = NSUserInterfaceLayoutOrientationHorizontal;
	row.spacing = 10;
	row.alignment = NSLayoutAttributeCenterY;
	return row;
}

void occam_window_build(const char **bandLabels, int minDB, int maxDB) {
	@autoreleasepool {
		if (gWindow) return;
		gWinTarget = [[OccamWindowTarget alloc] init];

		NSMutableArray<NSView *> *rows = [NSMutableArray array];

		gSlotPicker = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
		gSlotPicker.target = gWinTarget;
		gSlotPicker.action = @selector(slotPicked:);
		[rows addObject:occam_row(@"Preset", gSlotPicker, occam_label(@"", 40, NSTextAlignmentLeft))];

		NSBox *sep = [[NSBox alloc] initWithFrame:NSZeroRect];
		sep.boxType = NSBoxSeparator;
		[rows addObject:sep];

		for (int i = 0; i < OCCAM_BANDS; i++) {
			gBand[i] = [NSSlider sliderWithValue:0 minValue:minDB maxValue:maxDB
			                              target:gWinTarget action:@selector(bandMoved:)];
			gBand[i].tag = i;
			gBand[i].numberOfTickMarks = (maxDB - minDB) + 1;
			gBand[i].allowsTickMarkValuesOnly = YES;
			[gBand[i].widthAnchor constraintGreaterThanOrEqualToConstant:220].active = YES;

			gBandValue[i] = occam_label(@"+0", 40, NSTextAlignmentLeft);
			[rows addObject:occam_row([NSString stringWithUTF8String:bandLabels[i]],
			                          gBand[i], gBandValue[i])];
		}

		NSBox *sep2 = [[NSBox alloc] initWithFrame:NSZeroRect];
		sep2.boxType = NSBoxSeparator;
		[rows addObject:sep2];

		// Synapse's mic monitoring slider runs 0 to 15, not 0 to 255.
		gSidetone = [NSSlider sliderWithValue:0 minValue:0 maxValue:15
		                               target:gWinTarget action:@selector(sidetoneMoved:)];
		gSidetone.numberOfTickMarks = 16;
		gSidetone.allowsTickMarkValuesOnly = YES;
		gSidetoneValue = occam_label(@"0", 40, NSTextAlignmentLeft);
		[rows addObject:occam_row(@"Sidetone", gSidetone, gSidetoneValue)];

		NSBox *sep3 = [[NSBox alloc] initWithFrame:NSZeroRect];
		sep3.boxType = NSBoxSeparator;
		[rows addObject:sep3];

		gANCMode = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
		gANCMode.target = gWinTarget;
		gANCMode.action = @selector(ancPicked:);
		[rows addObject:occam_row(@"Noise", gANCMode, occam_label(@"", 40, NSTextAlignmentLeft))];

		// Synapse offers levels 1 through 4, with no zero.
		gANCLevel = [NSSlider sliderWithValue:1 minValue:1 maxValue:4
		                               target:gWinTarget action:@selector(ancLevelMoved:)];
		gANCLevel.numberOfTickMarks = 4;
		gANCLevel.allowsTickMarkValuesOnly = YES;
		gANCValue = occam_label(@"1", 40, NSTextAlignmentLeft);
		[rows addObject:occam_row(@"Level", gANCLevel, gANCValue)];

		gMicMute = [NSButton checkboxWithTitle:@"Mute microphone"
		                                target:gWinTarget action:@selector(micToggled:)];
		[rows addObject:gMicMute];

		gBalance = [NSSlider sliderWithValue:10 minValue:0 maxValue:20
		                              target:gWinTarget action:@selector(balanceMoved:)];
		gBalanceValue = occam_label(@"10", 40, NSTextAlignmentLeft);
		[rows addObject:occam_row(@"Game/Chat", gBalance, gBalanceValue)];

		gDongleLED = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
		gDongleLED.target = gWinTarget;
		gDongleLED.action = @selector(ledPicked:);
		[rows addObject:occam_row(@"Light", gDongleLED, occam_label(@"", 40, NSTextAlignmentLeft))];

		gPowerOff = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
		gPowerOff.target = gWinTarget;
		gPowerOff.action = @selector(powerOffPicked:);
		[rows addObject:occam_row(@"Sleep", gPowerOff, occam_label(@"", 40, NSTextAlignmentLeft))];

		gLowLatency = [NSButton checkboxWithTitle:@"Ultra-low latency"
		                                   target:gWinTarget action:@selector(lowLatencyToggled:)];
		[rows addObject:gLowLatency];

		NSButton *save = [NSButton buttonWithTitle:@"Save to Profile"
		                                    target:gWinTarget action:@selector(actionClicked:)];
		save.tag = 1;
		save.keyEquivalent = @"\r";
		NSButton *reload = [NSButton buttonWithTitle:@"Reload from Headset"
		                                      target:gWinTarget action:@selector(actionClicked:)];
		reload.tag = 2;

		gStatus = occam_label(@"", 200, NSTextAlignmentLeft);
		NSStackView *buttons = [NSStackView stackViewWithViews:@[gStatus, reload, save]];
		buttons.orientation = NSUserInterfaceLayoutOrientationHorizontal;
		buttons.spacing = 8;
		[rows addObject:buttons];

		NSStackView *stack = [NSStackView stackViewWithViews:rows];
		stack.orientation = NSUserInterfaceLayoutOrientationVertical;
		stack.alignment = NSLayoutAttributeLeading;
		stack.spacing = 8;
		stack.edgeInsets = NSEdgeInsetsMake(18, 18, 18, 18);

		gWindow = [[NSWindow alloc]
			initWithContentRect:NSMakeRect(0, 0, 460, 520)
			          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
			                    NSWindowStyleMaskMiniaturizable
			            backing:NSBackingStoreBuffered
			              defer:NO];
		gWindow.title = @"occam";
		gWindow.delegate = gWinTarget;
		gWindow.releasedWhenClosed = NO;
		gWindow.contentView = stack;
		[gWindow center];
	}
}

void occam_window_show(void) {
	@autoreleasepool {
		if (!gWindow) return;
		// Accessory apps are not active by default, so the window would open
		// behind whatever has focus without this.
		[NSApp activateIgnoringOtherApps:YES];
		[gWindow makeKeyAndOrderFront:nil];
	}
}

void occam_window_set_slots(const char **names, int count, int selected) {
	@autoreleasepool {
		if (!gSlotPicker) return;
		gQuiet = YES;
		[gSlotPicker removeAllItems];
		for (int i = 0; i < count; i++) {
			[gSlotPicker addItemWithTitle:[NSString stringWithUTF8String:names[i]]];
		}
		if (selected >= 0 && selected < count) [gSlotPicker selectItemAtIndex:selected];
		gQuiet = NO;
	}
}

void occam_window_set_bands(const int *values, int count) {
	@autoreleasepool {
		gQuiet = YES;
		for (int i = 0; i < count && i < OCCAM_BANDS; i++) {
			gBand[i].doubleValue = values[i];
			gBandValue[i].stringValue = [NSString stringWithFormat:@"%+d", values[i]];
		}
		gQuiet = NO;
	}
}

void occam_window_set_sidetone(int value) {
	@autoreleasepool {
		gQuiet = YES;
		gSidetone.doubleValue = value;
		gSidetoneValue.stringValue = [NSString stringWithFormat:@"%d", value];
		gQuiet = NO;
	}
}

static void occam_fill(NSPopUpButton *b, const char **names, int count) {
	if (!b) return;
	gQuiet = YES;
	[b removeAllItems];
	for (int i = 0; i < count; i++) {
		[b addItemWithTitle:[NSString stringWithUTF8String:names[i]]];
	}
	gQuiet = NO;
}

void occam_window_set_led_modes(const char **names, int count) {
	@autoreleasepool { occam_fill(gDongleLED, names, count); }
}

void occam_window_set_anc_modes(const char **names, int count) {
	@autoreleasepool { occam_fill(gANCMode, names, count); }
}

void occam_window_set_sleep_options(const char **names, int count) {
	@autoreleasepool { occam_fill(gPowerOff, names, count); }
}

void occam_window_set_extras(int ancMode, int ancLevel, int micMuted,
                             int balance, int ledMode, int sleepIndex,
                             int lowLatency) {
	@autoreleasepool {
		gQuiet = YES;
		if (ancMode >= 0 && ancMode < gANCMode.numberOfItems) {
			[gANCMode selectItemAtIndex:ancMode];
		}
		gANCLevel.doubleValue = ancLevel;
		gANCValue.stringValue = [NSString stringWithFormat:@"%d", ancLevel];
		gLowLatency.state = lowLatency ? NSControlStateValueOn : NSControlStateValueOff;
		gMicMute.state = micMuted ? NSControlStateValueOn : NSControlStateValueOff;
		gBalance.doubleValue = balance;
		gBalanceValue.stringValue = [NSString stringWithFormat:@"%d", balance];
		if (ledMode >= 0 && ledMode < gDongleLED.numberOfItems) {
			[gDongleLED selectItemAtIndex:ledMode];
		}
		if (sleepIndex >= 0 && sleepIndex < gPowerOff.numberOfItems) {
			[gPowerOff selectItemAtIndex:sleepIndex];
		}
		gQuiet = NO;
	}
}

void occam_window_set_status(const char *text) {
	@autoreleasepool {
		if (gStatus) gStatus.stringValue = [NSString stringWithUTF8String:text];
	}
}
