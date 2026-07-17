// Package task defines the claudeq task model and its validation rules.
package task

import (
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// Trigger is how a task becomes eligible to run.
type Trigger string

const (
	// TriggerASAP runs the task once, as soon as the limit allows.
	TriggerASAP Trigger = "asap"
	// TriggerFixed runs the task once, at or after a fixed time (earliest start).
	TriggerFixed Trigger = "fixed"
	// TriggerCron runs the task repeatedly on a crontab schedule.
	TriggerCron Trigger = "cron"
)

// Permissions selects how Claude Code's permission prompts are handled.
type Permissions string

const (
	// PermissionsDefault uses the global default permission behaviour.
	PermissionsDefault Permissions = "default"
	// PermissionsSkip bypasses all permission prompts for this task.
	PermissionsSkip Permissions = "skip"
)

// ModelDefault means the task uses the global default model.
const ModelDefault = ""

// CronParser accepts standard 5-field crontab expressions.
var CronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// Task is a single queued unit of work for Claude Code.
type Task struct {
	// ID is a stable unique identifier.
	ID string `toml:"id"`
	// Name is a human-readable label.
	Name string `toml:"name"`
	// Prompt is the instruction sent to Claude Code.
	Prompt string `toml:"prompt"`
	// WorkingDir is the directory Claude Code runs in (the task's context).
	WorkingDir string `toml:"working_dir"`

	// Trigger selects how the task becomes eligible.
	Trigger Trigger `toml:"trigger"`
	// FixedAt is the earliest start time for TriggerFixed.
	FixedAt time.Time `toml:"fixed_at,omitempty"`
	// Cron is the crontab expression for TriggerCron.
	Cron string `toml:"cron,omitempty"`

	// Parallel allows this task to run alongside other parallel tasks.
	Parallel bool `toml:"parallel"`
	// Enabled toggles the task without deleting it.
	Enabled bool `toml:"enabled"`

	// Model overrides the global default model when non-empty.
	Model string `toml:"model,omitempty"`
	// Permissions overrides the global default permission behaviour.
	Permissions Permissions `toml:"permissions"`
}

// ErrInvalidTask is the base error for validation failures.
var ErrInvalidTask = errors.New("invalid task")

// Validate reports whether the task is well-formed.
func (t Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidTask)
	}
	if t.Prompt == "" {
		return fmt.Errorf("%w: missing prompt", ErrInvalidTask)
	}
	if t.WorkingDir == "" {
		return fmt.Errorf("%w: missing working_dir", ErrInvalidTask)
	}

	switch t.Trigger {
	case TriggerASAP:
		// no extra fields required
	case TriggerFixed:
		if t.FixedAt.IsZero() {
			return fmt.Errorf("%w: trigger %q requires fixed_at", ErrInvalidTask, t.Trigger)
		}
	case TriggerCron:
		if t.Cron == "" {
			return fmt.Errorf("%w: trigger %q requires cron", ErrInvalidTask, t.Trigger)
		}
		if _, err := CronParser.Parse(t.Cron); err != nil {
			return fmt.Errorf("%w: invalid cron %q: %w", ErrInvalidTask, t.Cron, err)
		}
	default:
		return fmt.Errorf("%w: unknown trigger %q", ErrInvalidTask, t.Trigger)
	}

	switch t.Permissions {
	case PermissionsDefault, PermissionsSkip:
	default:
		return fmt.Errorf("%w: unknown permissions %q", ErrInvalidTask, t.Permissions)
	}

	return nil
}

// CronSchedule parses the task's cron expression. It must only be called on a
// validated TriggerCron task.
func (t Task) CronSchedule() (cron.Schedule, error) {
	return CronParser.Parse(t.Cron)
}
