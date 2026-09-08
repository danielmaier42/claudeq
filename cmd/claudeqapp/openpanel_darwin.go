//go:build darwin

package main

/*
#cgo LDFLAGS: -framework AppKit -framework WebKit
#include "openpanel_cocoa.h"
*/
import "C"

import (
	"log"
	"unsafe"
)

// installOpenPanel wires the dashboard's <input type="file"> (the Import…
// button) to a native open panel. webview_go's own delegate for this is
// dropped by the runtime before it is ever used (see openpanel_cocoa.m), so
// without this the Import… button silently does nothing.
func installOpenPanel(nsWindow unsafe.Pointer) {
	if C.cqInstallOpenPanel(nsWindow) == 0 {
		log.Print("claudeqapp: no WKWebView found in the window; file picker unavailable")
	}
}
