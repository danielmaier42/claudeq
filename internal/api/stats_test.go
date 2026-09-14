package api

import (
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

// TestStatsSplitByProvider: several providers mean several allowances, so the
// summary says which one the work went to instead of adding them together.
func TestStatsSplitByProvider(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	runs := []store.Run{
		{RunID: "1", StartedAt: now, Status: store.StatusSuccess, CostUSD: 1, InputTokens: 10,
			Provider: store.RunProvider{ID: "claude", Name: "Claude Code"}},
		{RunID: "2", StartedAt: now, Status: store.StatusFailed, CostUSD: 2, InputTokens: 20,
			Provider: store.RunProvider{ID: "codex", Name: "Codex"}},
		{RunID: "3", StartedAt: now, Status: store.StatusSuccess, CostUSD: 4, InputTokens: 30,
			Provider: store.RunProvider{ID: "codex", Name: "Codex"}},
	}
	s := computeStats(runs, now)

	if len(s.ByProvider) != 2 {
		t.Fatalf("got %d provider buckets, want 2", len(s.ByProvider))
	}
	// Busiest first.
	if s.ByProvider[0].ID != "codex" || s.ByProvider[0].Runs != 2 {
		t.Fatalf("first bucket = %+v, want codex with 2 runs", s.ByProvider[0])
	}
	if s.ByProvider[0].CostUSD != 6 || s.ByProvider[0].Failed != 1 {
		t.Fatalf("codex bucket = %+v, want its own cost and failures", s.ByProvider[0])
	}
	if s.ByProvider[1].ID != "claude" || s.ByProvider[1].InputTokens != 10 {
		t.Fatalf("second bucket = %+v, want claude alone", s.ByProvider[1])
	}
	if s.Totals.CostUSD != 7 {
		t.Fatalf("totals = %+v, want everything still summed", s.Totals)
	}
}

// TestStatsSkipTheSplitForOneProvider: a breakdown of one row is not a
// breakdown, and every existing installation has exactly one.
func TestStatsSkipTheSplitForOneProvider(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	runs := []store.Run{
		{RunID: "1", StartedAt: now, Status: store.StatusSuccess,
			Provider: store.RunProvider{ID: "claude", Name: "Claude Code"}},
		{RunID: "2", StartedAt: now, Status: store.StatusSuccess,
			Provider: store.RunProvider{ID: "claude", Name: "Claude Code"}},
	}
	if s := computeStats(runs, now); s.ByProvider != nil {
		t.Fatalf("by_provider = %+v, want nothing to break down", s.ByProvider)
	}
}

// TestStatsKeepRunsFromBeforeProvidersWereRecorded: history written before a
// run said what it ran on still counts, under a bucket of its own.
func TestStatsKeepRunsFromBeforeProvidersWereRecorded(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	runs := []store.Run{
		{RunID: "1", StartedAt: now, Status: store.StatusSuccess, CostUSD: 1},
		{RunID: "2", StartedAt: now, Status: store.StatusSuccess, CostUSD: 2,
			Provider: store.RunProvider{ID: "codex", Name: "Codex"}},
	}
	s := computeStats(runs, now)
	if len(s.ByProvider) != 2 {
		t.Fatalf("got %d buckets, want the unrecorded runs kept", len(s.ByProvider))
	}
	var unknown *ProviderBucket
	for i := range s.ByProvider {
		if s.ByProvider[i].ID == "" {
			unknown = &s.ByProvider[i]
		}
	}
	if unknown == nil || unknown.CostUSD != 1 {
		t.Fatalf("buckets = %+v, want the unrecorded run's cost kept", s.ByProvider)
	}
}
