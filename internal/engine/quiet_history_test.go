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
		return executor.Result{Status: status, SessionID: req.SessionID, ExitCode: 1, RetryAfter: time.Minute}
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

func TestQuietTaskLeavesNoTraceUnlessItNeedsAttention(t *testing.T) {
	cases := []struct {
		status store.RunStatus
		kept   bool
	}{
		{store.StatusSuccess, false},
		{store.StatusRateLimited, false}, // resolves itself: the daemon resumes
		{store.StatusFailed, true},
		{store.StatusAuthError, true},
	}
	for _, c := range cases {
		t.Run(string(c.status), func(t *testing.T) {
			st := runQuietTask(t, c.status)
			runs, err := st.Runs()
			if err != nil {
				t.Fatal(err)
			}
			_, logErr := os.Stat(st.LogPath("run-1"))
			if !c.kept {
				if len(runs) != 0 {
					t.Fatalf("a quiet %s must stay out of history, got %+v", c.status, runs)
				}
				if !os.IsNotExist(logErr) {
					t.Fatal("the unrecorded run's log must be deleted")
				}
				return
			}
			if len(runs) != 1 || runs[0].Status != c.status || runs[0].RunID != "run-1" {
				t.Fatalf("a quiet %s must be recorded like any other run, got %+v", c.status, runs)
			}
			if logErr != nil {
				t.Fatal("the kept run's log must remain for inspection")
			}
			if state, _ := st.LoadState(); state.IsRead("run-1") {
				t.Fatal("a kept quiet run is unread like any other")
			}
		})
	}
}

func TestQuietRunIsNotRecordedWhileRunning(t *testing.T) {
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
	waitFor(t, func() bool { return len(r.requests()) == 1 })
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Fatalf("a quiet run must not enter history while running: %+v", runs)
	}
	if ids := e.ActiveTaskIDs(); len(ids) != 1 || ids[0] != "watch" {
		t.Fatalf("the task still counts as running for the queue: %v", ids)
	}
	close(r.block)
	e.WaitIdle()
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Fatalf("finished quiet run must be gone: %+v", runs)
	}
}
