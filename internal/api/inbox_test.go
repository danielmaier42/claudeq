package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

func addNotification(t *testing.T, st *store.Store, id, kind string) {
	t.Helper()
	if err := st.AddInboxEntry(store.InboxEntry{ID: id, Kind: kind, Title: id, Message: "m", SentAt: time.Now()}); err != nil {
		t.Fatalf("AddInboxEntry: %v", err)
	}
}

func TestListNotificationsNewestFirstUnread(t *testing.T) {
	srv, st := newServer(t, nil)
	if err := st.AddInboxEntry(store.InboxEntry{ID: "i-0", Kind: store.InboxKindTask, Read: true, SentAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	addNotification(t, st, "i-1", store.InboxKindFailure)
	addNotification(t, st, "i-2", store.InboxKindArtifact)

	r := do(t, srv, "GET", "/api/notifications", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("list status = %d", r.Status)
	}
	var views []map[string]any
	r.into(t, &views)
	if len(views) != 3 || views[0]["id"] != "i-2" {
		t.Fatalf("want i-2 first of 3, got %v", views)
	}
	if views[0]["unread"] != true || views[1]["unread"] != true || views[2]["unread"] != false {
		t.Fatalf("unread flags = %v", views)
	}
	for _, v := range views {
		if _, has := v["read"]; has {
			t.Fatalf("the list says unread, not read as well: %v", v)
		}
	}
}

func TestReadNotificationAndReadAll(t *testing.T) {
	srv, st := newServer(t, nil)
	addNotification(t, st, "i-1", store.InboxKindFailure)
	addNotification(t, st, "i-2", store.InboxKindTask)

	if r := do(t, srv, "POST", "/api/notifications/i-1/read", nil); r.Status != http.StatusNoContent {
		t.Fatalf("read status = %d", r.Status)
	}
	var views []notificationView
	do(t, srv, "GET", "/api/notifications", nil).into(t, &views)
	for _, v := range views {
		if v.ID == "i-1" && v.Unread {
			t.Fatal("i-1 still unread after read")
		}
		if v.ID == "i-2" && !v.Unread {
			t.Fatal("i-2 was marked read along with i-1")
		}
	}

	if r := do(t, srv, "POST", "/api/notifications/read-all", nil); r.Status != http.StatusNoContent {
		t.Fatalf("read-all status = %d", r.Status)
	}
	do(t, srv, "GET", "/api/notifications", nil).into(t, &views)
	for _, v := range views {
		if v.Unread {
			t.Fatalf("%s still unread after read-all", v.ID)
		}
	}
}

func TestReadNotificationUnknownIDIs404(t *testing.T) {
	srv, _ := newServer(t, nil)
	if r := do(t, srv, "POST", "/api/notifications/i-none/read", nil); r.Status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", r.Status)
	}
}

// Opening an artifact or a run from its own view also settles the
// notification about it, so the Notifications badge does not keep counting
// things the operator has already looked at.
func TestReadingArtifactOrRunReadsItsNotification(t *testing.T) {
	srv, st := newServer(t, nil)
	publishTestArtifact(t, st, "a-1", "one.txt", "1")
	if err := st.AddInboxEntry(store.InboxEntry{ID: "i-art", Kind: store.InboxKindArtifact, ArtifactID: "a-1", SentAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddInboxEntry(store.InboxEntry{ID: "i-run", Kind: store.InboxKindFailure, RunID: "r-1", SentAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	do(t, srv, "POST", "/api/artifacts/a-1/read", nil)
	do(t, srv, "POST", "/api/runs/r-1/read", nil)

	var views []notificationView
	do(t, srv, "GET", "/api/notifications", nil).into(t, &views)
	for _, v := range views {
		if v.Unread {
			t.Errorf("%s still unread after its target was read", v.ID)
		}
	}
}
