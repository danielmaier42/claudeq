package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/provider"
)

// A run that ends has moved its account's allowance, so the engine says which
// provider it ran on. A script job runs on none and says nothing.
func TestRunFinishedNamesTheProviderOfAgentRunsOnly(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 10, 2, 2, 0, 0, 0, time.UTC))
	e, st := newTestEngine(t, &stub{}, fc)
	var mu sync.Mutex
	var got []string
	e.SetRunFinished(func(id string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, id)
	})
	saveTasks(t, st, asapTask("agent", true), scriptJob("watch"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != provider.DefaultInstanceID {
		t.Fatalf("run-finished calls = %v, want one for %q", got, provider.DefaultInstanceID)
	}
}
