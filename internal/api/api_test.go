package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func newServer(t *testing.T, runner RunNower) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, Runner: runner}))
	t.Cleanup(srv.Close)
	return srv, st
}

type resp struct {
	Status int
	Body   []byte
}

func (r resp) into(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		t.Fatalf("decode response: %v (%s)", err, string(r.Body))
	}
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any) resp {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = r.Body.Close() }()
	data, _ := io.ReadAll(r.Body)
	return resp{Status: r.StatusCode, Body: data}
}

func sampleTask(id string) task.Task {
	return task.Task{ID: id, Name: id, Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP, Enabled: true, Permissions: task.PermissionsDefault}
}

func TestAddAndListTasks(t *testing.T) {
	srv, _ := newServer(t, nil)

	if r := do(t, srv, "POST", "/api/tasks", sampleTask("a")); r.Status != http.StatusCreated {
		t.Fatalf("add status = %d", r.Status)
	}
	var tasks []task.Task
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "a" {
		t.Fatalf("expected [a], got %+v", tasks)
	}
}

func TestAddInvalidTaskRejected(t *testing.T) {
	srv, _ := newServer(t, nil)
	bad := sampleTask("x")
	bad.Prompt = ""
	if r := do(t, srv, "POST", "/api/tasks", bad); r.Status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", r.Status)
	}
}

func TestEnableDisableMoveDelete(t *testing.T) {
	srv, st := newServer(t, nil)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	do(t, srv, "POST", "/api/tasks", sampleTask("b"))

	if r := do(t, srv, "POST", "/api/tasks/a/disable", nil); r.Status != http.StatusNoContent {
		t.Fatalf("disable status = %d", r.Status)
	}
	if r := do(t, srv, "POST", "/api/tasks/b/move?to=0", nil); r.Status != http.StatusNoContent {
		t.Fatalf("move status = %d", r.Status)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Tasks[0].ID != "b" {
		t.Fatalf("move failed, order = %s,%s", cfg.Tasks[0].ID, cfg.Tasks[1].ID)
	}
	if cfg.Tasks[1].Enabled {
		t.Fatal("task a should be disabled")
	}
	if r := do(t, srv, "DELETE", "/api/tasks/a", nil); r.Status != http.StatusNoContent {
		t.Fatalf("delete status = %d", r.Status)
	}
}

func TestRunsAndReadAll(t *testing.T) {
	srv, st := newServer(t, nil)
	_ = st.AppendRun(store.Run{RunID: "r1", TaskID: "a", TaskName: "a", StartedAt: time.Now(), Status: store.StatusSuccess})

	var views []runView
	do(t, srv, "GET", "/api/runs", nil).into(t, &views)
	if len(views) != 1 || !views[0].Unread {
		t.Fatalf("expected 1 unread run, got %+v", views)
	}

	if r := do(t, srv, "POST", "/api/runs/read-all", nil); r.Status != http.StatusNoContent {
		t.Fatalf("read-all status = %d", r.Status)
	}
	do(t, srv, "GET", "/api/runs", nil).into(t, &views)
	if views[0].Unread {
		t.Fatal("run should be read after read-all")
	}
}

func TestRunLogNotFound(t *testing.T) {
	srv, _ := newServer(t, nil)
	if r := do(t, srv, "GET", "/api/runs/nope/log", nil); r.Status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", r.Status)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	srv, _ := newServer(t, nil)
	in := store.Settings{DefaultModel: "claude-opus-4-8", SkipPermissionsDefault: true, HeartbeatMinutes: 30}
	if r := do(t, srv, "PUT", "/api/settings", in); r.Status != http.StatusOK {
		t.Fatalf("put status = %d", r.Status)
	}
	var out store.Settings
	do(t, srv, "GET", "/api/settings", nil).into(t, &out)
	if out.DefaultModel != "claude-opus-4-8" || !out.SkipPermissionsDefault || out.HeartbeatMinutes != 30 {
		t.Fatalf("settings round-trip mismatch: %+v", out)
	}
}

type stubRunner struct{ done chan string }

func (s *stubRunner) RunTaskNow(_ context.Context, id string) error {
	s.done <- id
	return nil
}

func TestRunNowInvokesRunner(t *testing.T) {
	sr := &stubRunner{done: make(chan string, 1)}
	srv, _ := newServer(t, sr)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))

	if r := do(t, srv, "POST", "/api/tasks/a/run-now", nil); r.Status != http.StatusAccepted {
		t.Fatalf("run-now status = %d", r.Status)
	}
	select {
	case id := <-sr.done:
		if id != "a" {
			t.Fatalf("runner called with %q, want a", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner was not invoked")
	}
}

func TestAddTaskGeneratesID(t *testing.T) {
	srv, _ := newServer(t, nil)
	// No id supplied — the server must generate one.
	body := map[string]any{"name": "My Nightly Build", "prompt": "p", "working_dir": "/r", "trigger": "asap"}
	if r := do(t, srv, "POST", "/api/tasks", body); r.Status != http.StatusCreated {
		t.Fatalf("add status = %d (%s)", r.Status, r.Body)
	}
	var tasks []task.Task
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].ID == "" {
		t.Fatalf("expected a task with a generated id, got %+v", tasks)
	}
	if !strings.HasPrefix(tasks[0].ID, "my-nightly-build-") {
		t.Fatalf("generated id %q should derive from the name", tasks[0].ID)
	}
}

func TestListDir(t *testing.T) {
	srv, _ := newServer(t, nil)
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/sub-a", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir+"/.hidden", 0o755); err != nil {
		t.Fatal(err)
	}
	var listing dirListing
	do(t, srv, "GET", "/api/fs?path="+dir, nil).into(t, &listing)
	if listing.Path != dir {
		t.Fatalf("path = %q, want %q", listing.Path, dir)
	}
	if len(listing.Dirs) != 1 || listing.Dirs[0] != "sub-a" {
		t.Fatalf("dirs = %v, want [sub-a] (hidden excluded)", listing.Dirs)
	}
}

func TestListDirBadPath(t *testing.T) {
	srv, _ := newServer(t, nil)
	if r := do(t, srv, "GET", "/api/fs?path=/no/such/path/xyz", nil); r.Status != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing path, got %d", r.Status)
	}
}

func TestGetUsage(t *testing.T) {
	srv, st := newServer(t, nil)
	// No snapshot yet -> 204.
	if r := do(t, srv, "GET", "/api/usage", nil); r.Status != http.StatusNoContent {
		t.Fatalf("expected 204 when no usage, got %d", r.Status)
	}
	if err := st.SaveUsage(store.Usage{Utilization: 0.5, Status: "allowed", LimitType: "overage"}); err != nil {
		t.Fatal(err)
	}
	var u store.Usage
	r := do(t, srv, "GET", "/api/usage", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("expected 200, got %d", r.Status)
	}
	r.into(t, &u)
	if u.Utilization != 0.5 || u.LimitType != "overage" {
		t.Fatalf("unexpected usage: %+v", u)
	}
}

func TestRefreshUsageEndpoint(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref := &stubRefresher{store: st, usage: store.Usage{Utilization: 0.6, Status: "allowed"}}
	srv := httptest.NewServer(Handler(Deps{Store: st, Refresher: ref}))
	t.Cleanup(srv.Close)

	var u store.Usage
	r := do(t, srv, "POST", "/api/usage/refresh", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("refresh status = %d (%s)", r.Status, r.Body)
	}
	if !ref.called {
		t.Fatal("refresher was not invoked")
	}
	r.into(t, &u)
	if u.Utilization != 0.6 {
		t.Fatalf("expected refreshed usage, got %+v", u)
	}
}

func TestRefreshUsageUnavailable(t *testing.T) {
	srv, _ := newServer(t, nil) // no refresher
	if r := do(t, srv, "POST", "/api/usage/refresh", nil); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without a refresher, got %d", r.Status)
	}
}

func TestGetStatsEndpoint(t *testing.T) {
	srv, st := newServer(t, nil)
	_ = st.AppendRun(store.Run{RunID: "r1", TaskName: "a", StartedAt: time.Now(), Status: store.StatusSuccess, CostUSD: 0.2, InputTokens: 10, OutputTokens: 5})
	var s Stats
	do(t, srv, "GET", "/api/stats", nil).into(t, &s)
	if s.Totals.Runs != 1 || s.Totals.Success != 1 {
		t.Fatalf("unexpected stats: %+v", s.Totals)
	}
}

func TestListModels(t *testing.T) {
	srv, _ := newServer(t, nil)
	var models []Model
	do(t, srv, "GET", "/api/models", nil).into(t, &models)
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
	for _, m := range models {
		if m.ID == "" || m.Label == "" {
			t.Fatalf("model missing id/label: %+v", m)
		}
	}
}

func TestServesDashboard(t *testing.T) {
	srv, _ := newServer(t, nil)
	r := do(t, srv, "GET", "/", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("dashboard status = %d", r.Status)
	}
	if !bytes.Contains(r.Body, []byte("claudeq")) {
		t.Fatal("dashboard HTML should mention claudeq")
	}
}
