// Package feedback turns a short chat with the user into a GitHub issue draft
// for the ClaudeQ repository. It asks a harness as an aside — locked down,
// tool-free, in a directory of its own — and hands the resulting draft to the
// dashboard, which
// shows it for review before the user opens a prefilled "new issue" page in
// their browser. ClaudeQ itself never talks to GitHub and holds no credentials:
// the issue is created by the user, in their own logged-in browser session.
package feedback

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/danielmaier42/claudeq/internal/provider"
)

// DefaultModel is what a feedback turn runs on when Settings names no model.
// Drafting an issue from two short messages is cheap work, and it must not
// silently land on the expensive model someone picked for real tasks.
const DefaultModel = "haiku"

// MaxUserTurns caps the conversation: the opening report plus at most two
// clarifying rounds. On the last turn the assistant is told it must deliver.
const MaxUserTurns = 3

// turnTimeout bounds a single CLI call. A turn takes a few seconds in practice;
// anything past this is a hang, and the dashboard falls back to the plain form.
const turnTimeout = 90 * time.Second

// sessionTTL is how long an idle conversation stays resumable in memory.
const sessionTTL = time.Hour

// maxInputChars bounds what one user message may contribute, so a pasted log
// cannot blow up the prompt.
const maxInputChars = 4000

// Labels are the only labels a draft may carry: every report is either a defect
// or a wish, and a triage label beyond that is the maintainer's call, not the
// reporter's. Both exist in the repository — letting the model invent its own
// would produce labels GitHub drops.
var Labels = []string{"bug", "enhancement"}

// systemPrompt drives the whole conversation. It is deliberately explicit about
// privacy: the issue it writes ends up on a public tracker.
const systemPrompt = `You are the feedback assistant built into ClaudeQ, a local-only macOS app that queues Claude Code tasks during the day and runs them unattended at night through the Claude Code CLI. ClaudeQ is a background daemon (claudeqd, a launchd LaunchAgent), a native window wrapping a local web dashboard (the Queue, Activity, Artifacts, Usage and Settings views), and a claudeq command-line tool.

A user is reporting a bug or suggesting an improvement. Turn that into one GitHub issue for the project's public repository.

Ask a clarifying question only when the report cannot be acted on without it - a missing reproduction step, or a wish so broad that its goal is unclear. Never ask about something the user already told you, and never ask twice about the same thing. Most reports need no question at all.

Answer with the JSON schema and nothing else:
- status "ask": exactly one short question in "question", written in the language the user wrote in.
- status "ready": fill "title", "body" and "labels".

Rules for the issue:
- Write title and body in English, even when the user wrote in another language.
- Title: one line, under 80 characters, concrete. "Clicking a notification does not open the app", not "Notification problem".
- Body: short GitHub Markdown. For a bug use the sections "What happens", "What I expected" and, when they are known, "Steps". For a request use "Problem" and "Suggestion". Stay under 1200 characters. No heading larger than ###. No filler.
- Report only what the user actually said. Never invent versions, steps, error messages or a diagnosis, and do not speculate about the cause in the code.
- The issue is public. Never include personal data: no real names, e-mail addresses, user names, machine names, absolute file paths, folder or repository names, tokens, or the contents of the user's own task prompts. Where such a detail is needed for the report to make sense, replace it with a placeholder like <path>.
- Never mention this conversation, yourself, or that the text was generated.
- Labels: exactly one - "bug" when something is broken, "enhancement" when something is missing.`

// lastTurnNudge is appended to the final user message instead of the system
// prompt: the CLI records the system prompt on a conversation's first request
// and replays it verbatim on every resume, so a later system prompt would be
// ignored.
const lastTurnNudge = "\n\n[System note: this is the last exchange, another question is not possible. Return status \"ready\" with the best issue you can write from what the user has said so far.]"

// schema constrains the CLI's structured output to the draft shape.
var schema = `{"type":"object","properties":` +
	`{"status":{"type":"string","enum":["ask","ready"]},` +
	`"question":{"type":"string"},` +
	`"title":{"type":"string"},` +
	`"body":{"type":"string"},` +
	`"labels":{"type":"array","items":{"type":"string","enum":["` + strings.Join(Labels, `","`) + `"]}}},` +
	`"required":["status"]}`

// Draft is one assistant answer: either a question back to the user, or the
// finished issue.
type Draft struct {
	// SessionID identifies the conversation; the dashboard sends it back with
	// the next message so the CLI can resume the same session.
	SessionID string `json:"session_id"`
	// Status is "ask" (Question is set) or "ready" (Title/Body are set).
	Status string `json:"status"`
	// Question is the single clarifying question, in the user's language.
	Question string `json:"question,omitempty"`
	// Title is the issue title, English, one line.
	Title string `json:"title,omitempty"`
	// Body is the issue body as GitHub Markdown, English.
	Body string `json:"body,omitempty"`
	// Labels are repository labels, validated against [Labels].
	Labels []string `json:"labels,omitempty"`
	// Final is true when no further question is possible, so the dashboard can
	// hide the "answer" box even if the model still asked something.
	Final bool `json:"final"`
}

// Asker puts one question to a provider instance (satisfied by *aside.Runner).
// It is an interface so the conversation logic is testable against a stub
// instead of a real harness.
type Asker interface {
	Ask(ctx context.Context, inst provider.Instance, req provider.AsideRequest) (provider.Aside, error)
}

// Service holds the in-flight feedback conversations.
type Service struct {
	ask Asker
	now func() time.Time

	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	turns int
	seen  time.Time
}

// New returns a Service running its turns through a.
func New(a Asker) *Service {
	return &Service{ask: a, now: time.Now, sessions: map[string]*session{}}
}

// Turn sends text to the conversation identified by sessionID (empty starts a
// new one) and returns the assistant's answer. inst is the provider that
// answers, and model the model it answers with ("" uses DefaultModel).
func (s *Service) Turn(ctx context.Context, inst provider.Instance, model, sessionID, text string) (Draft, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Draft{}, errors.New("say what you would like to report before sending")
	}
	if len([]rune(text)) > maxInputChars {
		text = string([]rune(text)[:maxInputChars])
	}
	if s.ask == nil {
		return Draft{}, errors.New("the feedback assistant is not available")
	}
	if model == "" {
		model = DefaultModel
	}

	id, turns, resume := s.begin(sessionID)
	last := turns >= MaxUserTurns
	if last {
		text += lastTurnNudge
	}

	ctx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()
	req := provider.AsideRequest{
		Model:     model,
		Text:      text,
		Schema:    schema,
		SessionID: id,
		Resume:    resume,
		// A first answer may be a question, so the session has to survive it.
		// The last turn has to deliver, and nothing will resume it.
		Continues: !last,
	}
	if !resume {
		req.System = systemPrompt
	}
	answer, err := s.ask.Ask(ctx, inst, req)
	if err != nil {
		s.forget(id)
		return Draft{}, fmt.Errorf("the feedback assistant could not be reached: %w", err)
	}
	d, err := parse(answer)
	if err != nil {
		s.forget(id)
		return Draft{}, err
	}
	// A harness that names its own sessions answers with an id of its own; the
	// dashboard sends back whatever it is told here.
	if answer.SessionID != "" {
		id = s.rename(id, answer.SessionID)
	}
	d.SessionID = id
	d.Final = last
	if last && d.Status != "ready" {
		// The model asked anyway. There is no round left to answer in, so what it
		// has is what the user gets to edit.
		d.Status = "ready"
		d.Question = ""
	}
	if d.Status == "ready" || last {
		s.forget(id)
	}
	return d, nil
}

func parse(answer provider.Aside) (Draft, error) {
	raw := answer.Structured
	if len(raw) == 0 {
		// The harness did not validate the shape itself, so the object has to be
		// found in what it wrote.
		raw = json.RawMessage(jsonObject(answer.Text))
	}
	var d Draft
	if err := json.Unmarshal(raw, &d); err != nil {
		return Draft{}, errors.New("the feedback assistant returned no usable answer")
	}
	return sanitize(d)
}

// jsonObject pulls the outermost JSON object out of an answer, tolerating the
// code fence and the odd sentence models like to wrap it in.
func jsonObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

// sanitize normalizes a draft and rejects one that says nothing usable.
func sanitize(d Draft) (Draft, error) {
	d.Title = clip(strings.Join(strings.Fields(strings.TrimSpace(d.Title)), " "), 120)
	d.Body = clip(strings.TrimSpace(d.Body), 4000)
	d.Question = clip(strings.TrimSpace(d.Question), 500)
	d.Labels = keepKnownLabels(d.Labels)
	switch {
	case d.Status == "ask" && d.Question != "":
		return Draft{Status: "ask", Question: d.Question}, nil
	case d.Title != "" || d.Body != "":
		d.Status = "ready"
		d.Question = ""
		if d.Title == "" {
			d.Title = "Feedback from the ClaudeQ app"
		}
		return d, nil
	}
	return Draft{}, errors.New("the feedback assistant returned no usable answer")
}

func keepKnownLabels(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range in {
		l = strings.ToLower(strings.TrimSpace(l))
		if seen[l] {
			continue
		}
		for _, k := range Labels {
			if l == k {
				out = append(out, l)
				seen[l] = true
				break
			}
		}
		// The two labels are mutually exclusive, so a draft carries exactly one.
		if len(out) == 1 {
			break
		}
	}
	return out
}

// clip cuts s to at most n runes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n]))
}

// begin looks up (or opens) a conversation and counts this turn. It returns the
// session id, how many user turns it now has, and whether the CLI must resume
// an existing session rather than start one.
func (s *Service) begin(sessionID string) (id string, turns int, resume bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	if sess, ok := s.sessions[sessionID]; ok {
		sess.turns++
		sess.seen = s.now()
		return sessionID, sess.turns, true
	}
	// An unknown id (daemon restarted, session expired) starts over rather than
	// failing: the user keeps typing and gets an issue either way.
	id = newSessionID()
	s.sessions[id] = &session{turns: 1, seen: s.now()}
	return id, 1, false
}

// newSessionID names a conversation before it exists, so two started at once
// cannot be confused for one another. A harness that insists on naming its own
// reports that name back and rename adopts it.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffff, time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// rename moves a conversation's bookkeeping to the id the harness reported for
// it, so the next turn finds the same count under the id the dashboard holds.
func (s *Service) rename(from, to string) string {
	if from == to {
		return to
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[from]; ok {
		delete(s.sessions, from)
		s.sessions[to] = sess
	}
	return to
}

func (s *Service) forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *Service) pruneLocked() {
	cutoff := s.now().Add(-sessionTTL)
	for id, sess := range s.sessions {
		if sess.seen.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

// MaxURLLen bounds the prefilled issue URL. GitHub answers a longer request
// with 414, so the body is trimmed to fit rather than losing the whole report.
const MaxURLLen = 6000

// IssueURL builds GitHub's prefilled "new issue" page for repo ("owner/name").
// Opening it in the browser is the only step that touches GitHub, and the user
// still has to press "Create" there.
func IssueURL(repo, title, body string, labels []string) string {
	base := "https://github.com/" + repo + "/issues/new"
	q := url.Values{}
	q.Set("title", title)
	if ls := keepKnownLabels(labels); len(ls) > 0 {
		q.Set("labels", strings.Join(ls, ","))
	}
	body = strings.TrimSpace(body)
	for {
		q.Set("body", body)
		u := base + "?" + q.Encode()
		if len(u) <= MaxURLLen || body == "" {
			return u
		}
		body = trimForURL(body, len(u)-MaxURLLen)
	}
}

// truncMark closes a body that had to be shortened, so the reader knows.
const truncMark = "\n\n…(shortened)"

// trimForURL removes at least over encoded bytes from the end of body. Encoding
// inflates characters (a newline costs three bytes), so the cut is made on rune
// boundaries and the loop in IssueURL re-measures.
func trimForURL(body string, over int) string {
	body = strings.TrimSuffix(body, truncMark)
	r := []rune(body)
	// One rune costs at least one encoded byte; take a few extra so a body full
	// of newlines still converges quickly.
	cut := over/3 + 16
	if cut >= len(r) {
		return ""
	}
	return strings.TrimRight(string(r[:len(r)-cut]), " \n\t") + truncMark
}
