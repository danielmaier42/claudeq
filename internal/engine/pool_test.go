package engine

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// poolConfig is two Claude accounts in one pool, with the given tasks on it.
func poolConfig(tasks ...task.Task) store.Config {
	cfg := fallbackConfig("", tasks...)
	cfg.Pools = []store.Pool{{ID: "claude-pool", Name: "Claude", Members: []store.PoolMember{
		{Provider: store.DefaultProviderID}, {Provider: "second"},
	}}}
	return cfg
}

func poolTask(id string, parallel bool) task.Task {
	t := asapTask(id, parallel)
	t.Pool = "claude-pool"
	return t
}

// weekReading is a reading with the given share of the week used, resetting
// after resetsIn.
func weekReading(now time.Time, used float64, resetsIn time.Duration) provider.Limits {
	at := now.Add(resetsIn)
	return provider.Limits{State: provider.LimitsOK, Windows: []provider.LimitWindow{
		{ID: "week", Label: "Week", UsedPercent: used, ResetsAt: &at},
	}}
}

func readings(m map[string]provider.Limits) func(context.Context, provider.Instance) (provider.Limits, bool) {
	return func(_ context.Context, inst provider.Instance) (provider.Limits, bool) {
		l, ok := m[inst.ID]
		return l, ok
	}
}

// TestAPoolRunsOnTheMemberWhoseAllowanceExpiresFirst is the feature: of two
// accounts, the one whose weekly allowance would otherwise go unused first
// gets the run, and the run's log says why.
func TestAPoolRunsOnTheMemberWhoseAllowanceExpiresFirst(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	fc := clock.NewFake(now)
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, fc)
	e.SetLimits(readings(map[string]provider.Limits{
		store.DefaultProviderID: weekReading(now, 40, 5*24*time.Hour),
		"second":                weekReading(now, 90, 4*time.Hour),
	}))
	if err := st.SaveConfig(poolConfig(poolTask("a", false))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 1 || reqs[0].Provider.ID != "second" {
		t.Fatalf("runs = %+v, want one on the member resetting soon", reqs)
	}
	log, err := os.ReadFile(reqs[0].Log.(*os.File).Name())
	if err != nil {
		t.Fatalf("read run log: %v", err)
	}
	if !strings.Contains(string(log), "Pool Claude chose Second account") {
		t.Fatalf("the run log should explain the choice, got %q", log)
	}
	runs, err := st.Runs()
	if err != nil || len(runs) != 1 || runs[0].Provider.Pool != "Claude" || runs[0].Provider.ID != "second" {
		t.Fatalf("run record = %+v (%v), want the member and the pool", runs, err)
	}
	// A job this run queues names the pool too, not the member it landed on.
	if reqs[0].InheritProvider != "" {
		t.Fatalf("inherit = %q, want the pool to be inherited", reqs[0].InheritProvider)
	}
}

// TestPoolRunsStartedTogetherSpreadOut: two parallel pool tasks due in one
// tick go to two accounts, not both to the one that ranked first.
func TestPoolRunsStartedTogetherSpreadOut(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	fc := clock.NewFake(now)
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, fc)
	e.SetLimits(readings(map[string]provider.Limits{
		store.DefaultProviderID: weekReading(now, 50, 3*24*time.Hour),
		"second":                weekReading(now, 50, 3*24*time.Hour),
	}))
	if err := st.SaveConfig(poolConfig(poolTask("a", true), poolTask("b", true))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	got := map[string]bool{}
	for _, req := range r.requests() {
		got[req.Provider.ID] = true
	}
	if !got[store.DefaultProviderID] || !got["second"] {
		t.Fatalf("ran on %v, want both members", got)
	}
}

// TestAPoolSessionResumesOrMovesOn: a pool task whose session waits on one
// member continues there while that member is open, and starts over on another
// member while it is limited instead of waiting — that is what the pool is for.
func TestAPoolSessionResumesOrMovesOn(t *testing.T) {
	for _, tc := range []struct {
		name       string
		limited    bool
		wantOn     string
		wantResume bool
	}{
		{"member open: the session continues", false, "second", true},
		{"member limited: a fresh start elsewhere", true, store.DefaultProviderID, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			fc := clock.NewFake(now)
			r := &stub{}
			e, st, _ := newTestEngineWithProvider(t, r, fc)
			// The ranking alone would pick the default member.
			e.SetLimits(readings(map[string]provider.Limits{
				store.DefaultProviderID: weekReading(now, 10, 2*time.Hour),
				"second":                weekReading(now, 10, 6*24*time.Hour),
			}))
			if err := st.SaveConfig(poolConfig(poolTask("a", false))); err != nil {
				t.Fatalf("SaveConfig: %v", err)
			}
			if err := st.UpdateState(func(s *store.State) error {
				s.SetPendingResume("a", "sess-waiting", "second")
				return nil
			}); err != nil {
				t.Fatalf("UpdateState: %v", err)
			}
			if tc.limited {
				e.gates.For("second").BlockFor(time.Hour)
			}

			if err := e.Tick(context.Background()); err != nil {
				t.Fatalf("Tick: %v", err)
			}
			e.WaitIdle()
			reqs := r.requests()
			if len(reqs) != 1 {
				t.Fatalf("runs = %+v, want one", reqs)
			}
			got := reqs[0]
			if got.Provider.ID != tc.wantOn || got.Resume != tc.wantResume || (got.SessionID == "sess-waiting") != tc.wantResume {
				t.Fatalf("ran on %q resume=%v session=%q, want %q resume=%v",
					got.Provider.ID, got.Resume, got.SessionID, tc.wantOn, tc.wantResume)
			}
		})
	}
}

// TestAPoolWithEveryMemberLimitedWaits: nothing runs while no member has
// allowance; the task stays queued.
func TestAPoolWithEveryMemberLimitedWaits(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, _ := newTestEngineWithProvider(t, r, fc)
	if err := st.SaveConfig(poolConfig(poolTask("a", false))); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	e.gates.For(store.DefaultProviderID).BlockFor(time.Hour)
	e.gates.For("second").BlockFor(time.Hour)

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if got := r.requests(); len(got) != 0 {
		t.Fatalf("runs = %+v, want none while every member is limited", got)
	}
}
