// Package engine ties the store, scheduler, limit gate, and executor into the
// claudeq run loop: on each tick it starts every task that is due and permitted
// by priority/concurrency/limit rules, and records outcomes (PLAN.md §5.2/§7).
package engine

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/schedule"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
	"github.com/danielmaier42/claudeq/internal/wake"
)

// DefaultRateLimitBackoff is used when a rate-limit event does not carry a
// retry delay (PLAN.md V2: an absolute reset time is not always exposed).
const DefaultRateLimitBackoff = 15 * time.Minute

// Runner executes a single request. *executor.Executor satisfies it; tests use
// a stub.
type Runner interface {
	Run(ctx context.Context, req executor.Request) (executor.Result, error)
}

// Waker schedules the machine to wake at a future time. *wake.Scheduler
// satisfies it. It is optional; when nil the engine does not plan wakes.
type Waker interface {
	Schedule(ctx context.Context, at time.Time) error
}

// SetWaker enables wake planning after each loop tick.
func (e *Engine) SetWaker(w Waker) { e.waker = w }

// Engine orchestrates task execution. Construct it with [New].
type Engine struct {
	store *store.Store
	gate  *limit.Gate
	run   Runner
	clock clock.Clock

	newRunID     func() string
	newSessionID func() string
	backoff      time.Duration
	waker        Waker
	lastWakeErr  string

	mu                sync.Mutex
	active            map[string]bool // taskID -> currently running
	nonParallelActive int
	parallelActive    int
	wg                sync.WaitGroup
}

// New builds an Engine with production defaults (real UUIDs and run ids).
func New(st *store.Store, gate *limit.Gate, r Runner, c clock.Clock) *Engine {
	e := &Engine{
		store:   st,
		gate:    gate,
		run:     r,
		clock:   c,
		backoff: DefaultRateLimitBackoff,
		active:  map[string]bool{},
	}
	e.newRunID = func() string {
		return e.clock.Now().UTC().Format("20060102T150405") + "-" + shortHex(4)
	}
	e.newSessionID = uuidV4
	return e
}

// Tick starts every task that is due and permitted right now. Started tasks run
// asynchronously; use [Engine.WaitIdle] to await their completion.
func (e *Engine) Tick(ctx context.Context) error {
	if !e.gate.Open() {
		return nil
	}

	cfg, err := e.store.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	st, err := e.store.LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	now := e.clock.Now()
	if e.seedCronAnchors(cfg, st, now) {
		if err := e.store.SaveState(st); err != nil {
			return fmt.Errorf("seed cron anchors: %w", err)
		}
	}

	due := make([]task.Task, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		anchor, _ := st.LastStart(t.ID)
		in := schedule.Inputs{
			Now:           now,
			Running:       e.active[t.ID],
			CompletedOnce: st.IsCompletedOnce(t.ID),
			CronAnchor:    anchor,
		}
		ok, err := schedule.Due(t, in)
		if err != nil {
			return fmt.Errorf("evaluate task %q: %w", t.ID, err)
		}
		if ok {
			due = append(due, t)
		}
	}

	toStart := schedule.Select(due, e.runningState())
	for _, t := range toStart {
		if err := e.startLocked(ctx, t, cfg.Settings, st); err != nil {
			return err
		}
	}
	return e.store.SaveState(st)
}

// seedCronAnchors gives every not-yet-seen cron task an anchor of now, so its
// first run is the next occurrence after it was added (not a backfill).
func (e *Engine) seedCronAnchors(cfg store.Config, st *store.State, now time.Time) bool {
	changed := false
	for _, t := range cfg.Tasks {
		if t.Trigger != task.TriggerCron {
			continue
		}
		if _, ok := st.LastStart(t.ID); !ok {
			st.RecordStart(t.ID, now)
			changed = true
		}
	}
	return changed
}

func (e *Engine) runningState() schedule.Running {
	return schedule.Running{
		NonParallel: e.nonParallelActive > 0,
		Parallel:    e.parallelActive > 0,
	}
}

// startLocked launches a task. The caller must hold e.mu. State mutations
// (RecordStart) are applied to st and persisted by the caller.
func (e *Engine) startLocked(ctx context.Context, t task.Task, settings store.Settings, st *store.State) error {
	runID := e.newRunID()
	sessionID, resume := e.sessionFor(t, st)
	started := e.clock.Now()

	logFile, err := os.Create(e.store.LogPath(runID))
	if err != nil {
		return fmt.Errorf("create log for run %s: %w", runID, err)
	}

	rec := store.Run{
		RunID: runID, TaskID: t.ID, TaskName: t.Name,
		StartedAt: started, Status: store.StatusRunning,
		SessionID: sessionID, LogPath: e.store.LogPath(runID),
	}
	// Record the start before marking the task active, so a failure here leaves
	// no task stuck in the running set (which would block the scheduler).
	if err := e.store.AppendRun(rec); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("record run start: %w", err)
	}

	st.RecordStart(t.ID, started)
	e.active[t.ID] = true
	if t.Parallel {
		e.parallelActive++
	} else {
		e.nonParallelActive++
	}

	req := executor.Request{
		Task:            t,
		SessionID:       sessionID,
		Resume:          resume,
		Model:           effectiveModel(t, settings),
		SkipPermissions: skipPermissions(t, settings),
		Log:             logFile,
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() { _ = logFile.Close() }()
		res, runErr := e.run.Run(ctx, req)
		e.finish(t, rec, res, runErr)
	}()
	return nil
}

// sessionFor returns the session id to use and whether it is a resume. A task
// waiting to resume after a rate limit reuses its pending session id.
func (e *Engine) sessionFor(t task.Task, st *store.State) (string, bool) {
	if sid := st.PendingResume(t.ID); sid != "" {
		return sid, true
	}
	return e.newSessionID(), false
}

// finish records a completed run and updates scheduling state.
func (e *Engine) finish(t task.Task, rec store.Run, res executor.Result, runErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.active, t.ID)
	if t.Parallel {
		e.parallelActive--
	} else {
		e.nonParallelActive--
	}

	st, err := e.store.LoadState()
	if err != nil {
		// Best-effort: without state we can still record the run below.
		st = nil
	}

	finished := e.clock.Now()
	rec.FinishedAt = &finished
	rec.ExitCode = res.ExitCode
	rec.Status = res.Status
	if res.SessionID != "" {
		rec.SessionID = res.SessionID
	}

	if runErr != nil {
		rec.Status = store.StatusFailed
		rec.Error = runErr.Error()
	} else if res.Message != "" {
		rec.Error = res.Message
	}

	switch rec.Status {
	case store.StatusRateLimited:
		// Wait for reset, then resume this session.
		delay := res.RetryAfter
		if delay <= 0 {
			delay = e.backoff
		}
		e.gate.BlockFor(delay)
		if st != nil {
			st.SetPendingResume(t.ID, res.SessionID)
		}
	default:
		// Terminal outcome: clear any pending resume; one-shot tasks are done.
		if st != nil {
			st.ClearPendingResume(t.ID)
			if t.Trigger == task.TriggerASAP || t.Trigger == task.TriggerFixed {
				st.MarkCompletedOnce(t.ID)
			}
		}
	}

	_ = e.store.AppendRun(rec)
	if st != nil {
		_ = e.store.SaveState(st)
	}
}

// WaitIdle blocks until all in-flight runs have completed.
func (e *Engine) WaitIdle() { e.wg.Wait() }

// Loop runs Tick repeatedly until ctx is cancelled, then waits for in-flight
// runs to finish. Ticks no-op while the limit gate is closed. After each tick it
// plans the next wake (if a Waker is set), so the machine can sleep between runs
// and be woken when the next task is due (PLAN.md D8).
func (e *Engine) Loop(ctx context.Context, interval time.Duration) error {
	for {
		if err := e.Tick(ctx); err != nil {
			e.WaitIdle()
			return err
		}
		if e.waker != nil {
			// Wake scheduling is best-effort (needs root); never fatal. Log a
			// given failure only once to avoid spamming on every tick.
			if err := e.planWake(ctx); err != nil {
				if msg := err.Error(); msg != e.lastWakeErr {
					fmt.Fprintln(os.Stderr, "claudeqd: wake scheduling failed:", err)
					e.lastWakeErr = msg
				}
			} else {
				e.lastWakeErr = ""
			}
		}
		select {
		case <-ctx.Done():
			e.WaitIdle()
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// planWake computes the next relevant wake time and registers it via the Waker.
func (e *Engine) planWake(ctx context.Context) error {
	cfg, err := e.store.LoadConfig()
	if err != nil {
		return err
	}
	st, err := e.store.LoadState()
	if err != nil {
		return err
	}
	now := e.clock.Now()
	cands := e.wakeCandidates(cfg, st, now)
	if bu := e.gate.BlockedUntil(); !bu.IsZero() {
		cands = append(cands, bu)
	}
	at, ok := wake.NextWakeTime(now, cands, cfg.Settings.HeartbeatOrDefault())
	if !ok {
		return nil
	}
	return e.waker.Schedule(ctx, at)
}

// wakeCandidates returns concrete future times at which pending tasks want to
// run: future fixed starts and the next cron occurrences.
func (e *Engine) wakeCandidates(cfg store.Config, st *store.State, now time.Time) []time.Time {
	var out []time.Time
	for _, t := range cfg.Tasks {
		if !t.Enabled {
			continue
		}
		switch t.Trigger {
		case task.TriggerFixed:
			if !st.IsCompletedOnce(t.ID) && t.FixedAt.After(now) {
				out = append(out, t.FixedAt)
			}
		case task.TriggerCron:
			if sched, err := t.CronSchedule(); err == nil {
				anchor, ok := st.LastStart(t.ID)
				if !ok {
					anchor = now
				}
				out = append(out, sched.Next(anchor))
			}
		}
	}
	return out
}

// RunTaskNow runs a specific task once, synchronously, ignoring its trigger and
// completion state — the manual "run now" test trigger (FA-16). It still
// records history and honours resume-after-limit for that run.
func (e *Engine) RunTaskNow(ctx context.Context, taskID string) error {
	cfg, err := e.store.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	var target *task.Task
	for i := range cfg.Tasks {
		if cfg.Tasks[i].ID == taskID {
			target = &cfg.Tasks[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("task %q not found", taskID)
	}

	e.mu.Lock()
	st, err := e.store.LoadState()
	if err != nil {
		e.mu.Unlock()
		return fmt.Errorf("load state: %w", err)
	}
	startErr := e.startLocked(ctx, *target, cfg.Settings, st)
	if startErr == nil {
		startErr = e.store.SaveState(st)
	}
	e.mu.Unlock()

	e.WaitIdle()
	return startErr
}

// effectiveModel resolves the model: a task override wins over the global
// default; empty means Claude Code's own default (FA-28/30).
func effectiveModel(t task.Task, s store.Settings) string {
	if t.Model != task.ModelDefault {
		return t.Model
	}
	return s.DefaultModel
}

// skipPermissions resolves the permission behaviour (FA-29/31).
func skipPermissions(t task.Task, s store.Settings) bool {
	switch t.Permissions {
	case task.PermissionsSkip:
		return true
	case task.PermissionsDefault:
		return s.SkipPermissionsDefault
	default:
		return false
	}
}

func shortHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0000"
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}

func uuidV4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
