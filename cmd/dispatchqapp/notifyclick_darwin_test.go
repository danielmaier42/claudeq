//go:build darwin

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarkNotificationReadPostsToTheDaemon(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := markNotificationRead(srv.URL, "i-1"); err != nil {
		t.Fatalf("markNotificationRead: %v", err)
	}
	if got != "POST /api/notifications/i-1/read" {
		t.Fatalf("request = %q", got)
	}
}

func TestMarkNotificationReadReportsARefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := markNotificationRead(srv.URL, "i-gone"); err == nil {
		t.Fatal("a 404 from the daemon went unreported")
	}
}

// The mailbox hands a click out exactly once, so the launch-time check of the
// page and the nudge after a click cannot open the same target twice.
func TestPendingNotificationIsTakenOnce(t *testing.T) {
	if got := takePendingNotification(); got != nil {
		t.Fatalf("fresh mailbox holds %+v", got)
	}
	pending.mu.Lock()
	pending.target = &notificationTarget{ID: "i-1", RunID: "r-1"}
	pending.mu.Unlock()
	first := takePendingNotification()
	if first == nil || first.RunID != "r-1" {
		t.Fatalf("first take = %+v", first)
	}
	if again := takePendingNotification(); again != nil {
		t.Fatalf("second take = %+v, want nil", again)
	}
}
