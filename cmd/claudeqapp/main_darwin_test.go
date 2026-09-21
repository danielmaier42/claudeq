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
func TestReachableRetriesAProbeThatGotNoAnswer(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			// Drop the connection: what a probe that goes unanswered looks like
			// from the app's side.
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if !reachable(srv.URL, daemonProbeAttempts) {
		t.Fatal("reachable = false, want true once the daemon answers")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("probes = %d, want 2 (one unanswered, one answered)", got)
	}
}

// Whatever a daemon makes of the request, it is a daemon — including one too old
// to know the endpoint the app asks for. Starting a second one because of a 404
// would be the same mistake as starting one because a probe timed out.
func TestReachableAcceptsAnyAnswer(t *testing.T) {
	for _, code := range []int{http.StatusNoContent, http.StatusNotFound, http.StatusServiceUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		if !reachable(srv.URL, daemonProbeAttempts) {
			t.Errorf("a daemon answering %d was taken for no daemon at all", code)
		}
		srv.Close()
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
