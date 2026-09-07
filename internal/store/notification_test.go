package store

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

func queued(id string) Notification {
	return Notification{
		ID: id, Title: "Deploy drifted", Message: "prod is 3 commits behind", URL: "https://example.com/x",
		TaskID: "watch", TaskName: "Prod watch", RunID: "r1",
		QueuedAt: time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC),
	}
}

func TestNotificationOutboxQueueAndTake(t *testing.T) {
	s := openTemp(t)

	pending, err := s.PendingNotifications()
	if err != nil || len(pending) != 0 {
		t.Fatalf("fresh store: pending = %v, err = %v; want none", pending, err)
	}
	if _, err := os.Stat(s.path(notificationsFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a read must not create the outbox file")
	}

	if err := s.QueueNotification(queued("n1")); err != nil {
		t.Fatalf("QueueNotification: %v", err)
	}
	if err := s.QueueNotification(queued("n2")); err != nil {
		t.Fatalf("QueueNotification: %v", err)
	}
	pending, err = s.PendingNotifications()
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 2 || pending[0].ID != "n1" || pending[1].ID != "n2" {
		t.Fatalf("pending = %+v, want n1 then n2", pending)
	}
	if got := pending[0]; got != queued("n1") {
		t.Fatalf("round trip lost fields:\ngot  %+v\nwant %+v", got, queued("n1"))
	}

	taken, err := s.TakeNotifications()
	if err != nil {
		t.Fatalf("TakeNotifications: %v", err)
	}
	if len(taken) != 2 || taken[0].ID != "n1" || taken[1].ID != "n2" {
		t.Fatalf("taken = %+v, want n1 then n2", taken)
	}
	pending, _ = s.PendingNotifications()
	if len(pending) != 0 {
		t.Fatalf("outbox not emptied by take: %+v", pending)
	}
	// Taking from an empty outbox is not an error either.
	if taken, err := s.TakeNotifications(); err != nil || len(taken) != 0 {
		t.Fatalf("empty take: %v, %v", taken, err)
	}
}

func TestNotificationOutboxRejectsBadIDs(t *testing.T) {
	s := openTemp(t)
	if err := s.QueueNotification(Notification{Title: "no id"}); err == nil {
		t.Fatal("missing id must be rejected")
	}
	if err := s.QueueNotification(queued("dup")); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueNotification(queued("dup")); err == nil {
		t.Fatal("duplicate id must be rejected")
	}
}

func TestDropRunRemovesEveryEventAndTheLog(t *testing.T) {
	s := openTemp(t)
	for _, id := range []string{"r1", "r2", "r3"} {
		if err := s.AppendRun(Run{RunID: id, TaskID: "a", Status: StatusRunning, LogPath: s.LogPath(id)}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.LogPath(id), []byte("log"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// r2 has two events (start + finish); both must go.
	if err := s.AppendRun(Run{RunID: "r2", TaskID: "a", Status: StatusSuccess, LogPath: s.LogPath("r2")}); err != nil {
		t.Fatal(err)
	}

	if err := s.DropRun("r2"); err != nil {
		t.Fatalf("DropRun: %v", err)
	}
	runs, err := s.Runs()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].RunID != "r1" || runs[1].RunID != "r3" {
		t.Fatalf("runs after drop = %+v, want [r1 r3]", runs)
	}
	if _, err := os.Stat(s.LogPath("r2")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("dropped run's log should be deleted")
	}
	if _, err := os.Stat(s.LogPath("r1")); err != nil {
		t.Fatal("other runs' logs must remain")
	}
	raw, _ := os.ReadFile(s.path(historyFile))
	if got := string(raw); len(got) == 0 || strings.Contains(got, `"run_id":"r2"`) {
		t.Fatalf("history still mentions r2:\n%s", got)
	}

	// Dropping something unknown (or already dropped) is a no-op.
	if err := s.DropRun("r2"); err != nil {
		t.Fatalf("second DropRun: %v", err)
	}
	if err := openTemp(t).DropRun("nope"); err != nil {
		t.Fatalf("DropRun on an empty store: %v", err)
	}
}

func TestRunQuiet(t *testing.T) {
	if (Run{}).Quiet() {
		t.Fatal("a run without a task snapshot is not quiet")
	}
	loud := Run{Task: &task.Task{ID: "a"}}
	if loud.Quiet() {
		t.Fatal("a run of an ordinary task is not quiet")
	}
	quiet := Run{Task: &task.Task{ID: "a", QuietHistory: true}}
	if !quiet.Quiet() {
		t.Fatal("a run of a quiet-history task is quiet")
	}
}
