package api

import (
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// TestBackfillEndpointSetsOnlyTheThreshold: the Dashboard's slider applies on
// its own, refuses a value outside the range, and a stale settings form
// cannot put an old threshold back.
func TestBackfillEndpointSetsOnlyTheThreshold(t *testing.T) {
	srv, st := newServer(t, nil)
	if err := st.SaveConfig(store.Config{Settings: store.Settings{SystemPrompt: "be brief"}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if r := do(t, srv, "POST", "/api/backfill", map[string]float64{"urgency": 9}); r.Status != http.StatusBadRequest {
		t.Fatalf("out of range = %d (%s), want 400", r.Status, r.Body)
	}
	if r := do(t, srv, "POST", "/api/backfill", map[string]float64{"urgency": 2.5}); r.Status != http.StatusOK {
		t.Fatalf("set = %d (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Settings.BackfillUrgency != 2.5 || cfg.Settings.SystemPrompt != "be brief" {
		t.Fatalf("settings = %+v, want the threshold and nothing else changed", cfg.Settings)
	}
	if r := do(t, srv, "PUT", "/api/settings", map[string]any{"system_prompt": "x", "backfill_urgency": 1}); r.Status != http.StatusOK {
		t.Fatalf("put settings = %d", r.Status)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Settings.BackfillUrgency != 2.5 {
		t.Fatalf("a settings form reset the threshold to %v", cfg.Settings.BackfillUrgency)
	}
}

// TestListLimitsShowsUrgency: every provider's urgency is on the Dashboard,
// lit up above the threshold, and shared with the runs in flight.
func TestListLimitsShowsUrgency(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := st.SaveConfig(store.Config{Settings: store.Settings{BackfillUrgency: 0.5}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	reg := provider.NewRegistry(limitsStub{stubAdapter: stubAdapter{health: provider.Health{State: provider.HealthReady}}, reads: &atomic.Int32{}})
	running := map[string]int{}
	srv := httptest.NewServer(Handler(Deps{Store: st, Registry: reg, Providers: provider.NewChecker(reg),
		Limits: provider.NewLimitMonitor(reg, nil), RunningOn: func() map[string]int { return running }}))
	t.Cleanup(srv.Close)

	// The stub's week is 1% used and has no reset time: urgency 0.99.
	var got []limitsView
	do(t, srv, http.MethodGet, "/api/limits", nil).into(t, &got)
	if len(got) != 1 || got[0].Urgency == nil || math.Abs(got[0].Urgency.Urgency-0.99) > 1e-9 || !got[0].Backfill {
		t.Fatalf("limits = %+v, want urgency 0.99 above 0.5", got)
	}
	running[provider.DefaultInstanceID] = 1
	do(t, srv, http.MethodGet, "/api/limits", nil).into(t, &got)
	if got[0].Urgency.Running != 1 || got[0].Backfill {
		t.Fatalf("limits = %+v, want the run to share the urgency below 0.5", got[0].Urgency)
	}
}
