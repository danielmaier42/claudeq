//go:build darwin

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/fileaccess"
)

// checkFileAccess runs once at launch, while the user is present, to catch the
// macOS privacy (TCC) block that would otherwise stall an unattended overnight
// run. If any enabled task's working directory cannot be read, it offers to open
// the Full Disk Access settings pane.
//
// This lives in the app, not the daemon, on purpose: the daemon runs headless,
// so a consent prompt there hangs with no one to answer it (exactly the failure
// this guards against). Here a human can act. We steer the user to Full Disk
// Access rather than the per-folder prompt because FDA is granted to the app
// bundle and so covers the daemon too, and because it never re-prompts.
//
// Best-effort throughout: it never blocks startup and stays silent on any error.
func checkFileAccess() {
	// Only nag from a real install (the app bundle). A bare `go run` dev build or
	// a `claudeqapp` invoked from a checkout should not pop settings dialogs.
	if exe, err := os.Executable(); err != nil || !strings.Contains(exe, ".app/Contents/MacOS/") {
		return
	}

	// Give the main window a moment to appear so the dialog lands in front of it.
	time.Sleep(2 * time.Second)

	dirs := enabledTaskDirs()
	if len(dirs) == 0 {
		return
	}
	if res := fileaccess.Probe(dirs, fileaccess.DefaultProbeTimeout); !res.OK {
		promptForFullDiskAccess(res.BlockedPath)
	}
}

// enabledTaskDirs asks the daemon for the working directories of enabled tasks.
func enabledTaskDirs() []string {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(dashboardURL + "/api/tasks")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var tasks []struct {
		WorkingDir string `json:"working_dir"`
		Enabled    bool   `json:"enabled"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil
	}
	dirs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.Enabled && t.WorkingDir != "" {
			dirs = append(dirs, t.WorkingDir)
		}
	}
	return dirs
}

// promptForFullDiskAccess shows a native dialog naming the blocked folder and
// opens the Full Disk Access settings pane if the user agrees.
func promptForFullDiskAccess(blocked string) {
	msg := "ClaudeQ can't read “" + blocked + "”.\n\n" +
		"macOS is blocking access to your files, which will stall tasks that run " +
		"overnight. Grant ClaudeQ Full Disk Access to fix this — then it can run " +
		"without getting stuck on a permission prompt."
	script := "display dialog " + osaQuote(msg) +
		` with title "ClaudeQ needs file access"` +
		` buttons {"Later", "Open Settings"} default button "Open Settings"` +
		" with icon caution"
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		return // dismissed, "Later", or osascript unavailable — nothing to do
	}
	if strings.Contains(string(out), "Open Settings") {
		_ = exec.Command("open", fileaccess.FullDiskAccessSettingsURL).Start()
	}
}

// osaQuote renders s as an AppleScript double-quoted string literal. It escapes
// control characters (AppleScript interprets \n, \r and \t within a literal), so
// a multi-line message does not break the surrounding `display dialog` script.
func osaQuote(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return "\"" + r.Replace(s) + "\""
}
