//go:build !unix

package executor

import "os/exec"

// configureProcessGroup is a no-op on platforms without POSIX process groups.
func configureProcessGroup(_ *exec.Cmd) {}
