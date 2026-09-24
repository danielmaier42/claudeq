// Package review checks a prompt against the machine it will run on before the
// task is queued: the paths it names, the folders they would be written into,
// and the instruction files it tells the run to read. The findings and the
// rewrite are produced by a harness, asked as an aside — tool-free,
// short-lived, in a directory of its own — while claudeq supplies the
// filesystem facts. Which harness answers is the operator's choice; this
// package does not know one from another.
package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/aside"
	"github.com/danielmaier42/claudeq/internal/provider"
)

// Kind is which of claudeq's two prompt fields is being reviewed. They differ
// in what a finding means: a task prompt runs in one working directory, while
// the global system prompt is prepended to every task in every directory.
type Kind string

const (
	// KindTask reviews a single task's prompt against its working directory.
	KindTask Kind = "task"
	// KindSystem reviews the global custom system prompt, which has no working
	// directory of its own.
	KindSystem Kind = "system"
	// KindScript reviews a script job's program, which runs in its working
	// directory under the daemon's environment rather than going to a model.
	KindScript Kind = "script"
)

// DefaultTimeout bounds a single review. The reviewer runs tool-free on one
// turn, so anything slower is a stuck process, not a thorough answer.
const DefaultTimeout = 90 * time.Second

// maxMessage caps the finding text. The banner is a one-glance hint above the
// prompt, not a report; a model that writes an essay gets cut off.
const maxMessage = 600

// Request is one review.
type Request struct {
	// Kind selects which prompt field this is.
	Kind Kind
	// Prompt is the text to review.
	Prompt string
	// WorkingDir is the task's working directory; relative paths in the prompt
	// resolve against it. Empty for KindSystem.
	WorkingDir string
	// Model is the model to review with. Empty leaves the choice to the harness.
	Model string
	// Provider is the instance that answers. A zero instance cannot, and the
	// review reports itself unavailable.
	Provider provider.Instance
}

// Result is what the review found.
type Result struct {
	// OK is true when nothing needs saying — no banner is shown.
	OK bool `json:"ok"`
	// Message is the finding, in one or two plain sentences.
	Message string `json:"message,omitempty"`
	// RevisedPrompt is the prompt rewritten to fix the finding. Empty when the
	// finding is something only the operator can decide.
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// Asker puts one question to a provider instance (satisfied by *aside.Runner).
// It is an interface so this package can be driven end to end without starting
// a CLI.
type Asker interface {
	Ask(ctx context.Context, inst provider.Instance, req provider.AsideRequest) (provider.Aside, error)
}

// ErrUnavailable means no review can run right now: no provider is chosen for
// it, or the chosen one cannot answer claudeq's questions. The caller reports
// it as "review unavailable", never as a finding about the prompt.
var ErrUnavailable = aside.ErrUnavailable

// Reviewer runs prompt reviews as asides.
type Reviewer struct {
	// Home is the user's home directory, used to expand "~" in a prompt's paths.
	// Empty falls back to os.UserHomeDir.
	Home string
	// PATH is the search path a script job runs with, for looking up the
	// commands it calls. Empty falls back to this process's own PATH, which is
	// the one the daemon hands its script jobs.
	PATH string
	// Timeout bounds one review; zero uses DefaultTimeout.
	Timeout time.Duration
	// Ask runs the review turn.
	Ask Asker
}

// Review inspects the prompt and returns what to tell the operator. An empty
// prompt is fine by definition and costs no model call.
func (r *Reviewer) Review(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return Result{OK: true}, nil
	}
	if r.Ask == nil || req.Provider.ID == "" {
		return Result{}, fmt.Errorf("%w: no provider is configured to review prompts", ErrUnavailable)
	}

	home := r.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	facts := Inspect(req.Prompt, req.WorkingDir, home)
	var cmds []Command
	if req.Kind == KindScript && isShellScript(req.Prompt) {
		pathEnv := r.PATH
		if pathEnv == "" {
			pathEnv = os.Getenv("PATH")
		}
		cmds = InspectCommands(req.Prompt, pathEnv, home)
	}

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// One question, one answer: the review keeps no session, and reads nothing
	// itself — every filesystem fact it needs is already in the message.
	answer, err := r.Ask.Ask(ctx, req.Provider, provider.AsideRequest{
		Model:  req.Model,
		System: systemPrompt(req.Kind),
		Text:   userMessage(req, facts, cmds),
	})
	if err != nil {
		return Result{}, err
	}
	res, err := parseResult(answer)
	if err != nil {
		return Result{}, err
	}
	// A "rewrite" that changes nothing is not a suggestion; dropping it here
	// leaves the finding standing without an Apply button that does nothing.
	if res.RevisedPrompt == strings.TrimRight(req.Prompt, "\n") {
		res.RevisedPrompt = ""
	}
	return res, nil
}

// parseResult reads the review object out of what the harness said. A model
// that wraps its JSON in a code fence or a sentence has still answered, so the
// object is pulled out rather than the whole thing rejected.
func parseResult(answer provider.Aside) (Result, error) {
	body := string(answer.Structured)
	if body == "" {
		body = jsonObject(answer.Text)
	}
	if body == "" {
		return Result{}, fmt.Errorf("no review in the answer: %s", shorten(answer.Text, 200))
	}
	var res Result
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return Result{}, fmt.Errorf("parse review: %w", err)
	}
	return normalize(res), nil
}

// normalize makes a model's answer safe to show: a finding without text says
// nothing, and a "revision" identical to the input is not one.
func normalize(res Result) Result {
	if res.OK {
		return Result{OK: true}
	}
	res.Message = shorten(strings.TrimSpace(res.Message), maxMessage)
	res.RevisedPrompt = strings.TrimRight(res.RevisedPrompt, "\n")
	if res.Message == "" {
		return Result{OK: true}
	}
	return res
}

// jsonObject pulls the outermost JSON object out of a model answer, tolerating
// the code fence and the odd sentence models like to wrap it in.
func jsonObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}
