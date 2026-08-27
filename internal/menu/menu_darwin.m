//go:build darwin

#import <Cocoa/Cocoa.h>

// Implemented in Go, see menu_darwin.go.
extern void occamMenuSelect(int tag);
extern void occamMenuWillOpen(void);
extern void occamMenuValue(int tag, int value);

@interface OccamMenuTarget : NSObject <NSMenuDelegate>
@end

static NSStatusItem   *gItem = nil;
static NSMenu         *gMenu = nil;
static OccamMenuTarget *gTarget = nil;

@implementation OccamMenuTarget

// Must render from cached state: touching the device here hangs the menu
// behind HID I/O.
- (void)menuWillOpen:(NSMenu *)menu {
	occamMenuWillOpen();
}

- (void)itemClicked:(id)sender {
	occamMenuSelect((int)[(NSMenuItem *)sender tag]);
}

- (void)sliderMoved:(id)sender {
	NSSlider *s = (NSSlider *)sender;
	occamMenuValue((int)s.tag, (int)lround(s.doubleValue));
}

- (void)segmentPicked:(id)sender {
	NSSegmentedControl *c = (NSSegmentedControl *)sender;
	occamMenuValue((int)c.tag, (int)c.selectedSegment);
}

@end

// Without a main menu no window responds to the shortcuts every macOS window
// is expected to honour: command-W to close, command-Q to quit, and the edit
// commands a text field needs. An accessory app never shows this menu, but the
// key equivalents route through it all the same.
static void occam_install_main_menu(void) {
	NSMenu *bar = [[NSMenu alloc] init];

	NSMenuItem *appItem = [[NSMenuItem alloc] init];
	NSMenu *app = [[NSMenu alloc] init];
	[app addItemWithTitle:@"Quit occam" action:@selector(terminate:) keyEquivalent:@"q"];
	appItem.submenu = app;
	[bar addItem:appItem];

	NSMenuItem *editItem = [[NSMenuItem alloc] init];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
	editItem.submenu = edit;
	[bar addItem:editItem];

	NSMenuItem *windowItem = [[NSMenuItem alloc] init];
	NSMenu *window = [[NSMenu alloc] initWithTitle:@"Window"];
	[window addItemWithTitle:@"Close" action:@selector(performClose:) keyEquivalent:@"w"];
	[window addItemWithTitle:@"Minimise" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
	windowItem.submenu = window;
	[bar addItem:windowItem];

	NSApp.mainMenu = bar;
	NSApp.windowsMenu = window;
}

void occam_menu_start(const char *title) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		// Accessory: out of the Dock without needing an .app bundle.
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		occam_install_main_menu();

		gTarget = [[OccamMenuTarget alloc] init];
		gMenu = [[NSMenu alloc] init];
		[gMenu setAutoenablesItems:NO];
		gMenu.delegate = gTarget;

		gItem = [[NSStatusBar systemStatusBar]
			statusItemWithLength:NSVariableStatusItemLength];
		gItem.button.title = [NSString stringWithUTF8String:title];
		gItem.menu = gMenu;
	}
}

// Template images follow the menu bar's own light/dark and highlight states,
// which a text glyph or an emoji does not.
int occam_menu_set_symbol(const char *name, const char *fallback) {
	@autoreleasepool {
		if (!gItem) return 0;
		NSImage *img = [NSImage imageWithSystemSymbolName:[NSString stringWithUTF8String:name]
		                        accessibilityDescription:nil];
		if (!img) {
			gItem.button.title = [NSString stringWithUTF8String:fallback];
			return 0;
		}
		img.template = YES;
		gItem.button.image = img;
		gItem.button.title = @"";
		return 1;
	}
}

void occam_menu_set_title(const char *title) {
	@autoreleasepool {
		if (gItem) gItem.button.title = [NSString stringWithUTF8String:title];
	}
}

void occam_menu_clear(void) {
	@autoreleasepool {
		[gMenu removeAllItems];
	}
}

void occam_menu_add(const char *title, int tag, int checked, int enabled) {
	@autoreleasepool {
		NSMenuItem *item = [[NSMenuItem alloc]
			initWithTitle:[NSString stringWithUTF8String:title]
			       action:@selector(itemClicked:)
			keyEquivalent:@""];
		item.target = gTarget;
		item.tag = tag;
		item.state = checked ? NSControlStateValueOn : NSControlStateValueOff;
		[item setEnabled:(enabled ? YES : NO)];
		[gMenu addItem:item];
	}
}

// Real section headers, the way the system menus draw them. Added in macOS 14;
// older systems fall back to a disabled row.
void occam_menu_add_section(const char *title) {
	@autoreleasepool {
		NSString *t = [NSString stringWithUTF8String:title];
		if ([NSMenuItem respondsToSelector:@selector(sectionHeaderWithTitle:)]) {
			[gMenu addItem:[NSMenuItem sectionHeaderWithTitle:t]];
			return;
		}
		NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:t action:nil keyEquivalent:@""];
		[item setEnabled:NO];
		[gMenu addItem:item];
	}
}

// Sizes a control row to itself, with a floor so it does not read as narrower
// than the plain rows above it.
// Both control rows share a width so the noise segments and the level slider
// line up under each other.
#define OCCAM_ROW_WIDTH 252.0
#define OCCAM_ROW_INSET 14.0

static void occam_menu_add_row(NSStackView *row, int tag) {
	NSSize fit = row.fittingSize;
	row.frame = NSMakeRect(0, 0, MAX(fit.width, OCCAM_ROW_WIDTH), MAX(fit.height, 28.0));

	NSMenuItem *item = [[NSMenuItem alloc] init];
	item.view = row;
	item.tag = tag;
	[gMenu addItem:item];
}

// The menu is only rebuilt on open, so a row whose relevance changes under a
// click has to be hidden in place.
void occam_menu_set_row_hidden(int tag, int hidden) {
	@autoreleasepool {
		for (NSMenuItem *item in gMenu.itemArray) {
			if (item.tag == tag && item.view) {
				item.hidden = hidden ? YES : NO;
				return;
			}
		}
	}
}

// A control inside a menu item's own view, the way the Sound menu carries its
// volume slider. A plain menu row cannot hold one.
void occam_menu_add_slider(const char *label, int tag, double lo, double hi,
                           double value, int enabled) {
	@autoreleasepool {
		NSTextField *name = [NSTextField labelWithString:
			[NSString stringWithUTF8String:label]];
		name.font = [NSFont menuFontOfSize:0];
		name.textColor = enabled ? [NSColor labelColor] : [NSColor tertiaryLabelColor];

		NSSlider *slider = [NSSlider sliderWithValue:value minValue:lo maxValue:hi
		                                      target:gTarget
		                                      action:@selector(sliderMoved:)];
		slider.tag = tag;
		slider.enabled = enabled ? YES : NO;
		slider.continuous = NO;
		slider.numberOfTickMarks = (int)(hi - lo) + 1;
		slider.allowsTickMarkValuesOnly = YES;
		slider.controlSize = NSControlSizeRegular;
		[slider setContentHuggingPriority:NSLayoutPriorityDefaultLow
		                   forOrientation:NSLayoutConstraintOrientationHorizontal];

		NSStackView *row = [NSStackView stackViewWithViews:@[name, slider]];
		row.orientation = NSUserInterfaceLayoutOrientationHorizontal;
		row.spacing = 10;
		row.edgeInsets = NSEdgeInsetsMake(3, OCCAM_ROW_INSET, 3, OCCAM_ROW_INSET);
		occam_menu_add_row(row, tag);
	}
}

// Labels double as tooltips: the icons carry the row on their own, but a
// glyph alone is not something to make anyone guess at.
void occam_menu_add_segments(const char **labels, const char **symbols, int count,
                             int tag, int selected) {
	@autoreleasepool {
		NSSegmentedControl *seg = [[NSSegmentedControl alloc] initWithFrame:NSZeroRect];
		seg.segmentStyle = NSSegmentStyleRounded;
		seg.trackingMode = NSSegmentSwitchTrackingSelectOne;
		seg.target = gTarget;
		seg.action = @selector(segmentPicked:);
		seg.tag = tag;
		seg.font = [NSFont menuFontOfSize:0];
		seg.controlSize = NSControlSizeLarge;
		seg.segmentCount = count;

		// Filling the row rather than hugging three glyphs: at natural width
		// the control read as a stray cluster against the menu's own width.
		CGFloat segWidth = (OCCAM_ROW_WIDTH - OCCAM_ROW_INSET * 2) / (CGFloat)count;
		NSImageSymbolConfiguration *big = [NSImageSymbolConfiguration
			configurationWithPointSize:15 weight:NSFontWeightRegular
			                     scale:NSImageSymbolScaleMedium];
		for (int i = 0; i < count; i++) {
			NSString *label = [NSString stringWithUTF8String:labels[i]];
			NSImage *icon = symbols ? [NSImage
				imageWithSystemSymbolName:[NSString stringWithUTF8String:symbols[i]]
				 accessibilityDescription:label] : nil;
			if (icon) {
				[seg setImage:[icon imageWithSymbolConfiguration:big] forSegment:i];
				[seg setImageScaling:NSImageScaleProportionallyDown forSegment:i];
			} else {
				[seg setLabel:label forSegment:i];
			}
			[seg setToolTip:label forSegment:i];
			[seg setWidth:segWidth forSegment:i];
		}
		if (selected >= 0 && selected < count) {
			seg.selectedSegment = selected;
		}

		NSStackView *row = [NSStackView stackViewWithViews:@[seg]];
		row.orientation = NSUserInterfaceLayoutOrientationHorizontal;
		row.edgeInsets = NSEdgeInsetsMake(3, OCCAM_ROW_INSET, 5, OCCAM_ROW_INSET);
		occam_menu_add_row(row, tag);
	}
}

void occam_menu_add_separator(void) {
	@autoreleasepool {
		[gMenu addItem:[NSMenuItem separatorItem]];
	}
}

void occam_menu_run(void) {
	[NSApp run];
}

void occam_menu_quit(void) {
	[NSApp terminate:nil];
}
