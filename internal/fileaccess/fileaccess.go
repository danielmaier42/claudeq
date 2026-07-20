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
// individual read is bounded by perPath (see DefaultProbeTimeout).
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
	case <-time.After(timeout):
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
