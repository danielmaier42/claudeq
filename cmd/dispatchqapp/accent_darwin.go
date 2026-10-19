//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreFoundation -framework AppKit
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#import <CoreFoundation/CoreFoundation.h>

extern void goAccentChanged(void);

// cqReadAccentIndex reads AppleAccentColor from the global domain (through
// cfprefsd). Returns -100 when the key is unset (default/multicolor accent).
// After the user changes the accent, cfprefsd can serve the old value for a
// short moment, so callers re-read a few times (see applyAccent).
static long cqReadAccentIndex(void) {
    CFPreferencesAppSynchronize(kCFPreferencesAnyApplication);
    CFPropertyListRef v = CFPreferencesCopyAppValue(CFSTR("AppleAccentColor"), kCFPreferencesAnyApplication);
    long result = -100;
    if (v) {
        if (CFGetTypeID(v) == CFNumberGetTypeID()) {
            CFNumberGetValue((CFNumberRef)v, kCFNumberLongType, &result);
        }
        CFRelease(v);
    }
    return result;
}

// cqStartAccentObserver registers for the two signals that matter:
//   * Distributed AppleInterfaceThemeChangedNotification — dark/light toggle.
//   * Local NSSystemColorsDidChangeNotification — AppKit posts this in-process
//     when the accent/highlight color changes. A plain accent change fires NO
//     distributed notification, so this local one is the trigger for it.
// Both just ping Go, which re-reads the accent (with retries) and re-applies.
static void cqStartAccentObserver(void) {
    @autoreleasepool {
        [[NSDistributedNotificationCenter defaultCenter]
            addObserverForName:@"AppleInterfaceThemeChangedNotification" object:nil queue:nil
            usingBlock:^(NSNotification *n){ goAccentChanged(); }];
        [[NSNotificationCenter defaultCenter]
            addObserverForName:NSSystemColorsDidChangeNotification object:nil
            queue:[NSOperationQueue mainQueue]
            usingBlock:^(NSNotification *n){ goAccentChanged(); }];
        [[NSRunLoop currentRunLoop] run];
    }
}
*/
import "C"

import "runtime"

var onAccentChange func()

//export goAccentChanged
func goAccentChanged() {
	if f := onAccentChange; f != nil {
		f()
	}
}

// readAccentIndex returns the current macOS accent index (-100 if unset).
func readAccentIndex() int { return int(C.cqReadAccentIndex()) }

// startAccentObserver invokes onChange whenever the appearance or the accent
// color changes (instant, in the app's GUI session). Runs a Cocoa run loop on
// its own thread for the distributed observer.
func startAccentObserver(onChange func()) {
	onAccentChange = onChange
	go func() {
		runtime.LockOSThread()
		C.cqStartAccentObserver()
	}()
}
