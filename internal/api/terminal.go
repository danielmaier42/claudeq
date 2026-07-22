package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielmaier42/claudeq/internal/system"
)

// TerminalOpener opens a new macOS Terminal window that runs argv in dir.
// Injectable so the continue-run endpoint is testable without driving the
// real Terminal.
type TerminalOpener func(ctx context.Context, dir string, argv []string) error

// OSAScriptTerminalOpener returns a TerminalOpener backed by `osascript`
// driving Terminal.app. It works because the daemon runs in the user's GUI
// session (same mechanism as the folder chooser). The shell command reaches
// the AppleScript as a run-handler argument (`on run argv`), so it needs
// shell quoting only — never AppleScript string escaping.
func OSAScriptTerminalOpener(r system.Runner) TerminalOpener {
	return func(ctx context.Context, dir string, argv []string) error {
		parts := []string{"cd", shellQuote(dir), "&&"}
		for _, a := range argv {
			parts = append(parts, shellQuote(a))
		}
		out, err := r.Run(ctx, "osascript",
			"-e", "on run argv",
			"-e", `tell application "Terminal" to do script (item 1 of argv)`,
			"-e", `tell application "Terminal" to activate`,
			"-e", "end run",
			strings.Join(parts, " "))
		if err != nil {
			return fmt.Errorf("osascript open terminal: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}

// shellQuote wraps s in single quotes for the shell Terminal spawns, closing
// and reopening the quote around any embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
