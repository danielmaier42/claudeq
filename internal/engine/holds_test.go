package engine

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

func tick(t *testing.T, e *Engine) {
	t.Helper()
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

func TestHoldNamesThePause(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 10, 5, 15, 40, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	e.holdLog = &bytes.Buffer{}
	savePaused(t, st, asapTask("a", true))

	tick(t, e)
	h, ok := e.Holds()["a"]
	if !ok || h.Reason != "Runs are paused." || !h.Since.Equal(fc.Now()) {
		t.Fatalf("hold while paused = %+v, %v", h, ok)
	}

	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Settings.Paused = false
		return nil
	}); err != nil {
		t.Fatalf("unpause: %v", err)
	}
	tick(t, e)
	e.WaitIdle()
	if _, ok := e.Holds()["a"]; ok {
		t.Fatal("a started task still has a hold")
	}
}

func TestHoldNamesTheExclusiveRunAndIsLoggedOnce(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 10, 5, 15, 40, 0, 0, time.UTC))
	r := &stub{block: make(chan struct{})}
	e, st := newTestEngine(t, r, fc)
	var log bytes.Buffer
	e.holdLog = &log
	saveTasks(t, st, asapTask("first", false), asapTask("second", true))

	tick(t, e)
	waitFor(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.active == 1
	})
	holds := e.Holds()
	if _, ok := holds["first"]; ok {
		t.Fatalf("the started task has a hold: %+v", holds)
	}
	since := fc.Now()
	if h := holds["second"]; !strings.Contains(h.Reason, "runs alone") || !h.Since.Equal(since) {
		t.Fatalf("hold of second = %+v", h)
	}

	fc.Advance(4 * time.Minute)
	tick(t, e)
	if log.Len() != 0 {
		t.Fatalf("a short wait was logged: %q", log.String())
	}
	fc.Advance(2 * time.Minute)
	tick(t, e)
	tick(t, e)
	if got := strings.Count(log.String(), "\n"); got != 1 || !strings.Contains(log.String(), `task "second" has been due for 6m0s`) {
		t.Fatalf("log = %q", log.String())
	}
	if h := e.Holds()["second"]; !h.Since.Equal(since) {
		t.Fatalf("the wait lost its start: %+v", h)
	}

	close(r.block)
	e.WaitIdle()
	tick(t, e)
	e.WaitIdle()
	if len(e.Holds()) != 0 {
		t.Fatalf("holds after everything ran: %+v", e.Holds())
	}
}

func TestHoldNamesTheProviderVerdict(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 10, 5, 15, 40, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	e.holdLog = &bytes.Buffer{}
	ad.set(provider.Health{State: provider.HealthNotInstalled, Reason: "Claude Code is not installed."})
	saveTasks(t, st, asapTask("a", true))

	tick(t, e)
	if h := e.Holds()["a"]; h.Reason != "Claude Code is not installed." {
		t.Fatalf("hold = %+v", h)
	}
}

func TestHoldNamesTheExhaustedAllowance(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 10, 5, 15, 40, 0, 0, time.Local))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	e.holdLog = &bytes.Buffer{}
	saveTasks(t, st, asapTask("a", true))
	e.gates.For(store.DefaultProviderID).BlockFor(time.Hour)

	tick(t, e)
	if h := e.Holds()["a"]; !strings.Contains(h.Reason, "out of allowance until 16:40") {
		t.Fatalf("hold = %+v", h)
	}
}

func TestReopenTextNamesTheDayWhenItIsNotToday(t *testing.T) {
	now := time.Date(2026, 10, 5, 15, 40, 0, 0, time.Local)
	if got := reopenText(now.Add(time.Hour), now); got != "16:40" {
		t.Fatalf("same day = %q", got)
	}
	if got := reopenText(now.Add(72*time.Hour), now); got != "Thu 8 Oct 15:40" {
		t.Fatalf("later day = %q", got)
	}
}
