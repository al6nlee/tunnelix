#import <Cocoa/Cocoa.h>
#import "systemtray_darwin.h"
#include <assert.h>

extern int tunnelixEnableNativeTrayMenus(void);
static int opened, closed, clicked;
void systrayClickCallback(long tray, int button) { clicked++; }
int systrayPreClickCallback(long tray, int button) { return 1; }
void systrayMenuDisplayedCallback(long tray, int displayed) {
    if (displayed) opened++; else closed++;
}

int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        assert(tunnelixEnableNativeTrayMenus());
        assert(tunnelixEnableNativeTrayMenus()); // Installing twice must not recurse.
        NSStatusItem *item = (NSStatusItem *)systemTrayNew(1001);
        item.visible = NO;
        StatusItemController *controller = (StatusItemController *)item.target;
        NSMenu *first = [[NSMenu alloc] initWithTitle:@"First"];
        [first addItemWithTitle:@"Test" action:nil keyEquivalent:@""];
        systemTraySetCachedMenu(item, first);
        assert(item.menu == first && first.delegate == controller);

        [controller menuWillOpen:first];
        [controller menuDidClose:first];
        assert(opened == 1 && closed == 1);
        assert(item.menu == first && first.delegate == controller);

        // Rebuild/replace while open: close must restore the CURRENT menu,
        // not resurrect the old one or lose Wails' lifecycle notifications.
        [controller menuWillOpen:first];
        NSMenu *second = [[NSMenu alloc] initWithTitle:@"Second"];
        systemTraySetCachedMenu(item, second);
        [controller menuDidClose:first];
        assert(opened == 2 && closed == 2);
        assert(first.delegate == nil);
        assert(item.menu == second && second.delegate == controller);
        assert(clicked == 0); // Binding never synthesizes a mouse click.

        // Content refresh keeps the same attached native menu.
        [second addItemWithTitle:@"Updated" action:nil keyEquivalent:@""];
        assert(item.menu.numberOfItems == 1);
        systemTraySetCachedMenu(item, nil);
        assert(item.menu == nil);
        systemTrayDestroy(item);
        [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
        [first release];
        [second release];
    }
    return 0;
}
