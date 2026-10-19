//go:build darwin

// Objective-C side of the dashboard's file picker. webview_go does install a
// WKUIDelegate that opens an NSOpenPanel for <input type="file">, but it hands
// the delegate to WKWebView autoreleased and never keeps a reference — and
// WKWebView holds its UIDelegate weakly. Once the autorelease pool drains the
// delegate is gone, and a click on "Import…" opens nothing at all. This file
// installs a delegate that the process keeps alive.
//
// Lives in a .m file for the same reason as menu_cocoa.m: an @implementation
// in a cgo preamble is compiled into several object files and the linker then
// rejects the duplicate OBJC class symbols.

#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>
#include "openpanel_cocoa.h"

@interface CQOpenPanelDelegate : NSObject <WKUIDelegate>
@end

@implementation CQOpenPanelDelegate

- (void)webView:(WKWebView *)webView
    runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters
              initiatedByFrame:(WKFrameInfo *)frame
             completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.canChooseFiles = YES;
    panel.canChooseDirectories = parameters.allowsDirectories;
    panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
    NSWindow *window = webView.window;
    void (^finish)(NSModalResponse) = ^(NSModalResponse response) {
        completionHandler(response == NSModalResponseOK ? panel.URLs : nil);
    };
    if (window != nil) {
        [panel beginSheetModalForWindow:window completionHandler:finish];
    } else {
        finish([panel runModal]);
    }
}

@end

// The delegate must outlive the web view: WKWebView.UIDelegate is weak.
static CQOpenPanelDelegate *cqOpenPanelDelegate = nil;

static WKWebView *cqFindWebView(NSView *view) {
    if ([view isKindOfClass:[WKWebView class]]) {
        return (WKWebView *)view;
    }
    for (NSView *sub in view.subviews) {
        WKWebView *found = cqFindWebView(sub);
        if (found != nil) {
            return found;
        }
    }
    return nil;
}

int cqInstallOpenPanel(void *nsWindow) {
    NSWindow *window = (__bridge NSWindow *)nsWindow;
    WKWebView *webView = cqFindWebView(window.contentView);
    if (webView == nil) {
        return 0;
    }
    if (cqOpenPanelDelegate == nil) {
        cqOpenPanelDelegate = [CQOpenPanelDelegate new];
    }
    webView.UIDelegate = cqOpenPanelDelegate;
    return 1;
}
