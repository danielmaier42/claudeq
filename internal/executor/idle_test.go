package executor

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleStepKillsWhenInactive(t *testing.T) {
	const interval = 30 * time.Second
	const timeout = 30 * time.Minute
	var last atomic.Int64
	base := int64(1_000_000_000_000)
	last.Store(base) // last activity at base

	// Normal tick 31 minutes later, small gap since the previous tick.
	now := base + int64(31*time.Minute)
	lastTick := now - int64(interval) // previous tick 30s ago (no sleep)
	_, kill := idleStep(now, lastTick, interval, timeout, &last)
	if !kill {
		t.Fatal("expected kill after 31 min of inactivity with no sleep gap")
	}
}

func TestIdleStepRebasesAcrossSleep(t *testing.T) {
	const interval = 30 * time.Second
	const timeout = 30 * time.Minute
	var last atomic.Int64
	base := int64(1_000_000_000_000)
	last.Store(base)

	// A 2-hour gap since the last tick == the machine slept: must NOT kill, and
	// must rebase lastActivity to now.
	now := base + int64(2*time.Hour)
	lastTick := base // last tick was 2h ago
	newTick, kill := idleStep(now, lastTick, interval, timeout, &last)
	if kill {
		t.Fatal("must not kill across a sleep gap")
	}
	if last.Load() != now {
		t.Fatalf("activity clock not rebased: got %d, want %d", last.Load(), now)
	}
	if newTick != now {
		t.Fatalf("lastTick = %d, want %d", newTick, now)
	}

	// After the rebase, a fresh normal tick well within the timeout stays alive.
	if _, k := idleStep(now+int64(interval), now, interval, timeout, &last); k {
		t.Fatal("should stay alive right after waking")
	}
}
