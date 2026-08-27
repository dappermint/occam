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

// Must render from cached state: touching the device here hangs the menu
// behind HID I/O.
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
		// Accessory: out of the Dock without needing an .app bundle.
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
