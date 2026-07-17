package api

import (
	"context"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

func TestComputeStats(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	runs := []store.Run{
		{StartedAt: now.Add(-1 * time.Hour), Status: store.StatusSuccess, CostUSD: 0.10, InputTokens: 100, OutputTokens: 50, DurationMS: 2000},
		{StartedAt: now.Add(-2 * time.Hour), Status: store.StatusFailed, CostUSD: 0.05, InputTokens: 10, OutputTokens: 5, DurationMS: 1000},
		{StartedAt: now.AddDate(0, 0, -10), Status: store.StatusSuccess, CostUSD: 1.00, InputTokens: 1000, OutputTokens: 500, DurationMS: 9000}, // outside 7d, inside 14d
		{StartedAt: now.AddDate(0, 0, -30), Status: store.StatusSuccess, CostUSD: 2.00},                                                         // outside both windows
	}
	s := computeStats(runs, now)

	if s.Totals.Runs != 4 || s.Totals.Success != 3 || s.Totals.Failed != 1 {
		t.Fatalf("totals wrong: %+v", s.Totals)
	}
	if s.Last7d.Runs != 2 || s.Last7d.Success != 1 || s.Last7d.Failed != 1 {
		t.Fatalf("last7d wrong: %+v", s.Last7d)
	}
	if diff := s.Last7d.CostUSD - 0.15; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("last7d cost = %v, want 0.15", s.Last7d.CostUSD)
	}
	if len(s.PerDay) != perDayDays {
		t.Fatalf("per-day length = %d, want %d", len(s.PerDay), perDayDays)
	}
	// The two recent runs land on the last day (today, local).
	last := s.PerDay[perDayDays-1]
	if last.Runs != 2 {
		t.Fatalf("today's runs = %d, want 2", last.Runs)
	}
}

type stubRefresher struct {
	called bool
	err    error
	usage  store.Usage
	store  *store.Store
}

func (s *stubRefresher) RefreshUsage(_ context.Context) error {
	s.called = true
	if s.err != nil {
		return s.err
	}
	return s.store.SaveUsage(s.usage)
}
