package store

import (
	"errors"
	"os"
	"testing"
	"time"
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
	if _, err := os.Stat(s.path(notificationsFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("an emptied outbox must be removed, so the daemon's next check is a cheap ENOENT")
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

func TestTakeNotificationsSetsACorruptOutboxAside(t *testing.T) {
	s := openTemp(t)
	if err := os.WriteFile(s.path(notificationsFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakeNotifications(); err == nil {
		t.Fatal("a corrupt outbox must be reported")
	}
	if _, err := os.Stat(s.path(notificationsFile + ".corrupt")); err != nil {
		t.Fatal("the damaged file must be kept aside for inspection")
	}
	// From here on the outbox works again.
	if err := s.QueueNotification(queued("n1")); err != nil {
		t.Fatalf("queue after quarantine: %v", err)
	}
	if taken, err := s.TakeNotifications(); err != nil || len(taken) != 1 {
		t.Fatalf("take after quarantine = %v, %v", taken, err)
	}
}
