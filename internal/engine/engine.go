// Package engine ties the store, scheduler, limit gate, and executor into the
// claudeq run loop: on each tick it starts every task that is due and permitted
// by priority/concurrency/limit rules, and records outcomes (PLAN.md §5.2/§7).
package engine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/notify"
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

// SetNotifier enables outcome notifications (failure / auth error).
func (e *Engine) SetNotifier(n notify.Notifier) { e.notifier = n }

// WakeError reports the last wake-scheduling error, or "" if the most recent
// attempt succeeded (or wake is disabled / not yet attempted). Surfaced in the
// UI so a broken scheduled-wake setup (e.g. missing pmset sudoers entry) is
// visible instead of silently failing.
func (e *Engine) WakeError() string {
	if e.waker == nil {
		return ""
	}
	if p := e.wakeErr.Load(); p != nil {
		return *p
	}
	return ""
}

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
	lastWakeErr  string                 // loop-local, for once-only logging
	wakeErr      atomic.Pointer[string] // exposed to the API (thread-safe)
	notifier     notify.Notifier

	runCtx    context.Context
	runCancel context.CancelFunc

	mu                sync.Mutex
	active            map[string]bool // taskID -> currently running
	nonParallelActive int
	parallelActive    int
	wg                sync.WaitGroup
	awake             sleepGuard // keeps the Mac awake while runs are in flight
}

// ShutdownGrace is how long Loop lets in-flight runs finish on shutdown before
// terminating them, so a normal stop/restart doesn't fail running tasks.
const ShutdownGrace = 30 * time.Second

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
	// Runs use their own context so that cancelling the loop (SIGINT) does not
	// immediately kill in-flight Claude processes; shutdown drains them first.
	e.runCtx, e.runCancel = context.WithCancel(context.Background())
	e.newRunID = func() string {
		return e.clock.Now().UTC().Format("20060102T150405") + "-" + shortHex(4)
	}
	e.newSessionID = uuidV4
	return e
}

// Tick starts every task that is due and permitted right now. Started tasks run
// asynchronously; use [Engine.WaitIdle] to await their completion.
func (e *Engine) Tick(_ context.Context) error {
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
	// Seed the in-memory snapshot so freshly-added cron tasks have an anchor for
	// the due check below (their first run is the next occurrence, not now).
	seeded := e.seedCronAnchors(cfg, st, now)

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

	// Persist scheduling state only when something changed, and via a targeted
	// update so we never clobber read-status set concurrently through the API.
	if seeded || len(toStart) > 0 {
		startIDs := make([]string, len(toStart))
		for i, t := range toStart {
			startIDs[i] = t.ID
		}
		if err := e.store.UpdateState(func(cur *store.State) error {
			e.seedCronAnchors(cfg, cur, now)
			for _, id := range startIDs {
				cur.RecordStart(id, now)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("persist scheduling state: %w", err)
		}
	}

	for _, t := range toStart {
		sessionID, resume := e.sessionFor(t, st)
		if err := e.launchTask(t, cfg.Settings, sessionID, resume, now); err != nil {
			return err
		}
	}
	return nil
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

// launchTask starts a run for t. The caller must hold e.mu and have already
// persisted the RecordStart. sessionID/resume come from the caller's snapshot.
func (e *Engine) launchTask(t task.Task, settings store.Settings, sessionID string, resume bool, started time.Time) error {
	runID := e.newRunID()

	logFile, err := os.Create(e.store.LogPath(runID))
	if err != nil {
		return fmt.Errorf("create log for run %s: %w", runID, err)
	}

	snapshot := t
	rec := store.Run{
		RunID: runID, TaskID: t.ID, TaskName: t.Name,
		StartedAt: started, Status: store.StatusRunning,
		SessionID: sessionID, LogPath: e.store.LogPath(runID),
		Task: &snapshot,
	}
	// Record the start before marking the task active, so a failure here leaves
	// no task stuck in the running set (which would block the scheduler).
	if err := e.store.AppendRun(rec); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("record run start: %w", err)
	}

	e.active[t.ID] = true
	if t.Parallel {
		e.parallelActive++
	} else {
		e.nonParallelActive++
	}
	e.awake.acquire() // hold off idle system sleep until this run finishes

	req := executor.Request{
		Task:            t,
		SessionID:       sessionID,
		Resume:          resume,
		Model:           effectiveModel(t, settings),
		SkipPermissions: skipPermissions(t, settings),
		Bin:             settings.ClaudePath,
		IdleTimeout:     settings.IdleTimeout(),
		Log:             logFile,
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer func() { _ = logFile.Close() }()
		res, runErr := e.runGuarded(req)
		e.finish(t, rec, res, runErr)
	}()
	return nil
}

// runGuarded runs the request and turns a panic into a failed result instead of
// crashing the daemon, so one bad run never takes the whole queue down.
func (e *Engine) runGuarded(req executor.Request) (res executor.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = executor.Result{Status: store.StatusFailed, Message: fmt.Sprintf("internal error: %v", r)}
			err = nil
		}
	}()
	return e.run.Run(e.runCtx, req)
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
	delete(e.active, t.ID)
	if t.Parallel {
		e.parallelActive--
	} else {
		e.nonParallelActive--
	}
	e.mu.Unlock()
	e.awake.release()

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
	if m := res.Metrics; m != nil {
		rec.CostUSD = m.CostUSD
		rec.InputTokens = m.InputTokens
		rec.OutputTokens = m.OutputTokens
		rec.NumTurns = m.NumTurns
		rec.DurationMS = m.DurationMS
	}

	// Targeted state update: touch only this task's keys so a concurrent API
	// read-status change is preserved.
	switch rec.Status {
	case store.StatusRateLimited:
		delay := res.RetryAfter
		if delay <= 0 {
			delay = e.backoff
		}
		e.gate.BlockFor(delay) // wait for reset, then resume this session
		_ = e.store.UpdateState(func(st *store.State) error {
			st.SetPendingResume(t.ID, res.SessionID)
			return nil
		})
	default:
		oneShot := t.Trigger == task.TriggerASAP || t.Trigger == task.TriggerFixed
		_ = e.store.UpdateState(func(st *store.State) error {
			st.ClearPendingResume(t.ID)
			if oneShot {
				st.MarkCompletedOnce(t.ID)
			}
			return nil
		})
		// A finished one-shot task leaves the queue; it stays in history and can
		// be replayed from there. Recurring (cron) tasks remain.
		if oneShot {
			_ = e.store.UpdateConfig(func(cfg *store.Config) error {
				for i := range cfg.Tasks {
					if cfg.Tasks[i].ID == t.ID {
						cfg.Tasks = append(cfg.Tasks[:i], cfg.Tasks[i+1:]...)
						break
					}
				}
				return nil
			})
		}
	}

	_ = e.store.AppendRun(rec)

	// Bound disk usage: prune old runs/logs beyond the configured limit.
	if cfg, err := e.store.LoadConfig(); err == nil {
		_ = e.store.PruneHistory(cfg.Settings.RunHistoryLimit())
	}

	// Record the final status/reason into the log so it shows in both the raw
	// and chat views (especially useful for failures and interruptions).
	if rec.Status != store.StatusSuccess {
		reason := rec.Error
		if reason == "" {
			reason = string(rec.Status)
		}
		if line, err := json.Marshal(map[string]string{
			"type": "claudeq_status", "status": string(rec.Status), "message": reason,
		}); err == nil {
			_ = e.store.AppendRunLog(rec.RunID, append(line, '\n'))
		}
	}

	// Notify outside any lock so channel I/O never blocks other finishing runs.
	e.notifyOutcome(t, rec, res.ResultText)
}

// notifyOutcome sends a best-effort notification. Failures and auth problems
// always notify (FA-35/FA-38); successes notify only when the task opted in via
// NotifyOnResult. When opted in, the last result message is included.
func (e *Engine) notifyOutcome(t task.Task, rec store.Run, resultText string) {
	if e.notifier == nil {
		return
	}
	msg := strings.TrimSpace(resultText)
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}

	var n notify.Notification
	switch rec.Status {
	case store.StatusSuccess:
		if !t.NotifyOnResult {
			return
		}
		n.Title = "ClaudeQ ✓ " + rec.TaskName
		n.Message = msg
		if n.Message == "" {
			n.Message = "Completed successfully."
		}
	case store.StatusFailed:
		n.Title = "ClaudeQ ✗ " + rec.TaskName + " failed"
		if t.NotifyOnResult && msg != "" {
			n.Message = msg
		} else if rec.Error != "" {
			n.Message = rec.Error
		} else {
			n.Message = "Task failed."
		}
	case store.StatusAuthError:
		n.Title = "ClaudeQ: login problem"
		n.Message = rec.TaskName + ": Claude Code authentication problem — please re-login."
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = e.notifier.Notify(ctx, n)
}

// WaitIdle blocks until all in-flight runs have completed.
func (e *Engine) WaitIdle() { e.wg.Wait() }

// ActiveTaskIDs returns the ids of tasks currently running.
func (e *Engine) ActiveTaskIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids := make([]string, 0, len(e.active))
	for id := range e.active {
		ids = append(ids, id)
	}
	return ids
}

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
				msg := err.Error()
				if msg != e.lastWakeErr {
					fmt.Fprintln(os.Stderr, "claudeqd: wake scheduling failed:", err)
					e.lastWakeErr = msg
				}
				e.wakeErr.Store(&msg)
			} else {
				e.lastWakeErr = ""
				empty := ""
				e.wakeErr.Store(&empty)
			}
		}
		select {
		case <-ctx.Done():
			e.drain()
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// drain lets in-flight runs finish (up to ShutdownGrace) on shutdown, then
// terminates any stragglers — so a normal stop/restart doesn't fail runs.
func (e *Engine) drain() {
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(ShutdownGrace):
		e.runCancel() // terminate remaining runs
		e.wg.Wait()
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
func (e *Engine) RunTaskNow(_ context.Context, taskID string) error {
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
	if e.active[taskID] {
		e.mu.Unlock()
		return fmt.Errorf("task %q is already running", taskID)
	}
	st, err := e.store.LoadState()
	if err != nil {
		e.mu.Unlock()
		return fmt.Errorf("load state: %w", err)
	}
	sessionID, resume := e.sessionFor(*target, st)
	now := e.clock.Now()
	if err := e.store.UpdateState(func(cur *store.State) error {
		cur.RecordStart(taskID, now)
		return nil
	}); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("record run start: %w", err)
	}
	startErr := e.launchTask(*target, cfg.Settings, sessionID, resume, now)
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
