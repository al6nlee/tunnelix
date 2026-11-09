//go:build darwin && cgo && !purego

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// Compatibility adapter for Wails 3.1's menu-only status items. Keep AppKit's
// native menu tracking active before mouse-down, including clicks on a menu bar
// on another display. Wails' fallback synthesizes a click using button.window,
// which can still refer to the previously active display.
// No AppKit methods, screen coordinates, or backing scale factors are patched.
@protocol TunnelixTrayController <NSMenuDelegate>
@property (assign) NSStatusItem *statusItem;
@property (assign) NSMenu *cachedMenu;
@end

static void (*originalSetCachedMenu)(id, SEL, NSMenu *);
static void (*originalMenuDidClose)(id, SEL, NSMenu *);

static void bindNativeMenu(id<TunnelixTrayController> controller) {
    NSMenu *menu = controller.cachedMenu;
    menu.delegate = controller;
    controller.statusItem.menu = menu;
}

static void setCachedMenu(id controller, SEL selector, NSMenu *menu) {
    originalSetCachedMenu(controller, selector, menu);
    bindNativeMenu(controller);
}

static void menuDidClose(id controller, SEL selector, NSMenu *menu) {
    // Preserve Wails' displayed-menu lifecycle and retired callback IDs, then
    // restore native tracking for the next click (Wails normally clears it).
    originalMenuDidClose(controller, selector, menu);
    bindNativeMenu(controller);
}

static BOOL acceptsMenuArgument(Method method) {
    if (method == NULL || method_getNumberOfArguments(method) != 3) return NO;
    char result[8] = {0}, argument[8] = {0};
    method_getReturnType(method, result, sizeof(result));
    method_getArgumentType(method, 2, argument, sizeof(argument));
    return result[0] == 'v' && argument[0] == '@';
}

int tunnelixEnableNativeTrayMenus(void) {
    static dispatch_once_t once;
    static BOOL installed = NO;
    dispatch_once(&once, ^{
        Class controller = NSClassFromString(@"StatusItemController");
        Method setter = class_getInstanceMethod(controller, @selector(setCachedMenu:));
        Method close = class_getInstanceMethod(controller, @selector(menuDidClose:));
        if (!acceptsMenuArgument(setter) || !acceptsMenuArgument(close) ||
            ![controller instancesRespondToSelector:@selector(statusItem)] ||
            ![controller instancesRespondToSelector:@selector(cachedMenu)] ||
            ![controller instancesRespondToSelector:@selector(menuWillOpen:)]) return;

        originalSetCachedMenu = (void (*)(id, SEL, NSMenu *))method_getImplementation(setter);
        originalMenuDidClose = (void (*)(id, SEL, NSMenu *))method_getImplementation(close);
        method_setImplementation(setter, (IMP)setCachedMenu);
        method_setImplementation(close, (IMP)menuDidClose);
        installed = YES;
    });
    return installed;
}
