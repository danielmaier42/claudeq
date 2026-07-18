//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreFoundation
#import <Foundation/Foundation.h>
#import <CoreFoundation/CoreFoundation.h>

extern void goAccentChanged(void);

// cqReadAccentIndex reads AppleAccentColor in-process (through cfprefsd).
// Returns -100 when the key is unset (default/multicolor).
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

// Observe every distributed notification (name:nil) so we catch the appearance
// change regardless of its exact notification name; the Go side re-reads and
// dedupes, so unrelated notifications are cheap.
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

// startAccentObserver invokes onChange whenever a system notification fires
// (instant, in the app's GUI session). Runs a Cocoa run loop on its own thread.
func startAccentObserver(onChange func()) {
	onAccentChange = onChange
	go func() {
		runtime.LockOSThread()
		C.cqStartAccentObserver()
	}()
}
