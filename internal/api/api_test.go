package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
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

func TestWarmFileAccessOnAddAndUpdate(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	warmed := make(chan []string, 4)
	srv := httptest.NewServer(Handler(Deps{
		Store:          st,
		WarmFileAccess: func(dirs []string) { warmed <- dirs },
	}))
	t.Cleanup(srv.Close)

	add := sampleTask("a")
	add.WorkingDir = "/tmp/added"
	if r := do(t, srv, "POST", "/api/tasks", add); r.Status != http.StatusCreated {
		t.Fatalf("add status = %d", r.Status)
	}
	if dirs := waitWarm(t, warmed); len(dirs) != 1 || dirs[0] != "/tmp/added" {
		t.Fatalf("warm on add = %v, want [/tmp/added]", dirs)
	}

	upd := sampleTask("a")
	upd.WorkingDir = "/tmp/changed"
	if r := do(t, srv, "PUT", "/api/tasks/a", upd); r.Status != http.StatusOK {
		t.Fatalf("update status = %d", r.Status)
	}
	if dirs := waitWarm(t, warmed); len(dirs) != 1 || dirs[0] != "/tmp/changed" {
		t.Fatalf("warm on update = %v, want [/tmp/changed]", dirs)
	}
}

// waitWarm returns the next WarmFileAccess call's dirs, failing if none arrives
// (the hook fires in a goroutine, so the test must wait for it).
func waitWarm(t *testing.T, ch <-chan []string) []string {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("WarmFileAccess was not called")
		return nil
	}
}

// assertNoWarm fails if WarmFileAccess is called within a short window. The hook
// fires in a goroutine, so we give it a moment to (wrongly) arrive.
func assertNoWarm(t *testing.T, ch <-chan []string) {
	t.Helper()
	select {
	case d := <-ch:
		t.Fatalf("WarmFileAccess should not have been called, got %v", d)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestUpdateWarmsOnlyWhenDirChanges verifies the folder-change guard: editing a
// task without touching its working directory does not re-probe (the folder is
// already authorised), while a genuine folder change warms — even for a disabled
// task, since its scheduled run will still need the grant.
func TestUpdateWarmsOnlyWhenDirChanges(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	warmed := make(chan []string, 4)
	srv := httptest.NewServer(Handler(Deps{
		Store:          st,
		WarmFileAccess: func(dirs []string) { warmed <- dirs },
	}))
	t.Cleanup(srv.Close)

	add := sampleTask("a")
	add.WorkingDir = "/tmp/dir"
	if r := do(t, srv, "POST", "/api/tasks", add); r.Status != http.StatusCreated {
		t.Fatalf("add status = %d", r.Status)
	}
	waitWarm(t, warmed) // drain the add-time warm

	// Edit only the prompt, same folder → no warm.
	same := sampleTask("a")
	same.WorkingDir = "/tmp/dir"
	same.Prompt = "different prompt"
	if r := do(t, srv, "PUT", "/api/tasks/a", same); r.Status != http.StatusOK {
		t.Fatalf("update status = %d", r.Status)
	}
	assertNoWarm(t, warmed)

	// Change the folder on a now-disabled task → still warms the new folder.
	moved := sampleTask("a")
	moved.WorkingDir = "/tmp/moved"
	moved.Enabled = false
	if r := do(t, srv, "PUT", "/api/tasks/a", moved); r.Status != http.StatusOK {
		t.Fatalf("update status = %d", r.Status)
	}
	if dirs := waitWarm(t, warmed); len(dirs) != 1 || dirs[0] != "/tmp/moved" {
		t.Fatalf("warm on dir change = %v, want [/tmp/moved]", dirs)
	}
}

func TestWarmNowWarmsEnabledTaskFolders(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Seed directly through the store (not the API) so no per-task warm fires and
	// the only hook call we observe is the explicit /api/fs/warm one.
	seed := func(id, dir string, enabled bool) {
		tk := sampleTask(id)
		tk.WorkingDir = dir
		tk.Enabled = enabled
		if err := app.AddTask(st, tk); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("a", "/tmp/a", true)
	seed("b", "/tmp/b", false) // disabled → must be excluded
	seed("c", "/tmp/c", true)

	warmed := make(chan []string, 4)
	srv := httptest.NewServer(Handler(Deps{
		Store:          st,
		WarmFileAccess: func(dirs []string) { warmed <- dirs },
	}))
	t.Cleanup(srv.Close)

	if r := do(t, srv, "POST", "/api/fs/warm", nil); r.Status != http.StatusAccepted {
		t.Fatalf("warm status = %d, want 202", r.Status)
	}
	dirs := waitWarm(t, warmed)
	if len(dirs) != 2 || dirs[0] != "/tmp/a" || dirs[1] != "/tmp/c" {
		t.Fatalf("warmed dirs = %v, want [/tmp/a /tmp/c] (enabled only)", dirs)
	}
}

func TestWarmNowNoHookIsNoContent(t *testing.T) {
	srv, _ := newServer(t, nil) // Deps without WarmFileAccess
	if r := do(t, srv, "POST", "/api/fs/warm", nil); r.Status != http.StatusNoContent {
		t.Fatalf("warm status = %d, want 204 when no hook wired", r.Status)
	}
}

func TestListTasksHidesRunning(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, ActiveTasks: func() []string { return []string{"a"} }}))
	t.Cleanup(srv.Close)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	do(t, srv, "POST", "/api/tasks", sampleTask("b"))

	var tasks []task.Task
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "b" {
		t.Fatalf("running task 'a' should be hidden from the queue, got %v", tasks)
	}
}

func TestListTasksKeepsRunningCron(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, ActiveTasks: func() []string { return []string{"c"} }}))
	t.Cleanup(srv.Close)
	cron := sampleTask("c")
	cron.Trigger = task.TriggerCron
	cron.Cron = "0 20 * * *"
	do(t, srv, "POST", "/api/tasks", cron)

	var tasks []struct {
		task.Task
		Running bool `json:"running"`
	}
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].ID != "c" {
		t.Fatalf("running cron task should stay in the queue, got %v", tasks)
	}
	if !tasks[0].Running {
		t.Fatal("running cron task should be flagged running")
	}
}

func TestListTasksReportsCronNextRun(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st}))
	t.Cleanup(srv.Close)
	cron := sampleTask("c")
	cron.Trigger = task.TriggerCron
	cron.Cron = "0 20 * * *"
	do(t, srv, "POST", "/api/tasks", cron)

	asap := sampleTask("a") // default trigger, no schedule
	do(t, srv, "POST", "/api/tasks", asap)

	var tasks []struct {
		ID      string     `json:"id"`
		NextRun *time.Time `json:"next_run"`
	}
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)

	byID := map[string]*time.Time{}
	for _, tk := range tasks {
		byID[tk.ID] = tk.NextRun
	}
	if byID["c"] == nil {
		t.Fatal("cron task should report a next_run")
	}
	if !byID["c"].After(time.Now()) {
		t.Fatalf("cron next_run should be in the future, got %v", byID["c"])
	}
	if byID["a"] != nil {
		t.Fatalf("non-cron task should not report next_run, got %v", byID["a"])
	}
}

func TestHealthReportsWakeError(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, WakeError: func() string { return "pmset: sudo password required" }}))
	t.Cleanup(srv.Close)

	var got map[string]string
	do(t, srv, "GET", "/api/health", nil).into(t, &got)
	if got["wake_error"] != "pmset: sudo password required" {
		t.Fatalf("wake_error = %q, want the dep's value", got["wake_error"])
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

func TestUpdateTask(t *testing.T) {
	srv, st := newServer(t, nil)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))

	edited := sampleTask("a")
	edited.Name = "Renamed"
	edited.Prompt = "new prompt"
	if r := do(t, srv, "PUT", "/api/tasks/a", edited); r.Status != http.StatusOK {
		t.Fatalf("update status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 1 || cfg.Tasks[0].Name != "Renamed" || cfg.Tasks[0].Prompt != "new prompt" {
		t.Fatalf("task not updated: %+v", cfg.Tasks)
	}

	// Updating a missing task fails.
	if r := do(t, srv, "PUT", "/api/tasks/missing", sampleTask("missing")); r.Status != http.StatusBadRequest {
		t.Fatalf("expected 400 updating missing task, got %d", r.Status)
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

type stubCanceler struct {
	got string
	err error
}

func (s *stubCanceler) CancelRun(runID string) error { s.got = runID; return s.err }

func TestCancelRunEndpoint(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	sc := &stubCanceler{}
	srv := httptest.NewServer(Handler(Deps{Store: st, Canceler: sc}))
	t.Cleanup(srv.Close)

	if r := do(t, srv, "POST", "/api/runs/run-1/cancel", nil); r.Status != http.StatusNoContent {
		t.Fatalf("cancel status = %d (%s)", r.Status, r.Body)
	}
	if sc.got != "run-1" {
		t.Fatalf("canceler called with %q, want run-1", sc.got)
	}

	// A run that is not in flight yields 409.
	sc.err = errors.New("run \"run-1\" is not running")
	if r := do(t, srv, "POST", "/api/runs/run-1/cancel", nil); r.Status != http.StatusConflict {
		t.Fatalf("cancel of finished run status = %d", r.Status)
	}
}

func TestCancelRunUnavailable(t *testing.T) {
	srv, _ := newServer(t, nil) // no Canceler wired
	if r := do(t, srv, "POST", "/api/runs/run-1/cancel", nil); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("cancel without canceler status = %d", r.Status)
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
	// A new task must be enabled by default even if the client omits the flag,
	// otherwise the scheduler would skip it (only "run now" would work).
	if !tasks[0].Enabled {
		t.Fatal("newly added task should be enabled by default")
	}
}

func TestChooseFolder(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	chooser := func(_ context.Context, _ string) (string, bool, error) { return "/Users/me/proj", true, nil }
	srv := httptest.NewServer(Handler(Deps{Store: st, ChooseFolder: chooser}))
	t.Cleanup(srv.Close)

	var res struct {
		Path string `json:"path"`
	}
	do(t, srv, "POST", "/api/fs/choose", nil).into(t, &res)
	if res.Path != "/Users/me/proj" {
		t.Fatalf("path = %q, want /Users/me/proj", res.Path)
	}

	// Cancellation -> 204.
	cancelSrv := httptest.NewServer(Handler(Deps{Store: st, ChooseFolder: func(_ context.Context, _ string) (string, bool, error) { return "", false, nil }}))
	t.Cleanup(cancelSrv.Close)
	if r := do(t, cancelSrv, "POST", "/api/fs/choose", nil); r.Status != http.StatusNoContent {
		t.Fatalf("cancel should be 204, got %d", r.Status)
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
