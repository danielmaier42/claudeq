// Package schedule contains the pure scheduling logic: when a task is due, and
// which due tasks may start given the current concurrency state. It has no I/O
// so it is fully unit-testable (PLAN.md §7, FA-11/12/13/14).
package schedule

import (
	"fmt"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

// Inputs are the per-task facts needed to decide whether a task is due.
type Inputs struct {
	// Now is the evaluation time.
	Now time.Time
	// Running is true if a run of this task is currently active. A task never
	// starts a second concurrent run (one-shot guard and cron overlap-skip).
	Running bool
	// CompletedOnce is true if a one-shot (asap/fixed) task has already run.
	CompletedOnce bool
	// CronAnchor is the time from which the next cron occurrence is computed:
	// the last run's start, or the task's first-seen time. Required for cron.
	CronAnchor time.Time
}

// Due reports whether the task should start now.
func Due(t task.Task, in Inputs) (bool, error) {
	if !t.Enabled || in.Running {
		return false, nil
	}

	switch t.Trigger {
	case task.TriggerASAP:
		return !in.CompletedOnce, nil
	case task.TriggerFixed:
		return !in.CompletedOnce && !in.Now.Before(t.FixedAt), nil
	case task.TriggerCron:
		sched, err := t.CronSchedule()
		if err != nil {
			return false, fmt.Errorf("task %q: %w", t.ID, err)
		}
		next := sched.Next(in.CronAnchor)
		return !next.After(in.Now), nil
	default:
		return false, fmt.Errorf("task %q: unknown trigger %q", t.ID, t.Trigger)
	}
}

// Running summarizes what is currently executing, for concurrency decisions.
type Running struct {
	// NonParallel is true if an exclusive (non-parallel) task is running.
	NonParallel bool
	// Parallel is true if at least one parallel task is running.
	Parallel bool
}

// Select chooses which of the due tasks to start this tick. due must be in
// priority order (highest first). The rules (PLAN.md §7):
//
//   - An exclusive (non-parallel) task runs alone.
//   - Parallel tasks may run together with other parallel tasks.
//   - Strict priority is preserved: we never start a lower-priority task ahead
//     of a higher-priority due task that is merely waiting for exclusivity.
func Select(due []task.Task, running Running) []task.Task {
	if running.NonParallel {
		return nil // the exclusive slot is taken
	}

	var start []task.Task
	for _, t := range due {
		if t.Parallel {
			start = append(start, t)
			continue
		}
		// A non-parallel task needs the exclusive slot: nothing running and
		// nothing already selected this tick. If it can't have it, stop —
		// starting a lower-priority task ahead of it would break priority.
		if running.Parallel || len(start) > 0 {
			break
		}
		return []task.Task{t}
	}
	return start
}
