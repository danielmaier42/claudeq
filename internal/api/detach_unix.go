//go:build unix

package api

import (
	"os/exec"
	"syscall"
)

// detach puts the command in its own session, so it outlives this process.
// `claudeqd install` terminates the running daemon — this very process — before
// it re-bootstraps the LaunchAgent, and launchd tears down the rest of a job's
// processes when the job exits. Its own session keeps the hand-over alive long
// enough to finish.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
