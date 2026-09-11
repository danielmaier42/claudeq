package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNtfyPublishesJSONToTheServerRoot(t *testing.T) {
	var got struct {
		Topic, Title, Message, Click string
	}
	var auth, ctype, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ctype, path = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := Ntfy{Server: srv.URL, Topic: "claudeq-nightly", Client: srv.Client()}
	err := n.Notify(context.Background(), Notification{
		Title: "Task failed", Message: "nightly-sweep: exit 1", URL: "https://example.com/run/7",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if path != "/" {
		t.Fatalf("published to %q, want the server root", path)
	}
	if got.Topic != "claudeq-nightly" || got.Title != "Task failed" || got.Message != "nightly-sweep: exit 1" {
		t.Fatalf("payload = %+v", got)
	}
	if got.Click != "https://example.com/run/7" {
		t.Fatalf("link must be sent as the click action, got %q", got.Click)
	}
	if !strings.HasPrefix(ctype, "application/json") {
		t.Fatalf("content-type = %q", ctype)
	}
	if auth != "" {
		t.Fatalf("no token configured, but sent %q", auth)
	}
}

func TestNtfySendsTokenAsBearerAndOmitsAnEmptyClick(t *testing.T) {
	var auth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := Ntfy{Server: srv.URL, Topic: "t", Token: "tk_secret", Client: srv.Client()}
	if err := n.Notify(context.Background(), Notification{Title: "T", Message: "M"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if auth != "Bearer tk_secret" {
		t.Fatalf("Authorization = %q", auth)
	}
	if _, ok := body["click"]; ok {
		t.Fatalf("a notification without a link must not send click, got %v", body)
	}
}

func TestNtfyErrorQuotesTheServersReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	n := Ntfy{Server: srv.URL, Topic: "t", Client: srv.Client()}
	err := n.Notify(context.Background(), Notification{Title: "T", Message: "M"})
	if err == nil {
		t.Fatal("expected an error on 403")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error should name status and reason, got %v", err)
	}
}

func TestNtfyNotConfigured(t *testing.T) {
	if (Ntfy{}).Configured() {
		t.Fatal("a channel without a topic is not configured")
	}
	if (Ntfy{Topic: "   "}).Configured() {
		t.Fatal("whitespace is not a topic")
	}
	if (Ntfy{Topic: "nightly", Server: "ftp://ntfy.sh"}).Configured() {
		t.Fatal("a topic with an unusable server must not count as configured")
	}
	if err := (Ntfy{}).Notify(context.Background(), Notification{}); err == nil {
		t.Fatal("expected an error when not configured")
	}
}

func TestNtfyEndpoint(t *testing.T) {
	ok := map[string]string{
		"":                        DefaultNtfyServer,
		"  ":                      DefaultNtfyServer,
		"ntfy.example.com":        "https://ntfy.example.com",
		"https://ntfy.sh/":        "https://ntfy.sh",
		"http://192.168.1.9:8080": "http://192.168.1.9:8080",
		" https://ntfy.sh ":       "https://ntfy.sh",
	}
	for in, want := range ok {
		got, err := NtfyEndpoint(in)
		if err != nil {
			t.Errorf("NtfyEndpoint(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NtfyEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"ftp://ntfy.sh", "not a server", "https://", "https://ntfy.sh/mytopic"} {
		if got, err := NtfyEndpoint(in); err == nil {
			t.Errorf("NtfyEndpoint(%q) = %q, want an error", in, got)
		}
	}
}

func TestValidateNtfy(t *testing.T) {
	if err := ValidateNtfy("", "claudeq_nightly-1"); err != nil {
		t.Fatalf("a plain topic on the default server is valid: %v", err)
	}
	cases := map[string][2]string{
		"empty topic":              {"", "  "},
		"topic as a URL":           {"", "https://ntfy.sh/mytopic"},
		"topic with slash":         {"", "team/nightly"},
		"topic too long":           {"", strings.Repeat("a", 65)},
		"bad character":            {"", "nightly!"},
		"bad server":               {"ftp://ntfy.sh", "nightly"},
		"topic pasted into server": {"https://ntfy.sh/mytopic", "nightly"},
	}
	for name, c := range cases {
		if err := ValidateNtfy(c[0], c[1]); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// The hint has to name the fix, not just the fact: pasting the whole URL
	// into the topic field is the mistake people actually make.
	err := ValidateNtfy("", "https://ntfy.sh/mytopic")
	if err == nil || !strings.Contains(err.Error(), "server field") {
		t.Fatalf("error should point at the server field, got %v", err)
	}
}
