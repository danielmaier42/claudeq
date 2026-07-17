// Package executor runs a task through the Claude Code CLI in headless mode and
// classifies the outcome (success / rate-limited / auth-error / failed) from
// the stream-json output. Flags follow the verified CLI behaviour (PLAN.md §8).
package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// Executor builds and runs Claude Code invocations.
type Executor struct {
	// Bin is the Claude Code binary (default "claude").
	Bin string
}

// Request is a single execution. Model and SkipPermissions are the already
// resolved effective values (task override applied over global defaults).
type Request struct {
	// Task is what to run.
	Task task.Task
	// SessionID is the UUID claudeq assigns for this task's session so it can be
	// resumed later (PLAN.md V1).
	SessionID string
	// Resume continues an existing session instead of starting fresh.
	Resume bool
	// Model is the effective model; empty means use Claude Code's own default.
	Model string
	// SkipPermissions bypasses permission prompts for this run.
	SkipPermissions bool
	// Log receives the raw CLI output (stdout + stderr), streamed live.
	Log io.Writer
}

// Result is the classified outcome of a run.
type Result struct {
	Status     store.RunStatus
	SessionID  string
	ExitCode   int
	RetryAfter time.Duration // set when Status == StatusRateLimited
	Message    string        // short human-readable detail
	Usage      *UsageInfo    // latest rate-limit/usage info seen, if any
	Metrics    *Metrics      // cost/token/timing from the result event, if any
}

// Metrics are the cost/token/timing figures from the CLI's result event.
type Metrics struct {
	CostUSD      float64
	InputTokens  int
	OutputTokens int
	NumTurns     int
	DurationMS   int64
}

// UsageInfo is the rate_limit_info the CLI reports in stream-json.
type UsageInfo struct {
	Status         string
	LimitType      string
	Utilization    float64
	IsUsingOverage bool
	ResetsAtUnix   int64
}

// Args returns the CLI arguments for a request (excluding the binary name).
// Exposed for testing and transparency.
func (e *Executor) Args(req Request) []string {
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	if req.Resume {
		args = append(args, "--resume", req.SessionID)
	} else {
		args = append(args, "--session-id", req.SessionID)
	}
	args = append(args, req.Task.Prompt)
	return args
}

func (e *Executor) bin() string {
	if e.Bin != "" {
		return e.Bin
	}
	return "claude"
}

// Run executes the request, streaming output to req.Log, and returns the
// classified result. A non-nil error indicates claudeq failed to run the CLI
// at all (as opposed to the CLI reporting a task failure, which is a Result).
func (e *Executor) Run(ctx context.Context, req Request) (Result, error) {
	cmd := exec.CommandContext(ctx, e.bin(), e.Args(req)...)
	cmd.Dir = req.Task.WorkingDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stdout pipe: %w", err)
	}
	log := &syncWriter{w: req.Log}
	cmd.Stderr = log

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", e.bin(), err)
	}

	cls := classifier{sessionID: req.SessionID}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		_, _ = log.Write(append(cloneLine(line), '\n'))
		cls.consume(line)
	}
	scanErr := sc.Err()

	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode()
		} else {
			return Result{}, fmt.Errorf("wait %s: %w", e.bin(), waitErr)
		}
	}
	if scanErr != nil {
		// We could not read the output reliably, so we cannot trust the
		// classification; report it as a run error.
		return Result{}, fmt.Errorf("read %s output: %w", e.bin(), scanErr)
	}

	return cls.result(exitCode), nil
}

// cloneLine copies a scanner slice, whose backing array is reused on the next
// Scan, so we can safely hand it to the log writer.
func cloneLine(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// streamEvent covers the fields we read from both the final `result` envelope
// and intermediate `api_retry` system events. Unknown fields are ignored.
type streamEvent struct {
	Type           string         `json:"type"`
	Subtype        string         `json:"subtype"`
	IsError        bool           `json:"is_error"`
	APIErrorStatus *int           `json:"api_error_status"`
	ErrorStatus    *int           `json:"error_status"`
	Error          string         `json:"error"`
	RetryDelayMS   *int           `json:"retry_delay_ms"`
	SessionID      string         `json:"session_id"`
	RateLimitInfo  *rateLimitInfo `json:"rate_limit_info"`
	TotalCostUSD   float64        `json:"total_cost_usd"`
	NumTurns       int            `json:"num_turns"`
	DurationMS     int64          `json:"duration_ms"`
	Usage          *usageTokens   `json:"usage"`
}

type usageTokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type rateLimitInfo struct {
	Status         string  `json:"status"`
	ResetsAt       int64   `json:"resetsAt"`
	RateLimitType  string  `json:"rateLimitType"`
	Utilization    float64 `json:"utilization"`
	IsUsingOverage bool    `json:"isUsingOverage"`
}

type classifier struct {
	sessionID  string
	sawResult  bool
	resultErr  bool
	rateLimit  bool
	authError  bool
	retryDelay time.Duration
	usage      *UsageInfo
	metrics    *Metrics
}

func (c *classifier) consume(line []byte) {
	var ev streamEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return // non-JSON or partial line: ignore for classification
	}
	if ev.SessionID != "" {
		c.sessionID = ev.SessionID
	}
	switch ev.Error {
	case "rate_limit":
		c.rateLimit = true
	case "authentication_failed":
		c.authError = true
	}
	if statusIs(ev.APIErrorStatus, 429) || statusIs(ev.ErrorStatus, 429) {
		c.rateLimit = true
	}
	if statusIs(ev.APIErrorStatus, 401) || statusIs(ev.ErrorStatus, 401) {
		c.authError = true
	}
	if ev.RetryDelayMS != nil && *ev.RetryDelayMS > 0 {
		c.retryDelay = time.Duration(*ev.RetryDelayMS) * time.Millisecond
	}
	if ev.RateLimitInfo != nil {
		c.usage = &UsageInfo{
			Status:         ev.RateLimitInfo.Status,
			LimitType:      ev.RateLimitInfo.RateLimitType,
			Utilization:    ev.RateLimitInfo.Utilization,
			IsUsingOverage: ev.RateLimitInfo.IsUsingOverage,
			ResetsAtUnix:   ev.RateLimitInfo.ResetsAt,
		}
	}
	if ev.Type == "result" {
		c.sawResult = true
		c.resultErr = ev.IsError
		m := &Metrics{CostUSD: ev.TotalCostUSD, NumTurns: ev.NumTurns, DurationMS: ev.DurationMS}
		if ev.Usage != nil {
			m.InputTokens = ev.Usage.InputTokens
			m.OutputTokens = ev.Usage.OutputTokens
		}
		c.metrics = m
	}
}

func (c *classifier) result(exitCode int) Result {
	res := Result{SessionID: c.sessionID, ExitCode: exitCode, RetryAfter: c.retryDelay, Usage: c.usage, Metrics: c.metrics}
	switch {
	case c.authError:
		res.Status = store.StatusAuthError
		res.Message = "Claude Code reported an authentication problem"
	case c.rateLimit && (!c.sawResult || c.resultErr):
		res.Status = store.StatusRateLimited
		res.Message = "rate limit hit; waiting for reset"
	case c.sawResult && !c.resultErr && exitCode == 0:
		res.Status = store.StatusSuccess
	default:
		res.Status = store.StatusFailed
		res.Message = fmt.Sprintf("run failed (exit %d)", exitCode)
	}
	return res
}

func statusIs(p *int, want int) bool { return p != nil && *p == want }

// syncWriter serializes concurrent writes from the stdout scan loop and the
// stderr copy.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w == nil {
		return len(p), nil
	}
	return s.w.Write(p)
}
