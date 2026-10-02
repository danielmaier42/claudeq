package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// limitsStub is the test harness with an allowance: every read reports the
// week at the number of reads so far, so a test can tell a new reading from a
// remembered one.
type limitsStub struct {
	stubAdapter
	reads *atomic.Int32
}

func (s limitsStub) ReadLimits(context.Context, provider.Instance) ([]provider.LimitWindow, error) {
	n := s.reads.Add(1)
	return []provider.LimitWindow{{ID: "week", Label: "Week", UsedPercent: float64(n)}}, nil
}

func TestListLimits(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	reads := &atomic.Int32{}
	reg := provider.NewRegistry(limitsStub{stubAdapter: stubAdapter{health: provider.Health{State: provider.HealthReady}}, reads: reads})
	mon := provider.NewLimitMonitor(reg, nil)
	mon.MinAge = 1 // any reading is old enough for a fresh request
	srv := httptest.NewServer(Handler(Deps{Store: st, Registry: reg, Providers: provider.NewChecker(reg), Limits: mon}))
	t.Cleanup(srv.Close)

	var got []limitsView
	do(t, srv, http.MethodGet, "/api/limits", nil).into(t, &got)
	if len(got) != 1 || got[0].ID != provider.DefaultInstanceID || !got[0].Default || got[0].TypeName != "Claude" {
		t.Fatalf("limits = %+v, want the default instance", got)
	}
	if l := got[0].Limits; l.State != provider.LimitsOK || len(l.Windows) != 1 || l.Windows[0].UsedPercent != 1 {
		t.Fatalf("limits = %+v, want the first reading", l)
	}
	// The poll answers from memory…
	do(t, srv, http.MethodGet, "/api/limits", nil).into(t, &got)
	if got[0].Limits.Windows[0].UsedPercent != 1 {
		t.Fatalf("poll read again: %+v", got[0].Limits)
	}
	// …and opening the dashboard reads again.
	do(t, srv, http.MethodGet, "/api/limits?fresh=1", nil).into(t, &got)
	if got[0].Limits.Windows[0].UsedPercent != 2 || reads.Load() != 2 {
		t.Fatalf("fresh = %+v after %d reads, want a second reading", got[0].Limits, reads.Load())
	}
}

func TestListLimitsWithoutAMonitor(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	srv := httptest.NewServer(handler(Deps{Store: st}))
	t.Cleanup(srv.Close)
	var got []limitsView
	do(t, srv, http.MethodGet, "/api/limits", nil).into(t, &got)
	if len(got) != 0 {
		t.Fatalf("limits = %+v, want none", got)
	}
}
