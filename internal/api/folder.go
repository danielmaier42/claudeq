package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielmaier42/claudeq/internal/system"
)

// OSAScriptFolderChooser returns a FolderChooser that opens the native macOS
// folder-selection dialog via `osascript`. It works because the daemon runs in
// the user's GUI session. A user cancel is reported as chosen=false, not an error.
func OSAScriptFolderChooser(r system.Runner) FolderChooser {
	return func(ctx context.Context, start string) (string, bool, error) {
		expr := `choose folder with prompt "Select the task's working directory"`
		if start != "" {
			expr += fmt.Sprintf(` default location (POSIX file %q)`, start)
		}
		out, err := r.Run(ctx, "osascript", "-e", "POSIX path of ("+expr+")")
		text := strings.TrimSpace(string(out))
		if err != nil {
			// -128 / "User canceled" is a normal cancel, not a failure.
			if strings.Contains(text, "-128") || strings.Contains(strings.ToLower(text), "cancel") {
				return "", false, nil
			}
			return "", false, fmt.Errorf("osascript choose folder: %w (%s)", err, text)
		}
		if text == "" {
			return "", false, nil
		}
		return text, true, nil
	}
}
