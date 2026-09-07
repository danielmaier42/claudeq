package store

import (
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

// RunStatus is the outcome (or current state) of a run.
type RunStatus string

const (
	// StatusRunning means the run is currently executing.
	StatusRunning RunStatus = "running"
	// StatusSuccess means Claude Code finished successfully.
	StatusSuccess RunStatus = "success"
	// StatusFailed means the run failed (non-auth, non-rate-limit).
	StatusFailed RunStatus = "failed"
	// StatusRateLimited means the run stopped on a rate limit and is waiting
	// for the gate to reopen (PLAN.md D1).
	StatusRateLimited RunStatus = "rate_limited_waiting"
	// StatusAuthError means Claude Code reported a login/authentication problem.
	StatusAuthError RunStatus = "auth_error"
	// StatusCanceled means the user stopped the run from the dashboard.
	StatusCanceled RunStatus = "canceled"
)

// Terminal reports whether the status is a final outcome.
func (s RunStatus) Terminal() bool {
	switch s {
	case StatusSuccess, StatusFailed, StatusAuthError, StatusCanceled:
		return true
	default:
		return false
	}
}

// Run is one execution of a task. History is an append-only event log; the
// latest event for a run id is authoritative (see Store.Runs).
type Run struct {
	RunID      string     `json:"run_id"`
	TaskID     string     `json:"task_id"`
	TaskName   string     `json:"task_name"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     RunStatus  `json:"status"`
	SessionID  string     `json:"session_id,omitempty"`
	ExitCode   int        `json:"exit_code"`
	LogPath    string     `json:"log_path"`
	Error      string     `json:"error,omitempty"`

	// Task is a snapshot of the definition this run used, so the run can be
	// replayed from history even after the task leaves the queue.
	Task *task.Task `json:"task,omitempty"`

	// Metrics reported by the CLI's result event (zero when unavailable).
	CostUSD      float64 `json:"cost_usd,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	DurationMS   int64   `json:"duration_ms,omitempty"`
}
