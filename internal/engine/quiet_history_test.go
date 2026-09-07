package engine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// runQuietTask runs one quiet-history task to completion with the given
// outcome and returns the store to inspect.
func runQuietTask(t *testing.T, status store.RunStatus) *store.Store {
	t.Helper()
	fc := clock.NewFake(time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC))
	r := &stub{result: func(req executor.Request, _ int) executor.Result {
		return executor.Result{Status: status, SessionID: req.SessionID, ExitCode: 1}
	}}
	e, st := newTestEngine(t, r, fc)
	watcher := asapTask("watch", false)
	watcher.Trigger = task.TriggerCron
	watcher.Cron = "*/15 * * * *"
	watcher.QuietHistory = true
	saveTasks(t, st, watcher)

	if err := e.RunTaskNow(context.Background(), "watch"); err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}
	e.WaitIdle()
	return st
}

func TestQuietTaskSuccessLeavesNoTrace(t *testing.T) {
	st := runQuietTask(t, store.StatusSuccess)

	runs, err := st.Runs()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("a quiet task's success must be dropped from history, got %+v", runs)
	}
	if _, err := os.Stat(st.LogPath("run-1")); !os.IsNotExist(err) {
		t.Fatal("the dropped run's log must be deleted")
	}
	state, _ := st.LoadState()
	if len(state.ReadRuns) != 0 {
		t.Fatalf("no read-status must linger for a dropped run: %v", state.ReadRuns)
	}
}

func TestQuietTaskFailureIsKept(t *testing.T) {
	st := runQuietTask(t, store.StatusFailed)

	runs, err := st.Runs()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != store.StatusFailed {
		t.Fatalf("a quiet task's failure must stay in history, got %+v", runs)
	}
	if !runs[0].Quiet() {
		t.Fatal("the kept run must still carry the quiet flag (it never counts as unread)")
	}
	if _, err := os.Stat(st.LogPath("run-1")); err != nil {
		t.Fatal("the failed run's log must remain for inspection")
	}
}

func TestQuietRunIsRecordedWhileRunning(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{block: make(chan struct{})}
	e, st := newTestEngine(t, r, fc)
	watcher := asapTask("watch", false)
	watcher.QuietHistory = true
	saveTasks(t, st, watcher)

	// Tick (not RunTaskNow, which waits for the run) so the run can be
	// observed while it is still in flight.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	waitFor(t, func() bool { runs, _ := st.Runs(); return len(runs) == 1 })
	runs, _ := st.Runs()
	if runs[0].Status != store.StatusRunning || !runs[0].Quiet() {
		t.Fatalf("running quiet task must show up as running (and quiet): %+v", runs[0])
	}
	close(r.block)
	e.WaitIdle()
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Fatalf("finished quiet run must be gone: %+v", runs)
	}
}
