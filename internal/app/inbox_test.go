package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

func inboxStore(t *testing.T, entries ...store.InboxEntry) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	for _, e := range entries {
		if err := s.AddInboxEntry(e); err != nil {
			t.Fatalf("AddInboxEntry %s: %v", e.ID, err)
		}
	}
	return s
}

func readFlags(t *testing.T, s *store.Store) map[string]bool {
	t.Helper()
	entries, err := s.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	flags := map[string]bool{}
	for _, e := range entries {
		flags[e.ID] = e.Read
	}
	return flags
}

func TestMarkNotificationReadMarksOnlyThatEntry(t *testing.T) {
	s := inboxStore(t, store.InboxEntry{ID: "i-1"}, store.InboxEntry{ID: "i-2"})
	if err := MarkNotificationRead(s, "i-2"); err != nil {
		t.Fatalf("MarkNotificationRead: %v", err)
	}
	flags := readFlags(t, s)
	if flags["i-1"] || !flags["i-2"] {
		t.Fatalf("read flags = %v, want only i-2 read", flags)
	}
}

func TestMarkNotificationReadReportsAnUnknownID(t *testing.T) {
	s := inboxStore(t, store.InboxEntry{ID: "i-1"})
	if err := MarkNotificationRead(s, "i-missing"); err == nil {
		t.Fatal("an unknown notification id was marked read without complaint")
	}
}

func TestMarkAllNotificationsRead(t *testing.T) {
	s := inboxStore(t, store.InboxEntry{ID: "i-1"}, store.InboxEntry{ID: "i-2", Read: true}, store.InboxEntry{ID: "i-3"})
	if err := MarkAllNotificationsRead(s); err != nil {
		t.Fatalf("MarkAllNotificationsRead: %v", err)
	}
	for id, read := range readFlags(t, s) {
		if !read {
			t.Errorf("%s still unread", id)
		}
	}
}

// Reading an artifact reads the notification that announced it: the
// notification only ever pointed at the artifact.
func TestReadingAnArtifactReadsItsNotification(t *testing.T) {
	s := inboxStore(t,
		store.InboxEntry{ID: "i-art", Kind: store.InboxKindArtifact, ArtifactID: "a-1"},
		store.InboxEntry{ID: "i-other", Kind: store.InboxKindArtifact, ArtifactID: "a-2"},
		store.InboxEntry{ID: "i-run", Kind: store.InboxKindFailure, RunID: "r-1"},
	)
	if err := MarkArtifactRead(s, "a-1"); err != nil {
		t.Fatalf("MarkArtifactRead: %v", err)
	}
	flags := readFlags(t, s)
	if !flags["i-art"] || flags["i-other"] || flags["i-run"] {
		t.Fatalf("read flags after reading a-1 = %v", flags)
	}

	src := filepath.Join(t.TempDir(), "two.txt")
	if err := os.WriteFile(src, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishArtifact(s, PublishInput{ID: "a-2", SourcePath: src, Now: time.Now()}); err != nil {
		t.Fatalf("PublishArtifact: %v", err)
	}
	if err := MarkAllArtifactsRead(s); err != nil {
		t.Fatalf("MarkAllArtifactsRead: %v", err)
	}
	flags = readFlags(t, s)
	if !flags["i-other"] || flags["i-run"] {
		t.Fatalf("read flags after reading all artifacts = %v (run notifications must stay)", flags)
	}
}

// Reading a run's log reads the notification about that run's outcome, and
// nothing else — a task's own notification names its run only as attribution.
func TestReadingARunReadsItsOutcomeNotification(t *testing.T) {
	s := inboxStore(t,
		store.InboxEntry{ID: "i-fail", Kind: store.InboxKindFailure, RunID: "r-1"},
		store.InboxEntry{ID: "i-ok", Kind: store.InboxKindSuccess, RunID: "r-2"},
		store.InboxEntry{ID: "i-art", Kind: store.InboxKindArtifact, ArtifactID: "a-1"},
	)
	if err := MarkRead(s, "r-1"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	flags := readFlags(t, s)
	if !flags["i-fail"] || flags["i-ok"] || flags["i-art"] {
		t.Fatalf("read flags after reading r-1 = %v", flags)
	}
	if err := MarkAllRead(s); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	flags = readFlags(t, s)
	if !flags["i-ok"] || flags["i-art"] {
		t.Fatalf("read flags after reading all runs = %v (artifact notifications must stay)", flags)
	}
}
