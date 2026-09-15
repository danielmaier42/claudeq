package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DetectBinary finds the opencode CLI. It returns an absolute path, or "" if
// it cannot be located.
//
// It exists for the same reason the Claude Code and Codex ones do: claudeq's
// daemon and app run under launchd with a minimal PATH that excludes the usual
// CLI install locations, so a plain `opencode` lookup fails even when the CLI
// works fine in the operator's terminal. The common locations are checked
// directly, then the user's login shell is asked (which sources their profile
// and rebuilds the real PATH).
func DetectBinary() string {
	// An explicit override wins — used by CI/tests and by an installation in
	// an unusual place.
	if p := os.Getenv("CLAUDEQ_OPENCODE_BIN"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		"/opt/homebrew/bin/opencode",
		"/usr/local/bin/opencode",
		filepath.Join(home, ".local", "bin", "opencode"),
		filepath.Join(home, "bin", "opencode"),
	} {
		if isExecutableFile(c) {
			return c
		}
	}
	if p := viaLoginShell(); p != "" {
		return p
	}
	if p, err := exec.LookPath(BinaryName); err == nil {
		return p
	}
	return ""
}

// loginShellTimeout bounds the profile-sourcing probe. A shell profile is
// arbitrary code; one that blocks must not hold up whoever asked where the CLI
// is.
const loginShellTimeout = 5 * time.Second

// viaLoginShell asks the user's login+interactive shell to resolve `opencode`,
// so PATH additions in their profile are honoured.
func viaLoginShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginShellTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-ilc", "command -v "+BinaryName)
	// Without this, a grandchild holding the output pipe open would keep Output
	// waiting even after the shell itself was killed.
	cmd.WaitDelay = time.Second
	// The output is scanned even when the wait failed: a slow profile can trip
	// the timeout after the shell already printed the answer, and throwing that
	// away would leave the daemon believing the CLI is not installed.
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		p := strings.TrimSpace(lines[i])
		if filepath.IsAbs(p) && isExecutableFile(p) {
			return p
		}
	}
	return ""
}

func isExecutableFile(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode()&0o111 != 0
}
