package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockDaemonRefusesASecondDaemon(t *testing.T) {
	st := openTempStore(t)

	first, err := st.LockDaemon(0)
	if err != nil {
		t.Fatalf("first LockDaemon: %v", err)
	}
	defer func() { _ = first.Close() }()

	_, err = st.LockDaemon(50 * time.Millisecond)
	var running *ErrDaemonRunning
	if !errors.As(err, &running) {
		t.Fatalf("second LockDaemon = %v, want ErrDaemonRunning", err)
	}
	if running.PID != os.Getpid() {
		t.Errorf("holder pid = %d, want %d", running.PID, os.Getpid())
	}
	if running.Home != st.Home() {
		t.Errorf("home = %q, want %q", running.Home, st.Home())
	}
}

func TestLockDaemonAfterReleaseAndWait(t *testing.T) {
	st := openTempStore(t)

	first, err := st.LockDaemon(0)
	if err != nil {
		t.Fatalf("first LockDaemon: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Closing twice is what a daemon does when it both defers the release and
	// hits an error path that released already.
	if err := first.Close(); err != nil {
		t.Fatalf("release again: %v", err)
	}

	second, err := st.LockDaemon(0)
	if err != nil {
		t.Fatalf("LockDaemon after release: %v", err)
	}
	defer func() { _ = second.Close() }()

	// A predecessor that lets go inside the wait window is waited for rather
	// than reported as a running daemon — the update hand-over case.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = second.Close()
	}()
	third, err := st.LockDaemon(2 * time.Second)
	if err != nil {
		t.Fatalf("LockDaemon while the predecessor shuts down: %v", err)
	}
	_ = third.Close()
}

// The daemon holds its claim for its whole life, so it must not be the lock the
// CLI takes for a config or state update — that would wedge every claudeq
// command while the daemon runs.
func TestLockDaemonLeavesWritesAlone(t *testing.T) {
	st := openTempStore(t)

	lock, err := st.LockDaemon(0)
	if err != nil {
		t.Fatalf("LockDaemon: %v", err)
	}
	defer func() { _ = lock.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- st.UpdateState(func(s *State) error {
			s.SetNotifiedProviderState("codex", "ready")
			return nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("UpdateState while the daemon lock is held: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("UpdateState blocked on the daemon lock")
	}
}

func TestLockDaemonIsPerDataDirectory(t *testing.T) {
	first := openTempStore(t)
	second := openTempStore(t)

	a, err := first.LockDaemon(0)
	if err != nil {
		t.Fatalf("lock first home: %v", err)
	}
	defer func() { _ = a.Close() }()

	b, err := second.LockDaemon(0)
	if err != nil {
		t.Fatalf("lock second home: %v", err)
	}
	_ = b.Close()
}

func TestReadLockPIDIgnoresGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lock")
	if got := readLockPID(path); got != 0 {
		t.Errorf("missing file = %d, want 0", got)
	}
	if err := os.WriteFile(path, []byte("not a pid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readLockPID(path); got != 0 {
		t.Errorf("garbage = %d, want 0", got)
	}
	if err := os.WriteFile(path, []byte(" 4711 \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readLockPID(path); got != 4711 {
		t.Errorf("pid = %d, want 4711", got)
	}
}

func openTempStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}
