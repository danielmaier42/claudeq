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
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

//go:embed web/*
var webFS embed.FS

// RunNower runs a task immediately (satisfied by *engine.Engine). Optional.
type RunNower interface {
	RunTaskNow(ctx context.Context, taskID string) error
}

// FolderChooser opens a native folder-selection dialog and returns the chosen
// POSIX path (chosen=false if the user cancelled). Optional.
type FolderChooser func(ctx context.Context, start string) (path string, chosen bool, err error)

// Deps are the API server's dependencies.
type Deps struct {
	Store        *store.Store
	Runner       RunNower       // optional; enables the run-now endpoint
	Models       func() []Model // optional; enables dynamic model listing
	ChooseFolder FolderChooser  // optional; enables the native folder dialog
}

// Handler builds the HTTP handler (REST API under /api + dashboard at /).
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()
	s := &server{d: d}

	mux.HandleFunc("GET /api/tasks", s.listTasks)
	mux.HandleFunc("POST /api/tasks", s.addTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.updateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.deleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/enable", s.enableTask(true))
	mux.HandleFunc("POST /api/tasks/{id}/disable", s.enableTask(false))
	mux.HandleFunc("POST /api/tasks/{id}/move", s.moveTask)
	mux.HandleFunc("POST /api/tasks/{id}/run-now", s.runNow)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("POST /api/runs/read-all", s.readAll)
	mux.HandleFunc("POST /api/runs/{id}/read", s.readRun)
	mux.HandleFunc("GET /api/runs/{id}/log", s.runLog)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("POST /api/fs/choose", s.chooseFolder)
	mux.HandleFunc("GET /api/stats", s.getStats)

	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	return mux
}

type server struct{ d Deps }

func (s *server) listTasks(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.d.Store.LoadConfig()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg.Tasks)
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
	writeJSON(w, http.StatusCreated, t)
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
	err := s.d.Store.UpdateConfig(func(cfg *store.Config) error {
		for i := range cfg.Tasks {
			if cfg.Tasks[i].ID == t.ID {
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
	id := r.PathValue("id")
	// Run asynchronously; the result shows up in the run history.
	go func() { _ = s.d.Runner.RunTaskNow(context.Background(), id) }()
	w.WriteHeader(http.StatusAccepted)
}

// runView is a run plus its unread flag.
type runView struct {
	store.Run
	Unread bool `json:"unread"`
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
	views := make([]runView, 0, len(runs))
	// Newest first for the dashboard.
	for i := len(runs) - 1; i >= 0; i-- {
		views = append(views, runView{Run: runs[i], Unread: !st.IsRead(runs[i].RunID)})
	}
	writeJSON(w, http.StatusOK, views)
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
	cfg.Settings = in
	if err := s.d.Store.SaveConfig(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg.Settings)
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
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 32
		case r == ' ' || r == '-' || r == '_':
			return '-'
		default:
			return -1
		}
	}, name)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "task"
	}
	if len(slug) > 24 {
		slug = strings.Trim(slug[:24], "-")
	}
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
