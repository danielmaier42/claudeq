// Package feedback turns a short chat with the user into a GitHub issue draft
// for the ClaudeQ repository. It runs the Claude Code CLI in a locked-down,
// tool-less print session and hands the resulting draft to the dashboard, which
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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Model is the model every feedback turn runs on. Drafting an issue from two
// short messages is cheap work, and it must not depend on the user's default
// model (which may be an expensive one they picked for real tasks).
const Model = "haiku"

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

// Labels are the only labels a draft may carry. They exist in the repository;
// letting the model invent its own would produce labels GitHub silently drops.
var Labels = []string{"bug", "enhancement", "documentation", "question"}

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
- Labels: at most two, only from the list the schema allows.`

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

// CLIRunner runs the Claude Code CLI in dir and returns its stdout. Injectable
// so the conversation logic is testable against a stub instead of the real API.
type CLIRunner interface {
	Run(ctx context.Context, dir string, argv []string) ([]byte, error)
}

// ExecRunner runs the CLI as a child process.
type ExecRunner struct{}

// Run executes argv in dir and returns stdout only, so a warning the CLI prints
// on stderr can never corrupt the JSON we parse.
func (ExecRunner) Run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv[0] is the configured/detected claude binary
	cmd.Dir = dir
	// No stdin: with a terminal or an open pipe the CLI waits for piped input
	// for a few seconds before giving up.
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return out, err
	}
	return out, nil
}

// Service holds the in-flight feedback conversations.
type Service struct {
	run CLIRunner
	now func() time.Time

	mu       sync.Mutex
	sessions map[string]*session
	dir      string // stable cwd for the CLI's session store
	dirErr   error
}

type session struct {
	turns int
	seen  time.Time
}

// New returns a Service running turns through r (nil means the real CLI).
func New(r CLIRunner) *Service {
	if r == nil {
		r = ExecRunner{}
	}
	return &Service{run: r, now: time.Now, sessions: map[string]*session{}}
}

// Turn sends text to the conversation identified by sessionID (empty starts a
// new one) and returns the assistant's answer. bin is the Claude Code binary to
// invoke.
func (s *Service) Turn(ctx context.Context, bin, sessionID, text string) (Draft, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Draft{}, errors.New("say what you would like to report before sending")
	}
	if len([]rune(text)) > maxInputChars {
		text = string([]rune(text)[:maxInputChars])
	}
	if bin == "" {
		return Draft{}, errors.New("the Claude Code CLI was not found")
	}

	dir, err := s.sessionDir()
	if err != nil {
		return Draft{}, err
	}
	id, turns, resume := s.begin(sessionID)
	last := turns >= MaxUserTurns
	if last {
		text += lastTurnNudge
	}

	ctx, cancel := context.WithTimeout(ctx, turnTimeout)
	defer cancel()
	out, err := s.run.Run(ctx, dir, argv(bin, id, resume, text))
	if err != nil {
		s.forget(id)
		return Draft{}, fmt.Errorf("the feedback assistant could not be reached: %w", err)
	}
	d, err := parse(out)
	if err != nil {
		s.forget(id)
		return Draft{}, err
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

// argv builds the CLI invocation for one turn. The session is deliberately
// bare: no tools, no MCP servers, no user settings, skills or CLAUDE.md, so a
// feedback chat can neither touch the machine nor drag project context into a
// public issue.
func argv(bin, sessionID string, resume bool, text string) []string {
	a := []string{bin, "-p",
		"--model", Model,
		"--tools", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--safe-mode",
		"--output-format", "json",
		"--json-schema", schema,
	}
	if resume {
		a = append(a, "--resume", sessionID)
	} else {
		a = append(a, "--session-id", sessionID, "--system-prompt", systemPrompt)
	}
	return append(a, text)
}

// cliResult is the subset of `--output-format json` we read.
type cliResult struct {
	IsError          bool            `json:"is_error"`
	Subtype          string          `json:"subtype"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

func parse(out []byte) (Draft, error) {
	var res cliResult
	if err := json.Unmarshal(out, &res); err != nil {
		return Draft{}, fmt.Errorf("the feedback assistant returned no usable answer: %w", err)
	}
	if res.IsError {
		msg := strings.TrimSpace(res.Result)
		if msg == "" {
			msg = res.Subtype
		}
		return Draft{}, fmt.Errorf("the feedback assistant failed: %s", msg)
	}
	raw := res.StructuredOutput
	if len(raw) == 0 {
		// No structured output: some CLI versions only put the JSON in `result`.
		raw = json.RawMessage(strings.TrimSpace(res.Result))
	}
	var d Draft
	if err := json.Unmarshal(raw, &d); err != nil {
		return Draft{}, errors.New("the feedback assistant returned no usable answer")
	}
	return sanitize(d)
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
		if len(out) == 2 {
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

// sessionDirName is the throwaway directory the CLI runs in, inside the user's
// own temporary directory (per-user on macOS, so the fixed name is not shared).
const sessionDirName = "claudeq-feedback"

// sessionDir is a stable empty directory the CLI runs in, so resuming a
// conversation finds it in Claude Code's per-project session store — and so no
// real project directory (with its CLAUDE.md and history) is ever the cwd. The
// name is fixed rather than random: Claude Code keys its session store by the
// directory, and a fresh path per daemon start would leave a new stale project
// behind at every login.
func (s *Service) sessionDir() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir != "" || s.dirErr != nil {
		return s.dir, s.dirErr
	}
	dir := filepath.Join(os.TempDir(), sessionDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.dirErr = fmt.Errorf("preparing the feedback session: %w", err)
		return "", s.dirErr
	}
	s.dir = dir
	return s.dir, nil
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Only the CLI ever sees this id, and it is namespaced per directory; a
		// time-based fallback keeps feedback working if the pool ever fails.
		return fmt.Sprintf("%08x-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffff, time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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
