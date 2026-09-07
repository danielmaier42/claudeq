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

// deliver runs one delivery pass and waits for the sends it started.
func deliver(e *Engine) {
	e.deliverTaskNotifications()
	e.WaitIdle()
}

func TestTaskNotificationsAreDeliveredOnceAndAttributed(t *testing.T) {
	e, st, n := newArtifactEngine(t)
	now := e.clock.Now()

	deliver(e) // empty outbox: nothing happens
	if got := n.all(); len(got) != 0 {
		t.Fatalf("empty outbox must stay silent, got %+v", got)
	}

	queueNotification(t, st, store.Notification{
		ID: "n1", Title: "Prod drifted", Message: "3 commits behind", URL: "https://example.com/deploys",
		TaskID: "watch", TaskName: "Prod watch", RunID: "r1", QueuedAt: now,
	})
	queueNotification(t, st, store.Notification{ID: "n2", Title: "Manual", Message: "sent from a shell", QueuedAt: now})

	deliver(e)

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
	deliver(e)
	if got := n.all(); len(got) != 2 {
		t.Fatalf("re-delivered on the next pass: %+v", got)
	}
	if pending, _ := st.PendingNotifications(); len(pending) != 0 {
		t.Fatalf("outbox not emptied: %+v", pending)
	}
}

func TestTaskNotificationIsCutToChannelLimitsAndLinkRechecked(t *testing.T) {
	long := strings.Repeat("x", 2000)
	got := taskNotification(store.Notification{
		Title: strings.Repeat("t", 300), Message: long, TaskName: "Watch", URL: "file:///etc/passwd",
	})
	if r := []rune(got.Title); len(r) != taskNotifyTitleLimit+1 || !strings.HasSuffix(got.Title, "…") {
		t.Fatalf("title not cut to %d runes: len %d", taskNotifyTitleLimit, len(r))
	}
	if r := []rune(got.Message); len(r) != taskNotifyBodyLimit+1 || !strings.HasSuffix(got.Message, "…") {
		t.Fatalf("body not cut to %d runes: len %d", taskNotifyBodyLimit, len(r))
	}
	if got.URL != "" {
		t.Fatalf("a non-web link in the outbox must be dropped at delivery, got %q", got.URL)
	}
	short := taskNotification(store.Notification{Title: "T", Message: "M", TaskName: "Watch", URL: "https://ok.example/x"})
	if short.Title != "T" || short.Message != "M\n· from Watch" || short.URL != "https://ok.example/x" {
		t.Fatalf("short notification altered: %+v", short)
	}
}

func TestStaleTaskNotificationsAreDroppedNotDelivered(t *testing.T) {
	e, st, n := newArtifactEngine(t)
	now := e.clock.Now()
	queueNotification(t, st, store.Notification{ID: "old", Title: "Old news", Message: "M", QueuedAt: now.Add(-taskNotifyMaxAge - time.Minute)})
	queueNotification(t, st, store.Notification{ID: "fresh", Title: "Fresh", Message: "M", QueuedAt: now.Add(-time.Hour)})

	deliver(e)

	got := n.all()
	if len(got) != 1 || got[0].Title != "Fresh" {
		t.Fatalf("only the fresh notification must go out, got %+v", got)
	}
	if pending, _ := st.PendingNotifications(); len(pending) != 0 {
		t.Fatalf("stale entries must leave the outbox too: %+v", pending)
	}
}

func TestTaskNotificationsSurviveWithoutANotifier(t *testing.T) {
	e, st := newTestEngine(t, &stub{}, clock.NewFake(time.Now()))
	queueNotification(t, st, store.Notification{ID: "n1", Title: "T", Message: "M", QueuedAt: time.Now()})

	deliver(e) // no notifier configured: must not panic or drain

	if pending, _ := st.PendingNotifications(); len(pending) != 1 {
		t.Fatalf("without a notifier the outbox must be left alone, got %+v", pending)
	}
}

func TestTaskNotificationsRecoverFromACorruptOutbox(t *testing.T) {
	e, st, n := newArtifactEngine(t)
	if err := os.WriteFile(st.Home()+"/notifications.json", []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	deliver(e) // logs once, sends nothing, sets the file aside
	if got := n.all(); len(got) != 0 {
		t.Fatalf("corrupt outbox must not produce notifications: %+v", got)
	}
	if e.lastNotifyErr == "" {
		t.Fatal("the failure should be remembered so it is logged once")
	}

	// The outbox is usable again right away.
	queueNotification(t, st, store.Notification{ID: "n1", Title: "After", Message: "M", QueuedAt: e.clock.Now()})
	deliver(e)
	if got := n.all(); len(got) != 1 || got[0].Title != "After" {
		t.Fatalf("delivery after quarantine: %+v", got)
	}
	if e.lastNotifyErr != "" {
		t.Fatal("a successful pass clears the remembered failure")
	}
}
