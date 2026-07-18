//go:build darwin

package main

/*
#cgo LDFLAGS: -framework AppKit
#include "menu_cocoa.h"
*/
import "C"

var (
	onMenuNewTask  func()
	onMenuSettings func()
)

//export goMenuNewTask
func goMenuNewTask() {
	if f := onMenuNewTask; f != nil {
		f()
	}
}

//export goMenuSettings
func goMenuSettings() {
	if f := onMenuSettings; f != nil {
		f()
	}
}

// installMenu sets claudeq's native menu bar and wires the custom items back to
// the given handlers (which drive the dashboard in the WKWebView).
func installMenu(newTask, settings func()) {
	onMenuNewTask = newTask
	onMenuSettings = settings
	C.cqInstallMenu()
}
