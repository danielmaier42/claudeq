//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework UserNotifications
#include "notifyclick_cocoa.h"
*/
import "C"

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// notificationTarget is what a clicked notification asks the page to show: the
// inbox entry it is, and what it is about — the artifact, the run, or the link
// (at most one set; all empty means the Notifications view itself). The field
// names are what the page reads, see cqOpenNotificationTarget in the dashboard.
type notificationTarget struct {
	ID         string `json:"id,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	URL        string `json:"url,omitempty"`
}

// pending holds the target of the most recent notification click until the
// page asks for it. It is a one-slot mailbox rather than a direct call because
// the click can arrive before the window (and its page) exist: a click that
// launches the app is delivered while webview.New is still running its
// temporary launch loop.
var pending struct {
	mu      sync.Mutex
	target  *notificationTarget
	onClick func()
}

// readTimeout bounds the mark-read request sent for a click whose link opened
// in the browser — the one case the page is not involved in.
const readTimeout = 3 * time.Second

//export goNotificationClicked
func goNotificationClicked(notificationID, artifactID, runID, link *C.char, openedLink C.int) {
	t := notificationTarget{ID: C.GoString(notificationID), ArtifactID: C.GoString(artifactID),
		RunID: C.GoString(runID), URL: C.GoString(link)}
	if openedLink != 0 {
		// The link was the destination; the window has nothing to add. Marking
		// the notification read is this process's job then — off the main
		// thread, which the delegate callback runs on.
		if t.ID != "" {
			go func() {
				if err := markNotificationRead(dashboardURL, t.ID); err != nil {
					fmt.Fprintln(os.Stderr, "claudeqapp: could not mark notification read:", err)
				}
			}()
		}
		return
	}
	// Everything else is the page's: it marks the notification read and opens
	// the target, so the click is not kept waiting on a request from here.
	pending.mu.Lock()
	pending.target = &t
	onClick := pending.onClick
	pending.mu.Unlock()
	if onClick != nil {
		onClick()
	}
}

// markNotificationRead tells the daemon at base that the operator clicked the
// notification with the given inbox id. Best-effort: an entry the daemon no
// longer holds is reported, not retried.
func markNotificationRead(base, id string) error {
	c := http.Client{Timeout: readTimeout}
	resp, err := c.Post(base+"/api/notifications/"+id+"/read", "application/json", nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("daemon answered %s", resp.Status)
	}
	return nil
}

// installNotifyDelegate registers this process as the handler for clicks on
// ClaudeQ notifications. It must run before the application finishes launching
// (i.e. before webview.New), so a click that launched the app is delivered.
func installNotifyDelegate() { C.cqInstallNotifyDelegate() }

// onNotificationClick sets the callback that runs after a click has been
// recorded — it nudges the page to fetch the pending target. Clicks that
// arrive before it is set are picked up by the page itself when it loads.
func onNotificationClick(f func()) {
	pending.mu.Lock()
	pending.onClick = f
	pending.mu.Unlock()
}

// takePendingNotification returns and clears the pending target (nil when
// there is none). Taking is atomic, so the nudge above and the page's own
// load-time check can both run without opening the target twice.
func takePendingNotification() *notificationTarget {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	t := pending.target
	pending.target = nil
	return t
}
