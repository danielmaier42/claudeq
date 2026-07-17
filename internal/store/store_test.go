package store

import (
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
