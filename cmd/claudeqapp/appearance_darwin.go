//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

// cqSetAppearance pins the application to light or dark, or hands it back to
// the system for anything else. NSApp.appearance is what the title bar,
// menus and panels draw with, and what WKWebView reports to the page as
// prefers-color-scheme — so page and chrome always agree. Main thread only.
static void cqSetAppearance(const char *mode) {
    NSAppearanceName name = nil;
    if (strcmp(mode, "dark") == 0) name = NSAppearanceNameDarkAqua;
    else if (strcmp(mode, "light") == 0) name = NSAppearanceNameAqua;
    NSApp.appearance = name ? [NSAppearance appearanceNamed:name] : nil;
}
*/
import "C"

import "unsafe"

// setAppearance applies Settings → General → Appearance to the app: "light",
// "dark", or anything else to follow macOS. Must run on the main thread
// (callers go through webview.Dispatch).
func setAppearance(mode string) {
	cs := C.CString(mode)
	defer C.free(unsafe.Pointer(cs))
	C.cqSetAppearance(cs)
}
