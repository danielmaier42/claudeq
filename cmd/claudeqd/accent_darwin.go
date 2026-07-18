//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

extern void goAccentChanged(void);

// Block-based observers avoid defining an Objective-C class in the cgo preamble
// (which would produce duplicate class symbols across cgo translation units).
static void cqStartAccentObserver(void) {
    @autoreleasepool {
        NSDistributedNotificationCenter *c = [NSDistributedNotificationCenter defaultCenter];
        void (^h)(NSNotification *) = ^(NSNotification *n){ goAccentChanged(); };
        [c addObserverForName:@"AppleColorPreferencesChangedNotification" object:nil queue:nil usingBlock:h];
        [c addObserverForName:@"AppleAquaColorVariantChanged" object:nil queue:nil usingBlock:h];
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

// startAccentNotifications observes the macOS distributed notification for
// accent/color changes and invokes onChange immediately — this fires as soon as
// the color is picked, before cfprefsd lazily flushes the preferences file (the
// source of the 1–3s delay a file watch alone would see). It runs a Cocoa run
// loop on a dedicated OS thread.
func startAccentNotifications(onChange func()) {
	onAccentChange = onChange
	go func() {
		runtime.LockOSThread()
		C.cqStartAccentObserver()
	}()
}
