//go:build darwin

#import <Cocoa/Cocoa.h>

// Implemented in Go, see window_darwin.go.
extern void occamBandChanged(int band, int value);
extern void occamMicBandChanged(int band, int value);
extern void occamSlotChanged(int slot);
extern void occamMicPresetChanged(int index);
extern void occamMixEnabled(int on);
extern void occamMixLayout(int index);
extern void occamSidetoneChanged(int value);
extern void occamANCChanged(int mode, int level);
extern void occamMicChanged(int muted);
extern void occamBalanceChanged(int value);
extern void occamLEDChanged(int mode);
extern void occamPowerOffChanged(int index);
extern void occamLowLatencyChanged(int on);
extern void occamTHXChanged(int on);
extern void occamAction(int tag);
extern void occamMainCallback(void);

void occam_main_async(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ occamMainCallback(); });
}

#define OCCAM_BANDS 10

// A grid keeps label/control/value aligned whatever a row holds. Nested stacks
// did not: checkboxes sat outside the column and long labels clipped.
#define OCCAM_LABEL_WIDTH  80.0
#define OCCAM_VALUE_WIDTH  34.0
#define OCCAM_SLIDER_WIDTH 260.0

@interface OccamWindowTarget : NSObject <NSWindowDelegate, NSTabViewDelegate>
@end

static void occam_window_fit(NSTabViewItem *item);

// NSScrollView puts a document view shorter than the clip view at the bottom,
// which left the pages hanging off the wrong edge.
@interface OccamFlippedView : NSView
@end

@implementation OccamFlippedView
- (BOOL)isFlipped { return YES; }
@end

static NSWindow          *gWindow = nil;
static NSTabView         *gTabs = nil;
static OccamWindowTarget *gWinTarget = nil;

static NSPopUpButton *gSlotPicker = nil;
static NSSlider      *gBand[OCCAM_BANDS];
static NSTextField   *gBandValue[OCCAM_BANDS];
static NSSegmentedControl *gANCMode = nil;
static NSSlider      *gANCLevel = nil;
static NSGridRow     *gANCLevelRow = nil;
static NSTextField   *gANCValue = nil;
static NSSlider      *gBalance = nil;
static NSTextField   *gBalanceValue = nil;
static NSPopUpButton *gDongleLED = nil;
static NSPopUpButton *gPowerOff = nil;
static NSButton      *gLowLatency = nil;

static NSButton      *gMicMute = nil;
static NSSlider      *gSidetone = nil;
static NSTextField   *gSidetoneValue = nil;
static NSPopUpButton *gMicPreset = nil;
static NSButton      *gMixOn = nil;
static NSButton      *gTHX = nil;
static NSPopUpButton *gMixLayout = nil;
static NSTextField   *gMixStatus = nil;
static NSSlider      *gMicBand[OCCAM_BANDS];
static NSTextField   *gMicBandValue[OCCAM_BANDS];

static NSTextField   *gStatus = nil;

// Set while Go pushes values in, so programmatic changes do not echo back out
// as if the user had dragged something.
static BOOL gQuiet = NO;

static NSTextField *occam_label(NSString *text, CGFloat width, NSTextAlignment align) {
	NSTextField *f = [NSTextField labelWithString:text];
	f.alignment = align;
	f.font = [NSFont monospacedDigitSystemFontOfSize:11 weight:NSFontWeightRegular];
	f.textColor = [NSColor secondaryLabelColor];
	f.lineBreakMode = NSLineBreakByClipping;
	[f.widthAnchor constraintEqualToConstant:width].active = YES;
	return f;
}

static NSTextField *occam_row_label(NSString *text) {
	return occam_label(text, OCCAM_LABEL_WIDTH, NSTextAlignmentRight);
}

static NSTextField *occam_value(NSString *text) {
	return occam_label(text, OCCAM_VALUE_WIDTH, NSTextAlignmentLeft);
}

// Returns nil for a name this system does not ship, which NSTabViewItem takes
// as no image rather than a blank one.
static NSImage *occam_symbol(NSString *name) {
	return [NSImage imageWithSystemSymbolName:name accessibilityDescription:nil];
}

// The device ignores a level write outside noise cancelling, so the row goes
// away entirely rather than sitting there claiming to do something. Hiding a
// grid row collapses it, so nothing is left behind.
static void occam_anc_level_active(int active) {
	gANCLevelRow.hidden = active ? NO : YES;
}

static NSView *occam_spacer(void) {
	return [NSTextField labelWithString:@""];
}

@implementation OccamWindowTarget

- (void)bandMoved:(id)sender {
	if (gQuiet) return;
	NSSlider *s = (NSSlider *)sender;
	int band = (int)s.tag, value = (int)lround(s.doubleValue);
	gBandValue[band].stringValue = [NSString stringWithFormat:@"%+d", value];
	occamBandChanged(band, value);
}

- (void)micBandMoved:(id)sender {
	if (gQuiet) return;
	NSSlider *s = (NSSlider *)sender;
	int band = (int)s.tag, value = (int)lround(s.doubleValue);
	gMicBandValue[band].stringValue = [NSString stringWithFormat:@"%+d", value];
	occamMicBandChanged(band, value);
}

- (void)slotPicked:(id)sender {
	if (gQuiet) return;
	occamSlotChanged((int)[(NSPopUpButton *)sender indexOfSelectedItem]);
}

- (void)micPresetPicked:(id)sender {
	if (gQuiet) return;
	occamMicPresetChanged((int)[(NSPopUpButton *)sender indexOfSelectedItem]);
}

- (void)sidetoneMoved:(id)sender {
	if (gQuiet) return;
	int value = (int)lround([(NSSlider *)sender doubleValue]);
	gSidetoneValue.stringValue = [NSString stringWithFormat:@"%d", value];
	occamSidetoneChanged(value);
}

- (void)ancPicked:(id)sender {
	if (gQuiet) return;
	occamANCChanged((int)gANCMode.selectedSegment, (int)lround(gANCLevel.doubleValue));
}

- (void)ancLevelMoved:(id)sender {
	if (gQuiet) return;
	int level = (int)lround([(NSSlider *)sender doubleValue]);
	gANCValue.stringValue = [NSString stringWithFormat:@"%d", level];
	occamANCChanged((int)gANCMode.selectedSegment, level);
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

- (void)mixToggled:(id)sender {
	if (gQuiet) return;
	occamMixEnabled([(NSButton *)sender state] == NSControlStateValueOn ? 1 : 0);
}

- (void)thxToggled:(id)sender {
	if (gQuiet) return;
	occamTHXChanged([(NSButton *)sender state] == NSControlStateValueOn ? 1 : 0);
}

- (void)mixLayoutPicked:(id)sender {
	if (gQuiet) return;
	occamMixLayout((int)[(NSPopUpButton *)sender indexOfSelectedItem]);
}

// Each pane gets the window sized to it, the way a preferences window does.
// One shared height would leave Spatial's single card floating in a frame
// built for ten equaliser bands.
- (void)tabView:(NSTabView *)tabView didSelectTabViewItem:(NSTabViewItem *)item {
	occam_window_fit(item);
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

static NSPopUpButton *occam_popup(SEL action) {
	NSPopUpButton *b = [[NSPopUpButton alloc] initWithFrame:NSZeroRect pullsDown:NO];
	b.target = gWinTarget;
	b.action = action;
	return b;
}

static NSSlider *occam_slider(double lo, double hi, int ticks, SEL action) {
	NSSlider *s = [NSSlider sliderWithValue:lo minValue:lo maxValue:hi
	                                 target:gWinTarget action:action];
	s.numberOfTickMarks = ticks;
	s.allowsTickMarkValuesOnly = YES;
	[s.widthAnchor constraintEqualToConstant:OCCAM_SLIDER_WIDTH].active = YES;
	return s;
}

static NSGridView *occam_grid(NSArray<NSArray<NSView *> *> *rows) {
	NSGridView *grid = [NSGridView gridViewWithViews:rows];
	grid.rowSpacing = 10;
	grid.columnSpacing = 12;
	grid.rowAlignment = NSGridRowAlignmentFirstBaseline;
	[grid columnAtIndex:0].xPlacement = NSGridCellPlacementTrailing;
	[grid columnAtIndex:1].xPlacement = NSGridCellPlacementLeading;
	[grid columnAtIndex:2].xPlacement = NSGridCellPlacementLeading;
	grid.translatesAutoresizingMaskIntoConstraints = NO;
	return grid;
}

// One System Settings card: a rounded filled box with an optional header in
// small caps above it.
static NSView *occam_group_grid(NSString *title, NSArray<NSArray<NSView *> *> *rows,
                                NSGridView **out) {
	NSGridView *grid = occam_grid(rows);
	if (out) *out = grid;

	NSView *inner = [[NSView alloc] initWithFrame:NSZeroRect];
	[inner addSubview:grid];
	[NSLayoutConstraint activateConstraints:@[
		[grid.topAnchor constraintEqualToAnchor:inner.topAnchor constant:14],
		[grid.leadingAnchor constraintEqualToAnchor:inner.leadingAnchor constant:16],
		[grid.trailingAnchor constraintLessThanOrEqualToAnchor:inner.trailingAnchor constant:-16],
		[grid.bottomAnchor constraintEqualToAnchor:inner.bottomAnchor constant:-14],
	]];

	NSBox *card = [[NSBox alloc] initWithFrame:NSZeroRect];
	card.boxType = NSBoxCustom;
	card.borderWidth = 0;
	card.cornerRadius = 10;
	card.fillColor = [NSColor controlBackgroundColor];
	card.titlePosition = NSNoTitle;
	card.contentViewMargins = NSZeroSize;
	card.contentView = inner;

	if (title.length == 0) {
		return card;
	}

	NSTextField *header = [NSTextField labelWithString:title];
	header.font = [NSFont systemFontOfSize:NSFont.smallSystemFontSize
	                                weight:NSFontWeightSemibold];
	header.textColor = [NSColor secondaryLabelColor];

	NSStackView *stack = [NSStackView stackViewWithViews:@[header, card]];
	stack.orientation = NSUserInterfaceLayoutOrientationVertical;
	stack.alignment = NSLayoutAttributeLeading;
	stack.spacing = 6;
	[card.widthAnchor constraintEqualToAnchor:stack.widthAnchor].active = YES;
	return stack;
}

static NSView *occam_group(NSString *title, NSArray<NSArray<NSView *> *> *rows) {
	return occam_group_grid(title, rows, NULL);
}

// A tab's worth of cards, scrolling so a long page is reachable in a window
// sized for the short ones.
static NSView *occam_page(NSArray<NSView *> *groups) {
	NSStackView *column = [NSStackView stackViewWithViews:groups];
	column.orientation = NSUserInterfaceLayoutOrientationVertical;
	column.alignment = NSLayoutAttributeWidth;
	column.spacing = 18;
	column.edgeInsets = NSEdgeInsetsMake(18, 18, 18, 18);
	column.translatesAutoresizingMaskIntoConstraints = NO;

	OccamFlippedView *doc = [[OccamFlippedView alloc] initWithFrame:NSZeroRect];
	doc.translatesAutoresizingMaskIntoConstraints = NO;
	[doc addSubview:column];

	NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSZeroRect];
	scroll.hasVerticalScroller = YES;
	scroll.drawsBackground = NO;
	scroll.documentView = doc;
	[NSLayoutConstraint activateConstraints:@[
		[column.topAnchor constraintEqualToAnchor:doc.topAnchor],
		[column.leadingAnchor constraintEqualToAnchor:doc.leadingAnchor],
		[column.trailingAnchor constraintEqualToAnchor:doc.trailingAnchor],
		[column.bottomAnchor constraintEqualToAnchor:doc.bottomAnchor],
		[doc.widthAnchor constraintEqualToAnchor:scroll.contentView.widthAnchor],
	]];
	return scroll;
}

static NSView *occam_headset_tab(const char **bandLabels, int minDB, int maxDB) {
	gSlotPicker = occam_popup(@selector(slotPicked:));
	NSView *preset = occam_group(@"Preset",
		@[@[occam_row_label(@""), gSlotPicker, occam_spacer()]]);

	NSMutableArray *bands = [NSMutableArray array];
	for (int i = 0; i < OCCAM_BANDS; i++) {
		gBand[i] = occam_slider(minDB, maxDB, (maxDB - minDB) + 1, @selector(bandMoved:));
		gBand[i].tag = i;
		gBandValue[i] = occam_value(@"+0");
		[bands addObject:@[occam_row_label([NSString stringWithUTF8String:bandLabels[i]]),
		                   gBand[i], gBandValue[i]]];
	}
	NSView *eq = occam_group(@"Equaliser", bands);

	// Segmented rather than a popup: this is the control Apple uses for
	// AirPods noise control, and all three modes stay visible.
	gANCMode = [[NSSegmentedControl alloc] initWithFrame:NSZeroRect];
	gANCMode.segmentStyle = NSSegmentStyleRounded;
	gANCMode.trackingMode = NSSegmentSwitchTrackingSelectOne;
	gANCMode.target = gWinTarget;
	gANCMode.action = @selector(ancPicked:);

	gANCLevel = occam_slider(1, 4, 4, @selector(ancLevelMoved:));
	gANCValue = occam_value(@"1");
	gBalance = occam_slider(0, 20, 21, @selector(balanceMoved:));
	gBalanceValue = occam_value(@"10");
	NSGridView *soundGrid = nil;
	NSView *sound = occam_group_grid(@"Sound", @[
		@[occam_row_label(@"Noise"), gANCMode, occam_spacer()],
		@[occam_row_label(@"Level"), gANCLevel, gANCValue],
		@[occam_row_label(@"Game/Chat"), gBalance, gBalanceValue],
	], &soundGrid);
	gANCLevelRow = [soundGrid rowAtIndex:1];

	gDongleLED = occam_popup(@selector(ledPicked:));
	gPowerOff = occam_popup(@selector(powerOffPicked:));
	gLowLatency = [NSButton checkboxWithTitle:@"Ultra-low latency"
	                                   target:gWinTarget action:@selector(lowLatencyToggled:)];
	NSView *device = occam_group(@"Device", @[
		@[occam_row_label(@"Light"), gDongleLED, occam_spacer()],
		@[occam_row_label(@"Sleep"), gPowerOff, occam_spacer()],
		@[occam_row_label(@"Latency"), gLowLatency, occam_spacer()],
	]);

	return occam_page(@[preset, eq, sound, device]);
}

static NSView *occam_spatial_tab(void) {
	gMixOn = [NSButton checkboxWithTitle:@"Render system audio to binaural"
	                              target:gWinTarget action:@selector(mixToggled:)];
	gTHX = [NSButton checkboxWithTitle:@"THX Spatial Audio"
	                           target:gWinTarget action:@selector(thxToggled:)];
	gMixLayout = occam_popup(@selector(mixLayoutPicked:));
	gMixStatus = [NSTextField labelWithString:@""];
	gMixStatus.font = [NSFont systemFontOfSize:11];
	gMixStatus.textColor = [NSColor secondaryLabelColor];

	NSView *renderer = occam_group(@"Spatial audio", @[
		@[occam_row_label(@""), gTHX, occam_spacer()],
		@[occam_row_label(@""), gMixOn, occam_spacer()],
		@[occam_row_label(@"Layout"), gMixLayout, occam_spacer()],
		@[occam_row_label(@""), gMixStatus, occam_spacer()],
	]);
	return occam_page(@[renderer]);
}

static NSView *occam_mic_tab(const char **bandLabels, int minDB, int maxDB) {
	gMicMute = [NSButton checkboxWithTitle:@"Mute microphone"
	                                target:gWinTarget action:@selector(micToggled:)];
	gSidetone = occam_slider(0, 15, 16, @selector(sidetoneMoved:));
	gSidetoneValue = occam_value(@"0");
	NSView *input = occam_group(@"Input", @[
		@[occam_row_label(@"Microphone"), gMicMute, occam_spacer()],
		@[occam_row_label(@"Monitoring"), gSidetone, gSidetoneValue],
	]);

	gMicPreset = occam_popup(@selector(micPresetPicked:));
	NSMutableArray *bands = [NSMutableArray array];
	[bands addObject:@[occam_row_label(@"Preset"), gMicPreset, occam_spacer()]];
	for (int i = 0; i < OCCAM_BANDS; i++) {
		gMicBand[i] = occam_slider(minDB, maxDB, (maxDB - minDB) + 1, @selector(micBandMoved:));
		gMicBand[i].tag = i;
		gMicBandValue[i] = occam_value(@"+0");
		[bands addObject:@[occam_row_label([NSString stringWithUTF8String:bandLabels[i]]),
		                   gMicBand[i], gMicBandValue[i]]];
	}
	NSView *eq = occam_group(@"Equaliser", bands);

	return occam_page(@[input, eq]);
}

static CGFloat gChromeHeight = 0;

static void occam_window_fit(NSTabViewItem *item) {
	if (!gWindow || !gTabs || !item.view) return;

	NSScrollView *scroll = (NSScrollView *)item.view;
	if (![scroll isKindOfClass:[NSScrollView class]]) return;

	CGFloat page = scroll.documentView.fittingSize.height;
	if (page <= 0) return;

	// Measured once, while the window still holds its designed size: after a
	// resize the tab view's own frame has moved and the arithmetic drifts.
	if (gChromeHeight <= 0) {
		gChromeHeight = gWindow.contentView.frame.size.height -
		                gTabs.contentRect.size.height;
	}

	CGFloat content = MIN(MAX(page + gChromeHeight, 300.0), 780.0);
	NSRect frame = gWindow.frame;
	CGFloat want = [gWindow frameRectForContentRect:
		NSMakeRect(0, 0, frame.size.width, content)].size.height;
	if (fabs(want - frame.size.height) < 1.0) return;

	frame.origin.y -= want - frame.size.height;
	frame.size.height = want;
	[gWindow setFrame:frame display:YES animate:NO];
}

void occam_window_build(const char **bandLabels, int minDB, int maxDB) {
	@autoreleasepool {
		if (gWindow) return;
		gWinTarget = [[OccamWindowTarget alloc] init];

		NSTabView *tabs = [[NSTabView alloc] initWithFrame:NSZeroRect];
		tabs.tabPosition = NSTabPositionTop;
		tabs.tabViewBorderType = NSTabViewBorderTypeNone;
		tabs.drawsBackground = NO;
		tabs.delegate = gWinTarget;
		gTabs = tabs;

		NSTabViewItem *headset = [[NSTabViewItem alloc] initWithIdentifier:@"headset"];
		headset.label = @"Headset";
		headset.image = occam_symbol(@"headphones");
		headset.view = occam_headset_tab(bandLabels, minDB, maxDB);
		NSTabViewItem *mic = [[NSTabViewItem alloc] initWithIdentifier:@"mic"];
		mic.label = @"Microphone";
		mic.image = occam_symbol(@"mic");
		mic.view = occam_mic_tab(bandLabels, minDB, maxDB);
		NSTabViewItem *spatial = [[NSTabViewItem alloc] initWithIdentifier:@"spatial"];
		spatial.label = @"Spatial";
		spatial.image = occam_symbol(@"waveform");
		spatial.view = occam_spatial_tab();
		[tabs addTabViewItem:headset];
		[tabs addTabViewItem:mic];
		[tabs addTabViewItem:spatial];

		gStatus = [NSTextField labelWithString:@""];
		gStatus.font = [NSFont systemFontOfSize:11];
		gStatus.textColor = [NSColor secondaryLabelColor];
		gStatus.lineBreakMode = NSLineBreakByTruncatingTail;
		[gStatus setContentHuggingPriority:NSLayoutPriorityDefaultLow
		                    forOrientation:NSLayoutConstraintOrientationHorizontal];

		NSButton *reload = [NSButton buttonWithTitle:@"Reload from Headset"
		                                      target:gWinTarget action:@selector(actionClicked:)];
		reload.tag = 2;
		NSButton *save = [NSButton buttonWithTitle:@"Save to Profile"
		                                    target:gWinTarget action:@selector(actionClicked:)];
		save.tag = 1;
		save.keyEquivalent = @"\r";

		NSStackView *buttons = [NSStackView stackViewWithViews:@[gStatus, reload, save]];
		buttons.orientation = NSUserInterfaceLayoutOrientationHorizontal;
		buttons.spacing = 8;

		// The tab view carries no inset of its own: the cards inside each page
		// already hold the margin, and doubling it looks cramped.
		NSStackView *root = [NSStackView stackViewWithViews:@[tabs, buttons]];
		root.orientation = NSUserInterfaceLayoutOrientationVertical;
		root.spacing = 10;
		root.edgeInsets = NSEdgeInsetsMake(10, 16, 16, 16);
		root.alignment = NSLayoutAttributeWidth;

		gWindow = [[NSWindow alloc]
			initWithContentRect:NSMakeRect(0, 0, 560, 620)
			          styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
			                    NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
			            backing:NSBackingStoreBuffered
			              defer:NO];
		gWindow.title = @"occam";
		gWindow.delegate = gWinTarget;
		gWindow.releasedWhenClosed = NO;
		gWindow.contentView = root;
		gWindow.titlebarAppearsTransparent = NO;
		[gWindow setContentMinSize:NSMakeSize(520, 300)];
		[gWindow layoutIfNeeded];
		occam_window_fit(gTabs.selectedTabViewItem);
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

static void occam_fill(NSPopUpButton *b, const char **names, int count, int selected) {
	if (!b) return;
	gQuiet = YES;
	[b removeAllItems];
	for (int i = 0; i < count; i++) {
		[b addItemWithTitle:[NSString stringWithUTF8String:names[i]]];
	}
	if (selected >= 0 && selected < count) [b selectItemAtIndex:selected];
	gQuiet = NO;
}

void occam_window_set_slots(const char **names, int count, int selected) {
	@autoreleasepool { occam_fill(gSlotPicker, names, count, selected); }
}

void occam_window_set_mic_presets(const char **names, int count, int selected) {
	@autoreleasepool { occam_fill(gMicPreset, names, count, selected); }
}

void occam_window_set_led_modes(const char **names, int count) {
	@autoreleasepool { occam_fill(gDongleLED, names, count, -1); }
}

void occam_window_set_anc_modes(const char **names, int count) {
	@autoreleasepool {
		if (!gANCMode) return;
		gQuiet = YES;
		gANCMode.segmentCount = count;
		for (int i = 0; i < count; i++) {
			[gANCMode setLabel:[NSString stringWithUTF8String:names[i]] forSegment:i];
			[gANCMode setWidth:0 forSegment:i];
		}
		gQuiet = NO;
	}
}

void occam_window_set_sleep_options(const char **names, int count) {
	@autoreleasepool { occam_fill(gPowerOff, names, count, -1); }
}

static void occam_set_bands(NSSlider * const *sliders, NSTextField * const *values,
                            const int *v, int count) {
	gQuiet = YES;
	for (int i = 0; i < count && i < OCCAM_BANDS; i++) {
		sliders[i].doubleValue = v[i];
		values[i].stringValue = [NSString stringWithFormat:@"%+d", v[i]];
	}
	gQuiet = NO;
}

void occam_window_set_bands(const int *values, int count) {
	@autoreleasepool { occam_set_bands(gBand, gBandValue, values, count); }
}

void occam_window_set_mic_bands(const int *values, int count) {
	@autoreleasepool { occam_set_bands(gMicBand, gMicBandValue, values, count); }
}

void occam_window_set_sidetone(int value) {
	@autoreleasepool {
		gQuiet = YES;
		gSidetone.doubleValue = value;
		gSidetoneValue.stringValue = [NSString stringWithFormat:@"%d", value];
		gQuiet = NO;
	}
}

void occam_window_set_extras(int ancMode, int ancLevel, int ancLevelActive,
                             int micMuted, int balance, int ledMode,
                             int sleepIndex, int lowLatency) {
	@autoreleasepool {
		gQuiet = YES;
		if (ancMode >= 0 && ancMode < gANCMode.segmentCount) {
			gANCMode.selectedSegment = ancMode;
		}
		gANCLevel.doubleValue = ancLevel;
		gANCValue.stringValue = [NSString stringWithFormat:@"%d", ancLevel];
		occam_anc_level_active(ancLevelActive);
		gMicMute.state = micMuted ? NSControlStateValueOn : NSControlStateValueOff;
		gBalance.doubleValue = balance;
		gBalanceValue.stringValue = [NSString stringWithFormat:@"%d", balance];
		if (ledMode >= 0 && ledMode < gDongleLED.numberOfItems) {
			[gDongleLED selectItemAtIndex:ledMode];
		}
		if (sleepIndex >= 0 && sleepIndex < gPowerOff.numberOfItems) {
			[gPowerOff selectItemAtIndex:sleepIndex];
		}
		gLowLatency.state = lowLatency ? NSControlStateValueOn : NSControlStateValueOff;
		gQuiet = NO;
	}
}

void occam_window_set_mix(const char **layouts, int count, int selected,
                          int on, int enabled, const char *status) {
	@autoreleasepool {
		occam_fill(gMixLayout, layouts, count, selected);
		gQuiet = YES;
		gMixOn.state = on ? NSControlStateValueOn : NSControlStateValueOff;
		gMixOn.enabled = enabled ? YES : NO;
		gMixLayout.enabled = enabled ? YES : NO;
		gMixStatus.stringValue = [NSString stringWithUTF8String:status];
		gQuiet = NO;
	}
}

void occam_window_set_thx(int on) {
	@autoreleasepool {
		gQuiet = YES;
		gTHX.state = on ? NSControlStateValueOn : NSControlStateValueOff;
		gQuiet = NO;
	}
}

void occam_window_set_anc_level_active(int active) {
	@autoreleasepool { occam_anc_level_active(active); }
}

void occam_window_set_status(const char *text) {
	@autoreleasepool {
		if (gStatus) gStatus.stringValue = [NSString stringWithUTF8String:text];
	}
}
