package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func TestStatusJSONNamesOriginErrorAndLog(t *testing.T) {
	st := newTestStore(t)
	start := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	runs := []store.Run{
		// The watcher itself: created by the operator, so it is its own origin.
		{RunID: "r1", TaskID: "watch", TaskName: "Watcher", StartedAt: start,
			Status: store.StatusSuccess, LogPath: "/logs/r1.log",
			Task: &task.Task{ID: "watch", Name: "Watcher"}},
		// A job the watcher queued, failing.
		{RunID: "r2", TaskID: "review-1", TaskName: "Review 1", StartedAt: start.Add(time.Minute),
			Status: store.StatusFailed, ExitCode: 1, Error: "boom", LogPath: "/logs/r2.log",
			WorkflowID: "wf", ParentRunID: "r1",
			Task: &task.Task{ID: "review-1", Name: "Review 1", OriginID: "watch", OriginName: "Watcher"}},
		// An old record without a task snapshot.
		{RunID: "r3", TaskID: "old", TaskName: "Old", StartedAt: start.Add(2 * time.Minute),
			Status: store.StatusCanceled, LogPath: "/logs/r3.log"},
	}
	for _, r := range runs {
		if err := st.AppendRun(r); err != nil {
			t.Fatalf("AppendRun: %v", err)
		}
	}
	if err := st.UpdateState(func(s *store.State) error { s.MarkRead("r1"); return nil }); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	out := captureStdout(t, func() error { return cmdStatus(st, []string{"--all", "--json"}) })
	var got []statusRun
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 runs, got %d:\n%s", len(got), out)
	}
	want := []struct {
		runID, origin, err, log string
		unread                  bool
	}{
		{"r1", "watch", "", "/logs/r1.log", false},
		{"r2", "watch", "boom", "/logs/r2.log", true},
		{"r3", "old", "", "/logs/r3.log", true},
	}
	for i, w := range want {
		g := got[i]
		if g.RunID != w.runID || g.OriginID != w.origin || g.Error != w.err || g.LogPath != w.log || g.Unread != w.unread {
			t.Errorf("run %d = %+v, want %+v", i, g, w)
		}
	}
	if got[1].TaskID != "review-1" || got[1].ParentRunID != "r1" || got[1].ExitCode != 1 || got[1].Status != store.StatusFailed {
		t.Errorf("run r2 lost fields: %+v", got[1])
	}
	if strings.Contains(out, "\"task\"") || strings.Contains(out, "final_output") {
		t.Errorf("status --json must not carry the task snapshot or the answer:\n%s", out)
	}
}

func TestStatusJSONWithoutRunsIsAnEmptyList(t *testing.T) {
	st := newTestStore(t)
	out := captureStdout(t, func() error { return cmdStatus(st, []string{"--json"}) })
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("want [], got %q", out)
	}
}
