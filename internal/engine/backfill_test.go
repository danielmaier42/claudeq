package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

var backfillNow = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// Half the week left: resetting in a day that is urgency 3.5 (at risk), in six
// days 0.58 (on pace).
func atRisk() provider.Limits { return weekReading(backfillNow, 50, 24*time.Hour) }
func onPace() provider.Limits { return weekReading(backfillNow, 50, 6*24*time.Hour) }

func backfillTask(id string, parallel bool) task.Task {
	t := asapTask(id, parallel)
	t.Backfill = true
	return t
}

func tickOnce(t *testing.T, e *Engine) {
	t.Helper()
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
}

// TestABackfillTaskWaitsForAllowanceThatWouldGoUnused is the feature: a
// backfill task stays queued while its provider is on pace, says why in the
// queue, and runs once the provider's week would otherwise expire unused.
func TestABackfillTaskWaitsForAllowanceThatWouldGoUnused(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	lim := map[string]provider.Limits{store.DefaultProviderID: onPace()}
	e.SetLimits(readings(lim))
	if err := st.SaveConfig(fallbackConfig("", backfillTask("a", false))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("runs = %+v, want none while the provider is on pace", got)
	}
	hold := e.Holds()["a"].Reason
	if !strings.HasPrefix(hold, "Backfill: ") || !strings.Contains(hold, "urgency 0.6") || !strings.Contains(hold, "above 1.5") {
		t.Fatalf("hold = %q, want the urgency and the threshold", hold)
	}

	lim[store.DefaultProviderID] = atRisk()
	tickOnce(t, e)
	if got := r.requests(); len(got) != 1 || got[0].Provider.ID != store.DefaultProviderID {
		t.Fatalf("runs = %+v, want one once the week is at risk", got)
	}
}

// TestABackfillTaskHonoursTheThreshold: the slider decides, not a constant.
func TestABackfillTaskHonoursTheThreshold(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	e.SetLimits(readings(map[string]provider.Limits{store.DefaultProviderID: atRisk()}))
	cfg := fallbackConfig("", backfillTask("a", false))
	cfg.Settings.BackfillUrgency = 4
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("runs = %+v, want none: 3.5 is not above 4", got)
	}
}

// TestABackfillTaskNeverGoesToAFallback: a limited provider's backfill work
// waits; the fallback's allowance is not the one at risk.
func TestABackfillTaskNeverGoesToAFallback(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	e.SetLimits(readings(map[string]provider.Limits{store.DefaultProviderID: atRisk(), "second": atRisk()}))
	if err := st.SaveConfig(fallbackConfig("second", backfillTask("a", false))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	e.gates.For(store.DefaultProviderID).BlockFor(time.Hour)

	tickOnce(t, e)
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("runs = %+v, want none", got)
	}
	if hold := e.Holds()["a"].Reason; !strings.Contains(hold, "waiting for its rate limit") {
		t.Fatalf("hold = %q, want the rate limit named", hold)
	}
}

// TestABackfillTaskWithoutAReadingWaits: nothing known about the allowance is
// not an allowance at risk.
func TestABackfillTaskWithoutAReadingWaits(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	if err := st.SaveConfig(fallbackConfig("", backfillTask("a", false))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("runs = %+v, want none", got)
	}
	if hold := e.Holds()["a"].Reason; !strings.Contains(hold, "no limit reading yet") {
		t.Fatalf("hold = %q, want the missing reading named", hold)
	}
}

// TestABackfillPoolTaskGoesOnlyToAMemberAtRisk: the pool's own ranking would
// pick the larger plan, but only the other member's week is at risk, so the
// backfill run goes there.
func TestABackfillPoolTaskGoesOnlyToAMemberAtRisk(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	e.SetLimits(readings(map[string]provider.Limits{store.DefaultProviderID: onPace(), "second": atRisk()}))
	bt := backfillTask("a", false)
	bt.Pool = "claude-pool"
	cfg := poolConfig(bt)
	cfg.Pools[0].Members[0].Weight = 10 // 0.58 × 10 outranks 3.5 × 1
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	if got := r.requests(); len(got) != 1 || got[0].Provider.ID != "second" {
		t.Fatalf("runs = %+v, want one on the member at risk", got)
	}
}

// TestABackfillPoolTaskWaitsWhileNoMemberIsAtRisk names every member's state.
func TestABackfillPoolTaskWaitsWhileNoMemberIsAtRisk(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	e.SetLimits(readings(map[string]provider.Limits{store.DefaultProviderID: onPace(), "second": onPace()}))
	bt := backfillTask("a", false)
	bt.Pool = "claude-pool"
	if err := st.SaveConfig(poolConfig(bt)); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("runs = %+v, want none", got)
	}
	hold := e.Holds()["a"].Reason
	if !strings.Contains(hold, "pool Claude") || strings.Count(hold, "urgency 0.6") != 2 {
		t.Fatalf("hold = %q, want both members' urgency", hold)
	}
}

// TestBackfillGoesAfterEveryOtherTask: a backfill task first in the queue
// does not take the exclusive slot from real work due at the same time.
func TestBackfillGoesAfterEveryOtherTask(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	e.SetLimits(readings(map[string]provider.Limits{store.DefaultProviderID: atRisk()}))
	if err := st.SaveConfig(fallbackConfig("", backfillTask("spare", false), asapTask("real", false))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	got := r.requests()
	if len(got) != 1 || got[0].Task.ID != "real" {
		t.Fatalf("runs = %+v, want only the real task", got)
	}
}

// TestParallelBackfillSharesTheUrgency: each backfill run started on a
// provider shares its urgency, so parallel backfill work stops once the
// allowance at risk is covered instead of all starting at once.
func TestParallelBackfillSharesTheUrgency(t *testing.T) {
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, clock.NewFake(backfillNow))
	e.SetLimits(readings(map[string]provider.Limits{store.DefaultProviderID: atRisk()}))
	// 3.5 alone, 1.75 with one run on it, 1.17 with two: two start at 1.5.
	if err := st.SaveConfig(fallbackConfig("", backfillTask("a", true), backfillTask("b", true), backfillTask("c", true))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	tickOnce(t, e)
	if got := r.requests(); len(got) != 2 {
		t.Fatalf("runs = %+v, want two", got)
	}
}
