package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/feedback"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/update"
	"github.com/danielmaier42/claudeq/internal/version"
)

// fakeCLI replays one canned Claude Code answer.
type fakeCLI struct {
	out []byte
	err error
}

func (f fakeCLI) Run(context.Context, string, []string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

// newFeedbackServer wires a server whose feedback chat answers with out. The
// store gets an explicit claude path so availability does not depend on what is
// installed on the machine running the tests.
func newFeedbackServer(t *testing.T, cli feedback.CLIRunner) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Settings.ClaudePath = "/usr/local/bin/claude"
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	var svc *feedback.Service
	if cli != nil {
		svc = feedback.New(cli)
	}
	srv := httptest.NewServer(Handler(Deps{Store: st, Feedback: svc, OSVersion: func() string { return "15.6" }}))
	t.Cleanup(srv.Close)
	return srv, st
}

func TestFeedbackStatusReportsAvailability(t *testing.T) {
	srv, _ := newFeedbackServer(t, fakeCLI{})
	var st feedbackStatus
	do(t, srv, http.MethodGet, "/api/feedback", nil).into(t, &st)
	if !st.Available || st.Reason != "" {
		t.Fatalf("status = %+v, want available", st)
	}
	if st.Repo != update.DefaultRepo || st.AppVersion != version.String() || st.OSVersion != "15.6" {
		t.Fatalf("status = %+v, want repo/versions filled in", st)
	}
	if st.MaxTurns != feedback.MaxUserTurns {
		t.Fatalf("max_turns = %d, want %d", st.MaxTurns, feedback.MaxUserTurns)
	}
}

func TestFeedbackStatusExplainsAnUnavailableAssistant(t *testing.T) {
	srv, _ := newFeedbackServer(t, nil)
	var st feedbackStatus
	do(t, srv, http.MethodGet, "/api/feedback", nil).into(t, &st)
	if st.Available || st.Reason == "" {
		t.Fatalf("status = %+v, want unavailable with a reason", st)
	}
	// The dashboard still needs the versions for its manual form.
	if st.AppVersion == "" || st.OSVersion != "15.6" {
		t.Fatalf("status = %+v, want the environment details anyway", st)
	}
}

func TestFeedbackTurnReturnsTheDraft(t *testing.T) {
	out := []byte(`{"is_error":false,"structured_output":{"status":"ready","title":"T","body":"B","labels":["bug"]}}`)
	srv, _ := newFeedbackServer(t, fakeCLI{out: out})
	var d feedback.Draft
	r := do(t, srv, http.MethodPost, "/api/feedback/turn", map[string]string{"text": "something broke"})
	if r.Status != http.StatusOK {
		t.Fatalf("status = %d (%s)", r.Status, r.Body)
	}
	r.into(t, &d)
	if d.Status != "ready" || d.Title != "T" || d.SessionID == "" {
		t.Fatalf("draft = %+v", d)
	}
}

func TestFeedbackTurnReportsAFailingAssistant(t *testing.T) {
	srv, _ := newFeedbackServer(t, fakeCLI{err: errors.New("exit status 1")})
	r := do(t, srv, http.MethodPost, "/api/feedback/turn", map[string]string{"text": "hi"})
	if r.Status != http.StatusBadGateway {
		t.Fatalf("status = %d (%s), want 502", r.Status, r.Body)
	}
}

func TestFeedbackTurnIsUnavailableWithoutTheService(t *testing.T) {
	srv, _ := newFeedbackServer(t, nil)
	r := do(t, srv, http.MethodPost, "/api/feedback/turn", map[string]string{"text": "hi"})
	if r.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s), want 503", r.Status, r.Body)
	}
}

func TestFeedbackURLAppendsTheEditedEnvironment(t *testing.T) {
	srv, _ := newFeedbackServer(t, fakeCLI{})
	var got map[string]string
	do(t, srv, http.MethodPost, "/api/feedback/url", map[string]any{
		"title": "  A  title ", "body": "The report.", "labels": []string{"bug"},
		"app_version": "v9.9.9", "os_version": "26.0",
	}).into(t, &got)
	u, err := url.Parse(got["url"])
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if u.Query().Get("title") != "A title" {
		t.Fatalf("title = %q", u.Query().Get("title"))
	}
	body := u.Query().Get("body")
	if !strings.HasPrefix(body, "The report.") {
		t.Fatalf("body = %q", body)
	}
	if !strings.Contains(body, "ClaudeQ v9.9.9") || !strings.Contains(body, "macOS 26.0") {
		t.Fatalf("body does not carry the environment line: %q", body)
	}
}

func TestFeedbackURLLeavesOutClearedEnvironmentFields(t *testing.T) {
	srv, _ := newFeedbackServer(t, fakeCLI{})
	var got map[string]string
	do(t, srv, http.MethodPost, "/api/feedback/url", map[string]any{
		"title": "T", "body": "B", "app_version": "  ", "os_version": "",
	}).into(t, &got)
	u, _ := url.Parse(got["url"])
	if body := u.Query().Get("body"); body != "B" {
		t.Fatalf("body = %q, want no environment line", body)
	}
}

func TestFeedbackURLNeedsATitle(t *testing.T) {
	srv, _ := newFeedbackServer(t, fakeCLI{})
	r := do(t, srv, http.MethodPost, "/api/feedback/url", map[string]any{"title": "   ", "body": "B"})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("status = %d (%s), want 400", r.Status, r.Body)
	}
}

func TestFeedbackURLCapsAnOversizedTitle(t *testing.T) {
	// A title long enough to blow the URL budget on its own would make GitHub
	// answer with 414, and trimming the body cannot save it.
	srv, _ := newFeedbackServer(t, fakeCLI{})
	var got map[string]string
	do(t, srv, http.MethodPost, "/api/feedback/url", map[string]any{
		"title": strings.Repeat("very long title ", 600), "body": "B",
	}).into(t, &got)
	if len(got["url"]) > feedback.MaxURLLen {
		t.Fatalf("url is %d bytes, want at most %d", len(got["url"]), feedback.MaxURLLen)
	}
}
