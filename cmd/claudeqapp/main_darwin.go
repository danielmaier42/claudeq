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
	"strings"
	"time"

	webview "github.com/webview/webview_go"

	"github.com/danielmaier42/claudeq/internal/notify"
)

const dashboardURL = "http://127.0.0.1:10765"

// tasksURL is the endpoint the daemon probe asks: cheap, and it only answers 200
// once the daemon is actually serving.
const tasksURL = dashboardURL + "/api/tasks"

// How the app decides whether a daemon is already there. Several attempts with a
// generous timeout, because the cost of a false "no" is a second daemon on the
// same store while the cost of asking again is a few hundred milliseconds at
// startup.
const (
	daemonProbeAttempts = 3
	daemonProbeTimeout  = 2 * time.Second
	daemonProbePause    = 250 * time.Millisecond
	// daemonStartWait bounds how long the app waits for a daemon it just started.
	daemonStartWait = 5 * time.Second
)

// systemSettingsScheme is the URL scheme that opens a System Settings pane.
const systemSettingsScheme = "x-apple.systempreferences:"

func main() {
	runtime.LockOSThread()
	// Claim notification clicks before anything else: webview.New runs the app's
	// launch cycle, and a click that launched ClaudeQ is only delivered to a
	// delegate that was already in place by then.
	installNotifyDelegate()
	ensureDaemon()

	// Opening the window is a reliable "user is present" moment, so ask the daemon
	// to (re)provoke the macOS file-access prompt for the current task folders now
	// — the daemon is a persistent LaunchAgent that doesn't restart when the window
	// is reopened, so its own startup warm-up wouldn't fire here. Best-effort.
	go requestWarm()

	w := webview.New(false)
	defer w.Destroy()

	// Ask for notification permission from here, not from the daemon: this is the
	// foreground app, so macOS can actually put the prompt on screen. A request
	// from the background LaunchAgent errors out and leaves the app unauthorized,
	// which silently swallows every notification it posts.
	notify.RequestMacAuthorization()
	w.SetTitle("ClaudeQ")
	w.SetSize(1120, 760, webview.HintNone)
	installOpenPanel(w.Window())

	// Native menu bar (webview_go creates none). Custom items drive the dashboard
	// via the same JS the sidebar uses: openAdd() and select('settings').
	installMenu(
		func() { w.Dispatch(func() { w.Eval("window.openAdd && window.openAdd()") }) },
		func() { w.Dispatch(func() { w.Eval("window.select && window.select('settings')") }) },
	)

	// Open external (http/https) links in the default browser — WKWebView won't
	// open target=_blank links on its own. The System Settings scheme is allowed
	// too, so the Settings view can send the operator straight to the macOS
	// notification preferences.
	_ = w.Bind("cqOpenExternal", func(u string) {
		if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") ||
			strings.HasPrefix(u, systemSettingsScheme) {
			_ = exec.Command("open", u).Start()
		}
	})

	// Clicking a "new artifact" notification opens that artifact in the window.
	// The id is fetched by the page (not pushed) so that a click during launch —
	// before the page exists — is picked up as soon as it has loaded.
	_ = w.Bind("cqTakePendingArtifact", takePendingArtifact)
	onNotificationClick(func() {
		w.Dispatch(func() { w.Eval("window.cqOpenPendingArtifact && window.cqOpenPendingArtifact()") })
	})

	// Expose the current accent to the page and (re)apply it on each load.
	_ = w.Bind("cqReadAccent", func() string { return accentHex() })
	w.Init(`
		window.cqApplyAccent = function(hex){
			var r = document.documentElement.style;
			if (hex) r.setProperty('--accent', hex); else r.removeProperty('--accent');
		};
		window.cqOpenPendingArtifact = async function(){
			try {
				var id = await window.cqTakePendingArtifact();
				if (id && window.cqOpenArtifact) window.cqOpenArtifact(id);
			} catch (e) {}
		};
		window.addEventListener('DOMContentLoaded', async function(){
			try { window.cqApplyAccent(await window.cqReadAccent()); } catch (e) {}
			window.cqOpenPendingArtifact();
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

// applyAccent pushes the current accent color into the page, re-reading over a
// few seconds because cfprefsd can serve the old AppleAccentColor for 1-3s after
// a change — the spread of re-reads makes the new color land without needing a
// second event (like a dark/light toggle).
var accentRetries = []time.Duration{
	0,
	200 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
	1800 * time.Millisecond,
	2800 * time.Millisecond,
}

func applyAccent(w webview.WebView) {
	for _, d := range accentRetries {
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
	if daemonReachable() {
		return
	}
	bin := "claudeqd"
	if exe, err := os.Executable(); err == nil {
		if cand := filepath.Join(filepath.Dir(exe), "claudeqd"); fileExists(cand) {
			bin = cand
		}
	}
	cmd := exec.Command(bin, "run")
	// Its own words matter: a daemon that refuses because another one already owns
	// the store says so on stderr, and that line is the whole explanation.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "claudeqapp: could not start claudeqd:", err)
		return
	}
	// Reap it: it may exit right away (another daemon owns the store) and must not
	// be left as a zombie for the lifetime of the window.
	go func() { _ = cmd.Wait() }()
	// Wait briefly for it to come up. Short probes, because all this waits for is
	// the port to open — a long per-probe timeout here would keep the window from
	// opening at all.
	deadline := time.Now().Add(daemonStartWait)
	for {
		if answersOKWithin(tasksURL, 300*time.Millisecond) {
			return
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "claudeqapp: claudeqd did not come up; the window will be empty")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// requestWarm asks the daemon to provoke the macOS file-access prompt for the
// current tasks' folders. Best-effort: any failure is silently ignored.
func requestWarm() {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Post(dashboardURL+"/api/fs/warm", "application/json", nil)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// daemonReachable reports whether a daemon already serves the dashboard. It asks
// several times: a single 400 ms probe was too tight, and a daemon busy with a
// tick (a provider check, a run starting) that missed it had the app start a
// second daemon against the same store. Nothing is lost on the answer "no" — the
// port is then refused right away rather than timing out, so an actually absent
// daemon is still detected in milliseconds.
func daemonReachable() bool { return reachable(tasksURL, daemonProbeAttempts) }

// reachable polls url until it answers 200 or the attempts run out.
func reachable(url string, attempts int) bool {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(daemonProbePause)
		}
		if answersOK(url) {
			return true
		}
	}
	return false
}

func answersOK(url string) bool { return answersOKWithin(url, daemonProbeTimeout) }

func answersOKWithin(url string, timeout time.Duration) bool {
	c := http.Client{Timeout: timeout}
	resp, err := c.Get(url)
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

// accentHex maps the current macOS accent index to a hex color, or "" for the
// default/unset accent (multicolor), which leaves the dashboard on its default.
func accentHex() string {
	return macAccentHex[readAccentIndex()]
}

var macAccentHex = map[int]string{
	-1: "#8e8e93", // graphite
	0:  "#ff5257", // red
	1:  "#f7821b", // orange
	2:  "#ffc600", // yellow
	3:  "#62ba46", // green
	4:  "#007aff", // blue
	5:  "#8944ab", // purple
	6:  "#f74f9e", // pink
}
