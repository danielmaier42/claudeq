package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookPostsTheDefaultBody(t *testing.T) {
	var body map[string]string
	var ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("the default body must be JSON: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	wh := Webhook{URL: srv.URL, Client: srv.Client()}
	err := wh.Notify(context.Background(), Notification{
		Title: "Task failed", Message: "nightly-sweep: exit 1", URL: "https://example.com/run/7",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	want := map[string]string{
		"title":   "Task failed",
		"message": "nightly-sweep: exit 1",
		"url":     "https://example.com/run/7",
		"source":  "claudeq",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("body[%q] = %q, want %q", k, body[k], v)
		}
	}
	if !strings.HasPrefix(ctype, "application/json") {
		t.Fatalf("content-type = %q", ctype)
	}
}

func TestWebhookPostsACustomTemplate(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 512)
		n, _ := r.Body.Read(b)
		raw = string(b[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := Webhook{URL: srv.URL, Template: `{"text":"{{title}} — {{message}}"}`, Client: srv.Client()}
	if err := wh.Notify(context.Background(), Notification{Title: "ClaudeQ", Message: "done"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	var slack struct{ Text string }
	if err := json.Unmarshal([]byte(raw), &slack); err != nil {
		t.Fatalf("body %q is not JSON: %v", raw, err)
	}
	if slack.Text != "ClaudeQ — done" {
		t.Fatalf("text = %q", slack.Text)
	}
}

// A run's output ends up in a notification verbatim, quotes, backslashes and
// newlines included. That must not be able to break the JSON body it is placed
// into — the value has to survive the round trip unchanged.
func TestWebhookEscapesTheValuesItSubstitutes(t *testing.T) {
	n := Notification{
		Title:   `he said "boom"`,
		Message: "line one\nline two\\end\ttab",
		URL:     "https://example.com/a?b=1&c=2",
	}
	body := RenderWebhookBody("", n)
	var got map[string]string
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body %q is not valid JSON: %v", body, err)
	}
	if got["title"] != n.Title || got["message"] != n.Message || got["url"] != n.URL {
		t.Fatalf("values did not survive the round trip: %+v", got)
	}
}

func TestRenderWebhookBodyLeavesAnUnknownPlaceholderAlone(t *testing.T) {
	body := RenderWebhookBody(`{"a":"{{titel}}","b":"{{title}}"}`, Notification{Title: "T"})
	if !strings.Contains(body, "{{titel}}") {
		t.Fatalf("an unknown placeholder must stay verbatim (ValidateWebhook is what refuses it), got %q", body)
	}
	if !strings.Contains(body, `"b":"T"`) {
		t.Fatalf("known placeholders must still be filled, got %q", body)
	}
}

func TestWebhookNotConfigured(t *testing.T) {
	for _, u := range []string{"", "   ", "example.com/hook", "file:///tmp/x"} {
		if (Webhook{URL: u}).Configured() {
			t.Errorf("URL %q must not count as configured", u)
		}
	}
	if !(Webhook{URL: " https://example.com/hook "}).Configured() {
		t.Error("a padded URL is still a URL")
	}
	if err := (Webhook{}).Notify(context.Background(), Notification{}); err == nil {
		t.Fatal("expected an error when not configured")
	}
}

func TestValidateWebhook(t *testing.T) {
	good := []string{
		"",
		DefaultWebhookTemplate,
		`{"text":"{{title}}: {{message}} {{url}}"}`,
		`{"content":"{{ title }}"}`,
	}
	for _, tpl := range good {
		if err := ValidateWebhook("https://hooks.example.com/abc", tpl); err != nil {
			t.Errorf("template %q should be valid: %v", tpl, err)
		}
	}

	bad := map[string]struct{ url, tpl string }{
		"no url":              {"", DefaultWebhookTemplate},
		"not http":            {"ftp://example.com/hook", DefaultWebhookTemplate},
		"not a url at all":    {"hooks.example.com", DefaultWebhookTemplate},
		"unknown placeholder": {"https://example.com/h", `{"text":"{{titel}}"}`},
		"missing brace":       {"https://example.com/h", `{"text":"{{title}}"`},
		"not json":            {"https://example.com/h", `title={{title}}`},
		"unquoted value":      {"https://example.com/h", `{"text":{{title}}}`},
	}
	for name, c := range bad {
		if err := ValidateWebhook(c.url, c.tpl); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	err := ValidateWebhook("https://example.com/h", `{"text":"{{titel}}"}`)
	if err == nil || !strings.Contains(err.Error(), "{{titel}}") {
		t.Fatalf("the error should name the placeholder it does not know, got %v", err)
	}
}
