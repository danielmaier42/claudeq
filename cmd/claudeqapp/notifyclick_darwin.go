//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework UserNotifications
#include "notifyclick_cocoa.h"
*/
import "C"

import "sync"

// pending holds the artifact id from the most recent notification click until
// the page asks for it. It is a one-slot mailbox rather than a direct call
// because the click can arrive before the window (and its page) exist: a click
// that launches the app is delivered while webview.New is still running its
// temporary launch loop.
var pending struct {
	mu         sync.Mutex
	artifactID string
	onClick    func()
}

//export goNotificationClicked
func goNotificationClicked(artifactID *C.char) {
	id := C.GoString(artifactID)
	if id == "" {
		return // nothing to open; the click activated the app, which is enough
	}
	pending.mu.Lock()
	pending.artifactID = id
	onClick := pending.onClick
	pending.mu.Unlock()
	if onClick != nil {
		onClick()
	}
}

// installNotifyDelegate registers this process as the handler for clicks on
// ClaudeQ notifications. It must run before the application finishes launching
// (i.e. before webview.New), so a click that launched the app is delivered.
func installNotifyDelegate() { C.cqInstallNotifyDelegate() }

// onNotificationClick sets the callback that runs after a click has been
// recorded — it nudges the page to fetch the pending artifact. Clicks that
// arrive before it is set are picked up by the page itself when it loads.
func onNotificationClick(f func()) {
	pending.mu.Lock()
	pending.onClick = f
	pending.mu.Unlock()
}

// takePendingArtifact returns and clears the pending artifact id ("" when there
// is none). Taking is atomic, so the nudge above and the page's own load-time
// check can both run without opening the artifact twice.
func takePendingArtifact() string {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	id := pending.artifactID
	pending.artifactID = ""
	return id
}
