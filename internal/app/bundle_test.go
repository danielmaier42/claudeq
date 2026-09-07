package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func TestExportImportRoundTrip(t *testing.T) {
	src, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig := task.Task{
		ID: "nightly", Name: "Nightly sweep", Prompt: "do the thing\n", WorkingDir: "/repo",
		Trigger: task.TriggerCron, Cron: "0 3 * * *", Parallel: true, Enabled: true,
		Model: "opus", Permissions: task.PermissionsSkip, NotifyOnResult: true,
	}
	if err := AddTask(src, orig); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	exported, err := ExportTask(src, "nightly", &buf, time.Now())
	if err != nil {
		t.Fatalf("ExportTask: %v", err)
	}
	if exported != orig {
		t.Errorf("ExportTask returned %+v", exported)
	}
	if _, err := ExportTask(src, "nope", &buf, time.Now()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown id: err = %v", err)
	}

	dst, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	read, err := bundle.Read(buf.Bytes())
	if err != nil {
		t.Fatalf("bundle.Read: %v", err)
	}
	got, err := ImportTask(dst, read)
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	if got != orig {
		t.Errorf("imported task differs:\n got %+v\nwant %+v", got, orig)
	}
	cfg, err := dst.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tasks) != 1 || cfg.Tasks[0] != orig {
		t.Errorf("stored tasks = %+v", cfg.Tasks)
	}
}

func TestImportTaskSuffixesTakenIDs(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := task.Task{ID: "nightly", Name: "N", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP, Permissions: task.PermissionsDefault}
	var ids []string
	for i := 0; i < 3; i++ {
		got, err := ImportTask(s, base)
		if err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
		ids = append(ids, got.ID)
	}
	if want := "nightly nightly-2 nightly-3"; strings.Join(ids, " ") != want {
		t.Errorf("ids = %q, want %q", strings.Join(ids, " "), want)
	}
}

func TestImportTaskFillsOnlyTheGaps(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ImportTask(s, task.Task{Name: "Weekly Report", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP, Enabled: false})
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	if got.ID != "weekly-report" || got.Permissions != task.PermissionsDefault || got.Enabled {
		t.Errorf("got %+v", got)
	}
	got, err = ImportTask(s, task.Task{ID: "only-id", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP})
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	if got.Name != "only-id" {
		t.Errorf("name = %q, want the id", got.Name)
	}
}

func TestImportTaskRejectsInvalid(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]task.Task{
		"no prompt":      {ID: "a", WorkingDir: "/r", Trigger: task.TriggerASAP},
		"no working dir": {ID: "a", Prompt: "p", Trigger: task.TriggerASAP},
		"bad trigger":    {ID: "a", Prompt: "p", WorkingDir: "/r", Trigger: "sometimes"},
		"bad cron":       {ID: "a", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerCron, Cron: "nope"},
		"unsafe id":      {ID: "team/nightly", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP},
		"dot id":         {ID: "..", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP},
	}
	for name, in := range cases {
		if _, err := ImportTask(s, in); err == nil {
			t.Errorf("%s: import succeeded", name)
		}
	}
	cfg, _ := s.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Errorf("invalid imports were stored: %+v", cfg.Tasks)
	}
}

// A re-imported id must not inherit a dead task's scheduling state: a
// completed-once flag would keep the new one-shot task from ever running, and
// a stale cron anchor would fire it immediately.
func TestImportTaskStartsFromCleanState(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := s.UpdateState(func(st *store.State) error {
		st.MarkCompletedOnce("nightly")
		st.RecordStart("nightly", stale)
		st.SetPendingResume("nightly", "sess")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ImportTask(s, task.Task{ID: "nightly", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP})
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.LastStart(got.ID); ok || st.IsCompletedOnce(got.ID) || st.PendingResume(got.ID) != "" {
		t.Errorf("stale state survived the import: %+v", st)
	}
}
