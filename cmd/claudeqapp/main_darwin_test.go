//go:build darwin

package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A daemon that misses one probe must not be declared absent: the app would then
// start a second daemon on the same store.
func TestReachableRetriesABusyDaemon(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !reachable(srv.URL, daemonProbeAttempts) {
		t.Fatal("reachable = false, want true once the daemon answers")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("probes = %d, want 2 (one missed, one answered)", got)
	}
}

func TestReachableGivesUpWhenNothingListens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	if reachable(url, daemonProbeAttempts) {
		t.Fatal("reachable = true, want false with no daemon there")
	}
}
