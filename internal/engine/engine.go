// Package engine ties the store, scheduler, limit gate, and executor into the
// claudeq run loop: on each tick it starts every task that is due and permitted
// by priority/concurrency/limit rules, and records outcomes (PLAN.md §5.2/§7).
package engine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/schedule"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
	"github.com/danielmaier42/claudeq/internal/wake"
)

// DefaultRateLimitBackoff is used when a rate-limit event does not carry a
// retry delay (PLAN.md V2: an absolute reset time is not always exposed).
const DefaultRateLimitBackoff = 15 * time.Minute

// RateLimitResetBuffer is added on top of the absolute reset time reported by
// the CLI before resuming, so a run never restarts marginally before the
// window actually reopens (clock skew, server-side rounding).
const RateLimitResetBuffer = time.Minute

// Runner executes a single request. *executor.Executor satisfies it; tests use
// a stub.
type Runner interface {
	Run(ctx context.Context, req executor.Request) (provider.Result, error)
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

// LimitedUntil returns the earliest time a blocked provider's rate limit
// reopens, or the zero time when none is blocked. Surfaced in the UI so a queue
// that is waiting (rather than stuck) says so, and names the time it continues.
func (e *Engine) LimitedUntil() time.Time { return e.gates.BlockedUntil() }

// Engine orchestrates task execution. Construct it with [New].
type Engine struct {
	store *store.Store
	// gates hold the rate limits, one per provider instance: an allowance
	// belongs to an account, so a blocked Codex says nothing about Claude and a
	// second subscription is not held up by the first.
	gates *limit.Gates
	run   Runner
	clock clock.Clock

	newRunID     func() string
	newSessionID func() string
	backoff      time.Duration
	waker        Waker
	lastWakeErr  string // loop-local, for once-only logging
	// lastArtifactErr and lastNotifyErr are loop-local too (see
	// notifyNewArtifacts and deliverTaskNotifications).
	lastArtifactErr string
	lastNotifyErr   string
	wakeErr         atomic.Pointer[string] // exposed to the API (thread-safe)
	notifier        notify.Notifier

	runCtx    context.Context
	runCancel context.CancelFunc

	// providers answers whether a task's harness can run right now. It is
	// required: without it the daemon would start jobs on a CLI that is missing
	// or logged out and record the failure at three in the morning.
	providers *provider.Checker

	mu                sync.Mutex
	active            map[string]bool               // taskID -> currently running
	cancels           map[string]context.CancelFunc // runID -> stops that run's process
	canceled          map[string]bool               // runID -> user requested cancellation
	nonParallelActive int
	parallelActive    int
	wg                sync.WaitGroup
	awake             sleepGuard // keeps the Mac awake while runs are in flight

	// providerHealth is the health state last announced per provider, so an
	// unresolved problem is reported once instead of on every tick. It is seeded
	// from state.json, which is what makes the memo survive a restart. It has its
	// own lock because the readiness pass deliberately runs outside mu.
	healthMu       sync.Mutex
	providerHealth map[string]string
}

// ShutdownGrace is how long Loop lets in-flight runs finish on shutdown before
// terminating them, so a normal stop/restart doesn't fail running tasks.
const ShutdownGrace = 30 * time.Second

// New builds an Engine with production defaults (real UUIDs and run ids).
// providers is how the engine finds out whether a task's harness can run at
// all; it must not be nil.
func New(st *store.Store, gates *limit.Gates, r Runner, c clock.Clock, providers *provider.Checker) *Engine {
	e := &Engine{
		store:          st,
		gates:          gates,
		run:            r,
		clock:          c,
		providers:      providers,
		backoff:        DefaultRateLimitBackoff,
		active:         map[string]bool{},
		cancels:        map[string]context.CancelFunc{},
		canceled:       map[string]bool{},
		providerHealth: map[string]string{},
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
func (e *Engine) Tick(ctx context.Context) error {
	cfg, err := e.store.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// The global pause switch stops the queue dead: nothing new is started, and
	// no scheduling state is touched, so a task that came due while paused runs
	// as soon as the switch goes off again.
	if cfg.Settings.Paused {
		return nil
	}

	providers, err := provider.FromConfig(cfg)
	if err != nil {
		return fmt.Errorf("read provider configuration: %w", err)
	}

	// Which tasks are due is decided without touching a CLI, so the lock is held
	// only for the scheduling arithmetic. A task whose provider is waiting out
	// its own rate limit is dropped here too — there is no point probing a
	// harness that may not be asked to run anyway, and the allowance belongs to
	// that account alone, so the other providers carry on.
	due, err := e.dueTasks(cfg)
	if err != nil {
		return err
	}
	due = e.notRateLimited(providers, due)

	// Their harnesses are then probed *outside* the lock. A readiness check
	// spawns a CLI and a CLI can hang; holding e.mu across that would stall the
	// dashboard, a cancel and every finishing run for as long as the timeout.
	// Only the providers of tasks that are actually due are asked, and they are
	// asked without the cache — this is the moment right before a start, which is
	// exactly when a stale verdict would cost a recorded run.
	health := e.refreshProviderHealth(ctx, providers, due)

	// Back under the lock, with the scheduling state read *here*: a run that
	// finished while the probes were out has already written its completion, and
	// deciding against a snapshot from before that would start a one-shot task a
	// second time.
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

	// A task whose harness cannot run it is not started and not retired: it keeps
	// its place in the queue, takes no concurrency slot, and advances no
	// one-shot or cron state, so it simply runs once the provider is ready again.
	runnable := make([]task.Task, 0, len(due))
	for _, t := range due {
		stillDue, err := e.isDue(t, st, now)
		if err != nil {
			return err
		}
		if !stillDue {
			continue
		}
		if _, err := health.resolve(providers, t); err == nil {
			runnable = append(runnable, t)
		}
	}

	selected := schedule.Select(runnable, e.runningState())
	starts := make([]start, 0, len(selected))
	for _, t := range selected {
		if res, err := health.resolve(providers, t); err == nil {
			starts = append(starts, start{task: t, resolved: res})
		}
	}

	// Persist scheduling state only when something changed, and via a targeted
	// update so we never clobber read-status set concurrently through the API.
	if seeded || len(starts) > 0 {
		if err := e.store.UpdateState(func(cur *store.State) error {
			e.seedCronAnchors(cfg, cur, now)
			for _, s := range starts {
				cur.RecordStart(s.task.ID, now)
				cur.RecordRun(s.task.ID, now)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("persist scheduling state: %w", err)
		}
	}

	for _, s := range starts {
		sessionID, resume, dropped := e.sessionFor(s.task, st, s.resolved.Instance.ID)
		if err := e.launchTask(s.task, cfg.Settings, s.resolved, sessionID, resume, dropped, now); err != nil {
			return err
		}
	}
	return nil
}

// dueTasks reports the tasks whose trigger has come round, without asking any
// provider anything. The verdict is re-taken under the lock before a start is
// recorded (see Tick); this first pass exists only to find out which harnesses
// are worth probing at all.
func (e *Engine) dueTasks(cfg store.Config) ([]task.Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	st, err := e.store.LoadState()
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	now := e.clock.Now()
	// Give a freshly added cron task an anchor of now, so its first run is the
	// next occurrence rather than a backfill. The anchor is only persisted later,
	// under the lock that records the starts.
	e.seedCronAnchors(cfg, st, now)
	due := make([]task.Task, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		ok, err := e.isDue(t, st, now)
		if err != nil {
			return nil, err
		}
		if ok {
			due = append(due, t)
		}
	}
	return due, nil
}

// notRateLimited drops the tasks whose provider instance is waiting out a rate
// limit. An allowance belongs to an account, so one provider's pause holds up
// only its own work.
func (e *Engine) notRateLimited(set provider.Set, due []task.Task) []task.Task {
	out := due[:0]
	for _, t := range due {
		res, err := set.Resolve(provider.Selection{ProviderID: t.Provider})
		// An unresolvable provider is not a rate-limit question; it is reported
		// as a blocked task further down.
		if err != nil || e.gates.For(res.Instance.ID).Open() {
			out = append(out, t)
		}
	}
	return out
}

// isDue evaluates one task's trigger. The caller holds e.mu, because whether the
// task is already running is part of the answer.
func (e *Engine) isDue(t task.Task, st *store.State, now time.Time) (bool, error) {
	anchor, _ := st.LastStart(t.ID)
	ok, err := schedule.Due(t, schedule.Inputs{
		Now:           now,
		Running:       e.active[t.ID],
		CompletedOnce: st.IsCompletedOnce(t.ID),
		CronAnchor:    anchor,
	})
	if err != nil {
		return false, fmt.Errorf("evaluate task %q: %w", t.ID, err)
	}
	return ok, nil
}

// start is one task the scheduler picked, together with the execution identity
// it was cleared to run under.
type start struct {
	task     task.Task
	resolved provider.Resolved
}

// providerHealth is one tick's readiness verdict per provider id, taken before
// the scheduler lock so the decisions under it cost nothing.
type providerHealth map[string]provider.Health

// resolve turns a task into the execution identity it may run under, or reports
// why it may not. There is no fallback: a task that names a provider claudeq
// cannot honour is not moved to another one. The scheduler leaves it queued; a
// manual run reports the reason to whoever asked for it.
func (h providerHealth) resolve(set provider.Set, t task.Task) (provider.Resolved, error) {
	res, err := set.Resolve(provider.Selection{ProviderID: t.Provider, Model: t.Model})
	if err != nil {
		return provider.Resolved{}, err
	}
	got, ok := h[res.Instance.ID]
	if !ok {
		// Not probed this tick (the task was enabled moments ago). Not knowing is
		// not a reason to start: it waits for the next tick, which will know.
		return provider.Resolved{}, fmt.Errorf("provider %q has not been checked yet", res.Instance.ID)
	}
	if !got.Ready() {
		return provider.Resolved{}, fmt.Errorf("provider %q is not ready: %s",
			res.Instance.ID, got.ReasonOr("it cannot run tasks right now"))
	}
	return res, nil
}

// resolveRunnable probes one task's provider and reports the execution identity
// it may run under, or why it may not. It is the single-task form of
// refreshProviderHealth, for the manual "run now".
func (e *Engine) resolveRunnable(ctx context.Context, set provider.Set, st *store.State, t task.Task) (provider.Resolved, error) {
	res, err := set.Resolve(provider.Selection{ProviderID: t.Provider, Model: t.Model})
	if err != nil {
		return provider.Resolved{}, err
	}
	h := e.providers.CheckFresh(ctx, res.Instance)
	e.noteProviderHealth(st, res.Instance, h)
	if !h.Ready() {
		return provider.Resolved{}, fmt.Errorf("provider %q is not ready: %s",
			res.Instance.ID, h.ReasonOr("it cannot run tasks right now"))
	}
	return res, nil
}

// refreshProviderHealth probes the provider of every task that is due, once per
// distinct provider, and announces whatever changed. It runs outside the
// scheduler lock, and it probes without the cache — a task is due only when it
// is actually time to run it, so this is the check immediately before a start
// rather than something the tick does every five seconds.
func (e *Engine) refreshProviderHealth(ctx context.Context, set provider.Set, due []task.Task) providerHealth {
	out := providerHealth{}
	known := map[string]bool{}
	for _, inst := range set.All() {
		known[inst.ID] = true
	}
	st, err := e.store.LoadState()
	if err != nil {
		st = nil // the memo falls back to memory; a health check is not worth failing a tick over
	}
	for _, t := range due {
		known[t.Provider] = true // an id no instance answers to is remembered too
		res, err := set.Resolve(provider.Selection{ProviderID: t.Provider})
		if err != nil {
			// The instance is gone from the configuration, or switched off. There
			// is nothing to probe, so it is reported under the id the task asked
			// for.
			e.noteProviderHealth(st, provider.Instance{ID: t.Provider, Name: t.Provider},
				provider.Health{State: provider.HealthInvalidConfiguration, Reason: err.Error()})
			continue
		}
		if _, done := out[res.Instance.ID]; done {
			continue
		}
		h := e.providers.CheckFresh(ctx, res.Instance)
		out[res.Instance.ID] = h
		e.noteProviderHealth(st, res.Instance, h)
	}
	e.forgetUnknownProviders(known)
	return out
}

// forgetUnknownProviders drops the announcement memo of provider ids nothing
// refers to any more, so an instance removed and later re-added under the same
// id is announced again instead of inheriting the old one's state.
func (e *Engine) forgetUnknownProviders(known map[string]bool) {
	e.healthMu.Lock()
	defer e.healthMu.Unlock()
	for id := range e.providerHealth {
		if !known[id] {
			delete(e.providerHealth, id)
		}
	}
}

// noteProviderHealth announces a provider's readiness when it changes, and only
// then: the scheduler looks at every blocked task on every tick, and an alert
// per tick would bury the one that matters. The memo is kept in state.json as
// well as in memory, so a restart does not re-announce a condition the operator
// already knows about.
func (e *Engine) noteProviderHealth(st *store.State, inst provider.Instance, h provider.Health) {
	current := string(h.State)
	e.healthMu.Lock()
	previous, seen := e.providerHealth[inst.ID]
	if !seen && st != nil {
		previous = st.NotifiedProviderState(inst.ID)
	}
	e.providerHealth[inst.ID] = current
	e.healthMu.Unlock()
	if previous == current {
		return
	}
	if err := e.store.UpdateState(func(s *store.State) error {
		s.SetNotifiedProviderState(inst.ID, current)
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "claudeqd: record provider health for %q: %v\n", inst.ID, err)
	}
	if e.notifier == nil {
		return
	}
	var n notify.Notification
	switch {
	case h.Ready() && previous == "":
		// First look at a working provider: nothing happened worth announcing.
		return
	case h.Ready():
		n = notify.Notification{
			Title:   "ClaudeQ: " + inst.Label() + " is ready again",
			Message: "Tasks waiting for this provider will start on the next check.",
		}
	default:
		n = notify.Notification{
			Title:   "ClaudeQ: " + inst.Label() + " cannot run tasks",
			Message: h.ReasonOr("The provider is not ready.") + "\nTasks for it stay queued until it works again.",
		}
	}
	// Off the scheduler goroutine: a channel that takes its time must not delay
	// the tick that noticed the problem.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.send(n)
	}()
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

// launchTask starts a run for t on the already-resolved execution identity. The
// caller must hold e.mu, have checked that the provider can run, and have
// persisted the RecordStart. sessionID/resume come from the caller's snapshot.
func (e *Engine) launchTask(t task.Task, settings store.Settings, resolved provider.Resolved, sessionID string, resume bool, dropped string, started time.Time) error {
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
		// The identity is written down now, while it is true. A provider renamed
		// or removed tomorrow must not change what this run says it ran on.
		Provider: store.RunProvider{
			ID:              resolved.Instance.ID,
			Kind:            string(resolved.Instance.Kind),
			Name:            resolved.Instance.Label(),
			Model:           resolved.Model,
			ReasoningEffort: t.ReasoningEffort,
			AccessMode:      string(accessMode(t.Permissions)),
		},
	}
	// Record the start before marking the task active, so a failure here leaves
	// no task stuck in the running set (which would block the scheduler). A
	// quiet-history task is the exception: its runs enter history only if they
	// end in something worth seeing (see finish), so nothing is written now.
	if !t.QuietHistory {
		if err := e.store.AppendRun(rec); err != nil {
			_ = logFile.Close()
			return fmt.Errorf("record run start: %w", err)
		}
	}

	// A session that could not be continued is said so in the run's own log,
	// where whoever reads the run will look for it. It goes through the same
	// handle the executor writes to, so it cannot collide with the harness's
	// first line.
	if dropped != "" {
		writeNote(logFile, dropped)
	}

	e.active[t.ID] = true
	if t.Parallel {
		e.parallelActive++
	} else {
		e.nonParallelActive++
	}
	e.awake.acquire() // hold off idle system sleep until this run finishes

	req := executor.Request{
		Task:               t,
		Provider:           resolved.Instance,
		RunID:              runID,
		SessionID:          sessionID,
		Resume:             resume,
		Model:              resolved.Model,
		AccessMode:         accessMode(t.Permissions),
		ReasoningEffort:    t.ReasoningEffort,
		CustomSystemPrompt: settings.SystemPrompt,
		IdleTimeout:        settings.IdleTimeout(),
		Log:                logFile,
	}
	// Per-run context so a single run can be cancelled from the dashboard
	// without touching the daemon-wide runCtx (which shutdown owns).
	runCtx, cancelRun := context.WithCancel(e.runCtx)
	e.cancels[runID] = cancelRun

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer cancelRun()
		defer func() { _ = logFile.Close() }()
		res, runErr := e.runGuarded(runCtx, req)
		e.finish(t, resolved.Instance.ID, rec, res, runErr)
	}()
	return nil
}

// runGuarded runs the request and turns a panic into a failed result instead of
// crashing the daemon, so one bad run never takes the whole queue down.
func (e *Engine) runGuarded(ctx context.Context, req executor.Request) (res provider.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			res = provider.Result{Status: store.StatusFailed, Message: fmt.Sprintf("internal error: %v", r)}
			err = nil
		}
	}()
	return e.run.Run(ctx, req)
}

// CancelRun stops a run the user no longer wants. A run that is in flight has
// its process (group) terminated and is recorded as canceled. A run that has
// already paused on the rate limit is not a process any more but a plan — its
// scheduled resume is dropped instead, so the interrupted session is not picked
// up when the gate reopens (see cancelResume). Returns an error when the run id
// is neither in flight nor waiting to resume.
func (e *Engine) CancelRun(runID string) error {
	e.mu.Lock()
	cancel, ok := e.cancels[runID]
	if ok {
		e.canceled[runID] = true
	}
	e.mu.Unlock()
	if ok {
		cancel()
		return nil
	}
	return e.cancelResume(runID)
}

// cancelResume drops the scheduled resume of a rate-limited run: the pending
// session is forgotten, a one-shot task leaves the queue (so it never runs
// again), and the run is recorded as canceled. A recurring task keeps its
// schedule — only the interrupted session is discarded, its next occurrence
// starts fresh.
func (e *Engine) cancelResume(runID string) error {
	runs, err := e.store.Runs()
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	var rec *store.Run
	for i := range runs {
		if runs[i].RunID == runID {
			rec = &runs[i]
			break
		}
	}
	switch {
	case rec == nil:
		return fmt.Errorf("run %q is not running", runID)
	case rec.Status != store.StatusRateLimited:
		return fmt.Errorf("run %q is not waiting to resume", runID)
	}

	e.mu.Lock()
	active := e.active[rec.TaskID]
	e.mu.Unlock()
	if active {
		// The task is running again already (a resume in flight); cancel that
		// run by its own id instead of retracting a plan that has been acted on.
		return fmt.Errorf("run %q has already resumed", runID)
	}

	st, err := e.store.LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if pending, ok := st.PendingResume(rec.TaskID); !ok || pending.SessionID != rec.SessionID {
		return fmt.Errorf("run %q is no longer scheduled to resume", runID)
	}

	e.retire(rec.TaskID, e.isOneShot(*rec))

	canceled := *rec
	canceled.Status = store.StatusCanceled
	canceled.ResumeAt = nil
	canceled.Error = "resume canceled by the user"
	if canceled.FinishedAt == nil {
		now := e.clock.Now()
		canceled.FinishedAt = &now
	}
	if err := e.store.AppendRun(canceled); err != nil {
		return fmt.Errorf("record canceled resume: %w", err)
	}
	e.logStatus(canceled)
	return nil
}

// isOneShot reports whether the run's task runs only once, so cancelling its
// resume must take it out of the queue. The queued definition decides (its
// trigger may have been edited since the run started); the run's own snapshot
// is the fallback for a task that has meanwhile left the queue.
func (e *Engine) isOneShot(rec store.Run) bool {
	if cfg, err := e.store.LoadConfig(); err == nil {
		for _, t := range cfg.Tasks {
			if t.ID == rec.TaskID {
				return oneShotTrigger(t.Trigger)
			}
		}
	}
	return rec.Task != nil && oneShotTrigger(rec.Task.Trigger)
}

func oneShotTrigger(tr task.Trigger) bool {
	return tr == task.TriggerASAP || tr == task.TriggerFixed
}

// retire clears a task's pending resume and, for a one-shot task, marks it
// completed and takes it out of the queue — it stays in history and can be
// replayed from there. Recurring tasks remain queued for their next occurrence.
func (e *Engine) retire(taskID string, oneShot bool) {
	_ = e.store.UpdateState(func(st *store.State) error {
		st.ClearPendingResume(taskID)
		if oneShot {
			st.MarkCompletedOnce(taskID)
		}
		return nil
	})
	if !oneShot {
		return
	}
	_ = e.store.UpdateConfig(func(cfg *store.Config) error {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == taskID {
				cfg.Tasks = append(cfg.Tasks[:i], cfg.Tasks[i+1:]...)
				break
			}
		}
		return nil
	})
}

// logStatus records a run's final status and reason into its log, so the reason
// shows in both the raw and the chat view (especially for failures, pauses and
// interruptions).
func (e *Engine) logStatus(rec store.Run) {
	reason := rec.Error
	if reason == "" {
		reason = string(rec.Status)
	}
	line, err := json.Marshal(map[string]string{
		"type": "claudeq_status", "status": string(rec.Status), "message": reason,
	})
	if err != nil {
		return
	}
	_ = e.store.AppendRunLog(rec.RunID, append(line, '\n'))
}

// writeNote records a claudeq remark in a run's log, so something the operator
// needs to know about the run is where they read the run.
func writeNote(log io.Writer, message string) {
	line, err := json.Marshal(map[string]string{"type": "claudeq_status", "status": "note", "message": message})
	if err != nil {
		return
	}
	_, _ = log.Write(append(line, '\n'))
}

// sessionFor returns the session to use, whether it is a resume, and — when a
// waiting session had to be abandoned — why.
//
// A task waiting after a rate limit continues its own session. It can only do
// that on the provider that issued it: a session id means nothing to another
// harness, and nothing at all to another account. A task moved in the meantime
// therefore starts fresh rather than handing Codex a Claude conversation.
func (e *Engine) sessionFor(t task.Task, st *store.State, providerID string) (sessionID string, resume bool, dropped string) {
	pending, ok := st.PendingResume(t.ID)
	switch {
	case !ok:
		return e.newSessionID(), false, ""
	case pending.ProviderID == "" || pending.ProviderID == providerID:
		// No provider recorded means it was written before sessions carried one,
		// when there was only the one provider to have issued it.
		return pending.SessionID, true, ""
	}
	return e.newSessionID(), false, fmt.Sprintf(
		"the interrupted session belongs to provider %q and this task now runs on %q, so it starts fresh",
		pending.ProviderID, providerID)
}

// finish records a completed run and updates scheduling state. providerID is
// the instance the run executed on, because a rate limit closes that
// provider's gate and no other.
func (e *Engine) finish(t task.Task, providerID string, rec store.Run, res provider.Result, runErr error) {
	e.mu.Lock()
	delete(e.active, t.ID)
	wasCanceled := e.canceled[rec.RunID]
	delete(e.canceled, rec.RunID)
	delete(e.cancels, rec.RunID)
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
	// A user-requested cancellation surfaces as an interrupted/failed run from
	// the executor; record it as canceled instead. If the process managed to
	// finish successfully before the kill landed, keep the success.
	if wasCanceled && rec.Status != store.StatusSuccess {
		rec.Status = store.StatusCanceled
		rec.Error = "stopped by the user"
	}

	// Targeted state update: touch only this task's keys so a concurrent API
	// read-status change is preserved.
	switch rec.Status {
	case store.StatusRateLimited:
		// Prefer the absolute reset time the CLI reported (rate_limit_event):
		// block exactly until then plus a small buffer. Fall back to the retry
		// delay, then to the blind backoff, when no reset time is known or it
		// already lies in the past (stale event).
		// The limit belongs to the provider that hit it, so only its gate closes.
		gate := e.gates.For(providerID)
		if until := res.ResetAt; !until.IsZero() && until.After(e.clock.Now()) {
			gate.Block(until.Add(RateLimitResetBuffer))
		} else {
			delay := res.RetryAfter
			if delay <= 0 {
				delay = e.backoff
			}
			gate.BlockFor(delay) // wait for reset, then resume this session
		}
		// Record when the session is planned to continue (the gate keeps the
		// longest known block, so this is the real time, not just this run's),
		// so the pause reads as a scheduled resume instead of a dead end.
		if resume := gate.BlockedUntil(); !resume.IsZero() {
			rec.ResumeAt = &resume
		}
		_ = e.store.UpdateState(func(st *store.State) error {
			st.SetPendingResume(t.ID, res.SessionID, providerID)
			return nil
		})
	default:
		e.retire(t.ID, oneShotTrigger(t.Trigger))
	}

	if quietDrop(t, rec.Status) {
		// A quiet task's routine tick leaves no trace: the run was never
		// recorded (see launchTask), and its log goes too.
		if err := os.Remove(rec.LogPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "claudeqd: remove log of quiet run %s: %v\n", rec.RunID, err)
		}
	} else {
		_ = e.store.AppendRun(rec)

		// Bound disk usage: prune old runs/logs beyond the configured limit.
		if cfg, err := e.store.LoadConfig(); err == nil {
			_ = e.store.PruneHistory(cfg.Settings.RunHistoryLimit())
		}

		if rec.Status != store.StatusSuccess {
			e.logStatus(rec)
		}
	}

	// Notify outside any lock so channel I/O never blocks other finishing runs.
	e.notifyOutcome(t, rec, res.FinalOutput)
}

// quietDrop reports whether a run of a quiet-history task leaves history
// alone: a success is routine, and a rate-limit pause resolves itself (the
// daemon resumes the session). Failures, auth problems and cancellations are
// the outcomes the operator needs to see, so those are recorded like any other
// run's.
func quietDrop(t task.Task, status store.RunStatus) bool {
	if !t.QuietHistory {
		return false
	}
	return status == store.StatusSuccess || status == store.StatusRateLimited
}

// notifyOutcome sends a best-effort notification. Failures and auth problems
// always notify (FA-35/FA-38); successes notify only when the task opted in via
// NotifyOnResult. When opted in, the last result message is included.
func (e *Engine) notifyOutcome(t task.Task, rec store.Run, resultText string) {
	if e.notifier == nil {
		return
	}
	msg := truncateRunes(strings.TrimSpace(resultText), 300)

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
	e.send(n)
}

// send delivers one notification, best-effort, and reports a failed channel
// on stderr: the daemon runs unattended, so the log is the only place the
// operator can find out why an alert never arrived. Deliberately not bound to
// the loop's context — a notification raised moments before shutdown would
// otherwise be lost.
func (e *Engine) send(n notify.Notification) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.notifier.Notify(ctx, n); err != nil {
		fmt.Fprintf(os.Stderr, "claudeqd: notification %q not delivered: %v\n", n.Title, err)
	}
}

// noteErr logs a recurring failure once: err is written to stderr only when it
// differs from the last one recorded in last (this runs on every tick), and a
// nil err clears the memo so the next failure is logged again.
func noteErr(last *string, what string, err error) {
	if err == nil {
		*last = ""
		return
	}
	if msg := err.Error(); msg != *last {
		fmt.Fprintln(os.Stderr, "claudeqd: "+what+":", err)
		*last = msg
	}
}

// truncateRunes shortens s to at most limit runes, marking the cut with an
// ellipsis, so a long text still fits a notification channel's limits.
func truncateRunes(s string, limit int) string {
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
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
		// Artifacts are published by the task's own claudeq CLI call, so the
		// daemon learns about them by re-reading the list each tick.
		e.notifyNewArtifacts()
		// Same for notifications a task queued with `claudeq notify`.
		e.deliverTaskNotifications()
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
	// While paused nothing would run on wake, so nothing is worth waking for
	// (the heartbeat below stays, so the daemon notices the switch going off).
	var cands []time.Time
	if !cfg.Settings.Paused {
		cands = e.wakeCandidates(cfg, st, now)
		if bu := e.gates.BlockedUntil(); !bu.IsZero() {
			cands = append(cands, bu)
		}
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
// completion state — the manual "run now" test trigger (FA-16). It refuses to
// start while runs are globally paused, and still records history and honours
// resume-after-limit for that run.
func (e *Engine) RunTaskNow(ctx context.Context, taskID string) error {
	cfg, err := e.store.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Settings.Paused {
		return store.ErrPaused
	}
	providers, err := provider.FromConfig(cfg)
	if err != nil {
		return fmt.Errorf("read provider configuration: %w", err)
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

	st, err := e.store.LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	// Probed before the lock, like the scheduler's pass: a manual run reports the
	// problem to whoever pressed the button instead of filing a failed run, and a
	// hung CLI must not stall everything else in the meantime.
	resolved, err := e.resolveRunnable(ctx, providers, st, *target)
	if err != nil {
		return err
	}

	e.mu.Lock()
	if e.active[taskID] {
		e.mu.Unlock()
		return fmt.Errorf("task %q is already running", taskID)
	}
	sessionID, resume, dropped := e.sessionFor(*target, st, resolved.Instance.ID)
	now := e.clock.Now()
	if err := e.store.UpdateState(func(cur *store.State) error {
		cur.RecordStart(taskID, now)
		cur.RecordRun(taskID, now)
		return nil
	}); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("record run start: %w", err)
	}
	startErr := e.launchTask(*target, cfg.Settings, resolved, sessionID, resume, dropped, now)
	e.mu.Unlock()

	e.WaitIdle()
	return startErr
}

// accessMode maps a task's permission setting onto claudeq's provider-neutral
// access intent. The two stored values carry the same authority as before:
// leave the harness's own prompts in place, or bypass them entirely.
func accessMode(p task.Permissions) provider.AccessMode {
	if p == task.PermissionsSkip {
		return provider.AccessFullAccess
	}
	return provider.AccessProviderDefault
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
