package engine

import (
	"context"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// Every notification is written to the inbox before it goes out, under the id
// the channels are handed: that is what lets a click on the macOS banner mark
// exactly this entry read.
func TestFailedRunIsRecordedInTheInboxWithItsRunAsTarget(t *testing.T) {
	fc := clock.NewFake(time.Now())
	r := &stub{result: func(_ executor.Request, _ int) provider.Result {
		return provider.Result{Status: store.StatusFailed, ExitCode: 2, Message: "run failed (exit 2)"}
	}}
	e, st := newTestEngine(t, r, fc)
	n := &capturingNotifier{}
	e.SetNotifier(n)
	saveTasks(t, st, asapTask("boom", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	msgs := n.all()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(msgs))
	}
	inbox, err := st.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox entry, got %d", len(inbox))
	}
	got := inbox[0]
	if got.ID == "" || got.ID != msgs[0].ID {
		t.Fatalf("inbox id %q, notification id %q: the channels must carry the inbox id", got.ID, msgs[0].ID)
	}
	if got.Kind != store.InboxKindFailure {
		t.Errorf("kind = %q, want %q", got.Kind, store.InboxKindFailure)
	}
	if got.Title != msgs[0].Title || got.Message != msgs[0].Message {
		t.Errorf("inbox entry %+v does not say what the notification said %+v", got, msgs[0])
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || got.RunID == "" || got.RunID != runs[0].RunID || msgs[0].RunID != runs[0].RunID {
		t.Errorf("run target = %q (notification %q), want the failed run %v", got.RunID, msgs[0].RunID, runs)
	}
	if got.TaskID != "boom" || got.TaskName != "boom" {
		t.Errorf("attribution = %q/%q, want the task", got.TaskID, got.TaskName)
	}
	if got.Read {
		t.Error("a freshly sent notification must be unread")
	}
	if !got.SentAt.Equal(fc.Now()) {
		t.Errorf("sent_at = %v, want the clock's %v", got.SentAt, fc.Now())
	}
}

func TestArtifactNotificationIsRecordedWithItsArtifactAsTarget(t *testing.T) {
	fc := clock.NewFake(time.Now())
	e, st := newTestEngine(t, &stub{}, fc)
	n := &capturingNotifier{}
	e.SetNotifier(n)
	// Prime: the first look at the artifact list notifies for nothing.
	e.notifyNewArtifacts()

	publish(t, st, artifact("a-1", "Nightly report", "nightly"))
	e.notifyNewArtifacts()

	inbox, err := st.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox entry, got %d: %+v", len(inbox), inbox)
	}
	if inbox[0].Kind != store.InboxKindArtifact || inbox[0].ArtifactID != "a-1" || inbox[0].RunID != "" {
		t.Errorf("entry = %+v, want an artifact notification pointing at a-1", inbox[0])
	}
	if inbox[0].TaskName != "nightly" {
		t.Errorf("task name = %q, want the publisher", inbox[0].TaskName)
	}
}

// A task's own notification is recorded as what the task said, pointing at
// its link; the sending run is attribution only, not a click target.
func TestTaskNotificationIsRecordedWithItsLink(t *testing.T) {
	fc := clock.NewFake(time.Now())
	e, st := newTestEngine(t, &stub{}, fc)
	n := &capturingNotifier{}
	e.SetNotifier(n)
	if err := st.QueueNotification(store.Notification{
		ID: "n-1", Title: "Prod drifted", Message: "3 commits behind", URL: "https://example.com/x",
		TaskID: "watch", TaskName: "Watcher", RunID: "r-9", QueuedAt: fc.Now(),
	}); err != nil {
		t.Fatalf("QueueNotification: %v", err)
	}
	e.deliverTaskNotifications()
	e.WaitIdle()

	inbox, err := st.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 inbox entry, got %d", len(inbox))
	}
	got := inbox[0]
	if got.Kind != store.InboxKindTask || got.URL != "https://example.com/x" || got.RunID != "" || got.ArtifactID != "" {
		t.Errorf("entry = %+v, want a task notification whose only target is its link", got)
	}
	if got.TaskID != "watch" || got.TaskName != "Watcher" {
		t.Errorf("attribution = %q/%q", got.TaskID, got.TaskName)
	}
}
