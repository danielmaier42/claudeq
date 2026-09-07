package engine

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/store"
)

func queueNotification(t *testing.T, st *store.Store, n store.Notification) {
	t.Helper()
	if err := st.QueueNotification(n); err != nil {
		t.Fatalf("QueueNotification: %v", err)
	}
}

func TestTaskNotificationsAreDeliveredOnceAndAttributed(t *testing.T) {
	e, st, n := newArtifactEngine(t)

	e.deliverTaskNotifications() // empty outbox: nothing happens
	if got := n.all(); len(got) != 0 {
		t.Fatalf("empty outbox must stay silent, got %+v", got)
	}

	queueNotification(t, st, store.Notification{
		ID: "n1", Title: "Prod drifted", Message: "3 commits behind", URL: "https://example.com/deploys",
		TaskID: "watch", TaskName: "Prod watch", RunID: "r1", QueuedAt: time.Now(),
	})
	queueNotification(t, st, store.Notification{ID: "n2", Title: "Manual", Message: "sent from a shell", QueuedAt: time.Now()})

	e.deliverTaskNotifications()

	got := n.all()
	if len(got) != 2 {
		t.Fatalf("got %d notification(s), want 2: %+v", len(got), got)
	}
	if got[0].Title != "Prod drifted" || got[0].URL != "https://example.com/deploys" || got[0].ArtifactID != "" {
		t.Fatalf("first notification = %+v", got[0])
	}
	if !strings.HasPrefix(got[0].Message, "3 commits behind") || !strings.Contains(got[0].Message, "Prod watch") {
		t.Fatalf("message must carry the text and the sending task, got %q", got[0].Message)
	}
	if got[1].Message != "sent from a shell" {
		t.Fatalf("a notification without a task is sent as written, got %q", got[1].Message)
	}

	// Delivered means gone: the next pass sends nothing.
	e.deliverTaskNotifications()
	if got := n.all(); len(got) != 2 {
		t.Fatalf("re-delivered on the next pass: %+v", got)
	}
	if pending, _ := st.PendingNotifications(); len(pending) != 0 {
		t.Fatalf("outbox not emptied: %+v", pending)
	}
}

func TestTaskNotificationsSurviveWithoutANotifier(t *testing.T) {
	e, st := newTestEngine(t, &stub{}, clock.NewFake(time.Now()))
	queueNotification(t, st, store.Notification{ID: "n1", Title: "T", Message: "M", QueuedAt: time.Now()})

	e.deliverTaskNotifications() // no notifier configured: must not panic or drain

	if pending, _ := st.PendingNotifications(); len(pending) != 1 {
		t.Fatalf("without a notifier the outbox must be left alone, got %+v", pending)
	}
}

func TestTaskNotificationsToleratesACorruptOutbox(t *testing.T) {
	e, st, n := newArtifactEngine(t)
	if err := os.WriteFile(st.Home()+"/notifications.json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.deliverTaskNotifications() // logs once, sends nothing, keeps ticking
	e.deliverTaskNotifications()
	if got := n.all(); len(got) != 0 {
		t.Fatalf("corrupt outbox must not produce notifications: %+v", got)
	}
	if e.lastNotifyErr == "" {
		t.Fatal("the failure should be remembered so it is logged once")
	}
}
