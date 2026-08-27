//go:build darwin

#import <Cocoa/Cocoa.h>

// Implemented in Go, see menu_darwin.go.
extern void occamMenuSelect(int tag);
extern void occamMenuWillOpen(void);

@interface OccamMenuTarget : NSObject <NSMenuDelegate>
@end

static NSStatusItem   *gItem = nil;
static NSMenu         *gMenu = nil;
static OccamMenuTarget *gTarget = nil;

@implementation OccamMenuTarget

// Fires on the main thread just before the menu draws, which is what makes the
// contents live. The Go side must render from cached state here and never
// touch the device, or the menu hangs while HID I/O happens.
- (void)menuWillOpen:(NSMenu *)menu {
	occamMenuWillOpen();
}

- (void)itemClicked:(id)sender {
	occamMenuSelect((int)[(NSMenuItem *)sender tag]);
}

@end

void occam_menu_start(const char *title) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		// Accessory keeps it out of the Dock and the app switcher without
		// needing an .app bundle with LSUIElement.
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

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
