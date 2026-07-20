// Package fileaccess probes whether claudeq can read a task's working directory,
// used to provoke the macOS privacy (TCC) consent prompt at a safe time.
//
// Background: claudeq's daemon runs unattended overnight from a LaunchAgent. The
// first time it touches a protected location (~/Documents, ~/Desktop,
// ~/Downloads, external volumes, …) macOS shows the automatic "allow access?"
// consent prompt and blocks the access until someone answers — which never
// happens at 3am, so the run stalls. The daemon therefore reads its task folders
// at startup instead (install time and every login, while the user is present),
// so the prompt appears when it can be answered; once it is, the decision sticks
// and later runs proceed. The read is bounded so a pending prompt can never
// wedge the caller.
package fileaccess

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultProbeTimeout bounds a single directory check. A read blocked on an
// unanswered TCC consent prompt never returns on its own, so the probe must give
// up and report the path as blocked rather than wedge the caller.
const DefaultProbeTimeout = 3 * time.Second

// Reason describes why a directory could not be read.
type Reason string

const (
	// ReasonPermission means the read was denied (macOS TCC refused access, or
	// ordinary filesystem permissions did).
	ReasonPermission Reason = "permission"
	// ReasonTimeout means the read did not return in time — most likely a TCC
	// consent prompt is pending and blocking the underlying syscall.
	ReasonTimeout Reason = "timeout"
)

// Result reports the outcome of a Probe.
type Result struct {
	// OK is true when every checked directory was readable (or absent).
	OK bool
	// BlockedPath is the first directory that could not be read (empty when OK).
	BlockedPath string
	// Reason explains the block (empty when OK).
	Reason Reason
}

// Probe reports whether each directory in paths is readable. Directories are
// de-duplicated, and a path that does not exist is skipped — a missing folder is
// not a permission problem. It returns on the first path it cannot read; each
// individual read is bounded by perPath (see DefaultProbeTimeout). Use it for a
// pure yes/no readability check; to provoke the consent prompt for every folder
// in one pass, use ProbeAll.
func Probe(paths []string, perPath time.Duration) Result {
	if perPath <= 0 {
		perPath = DefaultProbeTimeout
	}
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if reason, blocked := probeDir(p, perPath); blocked {
			return Result{BlockedPath: p, Reason: reason}
		}
	}
	return Result{OK: true}
}

// ProbeAll reads every directory in paths and returns the ones that were
// blocked, in order. Unlike Probe it does not stop at the first block: when
// warming file access, a not-yet-granted folder reports ReasonTimeout (its
// consent prompt is now on screen), and stopping there would leave the remaining
// folders — which may be in distinct TCC categories (Documents, Downloads,
// Desktop, external volumes…) — never touched, so their prompts would never
// fire. Probing them all in one pass provokes every category's prompt at a time
// the user can answer it. Directories are de-duplicated and missing paths are
// skipped, as in Probe; each read is bounded by perPath. Returns nil (empty)
// when every directory was readable.
func ProbeAll(paths []string, perPath time.Duration) []Result {
	if perPath <= 0 {
		perPath = DefaultProbeTimeout
	}
	seen := make(map[string]bool, len(paths))
	var blocked []Result
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if reason, isBlocked := probeDir(p, perPath); isBlocked {
			blocked = append(blocked, Result{BlockedPath: p, Reason: reason})
		}
	}
	return blocked
}

// ConsentTargets collapses task directories to the paths whose readability
// stands for the same macOS Files & Folders privacy decision, so warming prompts
// once per protected category instead of once per folder.
//
// macOS grants access per category — Desktop, Documents, Downloads — and one
// grant covers every subfolder, so ten tasks under ~/Documents share a single
// prompt. Each such folder is therefore mapped to its category root. Everything
// else maps to itself: external/network volumes auto-prompt too but each has its
// own separate grant, and ordinary folders (~/projects, /tmp, …) are not
// protected and never prompt — probing them directly is a harmless fast read.
//
// Paths are de-duplicated with first appearance preserved. home is the user's
// home directory (from os.UserHomeDir); when empty, no category grouping is
// applied and paths are simply de-duplicated as given.
func ConsentTargets(paths []string, home string) []string {
	roots := categoryRoots(home)
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		target := consentTarget(p, roots)
		if seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	return out
}

// categoryRoots returns the per-category protected folders under home whose grant
// covers their whole subtree. Nil when home is unknown.
func categoryRoots(home string) []string {
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Downloads"),
	}
}

// consentTarget maps path to its category root when it lies within one, else to
// the cleaned path itself. The prefix check is anchored at a path boundary so
// ~/Documents-old is not mistaken for a child of ~/Documents.
func consentTarget(path string, roots []string) string {
	clean := filepath.Clean(path)
	for _, root := range roots {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return root
		}
	}
	return clean
}

// probeDir checks a single directory. It returns (reason, true) when access is
// blocked, or ("", false) when the directory is readable or simply absent.
//
// The read runs in its own goroutine so a syscall stuck behind an unanswered
// consent prompt cannot block us: if it does not return within timeout we report
// ReasonTimeout and move on. The stuck goroutine is left to unwind on its own
// (when the prompt is eventually answered or the process exits); that is an
// acceptable cost for not hanging the caller.
func probeDir(path string, timeout time.Duration) (Reason, bool) {
	ch := make(chan error, 1)
	go func() { ch <- readable(path) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-ch:
		switch {
		case err == nil, errors.Is(err, fs.ErrNotExist):
			return "", false
		case errors.Is(err, fs.ErrPermission):
			return ReasonPermission, true
		default:
			// Some other error (e.g. not a directory): not a privacy block, so we
			// do not flag it — that is not what this probe is here to catch.
			return "", false
		}
	case <-timer.C:
		return ReasonTimeout, true
	}
}

// readable opens path and reads a single entry, which is what trips the macOS
// privacy check. An empty directory (io.EOF) counts as readable.
func readable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
