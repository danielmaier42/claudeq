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
	"sync/atomic"
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
	// Bin overrides the Claude Code binary for this run (an absolute path from
	// settings). Empty falls back to the Executor's configured binary.
	Bin string
	// IdleTimeout kills the run if it produces no output for this long — a
	// hung/deadlocked process. Zero disables the watchdog.
	IdleTimeout time.Duration
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
	ResultText string        // the final result text from the CLI, if any
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

// binFor resolves the binary for a request: an explicit per-run override wins,
// otherwise the Executor's configured default.
func (e *Executor) binFor(req Request) string {
	if req.Bin != "" {
		return req.Bin
	}
	return e.bin()
}

// Run executes the request, streaming output to req.Log, and returns the
// classified result. A non-nil error indicates claudeq failed to run the CLI
// at all (as opposed to the CLI reporting a task failure, which is a Result).
func (e *Executor) Run(ctx context.Context, req Request) (Result, error) {
	bin := e.binFor(req)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin, e.Args(req)...)
	cmd.Dir = req.Task.WorkingDir
	configureProcessGroup(cmd) // so a killed run takes its child processes with it

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("stdout pipe: %w", err)
	}
	log := &syncWriter{w: req.Log}
	cmd.Stderr = log

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", bin, err)
	}

	// Idle watchdog: kill the process if it stops producing output for too long.
	var idleKilled atomic.Bool
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())
	if req.IdleTimeout > 0 {
		done := make(chan struct{})
		defer close(done)
		go idleWatch(req.IdleTimeout, &lastActivity, &idleKilled, cancel, done)
	}

	cls := classifier{sessionID: req.SessionID}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		lastActivity.Store(time.Now().UnixNano())
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
		} else if !idleKilled.Load() {
			return Result{}, fmt.Errorf("wait %s: %w", bin, waitErr)
		}
	}
	if idleKilled.Load() {
		return Result{
			Status:    store.StatusFailed,
			SessionID: cls.sessionID,
			ExitCode:  exitCode,
			Message:   fmt.Sprintf("stopped after %s of no output (looked hung)", req.IdleTimeout),
		}, nil
	}
	if scanErr != nil {
		// We could not read the output reliably, so we cannot trust the
		// classification; report it as a run error.
		return Result{}, fmt.Errorf("read %s output: %w", bin, scanErr)
	}

	return cls.result(exitCode), nil
}

// idleWatch cancels the run's context (killing the process) when no output has
// arrived for timeout. A working run keeps resetting lastActivity, so only a
// genuinely stalled process is killed.
func idleWatch(timeout time.Duration, lastActivity *atomic.Int64, killed *atomic.Bool, cancel context.CancelFunc, done <-chan struct{}) {
	interval := timeout / 4
	if interval < time.Second {
		interval = time.Second
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if time.Duration(time.Now().UnixNano()-lastActivity.Load()) > timeout {
				killed.Store(true)
				cancel()
				return
			}
		}
	}
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
	Type           string       `json:"type"`
	Subtype        string       `json:"subtype"`
	IsError        bool         `json:"is_error"`
	APIErrorStatus *int         `json:"api_error_status"`
	ErrorStatus    *int         `json:"error_status"`
	Error          string       `json:"error"`
	RetryDelayMS   *int         `json:"retry_delay_ms"`
	SessionID      string       `json:"session_id"`
	ResultText     string       `json:"result"`
	TotalCostUSD   float64      `json:"total_cost_usd"`
	NumTurns       int          `json:"num_turns"`
	DurationMS     int64        `json:"duration_ms"`
	Usage          *usageTokens `json:"usage"`
}

type usageTokens struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type classifier struct {
	sessionID  string
	sawResult  bool
	resultErr  bool
	rateLimit  bool
	authError  bool
	retryDelay time.Duration
	resultText string
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
	if ev.Type == "result" {
		c.sawResult = true
		c.resultErr = ev.IsError
		c.resultText = ev.ResultText
		m := &Metrics{CostUSD: ev.TotalCostUSD, NumTurns: ev.NumTurns, DurationMS: ev.DurationMS}
		if ev.Usage != nil {
			m.InputTokens = ev.Usage.InputTokens
			m.OutputTokens = ev.Usage.OutputTokens
		}
		c.metrics = m
	}
}

func (c *classifier) result(exitCode int) Result {
	res := Result{SessionID: c.sessionID, ExitCode: exitCode, RetryAfter: c.retryDelay, ResultText: c.resultText, Metrics: c.metrics}
	switch {
	case c.authError:
		res.Status = store.StatusAuthError
		res.Message = "Claude Code reported an authentication problem"
	case c.rateLimit && (!c.sawResult || c.resultErr):
		res.Status = store.StatusRateLimited
		res.Message = "rate limit hit; waiting for reset"
	case c.sawResult && !c.resultErr && exitCode == 0:
		res.Status = store.StatusSuccess
	case exitCode == -1:
		// Terminated by a signal (e.g. the daemon was stopped) before it could
		// finish. Not a task failure per se, but the run did not complete.
		res.Status = store.StatusFailed
		res.Message = "run was interrupted before completing (the process was terminated — e.g. the daemon stopped)"
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
