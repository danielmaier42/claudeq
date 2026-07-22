package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// stub is a controllable Runner.
type stub struct {
	mu        sync.Mutex
	reqs      []executor.Request
	active    int
	maxActive int
	block     chan struct{} // if non-nil, Run waits on it before returning
	result    func(executor.Request, int) executor.Result
	calls     int
}

func (s *stub) Run(_ context.Context, req executor.Request) (executor.Result, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	s.reqs = append(s.reqs, req)
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.mu.Unlock()

	if s.block != nil {
		<-s.block
	}

	s.mu.Lock()
	s.active--
	s.mu.Unlock()

	if s.result != nil {
		return s.result(req, n), nil
	}
	return executor.Result{Status: store.StatusSuccess, SessionID: req.SessionID}, nil
}

func (s *stub) requests() []executor.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]executor.Request(nil), s.reqs...)
}

func newTestEngine(t *testing.T, r Runner, fc clock.Clock) (*Engine, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	e := New(st, limit.New(fc), r, fc)

	var runN, sessN int
	e.newRunID = func() string { runN++; return fmt.Sprintf("run-%d", runN) }
	e.newSessionID = func() string { sessN++; return fmt.Sprintf("sess-%d", sessN) }
	return e, st
}

func asapTask(id string, parallel bool) task.Task {
	return task.Task{
		ID: id, Name: id, Prompt: "do " + id, WorkingDir: "/repo",
		Trigger: task.TriggerASAP, Enabled: true,
		Permissions: task.PermissionsDefault, Parallel: parallel,
	}
}

func saveTasks(t *testing.T, st *store.Store, tasks ...task.Task) {
	t.Helper()
	if err := st.SaveConfig(store.Config{Tasks: tasks}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestTickRunsAsapTaskOnce(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(reqs))
	}
	if reqs[0].Resume {
		t.Fatal("first run should not be a resume")
	}

	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Status != store.StatusSuccess {
		t.Fatalf("expected 1 successful run, got %+v", runs)
	}

	state, _ := st.LoadState()
	if !state.IsCompletedOnce("a") {
		t.Fatal("asap task should be marked completed after running")
	}

	// A second tick must not re-run a completed one-shot task.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	e.WaitIdle()
	if got := len(r.requests()); got != 1 {
		t.Fatalf("completed task re-ran: %d total runs", got)
	}

	// A finished one-shot task leaves the queue but stays in history with a
	// replayable snapshot.
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Fatalf("finished one-shot task should be removed from the queue, still have %d", len(cfg.Tasks))
	}
	hist, _ := st.Runs()
	if len(hist) != 1 || hist[0].Task == nil || hist[0].Task.Prompt == "" {
		t.Fatalf("run should carry a replayable task snapshot: %+v", hist)
	}
}

func TestRateLimitBlocksUntilReportedReset(t *testing.T) {
	// A five-hour session-limit hit reports an absolute reset time but no retry
	// delay. The gate must block until that reset (plus the safety buffer), not
	// just the 15-minute default backoff.
	start := time.Date(2026, 7, 17, 14, 40, 0, 0, time.UTC)
	reset := start.Add(5 * time.Hour)
	fc := clock.NewFake(start)
	r := &stub{result: func(req executor.Request, call int) executor.Result {
		if call == 1 {
			return executor.Result{Status: store.StatusRateLimited, SessionID: req.SessionID, ResetAt: reset}
		}
		return executor.Result{Status: store.StatusSuccess, SessionID: req.SessionID}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	e.WaitIdle()

	want := reset.Add(RateLimitResetBuffer)
	if got := e.gate.BlockedUntil(); !got.Equal(want) {
		t.Fatalf("gate blocked until %v, want reset + buffer = %v", got, want)
	}

	// Well past the default backoff but before the reset: still blocked.
	fc.Advance(time.Hour)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	e.WaitIdle()
	if len(r.requests()) != 1 {
		t.Fatalf("task restarted before the reported reset: %d runs", len(r.requests()))
	}

	// Past reset + buffer: the session resumes.
	fc.Advance(4*time.Hour + RateLimitResetBuffer)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 3: %v", err)
	}
	e.WaitIdle()
	reqs := r.requests()
	if len(reqs) != 2 || !reqs[1].Resume {
		t.Fatalf("expected a resume after the reset, got %d runs (resume=%v)", len(reqs), len(reqs) == 2 && reqs[1].Resume)
	}
}

func TestRateLimitStaleResetFallsBackToBackoff(t *testing.T) {
	// A reset time already in the past (stale event, clock skew) must not open
	// the gate immediately — fall back to the default backoff.
	start := time.Date(2026, 7, 17, 14, 40, 0, 0, time.UTC)
	fc := clock.NewFake(start)
	r := &stub{result: func(req executor.Request, call int) executor.Result {
		return executor.Result{Status: store.StatusRateLimited, SessionID: req.SessionID, ResetAt: start.Add(-time.Minute)}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	if got, want := e.gate.BlockedUntil(), start.Add(DefaultRateLimitBackoff); !got.Equal(want) {
		t.Fatalf("gate blocked until %v, want default backoff %v", got, want)
	}
}

func TestRateLimitThenResumeAfterReset(t *testing.T) {
	start := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	fc := clock.NewFake(start)
	r := &stub{result: func(req executor.Request, call int) executor.Result {
		if call == 1 {
			return executor.Result{Status: store.StatusRateLimited, SessionID: req.SessionID, RetryAfter: time.Hour}
		}
		return executor.Result{Status: store.StatusSuccess, SessionID: req.SessionID}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	// Tick 1: hits the rate limit -> gate closes, resume pending.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	e.WaitIdle()

	if e.gate.Open() {
		t.Fatal("gate should be closed after a rate limit")
	}
	state, _ := st.LoadState()
	if state.PendingResume("a") == "" {
		t.Fatal("expected a pending resume session")
	}
	if state.IsCompletedOnce("a") {
		t.Fatal("rate-limited task must not be marked completed")
	}

	// While the gate is closed, nothing starts.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	e.WaitIdle()
	if len(r.requests()) != 1 {
		t.Fatalf("task started again while gate closed: %d runs", len(r.requests()))
	}

	// After the reset, the task resumes its session.
	fc.Advance(time.Hour)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 3: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(reqs))
	}
	if !reqs[1].Resume {
		t.Fatal("second run should be a resume")
	}
	if reqs[1].SessionID != reqs[0].SessionID {
		t.Fatalf("resume used session %q, want original %q", reqs[1].SessionID, reqs[0].SessionID)
	}
	state, _ = st.LoadState()
	if !state.IsCompletedOnce("a") {
		t.Fatal("task should be completed after a successful resume")
	}
}

func TestParallelTasksRunConcurrently(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{block: make(chan struct{})}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("p1", true), asapTask("p2", true))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.active == 2
	})
	close(r.block)
	e.WaitIdle()

	if r.maxActive != 2 {
		t.Fatalf("expected 2 concurrent runs, got max %d", r.maxActive)
	}
}

func TestGracefulShutdownLetsRunFinish(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{block: make(chan struct{})}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	ctx, cancel := context.WithCancel(context.Background())
	loopDone := make(chan error, 1)
	go func() { loopDone <- e.Loop(ctx, 10*time.Millisecond) }()

	waitFor(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.active == 1 })
	cancel()                          // stop the daemon while the run is in flight
	time.Sleep(50 * time.Millisecond) // let Loop enter drain
	close(r.block)                    // the run finishes within the grace window

	select {
	case <-loopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop did not return after shutdown")
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Status != store.StatusSuccess {
		t.Fatalf("in-flight run should finish successfully on graceful shutdown, got %+v", runs)
	}
}

// ctxStub blocks each run until its context is cancelled, then reports the
// interrupted result the real executor produces for a killed process.
type ctxStub struct {
	mu      sync.Mutex
	started int
}

func (s *ctxStub) Run(ctx context.Context, req executor.Request) (executor.Result, error) {
	s.mu.Lock()
	s.started++
	s.mu.Unlock()
	<-ctx.Done()
	return executor.Result{
		Status: store.StatusFailed, SessionID: req.SessionID, ExitCode: -1,
		Message: "run was interrupted before completing (the process was terminated — e.g. the daemon stopped)",
	}, nil
}

func (s *ctxStub) activeStarted() int { s.mu.Lock(); defer s.mu.Unlock(); return s.started }

func TestCancelRunMarksRunCanceled(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &ctxStub{}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	waitFor(t, func() bool { return r.activeStarted() == 1 })

	if err := e.CancelRun("run-1"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	e.WaitIdle()

	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Status != store.StatusCanceled {
		t.Fatalf("expected 1 canceled run, got %+v", runs)
	}
	if runs[0].Error != "stopped by the user" {
		t.Fatalf("unexpected error text: %q", runs[0].Error)
	}
	// A canceled one-shot task leaves the queue like any other finished run.
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Fatalf("canceled one-shot task should leave the queue, still have %d", len(cfg.Tasks))
	}
	// Cancelling again (run already finished) reports an error.
	if err := e.CancelRun("run-1"); err == nil {
		t.Fatal("CancelRun on a finished run should error")
	}
}

func TestCancelRunUnknownID(t *testing.T) {
	fc := clock.NewFake(time.Now())
	e, _ := newTestEngine(t, &stub{}, fc)
	if err := e.CancelRun("nope"); err == nil {
		t.Fatal("expected error for unknown run id")
	}
}

func TestCancelRunLeavesOtherRunsAlive(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &ctxStub{}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("p1", true), asapTask("p2", true))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	waitFor(t, func() bool { return r.activeStarted() == 2 })

	// Cancel only the first run; the second keeps running.
	if err := e.CancelRun("run-1"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	waitFor(t, func() bool {
		runs, _ := st.Runs()
		for _, rn := range runs {
			if rn.RunID == "run-1" && rn.Status == store.StatusCanceled {
				return true
			}
		}
		return false
	})
	runs, _ := st.Runs()
	for _, rn := range runs {
		if rn.RunID == "run-2" && rn.Status != store.StatusRunning {
			t.Fatalf("run-2 should still be running, got %s", rn.Status)
		}
	}

	// Shut the second run down via its own cancel to end the test cleanly.
	if err := e.CancelRun("run-2"); err != nil {
		t.Fatalf("CancelRun run-2: %v", err)
	}
	e.WaitIdle()
}

func TestExclusiveTaskRunsAlone(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{block: make(chan struct{})}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("e1", false), asapTask("e2", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.active == 1
	})

	// A second tick while the exclusive task runs must not start the other.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	r.mu.Lock()
	active := r.active
	r.mu.Unlock()
	if active != 1 {
		t.Fatalf("exclusive task did not run alone: %d active", active)
	}

	close(r.block)
	e.WaitIdle()
	if r.maxActive != 1 {
		t.Fatalf("expected max 1 concurrent run, got %d", r.maxActive)
	}
}
