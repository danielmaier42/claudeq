//go:build unix

package executor

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts the CLI in its own process group and, on context
// cancellation (idle watchdog or shutdown), kills the whole group. Without this
// only the direct child dies while grandchildren keep the output pipe open, so
// the reader never unblocks and the kill effectively does nothing.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid targets the process group (pgid == leader pid).
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
}
