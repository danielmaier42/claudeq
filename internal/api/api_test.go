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

func TestAddTaskRejectsUnsafeID(t *testing.T) {
	srv, st := newServer(t, nil)
	bad := sampleTask("team/nightly")
	if r := do(t, srv, "POST", "/api/tasks", bad); r.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", r.Status)
	}
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Errorf("unsafe id was stored: %+v", cfg.Tasks)
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

func TestListTasksReportsCronLastRun(t *testing.T) {
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

	var tasks []struct {
		ID      string     `json:"id"`
		LastRun *time.Time `json:"last_run"`
	}
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].LastRun != nil {
		t.Fatalf("a cron task that never ran should report no last_run, got %v", tasks)
	}

	ran := time.Now().Add(-90 * time.Minute).Truncate(time.Second)
	if err := st.UpdateState(func(cur *store.State) error {
		// The scheduling anchor alone must not count as an execution.
		cur.RecordStart("c", time.Now())
		cur.RecordRun("c", ran)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tasks = nil
	do(t, srv, "GET", "/api/tasks", nil).into(t, &tasks)
	if len(tasks) != 1 || tasks[0].LastRun == nil {
		t.Fatalf("cron task should report a last_run, got %v", tasks)
	}
	if !tasks[0].LastRun.Equal(ran) {
		t.Fatalf("last_run = %v, want %v", tasks[0].LastRun, ran)
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

func TestHealthReportsNotifyStatus(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, NotifyStatus: func() string { return "denied" }}))
	t.Cleanup(srv.Close)

	var got map[string]string
	do(t, srv, "GET", "/api/health", nil).into(t, &got)
	if got["notify_status"] != "denied" {
		t.Fatalf("notify_status = %q, want the dep's value", got["notify_status"])
	}
}

// Without the dep (a build that cannot ask macOS) health must still answer, with
// an empty status rather than a made-up one.
func TestHealthWithoutNotifyStatusDep(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st}))
	t.Cleanup(srv.Close)

	var got map[string]string
	do(t, srv, "GET", "/api/health", nil).into(t, &got)
	if got["notify_status"] != "" {
		t.Fatalf("notify_status = %q, want empty", got["notify_status"])
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
	in := store.Settings{DefaultModel: "claude-opus-4-8", HeartbeatMinutes: 30}
	if r := do(t, srv, "PUT", "/api/settings", in); r.Status != http.StatusOK {
		t.Fatalf("put status = %d", r.Status)
	}
	var out store.Settings
	do(t, srv, "GET", "/api/settings", nil).into(t, &out)
	if out.DefaultModel != "claude-opus-4-8" || out.HeartbeatMinutes != 30 {
		t.Fatalf("settings round-trip mismatch: %+v", out)
	}
}

func TestPutSettingsRefusesAnEnabledChannelThatCannotDeliver(t *testing.T) {
	srv, st := newServer(t, nil)
	body := map[string]any{"webhook": map[string]any{"enabled": true, "url": ""}}
	r := do(t, srv, "PUT", "/api/settings", body)
	if r.Status != http.StatusBadRequest {
		t.Fatalf("put status = %d, want 400 (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Settings.Webhook.Enabled {
		t.Fatal("a rejected settings payload must not be persisted")
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

// continueFixture seeds a store with one run and returns a server whose
// TerminalOpener records its invocation. The run's fields are shaped by
// mutate, the stored settings by mutateSettings (both optional).
func continueFixture(t *testing.T, mutate func(*store.Run), mutateSettings func(*store.Settings)) (*httptest.Server, *struct {
	dir  string
	argv []string
}) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	// A fixed binary path keeps the expected argv deterministic (no host detection).
	cfg, _ := st.LoadConfig()
	cfg.Settings.ClaudePath = "/opt/claude"
	if mutateSettings != nil {
		mutateSettings(&cfg.Settings)
	}
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	tk := sampleTask("a")
	tk.WorkingDir = t.TempDir() // must exist on disk for the endpoint's check
	run := store.Run{RunID: "r1", TaskID: "a", TaskName: "a", StartedAt: time.Now(),
		Status: store.StatusSuccess, SessionID: "sess-1", Task: &tk}
	if mutate != nil {
		mutate(&run)
	}
	if err := st.AppendRun(run); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	got := &struct {
		dir  string
		argv []string
	}{}
	opener := func(_ context.Context, dir string, argv []string) error {
		got.dir, got.argv = dir, argv
		return nil
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, OpenTerminal: opener}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestContinueRunOpensTerminal(t *testing.T) {
	srv, got := continueFixture(t, nil, nil)
	if r := do(t, srv, "POST", "/api/runs/r1/continue", nil); r.Status != http.StatusNoContent {
		t.Fatalf("continue status = %d (%s)", r.Status, r.Body)
	}
	if got.dir == "" || !strings.HasPrefix(got.dir, "/") {
		t.Fatalf("opener dir = %q, want the task's working dir", got.dir)
	}
	want := []string{"/opt/claude", "--resume", "sess-1"}
	if len(got.argv) != len(want) || got.argv[0] != want[0] || got.argv[1] != want[1] || got.argv[2] != want[2] {
		t.Fatalf("opener argv = %v, want %v", got.argv, want)
	}
}

// TestContinueRunPermissionFlag verifies the resume carries the same permission
// mode the unattended run had, which is the task's own setting.
func TestContinueRunPermissionFlag(t *testing.T) {
	const flag = "--dangerously-skip-permissions"
	cases := []struct {
		name  string
		perms task.Permissions
		want  bool
	}{
		{"task skips", task.PermissionsSkip, true},
		{"task keeps prompts", task.PermissionsDefault, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, got := continueFixture(t,
				func(r *store.Run) {
					tk := *r.Task
					tk.Permissions = c.perms
					r.Task = &tk
				}, nil)
			if r := do(t, srv, "POST", "/api/runs/r1/continue", nil); r.Status != http.StatusNoContent {
				t.Fatalf("continue status = %d (%s)", r.Status, r.Body)
			}
			has := false
			for _, a := range got.argv {
				if a == flag {
					has = true
				}
			}
			if has != c.want {
				t.Fatalf("argv %v: %s present = %t, want %t", got.argv, flag, has, c.want)
			}
		})
	}
}

func TestContinueRunGuards(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*store.Run)
		status int
	}{
		{"running run", func(r *store.Run) { r.Status = store.StatusRunning }, http.StatusConflict},
		{"rate-limited run auto-resumes", func(r *store.Run) { r.Status = store.StatusRateLimited }, http.StatusConflict},
		{"no session recorded", func(r *store.Run) { r.SessionID = "" }, http.StatusConflict},
		{"no task snapshot", func(r *store.Run) { r.Task = nil }, http.StatusConflict},
		{"working dir gone", func(r *store.Run) {
			tk := *r.Task
			tk.WorkingDir = "/nonexistent/claudeq-test"
			r.Task = &tk
		}, http.StatusConflict},
		{"failed run is continuable", func(r *store.Run) { r.Status = store.StatusFailed }, http.StatusNoContent},
		{"canceled run is continuable", func(r *store.Run) { r.Status = store.StatusCanceled }, http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := continueFixture(t, c.mutate, nil)
			if r := do(t, srv, "POST", "/api/runs/r1/continue", nil); r.Status != c.status {
				t.Fatalf("status = %d (%s), want %d", r.Status, r.Body, c.status)
			}
		})
	}
}

func TestContinueRunNotFound(t *testing.T) {
	srv, _ := continueFixture(t, nil, nil)
	if r := do(t, srv, "POST", "/api/runs/nope/continue", nil); r.Status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", r.Status)
	}
}

func TestContinueRunUnavailable(t *testing.T) {
	srv, _ := newServer(t, nil) // no TerminalOpener wired
	if r := do(t, srv, "POST", "/api/runs/r1/continue", nil); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("continue without opener status = %d", r.Status)
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

func TestPauseEndpointTogglesOnlyThatSetting(t *testing.T) {
	srv, st := newServer(t, nil)
	if err := st.SaveConfig(store.Config{Settings: store.Settings{
		DefaultModel: "opus", HeartbeatMinutes: 30,
		Pushover: store.Pushover{Enabled: true, Token: "tok", UserKey: "usr"},
	}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	r := do(t, srv, "POST", "/api/pause", map[string]bool{"paused": true})
	if r.Status != http.StatusOK {
		t.Fatalf("pause status = %d (%s)", r.Status, r.Body)
	}
	var got map[string]bool
	r.into(t, &got)
	if !got["paused"] {
		t.Fatalf("response = %v, want paused:true", got)
	}
	cfg, _ := st.LoadConfig()
	if !cfg.Settings.Paused {
		t.Fatal("pause was not persisted")
	}
	// The switch travels alone: everything else must survive it.
	if cfg.Settings.DefaultModel != "opus" || cfg.Settings.HeartbeatMinutes != 30 || cfg.Settings.Pushover.Token != "tok" {
		t.Fatalf("pause clobbered other settings: %+v", cfg.Settings)
	}

	if r := do(t, srv, "POST", "/api/pause", map[string]bool{"paused": false}); r.Status != http.StatusOK {
		t.Fatalf("resume status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Settings.Paused {
		t.Fatal("resume was not persisted")
	}
}

func TestPutSettingsCannotClobberThePauseSwitch(t *testing.T) {
	srv, st := newServer(t, nil)
	if r := do(t, srv, "POST", "/api/pause", map[string]bool{"paused": true}); r.Status != http.StatusOK {
		t.Fatalf("pause status = %d", r.Status)
	}
	// A settings form filled in before the pause was flipped carries paused:false.
	body := map[string]any{"default_model": "opus", "paused": false}
	if r := do(t, srv, "PUT", "/api/settings", body); r.Status != http.StatusOK {
		t.Fatalf("put settings = %d", r.Status)
	}
	cfg, _ := st.LoadConfig()
	if !cfg.Settings.Paused {
		t.Fatal("a stale settings payload resumed the queue")
	}
	if cfg.Settings.DefaultModel != "opus" {
		t.Fatalf("the rest of the payload was not applied: %+v", cfg.Settings)
	}
}

func TestSettingsExposePausedState(t *testing.T) {
	srv, st := newServer(t, nil)
	if err := st.SaveConfig(store.Config{Settings: store.Settings{Paused: true}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	var s store.Settings
	do(t, srv, "GET", "/api/settings", nil).into(t, &s)
	if !s.Paused {
		t.Fatal("GET /api/settings did not report the pause state")
	}
}

func TestRunNowRefusedWhilePaused(t *testing.T) {
	sr := &stubRunner{done: make(chan string, 1)}
	srv, _ := newServer(t, sr)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	if r := do(t, srv, "POST", "/api/pause", map[string]bool{"paused": true}); r.Status != http.StatusOK {
		t.Fatalf("pause status = %d", r.Status)
	}

	r := do(t, srv, "POST", "/api/tasks/a/run-now", nil)
	if r.Status != http.StatusConflict {
		t.Fatalf("run-now while paused = %d, want 409 (%s)", r.Status, r.Body)
	}
	if !strings.Contains(string(r.Body), "paused") {
		t.Fatalf("409 body does not say why: %s", r.Body)
	}
	select {
	case id := <-sr.done:
		t.Fatalf("runner was invoked for %q while paused", id)
	case <-time.After(200 * time.Millisecond):
	}
}

// A rate-limited run is only "rescheduled" while the daemon still holds its
// session for a resume — that is what the dashboard labels and offers to cancel.
func TestRunsReportPendingResume(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	resume := start.Add(time.Hour)
	waiting := store.Run{
		RunID: "run-1", TaskID: "a", TaskName: "a", StartedAt: start,
		Status: store.StatusRateLimited, SessionID: "sess-1", ResumeAt: &resume,
	}
	stale := store.Run{
		RunID: "run-0", TaskID: "b", TaskName: "b", StartedAt: start,
		Status: store.StatusRateLimited, SessionID: "sess-0",
	}
	for _, r := range []store.Run{stale, waiting} {
		if err := st.AppendRun(r); err != nil {
			t.Fatalf("AppendRun: %v", err)
		}
	}
	if err := st.UpdateState(func(s *store.State) error {
		s.SetPendingResume("a", "sess-1")
		return nil
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	active := []string{}
	srv := httptest.NewServer(Handler(Deps{Store: st, ActiveTasks: func() []string { return active }}))
	t.Cleanup(srv.Close)

	get := func() map[string]runView {
		var views []runView
		do(t, srv, "GET", "/api/runs", nil).into(t, &views)
		byID := map[string]runView{}
		for _, v := range views {
			byID[v.RunID] = v
		}
		return byID
	}

	got := get()
	if !got["run-1"].ResumePending {
		t.Fatal("the run whose session is queued should report resume_pending")
	}
	if got["run-1"].ResumeAt == nil || !got["run-1"].ResumeAt.Equal(resume) {
		t.Fatalf("resume_at = %v, want %v", got["run-1"].ResumeAt, resume)
	}
	if got["run-0"].ResumePending {
		t.Fatal("a rate-limited run without a pending session must not report resume_pending")
	}

	// Once the task is running again the resume has happened; there is nothing
	// left to cancel on the old run.
	active = []string{"a"}
	if get()["run-1"].ResumePending {
		t.Fatal("a task that is running again must not report a pending resume")
	}
}

func TestTasksReportWaitingForLimit(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	waiting := task.Task{
		ID: "a", Name: "a", Prompt: "p", WorkingDir: "/repo",
		Trigger: task.TriggerCron, Cron: "*/30 * * * *", Enabled: true,
		Permissions: task.PermissionsDefault,
	}
	idle := waiting
	idle.ID, idle.Name = "b", "b"
	if err := st.SaveConfig(store.Config{Tasks: []task.Task{waiting, idle}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := st.UpdateState(func(s *store.State) error {
		s.SetPendingResume("a", "sess-1")
		return nil
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	srv := httptest.NewServer(Handler(Deps{Store: st}))
	t.Cleanup(srv.Close)

	var views []taskView
	do(t, srv, "GET", "/api/tasks", nil).into(t, &views)
	if len(views) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(views))
	}
	for _, v := range views {
		if want := v.ID == "a"; v.WaitingForLimit != want {
			t.Fatalf("task %q waiting_for_limit = %t, want %t", v.ID, v.WaitingForLimit, want)
		}
	}
}

func TestHealthReportsLimitedUntil(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	blocked := until
	srv := httptest.NewServer(Handler(Deps{Store: st, LimitedUntil: func() time.Time { return blocked }}))
	t.Cleanup(srv.Close)

	var got map[string]string
	do(t, srv, "GET", "/api/health", nil).into(t, &got)
	if got["limited_until"] != until.Format(time.RFC3339) {
		t.Fatalf("limited_until = %q, want %q", got["limited_until"], until.Format(time.RFC3339))
	}

	// An open gate reports nothing rather than a zero timestamp.
	blocked = time.Time{}
	do(t, srv, "GET", "/api/health", nil).into(t, &got)
	if got["limited_until"] != "" {
		t.Fatalf("limited_until = %q, want empty while the gate is open", got["limited_until"])
	}
}
