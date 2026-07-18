//go:build darwin

// Objective-C implementation of claudeq's native menu bar. It lives in a .m
// file (compiled exactly once by the Go toolchain) rather than in a cgo
// preamble, because a preamble @implementation gets compiled into multiple
// object files and the linker then rejects the duplicate OBJC class symbols.

#import <AppKit/AppKit.h>
#include "menu_cocoa.h"

// Defined in Go (//export). The custom menu items call back into these.
extern void goMenuNewTask(void);
extern void goMenuSettings(void);

// Target for the custom menu items. Standard items (Quit, Close, Cut/Copy/…)
// use built-in selectors routed through the responder chain and need no target.
@interface CQMenuTarget : NSObject
@end
@implementation CQMenuTarget
- (void)cqNewTask:(id)sender  { (void)sender; goMenuNewTask(); }
- (void)cqSettings:(id)sender { (void)sender; goMenuSettings(); }
- (void)cqAbout:(id)sender {
    // The standard About panel already shows the bundle icon (our logo),
    // CFBundleName and the version — a native, familiar dialog.
    [NSApp orderFrontStandardAboutPanel:sender];
    [NSApp activateIgnoringOtherApps:YES];
}
@end

// Kept alive for the process lifetime so the menu targets stay valid.
static CQMenuTarget *gMenuTarget = nil;

static NSMenuItem *cqAdd(NSMenu *m, NSString *title, SEL action, NSString *key, id target) {
    NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:title action:action keyEquivalent:key];
    if (target) { [it setTarget:target]; }
    [m addItem:it];
    return it;
}

static void cqBuildMenu(void) {
    NSString *app = @"ClaudeQ";
    gMenuTarget = [[CQMenuTarget alloc] init];

    NSMenu *main = [[NSMenu alloc] init];

    // App menu
    NSMenuItem *appItem = [[NSMenuItem alloc] init];
    [main addItem:appItem];
    NSMenu *appMenu = [[NSMenu alloc] init];
    [appItem setSubmenu:appMenu];
    cqAdd(appMenu, [@"About " stringByAppendingString:app], @selector(cqAbout:), @"", gMenuTarget);
    [appMenu addItem:[NSMenuItem separatorItem]];
    cqAdd(appMenu, @"Settings…", @selector(cqSettings:), @",", gMenuTarget);
    [appMenu addItem:[NSMenuItem separatorItem]];
    cqAdd(appMenu, [@"Hide " stringByAppendingString:app], @selector(hide:), @"h", nil);
    NSMenuItem *hideOthers = cqAdd(appMenu, @"Hide Others", @selector(hideOtherApplications:), @"h", nil);
    [hideOthers setKeyEquivalentModifierMask:(NSEventModifierFlagOption | NSEventModifierFlagCommand)];
    cqAdd(appMenu, @"Show All", @selector(unhideAllApplications:), @"", nil);
    [appMenu addItem:[NSMenuItem separatorItem]];
    cqAdd(appMenu, [@"Quit " stringByAppendingString:app], @selector(terminate:), @"q", nil);

    // File menu
    NSMenuItem *fileItem = [[NSMenuItem alloc] init];
    [main addItem:fileItem];
    NSMenu *fileMenu = [[NSMenu alloc] initWithTitle:@"File"];
    [fileItem setSubmenu:fileMenu];
    cqAdd(fileMenu, @"New Task", @selector(cqNewTask:), @"n", gMenuTarget);
    [fileMenu addItem:[NSMenuItem separatorItem]];
    cqAdd(fileMenu, @"Close Window", @selector(performClose:), @"w", nil);

    // Edit menu — needed for Cut/Copy/Paste/Select All and their shortcuts to
    // work inside the WKWebView's text fields.
    NSMenuItem *editItem = [[NSMenuItem alloc] init];
    [main addItem:editItem];
    NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
    [editItem setSubmenu:editMenu];
    cqAdd(editMenu, @"Undo", @selector(undo:), @"z", nil);
    NSMenuItem *redo = cqAdd(editMenu, @"Redo", @selector(redo:), @"z", nil);
    [redo setKeyEquivalentModifierMask:(NSEventModifierFlagShift | NSEventModifierFlagCommand)];
    [editMenu addItem:[NSMenuItem separatorItem]];
    cqAdd(editMenu, @"Cut", @selector(cut:), @"x", nil);
    cqAdd(editMenu, @"Copy", @selector(copy:), @"c", nil);
    cqAdd(editMenu, @"Paste", @selector(paste:), @"v", nil);
    cqAdd(editMenu, @"Select All", @selector(selectAll:), @"a", nil);

    // Window menu
    NSMenuItem *winItem = [[NSMenuItem alloc] init];
    [main addItem:winItem];
    NSMenu *winMenu = [[NSMenu alloc] initWithTitle:@"Window"];
    [winItem setSubmenu:winMenu];
    cqAdd(winMenu, @"Minimize", @selector(performMiniaturize:), @"m", nil);
    cqAdd(winMenu, @"Zoom", @selector(performZoom:), @"", nil);
    [NSApp setWindowsMenu:winMenu];

    [NSApp setMainMenu:main];
}

void cqInstallMenu(void) {
    if ([NSThread isMainThread]) {
        cqBuildMenu();
    } else {
        dispatch_sync(dispatch_get_main_queue(), ^{ cqBuildMenu(); });
    }
}
