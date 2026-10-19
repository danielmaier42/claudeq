package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The app was called ClaudeQ up to 1.x. These are the names it used, kept only
// so an installation from that time moves over to DispatchQ without losing a
// task or a run. Tasks that name ClaudeQ are not rewritten here: the What's new
// dialog has the reviewer propose each change, and the operator picks which
// to apply.
const (
	// LegacyEnvHome is the data directory override ClaudeQ read.
	LegacyEnvHome = "CLAUDEQ_HOME"

	homeDirName       = "dispatchq"
	legacyHomeDirName = "claudeq"
)

// migrateLegacyHome moves ClaudeQ's data directory to home and leaves a
// symlink at the old location. The symlink keeps every absolute path that still
// names the old directory working: the log path of each past run in
// history.jsonl, and scripts outside dispatchq that read the data.
//
// It does nothing when home already exists (moved before, or a fresh
// DispatchQ installation that never had ClaudeQ) or when there is nothing to
// move. Moving is a rename within one directory, so it is atomic: a CLI call
// racing the daemon either does the move or finds it done.
func migrateLegacyHome(legacy, home string) error {
	if _, err := os.Lstat(home); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect data dir: %w", err)
	}
	info, err := os.Lstat(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect ClaudeQ data dir: %w", err)
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.Rename(legacy, home); err != nil {
		if _, statErr := os.Stat(home); statErr == nil {
			return nil // another process moved it first
		}
		return fmt.Errorf("move ClaudeQ data dir to %s: %w", home, err)
	}
	if err := os.Symlink(filepath.Base(home), legacy); err != nil {
		// The move is done and is what matters; something that recreated the
		// old directory in between keeps it, and only the link is missing.
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("link ClaudeQ data dir to %s: %w", home, err)
	}
	return nil
}
