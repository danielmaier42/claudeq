package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postUninstall(t *testing.T, srv *httptest.Server, contentType, body string) int {
	t.Helper()
	return postUninstallAs(t, srv, "", contentType, body)
}

// postUninstallAs sends the request with host as its Host header ("" keeps
// the server's own 127.0.0.1 address).
func postUninstallAs(t *testing.T, srv *httptest.Server, host, contentType, body string) int {
	t.Helper()
	req, err := http.NewRequest("POST", srv.URL+"/api/uninstall", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	return r.StatusCode
}

func TestUninstallEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		depErr      error
		status      int
		calls       []bool
	}{
		{"keep data", "application/json", `{}`, nil, http.StatusAccepted, []bool{false}},
		{"purge", "application/json; charset=utf-8", `{"purge":true}`, nil, http.StatusAccepted, []bool{true}},
		// What a web page can send cross-origin without a preflight.
		{"form post", "application/x-www-form-urlencoded", `purge=true`, nil, http.StatusUnsupportedMediaType, nil},
		{"text post", "text/plain", `{"purge":true}`, nil, http.StatusUnsupportedMediaType, nil},
		{"no content type", "", `{"purge":true}`, nil, http.StatusUnsupportedMediaType, nil},
		{"bad json", "application/json", `{`, nil, http.StatusBadRequest, nil},
		{"cannot start", "application/json", `{}`, errors.New("no cli"), http.StatusInternalServerError, []bool{false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []bool
			srv := httptest.NewServer(Handler(Deps{
				Uninstall: func(purge bool) error { calls = append(calls, purge); return tc.depErr },
			}))
			defer srv.Close()
			if got := postUninstall(t, srv, tc.contentType, tc.body); got != tc.status {
				t.Fatalf("status = %d, want %d", got, tc.status)
			}
			if len(calls) != len(tc.calls) || (len(calls) == 1 && calls[0] != tc.calls[0]) {
				t.Fatalf("uninstall calls = %v, want %v", calls, tc.calls)
			}
		})
	}
}

// A page whose name was rebound to 127.0.0.1 posts same-origin, so only its
// Host header gives it away.
func TestUninstallEndpointRejectsRebinding(t *testing.T) {
	called := false
	srv := httptest.NewServer(Handler(Deps{Uninstall: func(bool) error { called = true; return nil }}))
	defer srv.Close()
	if got := postUninstallAs(t, srv, "evil.example:10765", "application/json", `{"purge":true}`); got != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", got)
	}
	if called {
		t.Fatal("uninstall started for a foreign Host")
	}
	for _, host := range []string{"localhost:10765", "[::1]:10765", "127.0.0.1"} {
		if !loopbackHost(host) {
			t.Errorf("loopbackHost(%q) = false", host)
		}
	}
}

func TestUninstallEndpointUnsupported(t *testing.T) {
	srv := httptest.NewServer(Handler(Deps{}))
	defer srv.Close()
	if got := postUninstall(t, srv, "application/json", `{}`); got != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", got)
	}
}

func TestDetachedUninstallerNeedsTheCLI(t *testing.T) {
	if DetachedUninstaller("") != nil {
		t.Fatal("an uninstaller without a CLI to run")
	}
}
