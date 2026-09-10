package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/feedback"
	"github.com/danielmaier42/claudeq/internal/update"
	"github.com/danielmaier42/claudeq/internal/version"
)

// feedbackStatus tells the dashboard whether the guided feedback chat can run,
// and pre-fills the two environment details that go into the issue.
type feedbackStatus struct {
	// Available is false when no Claude Code binary is configured or found; the
	// dashboard then offers the plain form instead of the chat.
	Available bool `json:"available"`
	// Reason explains an unavailable chat in one sentence.
	Reason string `json:"reason,omitempty"`
	// Repo is the "owner/name" the issue is filed against.
	Repo string `json:"repo"`
	// AppVersion is this build's version, editable by the user before filing.
	AppVersion string `json:"app_version"`
	// OSVersion is the macOS product version, also editable.
	OSVersion string `json:"os_version,omitempty"`
	// MaxTurns is how many messages the user may send before the assistant has
	// to deliver a draft.
	MaxTurns int `json:"max_turns"`
}

func (s *server) getFeedback(w http.ResponseWriter, _ *http.Request) {
	st := feedbackStatus{
		Repo:       update.DefaultRepo,
		AppVersion: version.String(),
		MaxTurns:   feedback.MaxUserTurns,
	}
	if s.d.OSVersion != nil {
		st.OSVersion = s.d.OSVersion()
	}
	switch {
	case s.d.Feedback == nil:
		st.Reason = "the feedback assistant is not available in this build"
	case s.feedbackBin() == "":
		st.Reason = "the Claude Code CLI was not found; set its path in Settings"
	default:
		st.Available = true
	}
	writeJSON(w, http.StatusOK, st)
}

type feedbackTurnReq struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

func (s *server) feedbackTurn(w http.ResponseWriter, r *http.Request) {
	if s.d.Feedback == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("the feedback assistant is not available"))
		return
	}
	var req feedbackTurnReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	bin := s.feedbackBin()
	if bin == "" {
		writeErr(w, http.StatusServiceUnavailable, errors.New("the Claude Code CLI was not found; set its path in Settings"))
		return
	}
	d, err := s.d.Feedback.Turn(r.Context(), bin, req.SessionID, req.Text)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type feedbackURLReq struct {
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Labels     []string `json:"labels"`
	AppVersion string   `json:"app_version"`
	OSVersion  string   `json:"os_version"`
}

// feedbackURL builds the prefilled GitHub "new issue" page from the text the
// user reviewed. Nothing is sent anywhere here: the dashboard opens the URL in
// the browser, where the user presses "Create" themselves.
func (s *server) feedbackURL(w http.ResponseWriter, r *http.Request) {
	var req feedbackURLReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	// The title is capped as well as collapsed: IssueURL can trim an oversized
	// body, but a pasted wall of text in the title would push the URL past
	// GitHub's request-URI limit with nothing left to give.
	title := oneLine(req.Title, 200)
	if title == "" {
		writeErr(w, http.StatusBadRequest, errors.New("the issue needs a title"))
		return
	}
	body := strings.TrimSpace(req.Body) + envFooter(req.AppVersion, req.OSVersion)
	writeJSON(w, http.StatusOK, map[string]string{
		"url": feedback.IssueURL(update.DefaultRepo, title, body, req.Labels),
	})
}

// envFooter renders the two environment details the user can edit (or clear) in
// the review step. An empty field is simply left out.
func envFooter(app, osv string) string {
	var parts []string
	if app = oneLine(app, 60); app != "" {
		parts = append(parts, "ClaudeQ "+app)
	}
	if osv = oneLine(osv, 60); osv != "" {
		parts = append(parts, "macOS "+osv)
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n\n---\n" + strings.Join(parts, " · ")
}

// oneLine collapses whitespace and caps the length, so an edited environment
// field cannot smuggle extra Markdown into the issue.
func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > limit {
		s = strings.TrimSpace(string(r[:limit]))
	}
	return s
}

// feedbackBin resolves the Claude Code binary for a feedback turn, reporting ""
// when there is none — unlike claudeBin, which falls back to a bare name for an
// interactive shell to resolve.
func (s *server) feedbackBin() string {
	if cfg, err := s.d.Store.LoadConfig(); err == nil && cfg.Settings.ClaudePath != "" {
		return cfg.Settings.ClaudePath
	}
	return executor.DetectBinary()
}
