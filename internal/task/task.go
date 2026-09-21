// Package task defines the claudeq task model and its validation rules.
package task

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	// PermissionsDefault leaves Claude Code's permission prompts in place.
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
	ID string `toml:"id" json:"id"`
	// Name is a human-readable label.
	Name string `toml:"name" json:"name"`
	// Prompt is the instruction sent to Claude Code.
	Prompt string `toml:"prompt" json:"prompt"`
	// WorkingDir is the directory Claude Code runs in (the task's context).
	WorkingDir string `toml:"working_dir" json:"working_dir"`
	// Group is the free-text folder the queue shows this task under. Empty means
	// the task is ungrouped and sits in the plain list. A group exists only as
	// long as a task names it: there is nothing else to create or delete.
	Group string `toml:"group,omitempty" json:"group,omitempty"`

	// Trigger selects how the task becomes eligible.
	Trigger Trigger `toml:"trigger" json:"trigger"`
	// FixedAt is the earliest start time for TriggerFixed.
	FixedAt time.Time `toml:"fixed_at,omitempty" json:"fixed_at,omitzero"`
	// Cron is the crontab expression for TriggerCron.
	Cron string `toml:"cron,omitempty" json:"cron,omitempty"`

	// Parallel allows this task to run alongside other parallel tasks.
	Parallel bool `toml:"parallel" json:"parallel"`
	// Enabled toggles the task without deleting it.
	Enabled bool `toml:"enabled" json:"enabled"`

	// Provider selects the configured provider instance this task runs on, by
	// its stable id (see internal/provider). Empty inherits the global default
	// provider, which is what every task migrated from a pre-provider config
	// does.
	Provider string `toml:"provider,omitempty" json:"provider,omitempty"`
	// Model overrides the effective provider's default model when non-empty.
	Model string `toml:"model,omitempty" json:"model,omitempty"`
	// ReasoningEffort asks the model to think harder or less hard. It is passed
	// only to a provider whose adapter takes such a setting, and ignored by the
	// rest, so it is safe to carry on a task that later moves to another one.
	ReasoningEffort string `toml:"reasoning_effort,omitempty" json:"reasoning_effort,omitempty"`
	// Permissions decides how Claude Code's permission prompts are handled.
	Permissions Permissions `toml:"permissions" json:"permissions"`
	// NotifyOnResult sends a notification with the outcome and last result
	// message when the run finishes (success or failure), not just on failures.
	NotifyOnResult bool `toml:"notify_on_result,omitempty" json:"notify_on_result,omitempty"`
	// DependsOn lists the one-shot job ids this task waits for. It becomes
	// eligible only once every one of them has reached a terminal result —
	// success, failure, auth error or cancellation. A rate-limited job that is
	// still scheduled to resume has not finished, so the wait continues.
	//
	// Dependencies are fixed when the task is queued and never change
	// afterwards. That is what rules out a cycle: a job can only name jobs that
	// already existed when it was created.
	DependsOn []string `toml:"depends_on,omitempty" json:"depends_on,omitempty"`
	// IncludeResults asks for what the dependencies answered to be put in front
	// of this task's prompt, so a join can consolidate them without going
	// looking for run logs. It means nothing without DependsOn.
	IncludeResults bool `toml:"include_results,omitempty" json:"include_results,omitempty"`

	// ParentRun is the run that queued this task, when one did.
	ParentRun string `toml:"parent_run,omitempty" json:"parent_run,omitempty"`
	// WorkflowID groups everything that came out of one piece of work. A task
	// queued by a run inherits that run's workflow; a run that has none starts
	// one under its own id.
	WorkflowID string `toml:"workflow_id,omitempty" json:"workflow_id,omitempty"`

	// QuietHistory keeps the task's routine runs out of the way: a run that
	// succeeds (or pauses on the rate limit, which resolves itself) is never
	// written to history and its log is deleted, so a frequent watcher job
	// neither floods the Activity view nor pushes real work out of the bounded
	// history. Runs that fail, hit an auth problem or are canceled are recorded
	// like any other.
	QuietHistory bool `toml:"quiet_history,omitempty" json:"quiet_history,omitempty"`
}

// PermissionsFor maps the CLI's "skip permission prompts" switch onto the
// stored value: on is PermissionsSkip, off is PermissionsDefault.
func PermissionsFor(skip bool) Permissions {
	if skip {
		return PermissionsSkip
	}
	return PermissionsDefault
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
		if err := CheckCron(t.Cron); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidTask, err)
		}
	default:
		return fmt.Errorf("%w: unknown trigger %q", ErrInvalidTask, t.Trigger)
	}

	if err := CheckGroup(t.Group); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTask, err)
	}

	switch t.Permissions {
	case PermissionsDefault, PermissionsSkip:
	default:
		return fmt.Errorf("%w: unknown permissions %q", ErrInvalidTask, t.Permissions)
	}

	// A recurring task cannot wait for a job that finishes once: its second
	// occurrence would find the same dependencies long terminal and run
	// immediately, which is not a dependency at all.
	if len(t.DependsOn) > 0 && t.Trigger == TriggerCron {
		return fmt.Errorf("%w: a recurring task cannot depend on other jobs", ErrInvalidTask)
	}
	for _, dep := range t.DependsOn {
		if dep == t.ID {
			return fmt.Errorf("%w: task %q cannot depend on itself", ErrInvalidTask, t.ID)
		}
		if err := CheckID(dep); err != nil {
			return fmt.Errorf("%w: dependency %q: %w", ErrInvalidTask, dep, err)
		}
	}

	return nil
}

// DependenciesEqual reports whether two tasks name the same dependencies, in
// the same order. Dependencies are fixed when a task is queued, so this is what
// an edit is checked against.
func DependenciesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CronSchedule parses the task's cron expression. It must only be called on a
// validated TriggerCron task.
func (t Task) CronSchedule() (cron.Schedule, error) {
	return CronParser.Parse(t.Cron)
}

// Slug derives a URL-safe id fragment from a human-readable name: lower-case
// letters and digits, with runs of spaces/dashes/underscores collapsed to one
// dash, at most 24 characters. An empty result becomes "task", so it is never
// empty.
func Slug(name string) string {
	var b strings.Builder
	dash := true // suppress a leading dash
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			dash = false
		case r == ' ' || r == '-' || r == '_':
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	slug := strings.TrimRight(b.String(), "-")
	if len(slug) > 24 {
		slug = strings.TrimRight(slug[:24], "-")
	}
	if slug == "" {
		return "task"
	}
	return slug
}

// MaxGroupLen is the longest group name the queue accepts. A group is a label
// in a list, not a description, and an unbounded one would push every row's
// controls off the screen.
const MaxGroupLen = 60

// ErrInvalidGroup is returned for a group name that cannot be stored.
var ErrInvalidGroup = errors.New("invalid group")

// CheckGroup reports whether name is usable as a group name. Empty (ungrouped)
// is always fine; anything else must be a single line of printable text, short
// enough to render as a header.
func CheckGroup(name string) error {
	if name == "" {
		return nil
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("%w: %q has leading or trailing whitespace", ErrInvalidGroup, name)
	}
	if utf8.RuneCountInString(name) > MaxGroupLen {
		return fmt.Errorf("%w: at most %d characters", ErrInvalidGroup, MaxGroupLen)
	}
	for _, r := range name {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsControl(r) {
			return fmt.Errorf("%w: %q contains a control character", ErrInvalidGroup, name)
		}
	}
	return nil
}

// CheckID reports whether id is usable as a task id: letters, digits, dot,
// dash and underscore only, starting with a letter or digit. Ids appear in
// API paths and file names, so a slash, space or "..", would break the app's
// own URLs. Generated ids (Slug plus a suffix) always pass; the check is for
// ids that a user or a shared file supplies.
func CheckID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: missing id", ErrInvalidTask)
	}
	for i, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			i > 0 && (r == '.' || r == '-' || r == '_')
		if !ok {
			return fmt.Errorf("%w: id %q may only contain letters, digits, '.', '-' and '_' (and must start with a letter or digit)", ErrInvalidTask, id)
		}
	}
	return nil
}
