//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreFoundation -framework AppKit
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>

extern void goAccentChanged(void);

// cqAccentHex returns the effective accent color as "#rrggbb" by asking AppKit
// directly (NSColor.controlAccentColor) — the same live value the system UI
// draws. This avoids the CFPreferences read lag: after the user changes the
// accent, cfprefsd serves the old AppleAccentColor for 1-3s, but AppKit's
// controlAccentColor reflects the change immediately. Caller frees the result.
static const char *cqAccentHex(void) {
    __block char *out = NULL;
    void (^work)(void) = ^{
        @autoreleasepool {
            NSColor *c = [NSColor controlAccentColor];
            // Resolve against the app's current appearance so the RGB matches
            // what is shown, then convert to sRGB for a stable hex.
            NSColor *s = [c colorUsingColorSpace:[NSColorSpace sRGBColorSpace]];
            if (!s) { return; }
            CGFloat r = 0, g = 0, b = 0, a = 0;
            [s getRed:&r green:&g blue:&b alpha:&a];
            char buf[8];
            snprintf(buf, sizeof buf, "#%02x%02x%02x",
                     (int)round(r * 255.0), (int)round(g * 255.0), (int)round(b * 255.0));
            out = strdup(buf);
        }
    };
    if ([NSThread isMainThread]) {
        work();
    } else {
        dispatch_sync(dispatch_get_main_queue(), work);
    }
    return out;
}

// Observe every distributed notification (name:nil) so we catch both the
// appearance change and the accent-color change regardless of exact name; the
// Go side re-reads (with retries) and applies, so unrelated notifications are
// cheap.
static void cqStartAccentObserver(void) {
    @autoreleasepool {
        [[NSDistributedNotificationCenter defaultCenter]
            addObserverForName:nil object:nil queue:nil
            usingBlock:^(NSNotification *n){ goAccentChanged(); }];
        [[NSRunLoop currentRunLoop] run];
    }
}
*/
import "C"

import (
	"runtime"
	"unsafe"
)

var onAccentChange func()

//export goAccentChanged
func goAccentChanged() {
	if f := onAccentChange; f != nil {
		f()
	}
}

// accentHex returns the current macOS accent color as "#rrggbb" (empty on
// failure), read live from AppKit.
func accentHex() string {
	c := C.cqAccentHex()
	if c == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(c))
	return C.GoString(c)
}

// startAccentObserver invokes onChange whenever a system notification fires
// (instant, in the app's GUI session). Runs a Cocoa run loop on its own thread.
func startAccentObserver(onChange func()) {
	onAccentChange = onChange
	go func() {
		runtime.LockOSThread()
		C.cqStartAccentObserver()
	}()
}
