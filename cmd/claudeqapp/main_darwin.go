//go:build darwin

// Command claudeqapp is the native macOS window for claudeq: a thin WKWebView
// that shows the dashboard served by the claudeqd daemon (PLAN.md D3, phase 5b).
// It starts the daemon if it isn't already running and injects the real macOS
// accent color (which WebKit's CSS AccentColor does not expose reliably).
package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	webview "github.com/webview/webview_go"
)

const dashboardURL = "http://127.0.0.1:8765"

func main() {
	runtime.LockOSThread()
	ensureDaemon()

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("claudeq")
	w.SetSize(1120, 760, webview.HintNone)

	// Expose the current accent to the page and (re)apply it on each load.
	_ = w.Bind("cqReadAccent", func() string { return accentHex() })
	w.Init(`
		window.cqApplyAccent = function(hex){
			var r = document.documentElement.style;
			if (hex) r.setProperty('--accent', hex); else r.removeProperty('--accent');
		};
		window.addEventListener('DOMContentLoaded', async function(){
			try { window.cqApplyAccent(await window.cqReadAccent()); } catch (e) {}
		});
	`)

	// Live updates: re-apply the accent whenever a system notification fires (the
	// app runs in the GUI session, so the distributed-notification observer
	// actually fires here). We re-read a few times over ~1.2s because a change to
	// the accent color reaches AppKit instantly but the underlying preference can
	// lag briefly — the retries make the update land without a second event.
	startAccentObserver(func() { applyAccent(w) })

	w.Navigate(dashboardURL)
	w.Run()
}

// applyAccent pushes the current accent color into the page, re-reading a few
// times so a brief preference-flush lag can't leave the old color in place.
func applyAccent(w webview.WebView) {
	for _, d := range []time.Duration{0, 150 * time.Millisecond, 500 * time.Millisecond, 1200 * time.Millisecond} {
		d := d
		go func() {
			if d > 0 {
				time.Sleep(d)
			}
			hex := accentHex()
			w.Dispatch(func() {
				w.Eval("window.cqApplyAccent && window.cqApplyAccent(" + strconv.Quote(hex) + ")")
			})
		}()
	}
}

// ensureDaemon starts claudeqd if the dashboard isn't already responding.
func ensureDaemon() {
	if daemonUp() {
		return
	}
	bin := "claudeqd"
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), "claudeqd"); fileExists(cand) {
			bin = cand
		}
	}
	cmd := exec.Command(bin, "run")
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "claudeqapp: could not start claudeqd:", err)
		return
	}
	// Wait briefly for it to come up.
	for i := 0; i < 50; i++ {
		if daemonUp() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func daemonUp() bool {
	c := http.Client{Timeout: 400 * time.Millisecond}
	resp, err := c.Get(dashboardURL + "/api/tasks")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
