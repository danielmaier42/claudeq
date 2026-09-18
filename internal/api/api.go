// Package api serves claudeq's local control surface: a JSON REST API plus an
// embedded web dashboard, bound to loopback only (PLAN.md D11, NFA-04). It is
// the frontend/backend core a native window app would wrap.
package api

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/feedback"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
	"github.com/danielmaier42/claudeq/internal/update"
	"github.com/danielmaier42/claudeq/internal/workflow"
)

//go:embed web/*
var webFS embed.FS

// RunNower runs a task immediately (satisfied by *engine.Engine). Optional.
type RunNower interface {
	RunTaskNow(ctx context.Context, taskID string) error
}

// RunCanceler stops a currently running run (satisfied by *engine.Engine).
// Optional; enables the cancel endpoint.
type RunCanceler interface {
	CancelRun(runID string) error
}

// FolderChooser opens a native folder-selection dialog and returns the chosen
// POSIX path (chosen=false if the user cancelled). Optional.
type FolderChooser func(ctx context.Context, start string) (path string, chosen bool, err error)

// SaveFileDialog opens a native "Save as" panel titled prompt, pre-filled with
// defaultName, and returns the chosen POSIX path (chosen=false if the user
// cancelled). Optional; enables exporting a task to a file from the app.
type SaveFileDialog func(ctx context.Context, prompt, defaultName string) (path string, chosen bool, err error)

// Deps are the API server's dependencies.
type Deps struct {
	Store        *store.Store
	Runner       RunNower        // optional; enables the run-now endpoint
	Canceler     RunCanceler     // optional; enables the cancel-run endpoint
	OpenTerminal TerminalOpener  // optional; enables the continue-run endpoint
	ChooseFolder FolderChooser   // optional; enables the native folder dialog
	SaveFile     SaveFileDialog  // optional; enables the task export dialog
	ActiveTasks  func() []string // optional; ids of currently-running tasks (hidden from the queue)
	WakeError    func() string   // optional; last scheduled-wake error ("" if healthy)
	// LimitedUntil reports when the global rate-limit gate reopens (zero time
	// when it is open), so the dashboard can say the queue is waiting rather
	// than stuck. Optional (engine.Engine.LimitedUntil).
	LimitedUntil func() time.Time
	// BlockedProviders reports the provider instance IDs currently waiting out a
	// rate limit, each with the time its own gate reopens, so the dashboard can
	// name which account it is waiting on. Optional (engine.Engine.BlockedProviders).
	BlockedProviders func() map[string]time.Time
	// NotifyStatus reports whether macOS will actually show notifications
	// (notify.MacAuthorization). Optional; empty means "don't know".
	NotifyStatus func() string
	// WarmFileAccess reads the given directories so macOS raises its file-access
	// consent prompt now — called right after a task is created or its folder
	// changed, while the user is present. Optional.
	WarmFileAccess func(dirs []string)
	// Updates checks GitHub for newer releases and downloads the installer.
	// Optional; when nil the update endpoints report "unsupported".
	Updates *update.Service
	// Feedback drafts a GitHub issue from a short chat with the user. Optional;
	// when nil the dashboard offers the plain feedback form instead.
	Feedback *feedback.Service
	// OSVersion reports the macOS product version (e.g. "15.6") for the
	// environment line of a feedback issue. Optional; empty leaves it out.
	OSVersion func() string
	// Review checks a draft prompt against this machine before it is queued.
	// Optional; when nil the dashboard shows no suggestions.
	Review PromptReviewer
	// Registry holds the provider adapters this build supports. Required for the
	// provider endpoints.
	Registry *provider.Registry
	// Providers answers whether a configured instance can run a job. It is
	// shared with the daemon's scheduler, so the dashboard's polling reuses the
	// scheduler's verdicts instead of probing the CLIs again. Required for the
	// provider endpoints and for the queue's blocked marker.
	Providers *provider.Checker
}

// Handler builds the HTTP handler (REST API under /api + dashboard at /).
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()
	s := &server{d: d}

	mux.HandleFunc("GET /api/tasks", s.listTasks)
	mux.HandleFunc("POST /api/tasks", s.addTask)
	mux.HandleFunc("POST /api/tasks/import", s.readImport)
	mux.HandleFunc("POST /api/tasks/{id}/export", s.exportTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.updateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.deleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/enable", s.enableTask(true))
	mux.HandleFunc("POST /api/tasks/{id}/disable", s.enableTask(false))
	mux.HandleFunc("POST /api/tasks/{id}/move", s.moveTask)
	mux.HandleFunc("POST /api/tasks/{id}/run-now", s.runNow)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("POST /api/runs/read-all", s.readAll)
	mux.HandleFunc("POST /api/runs/{id}/read", s.readRun)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.cancelRun)
	mux.HandleFunc("POST /api/runs/{id}/continue", s.continueRun)
	mux.HandleFunc("GET /api/runs/{id}/log", s.runLog)
	mux.HandleFunc("GET /api/artifacts", s.listArtifacts)
	mux.HandleFunc("POST /api/artifacts/read-all", s.readAllArtifacts)
	mux.HandleFunc("POST /api/artifacts/{id}/read", s.readArtifact)
	mux.HandleFunc("DELETE /api/artifacts/{id}", s.deleteArtifact)
	mux.HandleFunc("GET /api/artifacts/{id}/content", s.artifactContent)
	mux.HandleFunc("GET /api/providers", s.listProviders)
	mux.HandleFunc("GET /api/providers/kinds", s.listProviderKinds)
	mux.HandleFunc("POST /api/providers", s.addProvider)
	mux.HandleFunc("PUT /api/providers/{id}", s.updateProvider)
	mux.HandleFunc("DELETE /api/providers/{id}", s.deleteProvider)
	mux.HandleFunc("POST /api/providers/{id}/check", s.checkProvider)
	mux.HandleFunc("POST /api/providers/{id}/enable", s.enableProvider(true))
	mux.HandleFunc("POST /api/providers/{id}/disable", s.enableProvider(false))
	mux.HandleFunc("POST /api/providers/{id}/default", s.setDefaultProvider)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/pause", s.setPaused)
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("GET /api/cron/check", s.checkCron)
	mux.HandleFunc("POST /api/review/prompt", s.reviewPrompt)
	mux.HandleFunc("GET /api/review/context", s.reviewContext)
	mux.HandleFunc("POST /api/fs/choose", s.chooseFolder)
	mux.HandleFunc("POST /api/fs/warm", s.warmNow)
	mux.HandleFunc("GET /api/stats", s.getStats)
	mux.HandleFunc("GET /api/health", s.getHealth)
	mux.HandleFunc("GET /api/update", s.getUpdate)
	mux.HandleFunc("POST /api/update/check", s.checkUpdate)
	mux.HandleFunc("POST /api/update/dismiss", s.dismissUpdate)
	mux.HandleFunc("POST /api/update/download", s.downloadUpdate)
	mux.HandleFunc("POST /api/update/relaunch", s.relaunchUpdate)
	mux.HandleFunc("GET /api/feedback", s.getFeedback)
	mux.HandleFunc("POST /api/feedback/turn", s.feedbackTurn)
	mux.HandleFunc("POST /api/feedback/url", s.feedbackURL)

	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", noCache(http.FileServer(http.FS(sub))))
	return mux
}

// noCache tells the WKWebView (and any client) never to reuse a cached copy of
// the embedded dashboard assets, so a rebuilt daemon's new logo/CSS/JS always
// shows instead of a stale cached version.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		h.ServeHTTP(w, r)
	})
}

type server struct {
	d Deps
	// Only the newest prompt review matters, so each one cancels its
	// predecessor; the generation counter keeps a finishing review from
	// clearing a newer one's cancel (see beginReview).
	reviewMu     sync.Mutex
	reviewGen    uint64
	reviewCancel context.CancelFunc
}

// activeTasks is the set of task ids running right now (empty when the daemon
// does not report them).
func (s *server) activeTasks() map[string]bool {
	active := map[string]bool{}
	if s.d.ActiveTasks != nil {
		for _, id := range s.d.ActiveTasks() {
			active[id] = true
		}
	}
	return active
}

func (s *server) listTasks(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Scheduling bookkeeping is a nice-to-have here: without it the queue simply
	// shows no last-run time.
	st, _ := s.d.Store.LoadState()
	active := s.activeTasks()
	blocked := s.blockedReasons(r.Context(), cfg)
	waiting := s.waitingFor(cfg)
	out := make([]taskView, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		// A running one-shot task moves to Activity and is hidden here. Recurring
		// (cron) tasks stay in the queue even while running, since they remain
		// queued for their next occurrence — just flagged as running.
		if active[t.ID] && t.Trigger != task.TriggerCron {
			continue
		}
		v := taskView{Task: t, Running: active[t.ID], BlockedReason: blocked[t.Provider], WaitingFor: waiting[t.ID]}
		// A task whose session is waiting for the rate limit is not idle: say so
		// in the queue, so it does not look like a job that simply hangs.
		if !active[t.ID] && st != nil && hasPendingResume(st, t.ID) {
			v.WaitingForLimit = true
		}
		// For recurring tasks, surface the next scheduled occurrence so the UI can
		// show it (e.g. as a tooltip on the cron expression). The task is already
		// validated on save, so a parse error here is not expected; skip silently.
		if t.Trigger == task.TriggerCron {
			if sched, err := t.CronSchedule(); err == nil {
				next := sched.Next(time.Now())
				v.NextRun = &next
			}
			if st != nil {
				if last, ok := st.LastRun(t.ID); ok {
					v.LastRun = &last
				}
			}
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// taskView is a task plus its transient running state, for the queue view.
type taskView struct {
	task.Task
	Running bool `json:"running"`
	// NextRun is the next scheduled occurrence for a cron task, if computable.
	NextRun *time.Time `json:"next_run,omitempty"`
	// LastRun is when a cron task last actually started a run, if it ever did.
	LastRun *time.Time `json:"last_run,omitempty"`
	// WaitingForLimit marks a task whose interrupted Claude session is queued to
	// resume once the rate-limit gate reopens.
	WaitingForLimit bool `json:"waiting_for_limit,omitempty"`
	// BlockedReason says why the task's provider cannot run it. The task stays
	// queued and starts by itself once the provider works again, so the queue
	// has to say that rather than showing a job that silently never runs.
	BlockedReason string `json:"blocked_reason,omitempty"`
	// WaitingFor names the jobs this one is still waiting for. A join sitting in
	// the queue looks like a job that never starts unless the queue says what it
	// is waiting on.
	WaitingFor []string `json:"waiting_for,omitempty"`
}

// waitingFor names, per task, the jobs it is still waiting for. Only tasks that
// depend on something appear, and history is read at most once.
func (s *server) waitingFor(cfg store.Config) map[string][]string {
	var dependent []task.Task
	for _, t := range cfg.Tasks {
		if len(t.DependsOn) > 0 {
			dependent = append(dependent, t)
		}
	}
	if len(dependent) == 0 {
		return nil
	}
	runs, err := s.d.Store.Runs()
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, t := range dependent {
		if _, names := workflow.Ready(workflow.Resolve(t, cfg.Tasks, runs)); len(names) > 0 {
			out[t.ID] = names
		}
	}
	return out
}

// blockedReasons maps a task's provider field to why that provider cannot run
// anything right now, or holds no entry when it can. It is keyed by the field
// as stored (empty means "the default provider"), so the caller needs no second
// resolution step. Verdicts come from the shared checker, which the scheduler
// keeps warm — listing the queue never probes a CLI on its own.
func (s *server) blockedReasons(ctx context.Context, cfg store.Config) map[string]string {
	set, err := provider.FromConfig(cfg)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, t := range cfg.Tasks {
		if _, seen := out[t.Provider]; seen {
			continue
		}
		resolved, err := set.Resolve(provider.Selection{ProviderID: t.Provider})
		if err != nil {
			out[t.Provider] = err.Error()
			continue
		}
		if h := s.d.Providers.Check(ctx, resolved.Instance); !h.Ready() {
			out[t.Provider] = h.Reason
			if h.Reason == "" {
				out[t.Provider] = resolved.Instance.Label() + " cannot run tasks right now."
			}
		}
	}
	return out
}

func (s *server) addTask(w http.ResponseWriter, r *http.Request) {
	var t task.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if t.Permissions == "" {
		t.Permissions = task.PermissionsDefault
	}
	if t.ID == "" {
		t.ID = genTaskID(t.Name)
	} else if err := task.CheckID(t.ID); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	// A newly queued task is active by default; pausing is an explicit action
	// via the enable/disable endpoint. This avoids a silently-disabled task that
	// never runs on schedule (only via "run now").
	t.Enabled = true
	if err := t.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.ensureRunnable(r.Context(), t.Provider); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := app.AddTask(s.d.Store, t); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.warmAccess(t.WorkingDir)
	writeJSON(w, http.StatusCreated, t)
}

// storedTask reads a task as it is currently stored.
func (s *server) storedTask(id string) (task.Task, error) {
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		return task.Task{}, err
	}
	for _, t := range cfg.Tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return task.Task{}, fmt.Errorf("task %q not found", id)
}

// ensureRunnable refuses to file work for a provider that cannot run it, so a
// task is never queued that is known in advance to fail unattended.
func (s *server) ensureRunnable(ctx context.Context, providerID string) error {
	set, err := app.Providers(s.d.Store)
	if err != nil {
		return err
	}
	return app.EnsureRunnable(ctx, set, s.d.Providers, providerID)
}

// warmAccess provokes the macOS file-access prompt for a task's folder right
// after it is created or changed, while the user is present in the app — so a
// newly-used protected location (Downloads, Desktop, …) is authorised now, not
// at 3am mid-run. Best-effort; a no-op when the hook or path is unset.
func (s *server) warmAccess(dir string) {
	if s.d.WarmFileAccess != nil && dir != "" {
		go s.d.WarmFileAccess([]string{dir})
	}
}

// warmNow provokes the file-access prompt for every enabled task's folder. The
// app calls it on launch, so simply opening the window re-checks access while
// the user is present — covering folders that were added or edited while the app
// was closed, or that were never authorised. The daemon (this process) does the
// probing, so the grant lands on the identity that actually runs the tasks.
func (s *server) warmNow(w http.ResponseWriter, _ *http.Request) {
	if s.d.WarmFileAccess == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	dirs := make([]string, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		if t.Enabled {
			dirs = append(dirs, t.WorkingDir)
		}
	}
	go s.d.WarmFileAccess(dirs)
	w.WriteHeader(http.StatusAccepted)
}

func (s *server) updateTask(w http.ResponseWriter, r *http.Request) {
	var t task.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	t.ID = r.PathValue("id") // the id is fixed by the URL
	if t.Permissions == "" {
		t.Permissions = task.PermissionsDefault
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if err := t.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Only a *move* to another provider is checked. An existing task keeps the
	// one it has, and an unready provider must not stand in the way of editing
	// the prompt, the folder or the schedule — that edit may well be how the
	// operator is fixing it.
	if prev, err := s.storedTask(t.ID); err == nil && prev.Provider != t.Provider {
		if err := s.ensureRunnable(r.Context(), t.Provider); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	}
	var prevDir string
	err := s.d.Store.UpdateConfig(func(cfg *store.Config) error {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == t.ID {
				prevDir = cfg.Tasks[i].WorkingDir
				cfg.Tasks[i] = t
				return nil
			}
		}
		return fmt.Errorf("task %q not found", t.ID)
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// Only warm when the folder actually changed — editing just the prompt/model
	// keeps the same (already-authorised) directory, so re-probing it is wasted
	// work. A genuine folder change still provokes the prompt for the new one.
	if t.WorkingDir != prevDir {
		s.warmAccess(t.WorkingDir)
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := app.RemoveTask(s.d.Store, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) enableTask(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := app.SetEnabled(s.d.Store, r.PathValue("id"), enabled); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) moveTask(w http.ResponseWriter, r *http.Request) {
	to, err := strconv.Atoi(r.URL.Query().Get("to"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("query param 'to' must be an integer"))
		return
	}
	if err := app.Move(s.d.Store, r.PathValue("id"), to); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) runNow(w http.ResponseWriter, r *http.Request) {
	if s.d.Runner == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("run-now not available"))
		return
	}
	// A paused queue refuses manual runs too, and the caller should hear about it:
	// the run itself is fire-and-forget, so its error would go nowhere.
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if cfg.Settings.Paused {
		writeErr(w, http.StatusConflict, store.ErrPaused)
		return
	}
	id := r.PathValue("id")
	// Run asynchronously; the result shows up in the run history.
	go func() { _ = s.d.Runner.RunTaskNow(context.Background(), id) }()
	w.WriteHeader(http.StatusAccepted)
}

// cancelRun stops a currently running run. 409 when the run is not in flight
// (already finished, or unknown) — the UI then just refreshes its state.
func (s *server) cancelRun(w http.ResponseWriter, r *http.Request) {
	if s.d.Canceler == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("cancel not available"))
		return
	}
	if err := s.d.Canceler.CancelRun(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// continueRun opens a Terminal window that resumes the run's session
// interactively, in the run's provider and in the task's working directory,
// so a finished unattended chat can be picked up by hand with its full
// context. Only finished runs qualify: a running one still owns its session,
// and a rate-limited one will be resumed by the queue itself.
func (s *server) continueRun(w http.ResponseWriter, r *http.Request) {
	if s.d.OpenTerminal == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("continue not available"))
		return
	}
	runs, err := s.d.Store.Runs()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	id := r.PathValue("id")
	var run *store.Run
	for i := range runs {
		if runs[i].RunID == id {
			run = &runs[i]
			break
		}
	}
	switch {
	case run == nil:
		writeErr(w, http.StatusNotFound, errors.New("run not found"))
		return
	case run.Status == store.StatusRunning:
		writeErr(w, http.StatusConflict, errors.New("the run is still in progress; wait for it to finish (or cancel it)"))
		return
	case !run.Status.Terminal():
		writeErr(w, http.StatusConflict, errors.New("the run is waiting to resume automatically; continue it after it finishes"))
		return
	case run.SessionID == "":
		writeErr(w, http.StatusConflict, errors.New("no session recorded for this run"))
		return
	case run.Task == nil || run.Task.WorkingDir == "":
		writeErr(w, http.StatusConflict, errors.New("no working directory recorded for this run"))
		return
	}
	// The session lives in Claude Code's per-project store keyed by this
	// directory — a vanished folder means the resume cannot work.
	if _, err := os.Stat(run.Task.WorkingDir); err != nil {
		writeErr(w, http.StatusConflict, fmt.Errorf("the task's working directory is gone: %w", err))
		return
	}
	// The interactive resume goes to the provider instance that owns the
	// session, and the adapter says how that harness reopens one. claudeq never
	// offers to continue a session on another account, let alone another harness.
	argv, err := s.resumeCommand(*run, *run.Task)
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	if err := s.d.OpenTerminal(r.Context(), run.Task.WorkingDir, argv); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resumeCommand builds the argv that reopens a run's session in a terminal.
//
// The provider that owns the session is the one the run used, not the one the
// task points at today: a task moved to another provider since then has left
// its old conversation where it was, and offering to continue it with a harness
// that never had it would open an empty terminal at best. What the run recorded
// is therefore what is asked; only a run from before that was recorded falls
// back to the task's own provider.
func (s *server) resumeCommand(run store.Run, t task.Task) ([]string, error) {
	set, err := app.Providers(s.d.Store)
	if err != nil {
		return nil, err
	}
	owner := run.Provider.ID
	if owner == "" {
		owner = t.Provider
	}
	resolved, err := set.Resolve(provider.Selection{ProviderID: owner})
	if err != nil {
		return nil, err
	}
	ad, err := s.d.Registry.Lookup(resolved.Instance.Kind)
	if err != nil {
		return nil, err
	}
	if !ad.Capabilities().InteractiveResume {
		return nil, fmt.Errorf("%s cannot continue a session in a terminal", resolved.Instance.Label())
	}
	if ad.ResolveBinary(resolved.Instance) == "" {
		return nil, fmt.Errorf("%s is not installed on this Mac", resolved.Instance.Label())
	}
	cmd, err := ad.InteractiveResumeCommand(resolved.Instance, provider.Request{
		SessionID:  run.SessionID,
		WorkingDir: t.WorkingDir,
		AccessMode: resumeAccess(run, t),
	})
	if err != nil {
		return nil, err
	}
	return append([]string{cmd.Path}, cmd.Args...), nil
}

// resumeAccess is the authority a continued session gets: the one the run
// actually had, so changing the task's permissions afterwards does not hand a
// finished conversation more authority than it ever ran with. A run from before
// that was recorded falls back to the task.
func resumeAccess(run store.Run, t task.Task) provider.AccessMode {
	if run.Provider.AccessMode != "" {
		return provider.AccessMode(run.Provider.AccessMode)
	}
	return accessMode(t.Permissions)
}

// accessMode maps a task's permission setting onto claudeq's provider-neutral
// access intent, the same way the engine does for a run — so a continued session
// gets exactly the authority the run it continues had.
func accessMode(p task.Permissions) provider.AccessMode {
	if p == task.PermissionsSkip {
		return provider.AccessFullAccess
	}
	return provider.AccessProviderDefault
}

// runView is a run plus its unread flag and whether its interrupted session is
// still scheduled to resume.
type runView struct {
	store.Run
	// FinalOutput shadows the run record's own: the answer can be kilobytes and
	// the list carries hundreds of runs, so it is not sent with the list. What
	// the run said is read from its log.
	FinalOutput string `json:"final_output,omitempty"`
	Unread      bool   `json:"unread"`
	// ResumePending marks a rate-limited run whose session the daemon is still
	// going to pick up once the gate reopens. It is what separates a run that is
	// merely waiting from one whose pause is history (already resumed, canceled,
	// or the task is gone).
	ResumePending bool `json:"resume_pending,omitempty"`
}

func (s *server) listRuns(w http.ResponseWriter, _ *http.Request) {
	runs, err := s.d.Store.Runs()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	st, err := s.d.Store.LoadState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	active := s.activeTasks()
	views := make([]runView, 0, len(runs))
	// Newest first for the dashboard.
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		views = append(views, runView{
			Run:           r,
			Unread:        !st.IsRead(r.RunID),
			ResumePending: resumePending(r, st, active),
		})
	}
	writeJSON(w, http.StatusOK, views)
}

// resumePending reports whether a rate-limited run's session is still queued to
// be resumed: the task must still hold exactly this run's session as its
// pending resume, and must not be running right now (a resume already in
// flight shows up as its own running run).
func resumePending(r store.Run, st *store.State, active map[string]bool) bool {
	if r.Status != store.StatusRateLimited || active[r.TaskID] {
		return false
	}
	pending, ok := st.PendingResume(r.TaskID)
	return ok && r.SessionID != "" && pending.SessionID == r.SessionID
}

// hasPendingResume reports whether a task holds a session waiting to continue.
func hasPendingResume(st *store.State, taskID string) bool {
	_, ok := st.PendingResume(taskID)
	return ok
}

func (s *server) readRun(w http.ResponseWriter, r *http.Request) {
	if err := app.MarkRead(s.d.Store, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) readAll(w http.ResponseWriter, _ *http.Request) {
	if err := app.MarkAllRead(s.d.Store); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) runLog(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.d.Store.LogPath(r.PathValue("id")))
	if errors.Is(err, os.ErrNotExist) {
		writeErr(w, http.StatusNotFound, errors.New("log not found"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// artifactView is a published artifact plus its unread flag.
type artifactView struct {
	store.Artifact
	Unread bool `json:"unread"`
}

func (s *server) listArtifacts(w http.ResponseWriter, _ *http.Request) {
	arts, err := s.d.Store.Artifacts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	st, err := s.d.Store.LoadState()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	views := make([]artifactView, 0, len(arts))
	// Newest first for the dashboard.
	for i := len(arts) - 1; i >= 0; i-- {
		views = append(views, artifactView{Artifact: arts[i], Unread: !st.IsArtifactRead(arts[i].ID)})
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *server) readArtifact(w http.ResponseWriter, r *http.Request) {
	if err := app.MarkArtifactRead(s.d.Store, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) readAllArtifacts(w http.ResponseWriter, _ *http.Request) {
	if err := app.MarkAllArtifactsRead(s.d.Store); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteArtifact(w http.ResponseWriter, r *http.Request) {
	if err := app.DeleteArtifact(s.d.Store, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// artifactContent serves an artifact's stored file for the in-app viewer or a
// download. The file is user-generated, so it is served with a restrictive CSP
// (no network access) and is meant to be shown inside a sandboxed iframe, so a
// malicious HTML artifact can neither reach the loopback API nor exfiltrate data.
func (s *server) artifactContent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	arts, err := s.d.Store.Artifacts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	var art *store.Artifact
	for i := range arts {
		if arts[i].ID == id {
			art = &arts[i]
			break
		}
	}
	if art == nil {
		writeErr(w, http.StatusNotFound, errors.New("artifact not found"))
		return
	}

	f, err := os.Open(s.d.Store.ArtifactContentPath(*art))
	if errors.Is(err, os.ErrNotExist) {
		writeErr(w, http.StatusNotFound, errors.New("artifact file missing"))
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	ct := art.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Self-contained pages render (inline CSS/JS, data: images) but the artifact
	// can make no network requests, so it cannot phone home or reach the API.
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src data: blob:; media-src data: blob:; "+
			"style-src 'unsafe-inline'; font-src data:; script-src 'unsafe-inline'; "+
			"form-action 'none'; base-uri 'none'")
	if r.URL.Query().Get("download") != "" {
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(art.FileName))
	}
	http.ServeContent(w, r, art.FileName, info.ModTime(), f)
}

func (s *server) getSettings(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg.Settings)
}

func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// A channel that is switched on but cannot deliver is refused here, while
	// the operator is looking at the form — not at three in the morning, where
	// the only trace would be a line in the daemon's log.
	if err := app.ValidateNotifications(in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// The pause switch belongs to POST /api/pause alone. It can be flipped from
	// the Queue banner or the CLI at any time, so a settings form filled in
	// before that must not carry a stale value back and quietly resume the queue.
	in.Paused = cfg.Settings.Paused
	// Same for the default provider, which the Providers section sets on its own
	// endpoint: a form that does not show it must not be able to reset it.
	in.DefaultProvider = cfg.Settings.DefaultProvider
	cfg.Settings = in
	if err := s.d.Store.SaveConfig(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg.Settings)
}

// setPaused flips the global pause switch on its own, without going through the
// whole settings payload: the dashboard toggles it like a task's enable switch
// (no Save press) and from the Queue banner, so it must not carry — and thus
// overwrite — every other setting on the way.
func (s *server) setPaused(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := app.SetPaused(s.d.Store, in.Paused); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"paused": in.Paused})
}

func (s *server) getStats(w http.ResponseWriter, _ *http.Request) {
	runs, err := s.d.Store.Runs()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, computeStats(runs, time.Now()))
}

// listModels answers with the models one provider suggests. The provider is
// named by the `provider` query parameter; without one the default provider's
// list is returned, which is what the task form wants before anything is chosen.
//
// A catalog is never validation: a model a task names but this list does not is
// passed to the harness unchanged, so a discovery failure costs suggestions and
// nothing else.
func (s *server) listModels(w http.ResponseWriter, r *http.Request) {
	set, ok := s.providerSet(w)
	if !ok {
		return
	}
	resolved, err := set.Resolve(provider.Selection{ProviderID: r.URL.Query().Get("provider")})
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ad, err := s.d.Registry.Lookup(resolved.Instance.Kind)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	models := ad.ListModels(r.Context(), resolved.Instance, provider.ExecProber{})
	if models == nil {
		models = []provider.Model{}
	}
	writeJSON(w, http.StatusOK, models)
}

// limitedProvider names one provider instance waiting out a rate limit, for
// the health response's limited_providers list.
type limitedProvider struct {
	Name  string `json:"name"`
	Until string `json:"until"`
	// Fallback is the label of the provider that takes this one's tasks in the
	// meantime, empty when it has none that could. A banner that names it says
	// the queue is moving; without one the queue really is waiting.
	Fallback string `json:"fallback,omitempty"`
}

// getHealth reports daemon health the UI can warn about: whether scheduled-wake
// setup is working, and whether macOS is set to show ClaudeQ's notifications at
// all (a denied app posts into the void).
func (s *server) getHealth(w http.ResponseWriter, r *http.Request) {
	wakeErr := ""
	if s.d.WakeError != nil {
		wakeErr = s.d.WakeError()
	}
	notifyStatus := ""
	if s.d.NotifyStatus != nil {
		notifyStatus = s.d.NotifyStatus()
	}
	limitedUntil := ""
	if s.d.LimitedUntil != nil {
		if until := s.d.LimitedUntil(); !until.IsZero() {
			limitedUntil = until.Format(time.RFC3339)
		}
	}
	limitedProviders := []limitedProvider{}
	if s.d.BlockedProviders != nil {
		blocked := s.d.BlockedProviders()
		if len(blocked) > 0 {
			// Best-effort labels: a config that fails to load still leaves the
			// generic limited_until banner working, just without provider names.
			set, _ := app.Providers(s.d.Store)
			for id, until := range blocked {
				name, fallback := id, ""
				if inst, ok := set.Lookup(id); ok {
					name = inst.Label()
					fallback = s.fallbackLabel(r.Context(), set, inst, blocked)
				}
				limitedProviders = append(limitedProviders,
					limitedProvider{Name: name, Until: until.Format(time.RFC3339), Fallback: fallback})
			}
			sort.Slice(limitedProviders, func(i, j int) bool { return limitedProviders[i].Until < limitedProviders[j].Until })
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wake_error":        wakeErr,
		"notify_status":     notifyStatus,
		"limited_until":     limitedUntil,
		"limited_providers": limitedProviders,
	})
}

// fallbackLabel names the provider that is actually taking inst's tasks while
// its allowance is used up: the first one down its fallback chain that is
// switched on, not blocked itself, and not known to be unable to run. It is the
// walk the scheduler makes plus the verdict it would refuse on, so the banner
// says the queue keeps moving only when it really does. The readiness answer is
// the cached one — this is a banner, not a start, and a probe per poll would
// spawn a CLI every few seconds.
func (s *server) fallbackLabel(ctx context.Context, set provider.Set, inst provider.Instance, blocked map[string]time.Time) string {
	seen := map[string]struct{}{inst.ID: {}}
	for cur := inst; cur.FallbackProvider != ""; {
		next, ok := set.Lookup(cur.FallbackProvider)
		if _, visited := seen[cur.FallbackProvider]; !ok || visited || !next.Enabled {
			return ""
		}
		seen[next.ID] = struct{}{}
		_, limited := blocked[next.ID]
		if !limited && !s.d.Providers.Check(ctx, next).KnownUnready() {
			return next.Label()
		}
		cur = next
	}
	return ""
}

func (s *server) chooseFolder(w http.ResponseWriter, r *http.Request) {
	if s.d.ChooseFolder == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("folder dialog not available"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	path, chosen, err := s.d.ChooseFolder(ctx, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if !chosen {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// genTaskID builds a URL-safe id from a name plus a short random suffix, so the
// user never has to supply one.
func genTaskID(name string) string {
	slug := task.Slug(name)
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return slug
	}
	return slug + "-" + hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
