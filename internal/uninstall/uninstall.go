// Package uninstall removes ClaudeQ from this Mac (NFA-06): the app bundle, the
// LaunchAgent, the pmset sudoers entry the installer adds and the installer's
// package receipt, and on request everything ClaudeQ stored as well.
//
// Dragging the app to the Bin is not enough: launchd keeps the agent registered
// and, because it is KeepAlive, keeps trying to start a binary that is gone.
package uninstall

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/danielmaier42/claudeq/internal/launchd"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/system"
	"github.com/danielmaier42/claudeq/internal/update"
)

// BundleID identifies ClaudeQ everywhere macOS keeps track of it: the app
// bundle, the LaunchAgent label, the installer package and the app's own
// folders under ~/Library.
const BundleID = launchd.DefaultLabel

// SudoersPath is the pmset permission the installer's postinstall writes.
const SudoersPath = "/etc/sudoers.d/claudeq"

// ErrCancelled reports that the user dismissed the administrator prompt.
// Nothing has been changed at that point.
var ErrCancelled = errors.New("uninstall cancelled")

// libraryItems are the folders and files macOS creates for the app under
// ~/Library: the WebKit view's storage and cache, and its preferences.
var libraryItems = []string{
	filepath.Join("Caches", BundleID),
	filepath.Join("HTTPStorages", BundleID),
	filepath.Join("HTTPStorages", BundleID+".binarycookies"),
	filepath.Join("WebKit", BundleID),
	filepath.Join("Saved Application State", BundleID+".savedState"),
	filepath.Join("Preferences", BundleID+".plist"),
}

// dataMarkers are files only a data directory ClaudeQ has used contains. A
// purge of a CLAUDEQ_HOME outside the default location needs one of them, so a
// variable pointing somewhere unexpected never loses a file to a name clash.
var dataMarkers = []string{".lock", ".daemon.lock", "history.jsonl"}

// How long a process gets to exit after SIGTERM before it is killed, and after
// SIGKILL before the purge goes ahead anyway. The daemon cancels its runs on
// SIGTERM, which takes a few seconds per run tree.
const (
	appExitWait    = 5 * time.Second
	daemonExitWait = 15 * time.Second
	killWait       = 2 * time.Second
	pollInterval   = 100 * time.Millisecond
)

// Machine is the Mac an uninstall acts on. Every path and command goes through
// it, so tests point it at a temporary directory and a fake runner.
type Machine struct {
	Runner system.Runner
	UID    int
	// Home is the user's home directory; LaunchAgents and the app's Library
	// folders live under it.
	Home string
	// DataDir is ClaudeQ's data directory (store.DefaultHome).
	DataDir string
	// Apps are the places a ClaudeQ.app may be: the bundle the running binary
	// belongs to and /Applications/ClaudeQ.app.
	Apps    []string
	Sudoers string
	Sleep   func(time.Duration)
}

// ThisMac describes the Mac the current process runs on.
func ThisMac() (Machine, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Machine{}, fmt.Errorf("resolve home dir: %w", err)
	}
	data, err := store.DefaultHome()
	if err != nil {
		return Machine{}, err
	}
	var apps []string
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if app := update.EnclosingBundle(exe); app != "" {
			apps = append(apps, app)
		}
	}
	apps = append(apps, update.DefaultAppPath)
	return Machine{
		Runner: system.Real{}, UID: os.Getuid(), Home: home, DataDir: data,
		Apps: apps, Sudoers: SudoersPath, Sleep: time.Sleep,
	}, nil
}

// Plan is what an uninstall removes, worked out before anything changes so it
// can be shown and confirmed.
type Plan struct {
	Apps    []string // ClaudeQ app bundles
	Agent   string   // the LaunchAgent plist, "" if there is none
	Sudoers string   // the pmset sudoers entry, "" if there is none
	Receipt bool     // the installer left a package receipt
	// Data is filled only for a purge: the data directory and the app's
	// folders under ~/Library. For a CLAUDEQ_HOME outside the default
	// location it lists ClaudeQ's own entries in it instead of the directory.
	Data []string
	// SharedDir is such a CLAUDEQ_HOME. It is removed at the end only if
	// nothing but ClaudeQ's entries was in it.
	SharedDir string
	// Admin is set when part of it belongs to root, so macOS has to ask for an
	// administrator password.
	Admin bool
}

// Plan works out what an uninstall would remove. purge adds the data directory
// and the app's Library folders; it fails when the data directory does not look
// like ClaudeQ's.
func (m Machine) Plan(ctx context.Context, purge bool) (Plan, error) {
	var p Plan
	for _, app := range m.bundles() {
		p.Apps = append(p.Apps, app)
		if !removableByUser(app, m.UID) {
			p.Admin = true
		}
	}
	if exists(m.agentPlist()) {
		p.Agent = m.agentPlist()
	}
	if m.Sudoers != "" && exists(m.Sudoers) {
		p.Sudoers = m.Sudoers
		p.Admin = true
	}
	if _, err := m.Runner.Run(ctx, "pkgutil", "--pkg-info", BundleID); err == nil {
		p.Receipt = true
		p.Admin = true
	}
	if !purge {
		return p, nil
	}
	data, shared, err := m.dataEntries()
	if err != nil {
		return Plan{}, err
	}
	p.Data, p.SharedDir = data, shared
	for _, rel := range libraryItems {
		if path := filepath.Join(m.Home, "Library", rel); exists(path) {
			p.Data = append(p.Data, path)
		}
	}
	return p, nil
}

// Run carries out p. The parts that need root go first, in one administrator
// prompt: dismissing it returns ErrCancelled with nothing changed. Then the
// window and the background service are stopped, a user-owned app is deleted,
// and a purge deletes the data last, once no daemon is left to write to it.
//
// A process that runs Run must not belong to the LaunchAgent's job, or the
// bootout ends it halfway: the app's Uninstall button starts it in a session of
// its own.
func (m Machine) Run(ctx context.Context, p Plan) error {
	if p.Admin {
		if err := m.runAsAdmin(ctx, adminScript(p)); err != nil {
			return err
		}
	}

	// The window first: a dashboard left open would only show a daemon that is
	// gone.
	m.stop(ctx, "claudeqapp", appExitWait)
	agent := launchd.Agent{Runner: m.Runner, Dir: m.agentsDir(), Label: BundleID, UID: m.UID}
	if err := agent.Uninstall(ctx); err != nil {
		return err
	}
	// A daemon launchd does not manage (started by the window, or from another
	// copy of the app) would keep running the queue and writing to the data.
	m.stop(ctx, "claudeqd", daemonExitWait)

	if !p.Admin {
		for _, app := range p.Apps {
			if err := os.RemoveAll(app); err != nil {
				return fmt.Errorf("remove %s: %w", app, err)
			}
		}
	}

	if len(p.Data) == 0 {
		return nil
	}
	// Through cfprefsd, which would otherwise keep serving the deleted
	// preferences from its cache.
	_, _ = m.Runner.Run(ctx, "defaults", "delete", BundleID)
	for _, path := range p.Data {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	if p.SharedDir != "" {
		// Fails, on purpose, when something that is not ClaudeQ's is left.
		_ = os.Remove(p.SharedDir)
	}
	return nil
}

// defaultDataDir is where ClaudeQ keeps its data when CLAUDEQ_HOME is unset.
func (m Machine) defaultDataDir() string {
	return filepath.Join(m.Home, "Library", "Application Support", "claudeq")
}

// dataEntries lists what a purge deletes of the data directory. The default
// location is ClaudeQ's own folder and goes as a whole. A CLAUDEQ_HOME
// elsewhere may hold other files too, so only ClaudeQ's entries are listed and
// the directory itself is returned as shared.
func (m Machine) dataEntries() (data []string, shared string, err error) {
	dir := m.DataDir
	if err := checkDataDir(dir, m.Home); err != nil {
		return nil, "", err
	}
	dir = filepath.Clean(dir)
	if !exists(dir) {
		return nil, "", nil
	}
	if dir == m.defaultDataDir() {
		return []string{dir}, "", nil
	}
	if !slices.ContainsFunc(dataMarkers, func(name string) bool { return exists(filepath.Join(dir, name)) }) {
		return nil, "", fmt.Errorf("refusing to delete from %s: it does not look like a ClaudeQ data directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", fmt.Errorf("read data directory: %w", err)
	}
	for _, e := range entries {
		if store.IsEntry(e.Name()) {
			data = append(data, filepath.Join(dir, e.Name()))
		}
	}
	return data, dir, nil
}

func (m Machine) agentsDir() string { return filepath.Join(m.Home, "Library", "LaunchAgents") }

func (m Machine) agentPlist() string { return filepath.Join(m.agentsDir(), BundleID+".plist") }

// bundles returns the ClaudeQ app bundles among m.Apps, each once. A path only
// counts when its Info.plist names ClaudeQ's bundle identifier, so nothing else
// is ever deleted.
func (m Machine) bundles() []string {
	var found []string
	var seen []os.FileInfo
	for _, app := range m.Apps {
		if !filepath.IsAbs(app) || !strings.HasSuffix(app, ".app") {
			continue
		}
		app = filepath.Clean(app)
		fi, err := os.Stat(app)
		if err != nil || !fi.IsDir() || update.BundleIdentifier(app) != BundleID {
			continue
		}
		// os.SameFile, not string equality: the default volume is
		// case-insensitive, so /Applications/claudeq.app is the same bundle.
		dup := false
		for _, s := range seen {
			if os.SameFile(s, fi) {
				dup = true
			}
		}
		if !dup {
			seen = append(seen, fi)
			found = append(found, app)
		}
	}
	return found
}

// removableByUser reports whether the user can delete app without
// administrator rights: they own every file in it, can write to every folder,
// and may write to the folder it sits in. The installer package puts the app
// there as root; one root-owned file would stop a delete halfway.
func removableByUser(app string, uid int) bool {
	const wOK = 0x2
	if syscall.Access(filepath.Dir(app), wOK) != nil {
		return false
	}
	ok := true
	_ = filepath.WalkDir(app, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			ok = false
			return fs.SkipAll
		}
		fi, err := d.Info()
		if err != nil {
			ok = false
			return fs.SkipAll
		}
		st, isStat := fi.Sys().(*syscall.Stat_t)
		if !isStat || int(st.Uid) != uid || (d.IsDir() && fi.Mode().Perm()&0o200 == 0) {
			ok = false
			return fs.SkipAll
		}
		return nil
	})
	return ok
}

// checkDataDir refuses a purge of the root, the home folder or one of its
// parents.
func checkDataDir(dir, home string) error {
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("refusing to delete data directory %q: not an absolute path", dir)
	}
	dir = filepath.Clean(dir)
	home = filepath.Clean(home)
	if dir == "/" || dir == home || strings.HasPrefix(home+"/", dir+"/") {
		return fmt.Errorf("refusing to delete data directory %s: it contains your home folder", dir)
	}
	return nil
}

// adminScript is the shell command run as root: forget the package receipt,
// remove the sudoers entry, delete the app bundles. It stops at the first
// failure, and the apps go last, so a failure never leaves the LaunchAgent
// registered for an app that is already gone.
func adminScript(p Plan) string {
	var cmds []string
	if p.Receipt {
		cmds = append(cmds, "/usr/sbin/pkgutil --forget "+BundleID)
	}
	if p.Sudoers != "" {
		cmds = append(cmds, "/bin/rm -f -- "+shellQuote(p.Sudoers))
	}
	for _, app := range p.Apps {
		cmds = append(cmds, "/bin/rm -rf -- "+shellQuote(app))
	}
	return strings.Join(cmds, " && ")
}

// adminPrompt is the line macOS shows in its password dialog.
const adminPrompt = "ClaudeQ needs your password to remove itself from this Mac."

// runAsAdmin runs script as root behind the standard macOS administrator
// prompt. The script and the prompt travel as arguments, never spliced into
// the AppleScript source, so no path needs escaping for AppleScript.
func (m Machine) runAsAdmin(ctx context.Context, script string) error {
	out, err := m.Runner.Run(ctx, "osascript",
		"-e", "on run argv",
		"-e", "do shell script (item 1 of argv) with prompt (item 2 of argv) with administrator privileges",
		"-e", "end run",
		script, adminPrompt)
	if err != nil {
		text := strings.TrimSpace(string(out))
		if strings.Contains(text, "-128") {
			return ErrCancelled
		}
		return fmt.Errorf("remove as administrator: %w (%s)", err, text)
	}
	return nil
}

// stop ends every process named exactly name: SIGTERM, a wait of up to grace,
// then SIGKILL for whatever is left and a short wait for it to be gone.
func (m Machine) stop(ctx context.Context, name string, grace time.Duration) {
	if !m.running(ctx, name) {
		return
	}
	_, _ = m.Runner.Run(ctx, "pkill", "-x", name)
	if m.gone(ctx, name, grace) {
		return
	}
	_, _ = m.Runner.Run(ctx, "pkill", "-9", "-x", name)
	m.gone(ctx, name, killWait)
}

// gone waits up to d for every process named name to exit.
func (m Machine) gone(ctx context.Context, name string, d time.Duration) bool {
	for waited := time.Duration(0); waited < d; waited += pollInterval {
		if !m.running(ctx, name) {
			return true
		}
		m.Sleep(pollInterval)
	}
	return !m.running(ctx, name)
}

// running reports whether a process named exactly name exists. pgrep exits
// non-zero when none does.
func (m Machine) running(ctx context.Context, name string) bool {
	_, err := m.Runner.Run(ctx, "pgrep", "-x", name)
	return err == nil
}

// Alert shows a macOS alert. It reports the outcome when the uninstall was
// started from the app, which by then is no longer there to show it.
func Alert(ctx context.Context, r system.Runner, title, message string, critical bool) error {
	line := "display alert (item 1 of argv) message (item 2 of argv)"
	if critical {
		line += " as critical"
	}
	if out, err := r.Run(ctx, "osascript", "-e", "on run argv", "-e", line, "-e", "end run", title, message); err != nil {
		return fmt.Errorf("show alert: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// shellQuote wraps s in single quotes for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
