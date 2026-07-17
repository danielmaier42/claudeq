// Package wake schedules the machine to wake from sleep for upcoming work
// (PLAN.md D8/FA-32). It computes the next relevant wake time and registers it
// with `pmset schedule wake`. Scheduling requires root, so the real runner uses
// sudo; failures are surfaced to the caller, which treats them as non-fatal.
package wake

import (
	"context"
	"fmt"
	"time"

	"github.com/danielmaier42/claudeq/internal/system"
)

// pmsetTimeLayout is the date/time format expected by `pmset schedule`.
const pmsetTimeLayout = "01/02/2006 15:04:05"

// NextWakeTime returns the earliest time the daemon should wake, considering the
// concrete candidate times (future fixed/cron occurrences, a rate-limit reset)
// and a periodic heartbeat. The heartbeat guarantees a wake even when nothing
// concrete is pending (covers "as soon as possible" tasks). It returns false
// only when there is nothing to schedule (no candidates and no heartbeat).
func NextWakeTime(now time.Time, candidates []time.Time, heartbeat time.Duration) (time.Time, bool) {
	best := time.Time{}
	have := false
	consider := func(t time.Time) {
		if !t.After(now) {
			return
		}
		if !have || t.Before(best) {
			best, have = t, true
		}
	}
	for _, c := range candidates {
		consider(c)
	}
	if heartbeat > 0 {
		consider(now.Add(heartbeat))
	}
	return best, have
}

// defaultMinReschedule is how close a new wake must be to the last one to be
// treated as unchanged, avoiding pmset churn from a drifting heartbeat target.
const defaultMinReschedule = 60 * time.Second

// Scheduler registers wake times with pmset. It cancels the wake it previously
// scheduled before registering a new one, so events do not pile up.
type Scheduler struct {
	Runner system.Runner
	// Sudo wraps pmset in `sudo -n` (needed because pmset requires root).
	Sudo bool
	// MinReschedule suppresses rescheduling when the new wake is within this of
	// the last one. Zero uses defaultMinReschedule.
	MinReschedule time.Duration

	last    time.Time
	hasLast bool
}

// Schedule registers a single wake at t, cancelling any previously scheduled
// wake first. A new time within MinReschedule of the last is treated as
// unchanged (a no-op), so a drifting heartbeat target does not thrash pmset.
func (s *Scheduler) Schedule(ctx context.Context, t time.Time) error {
	if s.hasLast && within(t, s.last, s.minReschedule()) {
		return nil
	}
	if s.hasLast {
		// Best-effort cancel; ignore errors (the event may already be gone).
		_, _ = s.pmset(ctx, "schedule", "cancel", "wake", s.last.Format(pmsetTimeLayout))
	}
	if out, err := s.pmset(ctx, "schedule", "wake", t.Format(pmsetTimeLayout)); err != nil {
		return fmt.Errorf("pmset schedule wake: %w (%s)", err, string(out))
	}
	s.last, s.hasLast = t, true
	return nil
}

// Cancel removes the wake this scheduler last registered, if any.
func (s *Scheduler) Cancel(ctx context.Context) error {
	if !s.hasLast {
		return nil
	}
	out, err := s.pmset(ctx, "schedule", "cancel", "wake", s.last.Format(pmsetTimeLayout))
	s.hasLast = false
	if err != nil {
		return fmt.Errorf("pmset schedule cancel: %w (%s)", err, string(out))
	}
	return nil
}

func (s *Scheduler) minReschedule() time.Duration {
	if s.MinReschedule > 0 {
		return s.MinReschedule
	}
	return defaultMinReschedule
}

func within(a, b time.Time, tol time.Duration) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func (s *Scheduler) pmset(ctx context.Context, args ...string) ([]byte, error) {
	if s.Sudo {
		return s.Runner.Run(ctx, "sudo", append([]string{"-n", "pmset"}, args...)...)
	}
	return s.Runner.Run(ctx, "pmset", args...)
}
