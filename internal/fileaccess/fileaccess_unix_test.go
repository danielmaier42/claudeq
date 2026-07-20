//go:build unix

package fileaccess

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestProbeTimeout verifies that a read which never returns is reported as a
// timeout rather than hanging the probe. A read-only open of a FIFO with no
// writer blocks indefinitely — the same shape as a syscall stuck behind an
// unanswered macOS consent prompt.
func TestProbeTimeout(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	// Unblock the probe's leaked reader on cleanup by opening the write end, so
	// the goroutine unwinds before the process exits.
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_ = f.Close()
		}
	})

	start := time.Now()
	got := Probe([]string{fifo}, 200*time.Millisecond)
	if got.OK || got.Reason != ReasonTimeout {
		t.Fatalf("want timeout block, got %+v", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("probe took %s, expected it to give up near the 200ms bound", elapsed)
	}
}
