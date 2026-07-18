package store

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func sampleTask(id string) task.Task {
	return task.Task{
		ID:          id,
		Name:        id,
		Prompt:      "do " + id,
		WorkingDir:  "/repo/" + id,
		Trigger:     task.TriggerASAP,
		Enabled:     true,
		Permissions: task.PermissionsDefault,
	}
}

func TestLoadConfigMissingReturnsEmpty(t *testing.T) {
	s := openTemp(t)
	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Tasks) != 0 {
		t.Fatalf("expected no tasks, got %d", len(cfg.Tasks))
	}
}

func TestSaveLoadConfigRoundTripPreservesOrder(t *testing.T) {
	s := openTemp(t)
	in := Config{
		Settings: Settings{
			DefaultModel:           "claude-opus-4-8",
			SkipPermissionsDefault: true,
			Pushover:               Pushover{Token: "tok", UserKey: "usr"},
		},
		Tasks: []task.Task{sampleTask("a"), sampleTask("b"), sampleTask("c")},
	}
	if err := s.SaveConfig(in); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	out, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if out.Settings != in.Settings {
		t.Fatalf("settings mismatch: %+v vs %+v", out.Settings, in.Settings)
	}
	if len(out.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(out.Tasks))
	}
	for i, want := range []string{"a", "b", "c"} {
		if out.Tasks[i].ID != want {
			t.Fatalf("task %d id = %q, want %q (order not preserved)", i, out.Tasks[i].ID, want)
		}
	}
}

func TestSaveConfigRejectsInvalidTask(t *testing.T) {
	s := openTemp(t)
	bad := sampleTask("x")
	bad.Prompt = ""
	if err := s.SaveConfig(Config{Tasks: []task.Task{bad}}); err == nil {
		t.Fatal("expected error saving invalid task")
	}
}

func TestSaveConfigRejectsDuplicateIDs(t *testing.T) {
	s := openTemp(t)
	err := s.SaveConfig(Config{Tasks: []task.Task{sampleTask("dup"), sampleTask("dup")}})
	if err == nil {
		t.Fatal("expected error on duplicate ids")
	}
}

func TestRunHistoryCollapsesToLatest(t *testing.T) {
	s := openTemp(t)
	start := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	fin := start.Add(time.Minute)

	if err := s.AppendRun(Run{RunID: "r1", TaskID: "a", StartedAt: start, Status: StatusRunning}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	if err := s.AppendRun(Run{RunID: "r2", TaskID: "b", StartedAt: start, Status: StatusRunning}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	if err := s.AppendRun(Run{RunID: "r1", TaskID: "a", StartedAt: start, FinishedAt: &fin, Status: StatusSuccess}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}

	runs, err := s.Runs()
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 collapsed runs, got %d", len(runs))
	}
	// First-seen order preserved: r1 then r2.
	if runs[0].RunID != "r1" || runs[1].RunID != "r2" {
		t.Fatalf("order = %q,%q; want r1,r2", runs[0].RunID, runs[1].RunID)
	}
	if runs[0].Status != StatusSuccess {
		t.Fatalf("r1 status = %q, want latest %q", runs[0].Status, StatusSuccess)
	}
}

func TestStateReadStatusPersists(t *testing.T) {
	s := openTemp(t)
	st, err := s.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st.IsRead("r1") {
		t.Fatal("run should start unread")
	}
	st.MarkRead("r1")
	st.RecordStart("taskA", time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	st.MarkCompletedOnce("taskA")
	if err := s.SaveState(st); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	reloaded, err := s.LoadState()
	if err != nil {
		t.Fatalf("LoadState (reload): %v", err)
	}
	if !reloaded.IsRead("r1") {
		t.Fatal("read status did not persist")
	}
	if !reloaded.IsCompletedOnce("taskA") {
		t.Fatal("completed-once did not persist")
	}
	if _, ok := reloaded.LastStart("taskA"); !ok {
		t.Fatal("last-start did not persist")
	}
}

func TestUpdateConfigConcurrentAddsNoLoss(t *testing.T) {
	s := openTemp(t)
	const n = 20
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.UpdateConfig(func(cfg *Config) error {
				cfg.Tasks = append(cfg.Tasks, sampleTask(fmt.Sprintf("t%02d", i)))
				return nil
			})
		}(i)
	}
	wg.Wait()

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Tasks) != n {
		t.Fatalf("lost updates under concurrency: got %d tasks, want %d", len(cfg.Tasks), n)
	}
}

func TestUpdateStatePreservesOtherKeys(t *testing.T) {
	s := openTemp(t)
	// One writer sets read-status (as the API would)...
	if err := s.UpdateState(func(st *State) error { st.MarkRead("r1"); return nil }); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	// ...another touches only scheduling keys (as the engine would).
	if err := s.UpdateState(func(st *State) error { st.RecordStart("taskA", time.Now()); return nil }); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	st, _ := s.LoadState()
	if !st.IsRead("r1") {
		t.Fatal("read-status was clobbered by an unrelated state update")
	}
	if _, ok := st.LastStart("taskA"); !ok {
		t.Fatal("last-start not persisted")
	}
}

func TestDefaultHomeHonoursEnv(t *testing.T) {
	t.Setenv(EnvHome, "/tmp/claudeq-test-home")
	home, err := DefaultHome()
	if err != nil {
		t.Fatalf("DefaultHome: %v", err)
	}
	if home != "/tmp/claudeq-test-home" {
		t.Fatalf("DefaultHome = %q, want override", home)
	}
}

func TestReconcileRunningRuns(t *testing.T) {
	s := openTemp(t)
	now := time.Now()
	if err := s.AppendRun(Run{RunID: "r1", TaskID: "a", Status: StatusRunning, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendRun(Run{RunID: "r2", TaskID: "b", Status: StatusSuccess, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	n, err := s.ReconcileRunningRuns(now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reconciled %d, want 1", n)
	}
	runs, _ := s.Runs()
	got := map[string]Run{}
	for _, r := range runs {
		got[r.RunID] = r
	}
	if got["r1"].Status != StatusFailed || got["r1"].FinishedAt == nil {
		t.Fatalf("r1 = %+v, want failed with finish time", got["r1"])
	}
	if got["r2"].Status != StatusSuccess {
		t.Fatalf("r2 status = %q, want untouched success", got["r2"].Status)
	}
}

func TestPruneHistoryKeepsRecentAndDeletesLogs(t *testing.T) {
	s := openTemp(t)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("r%d", i)
		if err := s.AppendRun(Run{RunID: id, TaskID: "a", Status: StatusSuccess, LogPath: s.LogPath(id)}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.LogPath(id), []byte("log"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PruneHistory(2); err != nil {
		t.Fatal(err)
	}
	runs, _ := s.Runs()
	if len(runs) != 2 || runs[0].RunID != "r3" || runs[1].RunID != "r4" {
		t.Fatalf("kept %v, want [r3 r4]", runs)
	}
	if _, err := os.Stat(s.LogPath("r0")); !os.IsNotExist(err) {
		t.Fatal("dropped run's log should be deleted")
	}
	if _, err := os.Stat(s.LogPath("r4")); err != nil {
		t.Fatal("kept run's log should remain")
	}
}

func TestIdleTimeoutAndHistoryDefaults(t *testing.T) {
	var s Settings
	if s.IdleTimeout() != 30*time.Minute {
		t.Fatalf("default idle = %v, want 30m", s.IdleTimeout())
	}
	s.IdleTimeoutMinutes = -1
	if s.IdleTimeout() != 0 {
		t.Fatal("negative idle should disable")
	}
	s.IdleTimeoutMinutes = 45
	if s.IdleTimeout() != 45*time.Minute {
		t.Fatalf("explicit idle = %v, want 45m", s.IdleTimeout())
	}

	var h Settings
	if h.RunHistoryLimit() != 500 {
		t.Fatalf("default history = %d, want 500", h.RunHistoryLimit())
	}
	h.MaxRunHistory = -1
	if h.RunHistoryLimit() != 0 {
		t.Fatal("negative history should mean unlimited (0)")
	}
	h.MaxRunHistory = 100
	if h.RunHistoryLimit() != 100 {
		t.Fatalf("explicit history = %d, want 100", h.RunHistoryLimit())
	}
}
