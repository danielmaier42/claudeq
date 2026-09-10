// Package review checks a prompt against the machine it will run on before the
// task is queued: the paths it names, the folders they would be written into,
// and the instruction files it tells the run to read. The findings and the
// rewrite are produced by Claude Code itself, run headless, tool-free and
// short-lived — claudeq only supplies the filesystem facts.
package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
	// Model is the model to review with. Empty lets Claude Code pick.
	Model string
	// Bin overrides the Claude Code binary (an absolute path from settings).
	Bin string
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

// Runner executes the Claude Code binary and returns its stdout. It is a field
// on Reviewer so tests can drive the whole pipeline without the real CLI.
type Runner func(ctx context.Context, bin, dir string, args []string) ([]byte, error)

// Reviewer runs prompt reviews through the Claude Code CLI.
type Reviewer struct {
	// Bin is the Claude Code binary used when a request does not override it.
	Bin string
	// Home is the user's home directory, used to expand "~" in a prompt's paths
	// and as the neutral working directory of the review process itself. Empty
	// falls back to os.UserHomeDir.
	Home string
	// Timeout bounds one review; zero uses DefaultTimeout.
	Timeout time.Duration
	// Run executes the CLI; nil uses the real one.
	Run Runner
}

// ErrNoBinary means claudeq does not know where the Claude Code binary is, so
// no review can run. The caller reports it as "review unavailable", never as a
// finding about the prompt.
var ErrNoBinary = errors.New("no claude binary configured")

// Review inspects the prompt and returns what to tell the operator. An empty
// prompt is fine by definition and costs no model call.
func (r *Reviewer) Review(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return Result{OK: true}, nil
	}
	bin := req.Bin
	if bin == "" {
		bin = r.Bin
	}
	if bin == "" {
		return Result{}, ErrNoBinary
	}

	home := r.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	facts := Inspect(req.Prompt, req.WorkingDir, home)

	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	run := r.Run
	if run == nil {
		run = execRun
	}
	// The review reads nothing itself, so it runs in a neutral directory rather
	// than in the task's folder — which may not even exist yet.
	dir := home
	if dir == "" {
		dir = os.TempDir()
	}
	out, err := run(ctx, bin, dir, Args(req.Model, systemPrompt(req.Kind), userMessage(req, facts)))
	if err != nil {
		return Result{}, fmt.Errorf("run %s: %w", bin, err)
	}
	res, err := parseResult(out)
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

// Args builds the CLI arguments for a review (excluding the binary name).
// Exposed for testing and transparency.
//
// The review is deliberately the narrowest Claude Code invocation claudeq can
// make: --tools "" removes every tool (all the filesystem facts it needs are in
// the message already), --safe-mode drops CLAUDE.md, skills, plugins, hooks and
// MCP servers so a review costs the same everywhere and cannot be steered by a
// project's own configuration, and --no-session-persistence keeps these
// throwaway turns out of the user's resumable session history.
func Args(model, system, message string) []string {
	args := []string{
		"-p", "--output-format", "json",
		"--safe-mode", "--no-session-persistence", "--tools", "",
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "--system-prompt", system, message)
	return args
}

func execRun(ctx context.Context, bin, dir string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin is the configured Claude Code binary
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(shorten(string(ee.Stderr), 300)))
		}
		return nil, err
	}
	return out, nil
}

// cliEnvelope is the part of `claude -p --output-format json` claudeq reads.
type cliEnvelope struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
	Subtype string `json:"subtype"`
}

// parseResult unwraps the CLI's JSON envelope and then the review object the
// model wrote into it.
func parseResult(out []byte) (Result, error) {
	var env cliEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		return Result{}, fmt.Errorf("parse claude output: %w", err)
	}
	if env.IsError {
		return Result{}, fmt.Errorf("claude reported %s: %s", orDefault(env.Subtype, "an error"), shorten(env.Result, 200))
	}
	body := jsonObject(env.Result)
	if body == "" {
		return Result{}, fmt.Errorf("no review in claude's answer: %s", shorten(env.Result, 200))
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

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
