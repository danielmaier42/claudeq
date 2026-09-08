//go:build !unix

package api

import "os/exec"

// detach is a no-op on platforms without sessions; claudeq targets macOS.
func detach(_ *exec.Cmd) {}
