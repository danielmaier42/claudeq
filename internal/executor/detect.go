package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectBinary finds the Claude Code CLI. It returns an absolute path, or ""
// if it cannot be located.
//
// This exists because claudeq's daemon and app run under launchd with a minimal
// PATH (/usr/bin:/bin:…) that excludes the usual CLI install locations like
// ~/.local/bin — so a plain `claude` lookup fails even when the CLI is
// installed and works from the user's terminal. We therefore check the common
// install locations directly, then fall back to the user's login shell (which
// sources their profile and reconstructs the real PATH).
func DetectBinary() string {
	// An explicit env override wins — used by CI/tests and power users whose
	// claude lives somewhere unusual.
	if p := os.Getenv("CLAUDEQ_CLAUDE_BIN"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		filepath.Join(home, ".local", "bin", "claude"),
		filepath.Join(home, ".claude", "local", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
		filepath.Join(home, "bin", "claude"),
	} {
		if isExecutableFile(c) {
			return c
		}
	}
	if p := viaLoginShell(); p != "" {
		return p
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	return ""
}

// viaLoginShell asks the user's login+interactive shell to resolve `claude`, so
// PATH additions in their profile (~/.zprofile, ~/.zshrc, …) are honoured.
func viaLoginShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	// -i -l so both login and interactive profiles are sourced (~/.local/bin is
	// commonly added in ~/.zshrc, which only an interactive shell reads).
	out, err := exec.Command(shell, "-ilc", "command -v claude").Output()
	if err != nil {
		return ""
	}
	// A login/interactive shell may print profile noise; take the last line that
	// is an absolute path to an existing executable.
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
