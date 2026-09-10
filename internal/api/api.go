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
	"strconv"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
	"github.com/danielmaier42/claudeq/internal/update"
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
	Models       func() []Model  // optional; enables dynamic model listing
	ChooseFolder FolderChooser   // optional; enables the native folder dialog
	SaveFile     SaveFileDialog  // optional; enables the task export dialog
	ActiveTasks  func() []string // optional; ids of currently-running tasks (hidden from the queue)
	WakeError    func() string   // optional; last scheduled-wake error ("" if healthy)
	// LimitedUntil reports when the global rate-limit gate reopens (zero time
	// when it is open), so the dashboard can say the queue is waiting rather
	// than stuck. Optional (engine.Engine.LimitedUntil).
	LimitedUntil func() time.Time
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
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/pause", s.setPaused)
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("GET /api/claude/which", s.whichClaude)
	mux.HandleFunc("POST /api/fs/choose", s.chooseFolder)
	mux.HandleFunc("POST /api/fs/warm", s.warmNow)
	mux.HandleFunc("GET /api/stats", s.getStats)
	mux.HandleFunc("GET /api/health", s.getHealth)
	mux.HandleFunc("GET /api/update", s.getUpdate)
	mux.HandleFunc("POST /api/update/check", s.checkUpdate)
	mux.HandleFunc("POST /api/update/dismiss", s.dismissUpdate)
	mux.HandleFunc("POST /api/update/download", s.downloadUpdate)
	mux.HandleFunc("POST /api/update/relaunch", s.relaunchUpdate)

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

type server struct{ d Deps }

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

func (s *server) listTasks(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Scheduling bookkeeping is a nice-to-have here: without it the queue simply
	// shows no last-run time.
	st, _ := s.d.Store.LoadState()
	active := s.activeTasks()
	out := make([]taskView, 0, len(cfg.Tasks))
	for _, t := range cfg.Tasks {
		// A running one-shot task moves to Activity and is hidden here. Recurring
		// (cron) tasks stay in the queue even while running, since they remain
		// queued for their next occurrence — just flagged as running.
		if active[t.ID] && t.Trigger != task.TriggerCron {
			continue
		}
		v := taskView{Task: t, Running: active[t.ID]}
		// A task whose session is waiting for the rate limit is not idle: say so
		// in the queue, so it does not look like a job that simply hangs.
		if !active[t.ID] && st != nil && st.PendingResume(t.ID) != "" {
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
	if err := app.AddTask(s.d.Store, t); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.warmAccess(t.WorkingDir)
	writeJSON(w, http.StatusCreated, t)
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

// continueRun opens a Terminal window that resumes the run's Claude session
// interactively (`claude --resume <session-id>` in the task's working
// directory), so a finished unattended chat can be picked up by hand with its
// full context. Only finished runs qualify: a running one still owns its
// session, and a rate-limited one will be resumed by the queue itself.
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
		writeErr(w, http.StatusConflict, errors.New("no Claude session recorded for this run"))
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
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	argv := []string{claudeBin(cfg.Settings), "--resume", run.SessionID}
	if run.Task.Permissions == task.PermissionsSkip {
		argv = append(argv, "--dangerously-skip-permissions")
	}
	if err := s.d.OpenTerminal(r.Context(), run.Task.WorkingDir, argv); err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// claudeBin resolves the Claude Code binary for the resume command the same
// way the daemon does for runs: explicit setting first, then auto-detection,
// then a bare name for the interactive shell to resolve.
func claudeBin(s store.Settings) string {
	if s.ClaudePath != "" {
		return s.ClaudePath
	}
	if p := executor.DetectBinary(); p != "" {
		return p
	}
	return "claude"
}

// runView is a run plus its unread flag and whether its interrupted session is
// still scheduled to resume.
type runView struct {
	store.Run
	Unread bool `json:"unread"`
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
	return r.SessionID != "" && st.PendingResume(r.TaskID) == r.SessionID
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
	// The pause switch belongs to POST /api/pause alone. It can be flipped from
	// the Queue banner or the CLI at any time, so a settings form filled in
	// before that must not carry a stale value back and quietly resume the queue.
	in.Paused = cfg.Settings.Paused
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

func (s *server) listModels(w http.ResponseWriter, _ *http.Request) {
	if s.d.Models != nil {
		writeJSON(w, http.StatusOK, s.d.Models())
		return
	}
	writeJSON(w, http.StatusOK, fallbackModels)
}

// getHealth reports daemon health the UI can warn about: whether scheduled-wake
// setup is working, and whether macOS is set to show ClaudeQ's notifications at
// all (a denied app posts into the void).
func (s *server) getHealth(w http.ResponseWriter, _ *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]string{
		"wake_error":    wakeErr,
		"notify_status": notifyStatus,
		"limited_until": limitedUntil,
	})
}

// whichClaude reports the auto-detected Claude Code binary path so the GUI can
// pre-fill the setting. Empty path means it could not be located.
func (s *server) whichClaude(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"path": executor.DetectBinary()})
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
