package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
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
	s := computeStats(runs, now, statsWindow{Days: defaultStatsDays})

	if s.Totals.Runs != 4 || s.Totals.Success != 3 || s.Totals.Failed != 1 {
		t.Fatalf("totals wrong: %+v", s.Totals)
	}
	if s.Last7d.Runs != 2 || s.Last7d.Success != 1 || s.Last7d.Failed != 1 {
		t.Fatalf("last7d wrong: %+v", s.Last7d)
	}
	if diff := s.Last7d.CostUSD - 0.15; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("last7d cost = %v, want 0.15", s.Last7d.CostUSD)
	}
	if len(s.PerDay) != defaultStatsDays {
		t.Fatalf("per-day length = %d, want %d", len(s.PerDay), defaultStatsDays)
	}
	// The two recent runs land on the last day (today, local).
	last := s.PerDay[defaultStatsDays-1]
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
	s := computeStats(runs, now, statsWindow{Days: defaultStatsDays})

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
	if s := computeStats(runs, now, statsWindow{Days: defaultStatsDays}); s.ByProvider != nil {
		t.Fatalf("by_provider = %+v, want nothing to break down", s.ByProvider)
	}
}

// TestStatsSplitEachProviderByModel: under each provider the runs split per
// model, busiest first, with the harness default as a model of its own.
func TestStatsSplitEachProviderByModel(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	run := func(id, provider, model string, cost float64) store.Run {
		return store.Run{RunID: id, StartedAt: now, Status: store.StatusSuccess, CostUSD: cost, InputTokens: 10,
			Provider: store.RunProvider{ID: provider, Name: provider, Model: model}}
	}
	runs := []store.Run{
		run("1", "claude", "sonnet", 1),
		run("2", "claude", "opus", 2),
		run("3", "claude", "opus", 4),
		run("4", "claude", "", 8),
		run("5", "codex", "gpt", 16),
	}
	s := computeStats(runs, now, statsWindow{Days: defaultStatsDays})
	if len(s.ByProvider) != 2 {
		t.Fatalf("got %d provider buckets, want 2", len(s.ByProvider))
	}
	claude := s.ByProvider[0]
	if claude.ID != "claude" || claude.CostUSD != 15 {
		t.Fatalf("first bucket = %+v, want claude with its whole cost", claude)
	}
	var got []string
	for _, m := range claude.Models {
		got = append(got, fmt.Sprintf("%s:%d:%g", m.Model, m.Runs, m.CostUSD))
	}
	if want := []string{"opus:2:6", ":1:8", "sonnet:1:1"}; !slices.Equal(got, want) {
		t.Fatalf("claude models = %v, want %v", got, want)
	}
	if len(s.ByProvider[1].Models) != 1 || s.ByProvider[1].Models[0].Model != "gpt" {
		t.Fatalf("codex models = %+v, want gpt alone", s.ByProvider[1].Models)
	}
}

// TestStatsSplitOneProviderRunningSeveralModels: one provider with several
// models still has something to break down.
func TestStatsSplitOneProviderRunningSeveralModels(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	runs := []store.Run{
		{RunID: "1", StartedAt: now, Status: store.StatusSuccess,
			Provider: store.RunProvider{ID: "claude", Model: "opus"}},
		{RunID: "2", StartedAt: now, Status: store.StatusSuccess,
			Provider: store.RunProvider{ID: "claude", Model: "sonnet"}},
	}
	s := computeStats(runs, now, statsWindow{Days: defaultStatsDays})
	if len(s.ByProvider) != 1 || len(s.ByProvider[0].Models) != 2 {
		t.Fatalf("by_provider = %+v, want one provider with two models", s.ByProvider)
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
	s := computeStats(runs, now, statsWindow{Days: defaultStatsDays})
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

func TestStatsLeaveScriptRunsOutOfTheProviderSplit(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	runs := []store.Run{
		{RunID: "1", Status: store.StatusSuccess, StartedAt: now,
			Provider: store.RunProvider{ID: "claude", Name: "Claude Code"}},
		{RunID: "2", Status: store.StatusSuccess, StartedAt: now,
			Provider: store.RunProvider{ID: "codex", Name: "Codex"}},
		// A script run spends no allowance, so it belongs to no account.
		{RunID: "3", Status: store.StatusSuccess, StartedAt: now,
			Provider: store.RunProvider{Kind: store.ScriptRunKind, Name: "Script"}},
	}
	s := computeStats(runs, now, statsWindow{Days: defaultStatsDays})
	if s.Totals.Runs != 3 {
		t.Fatalf("total runs = %d, want every run counted", s.Totals.Runs)
	}
	if len(s.ByProvider) != 2 {
		t.Fatalf("by-provider rows = %+v, want only the two accounts", s.ByProvider)
	}
	for _, b := range s.ByProvider {
		if b.Name == "Script" {
			t.Fatal("a script run must not appear as a provider")
		}
	}
}

func TestParseStatsWindow(t *testing.T) {
	tests := []struct {
		in      string
		want    statsWindow
		wantErr string
	}{
		{in: "", want: statsWindow{Days: defaultStatsDays}},
		{in: "7", want: statsWindow{Days: 7}},
		{in: "1", want: statsWindow{Days: 1}},
		{in: "366", want: statsWindow{Days: maxStatsDays}},
		{in: "all", want: statsWindow{All: true}},
		{in: "0", wantErr: "between 1 and 366"},
		{in: "-3", wantErr: "between 1 and 366"},
		{in: "367", wantErr: "between 1 and 366"},
		{in: "week", wantErr: "number of days or 'all'"},
		{in: "7.5", wantErr: "number of days or 'all'"},
		{in: "ALL", wantErr: "number of days or 'all'"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseStatsWindow(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one mentioning %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

// TestStatsWindow: the period, the chart and the provider split all follow the
// chosen window, while the all-time totals and the last week stay what they are.
func TestStatsWindow(t *testing.T) {
	loc := time.FixedZone("CEST", 2*60*60)
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, loc)
	at := func(daysAgo, hour int) time.Time {
		return time.Date(2026, 10, 19-daysAgo, hour, 0, 0, 0, loc)
	}
	claude := store.RunProvider{ID: "claude", Name: "Claude Code"}
	codex := store.RunProvider{ID: "codex", Name: "Codex"}
	runs := []store.Run{
		{RunID: "1", StartedAt: at(0, 8), Status: store.StatusSuccess, CostUSD: 1, InputTokens: 10, Provider: claude},
		// Six days ago at the first minute still lies inside a 7-day window: the
		// window is calendar days, not 7×24h back from now.
		{RunID: "2", StartedAt: at(6, 0), Status: store.StatusFailed, CostUSD: 2, InputTokens: 20, Provider: codex},
		{RunID: "3", StartedAt: at(7, 23), Status: store.StatusSuccess, CostUSD: 4, InputTokens: 40, Provider: codex},
		{RunID: "4", StartedAt: at(40, 12), Status: store.StatusSuccess, CostUSD: 8, InputTokens: 80, Provider: codex},
		// Two years back: inside "all", outside any numbered window and the chart.
		{RunID: "5", StartedAt: time.Date(2024, 10, 1, 12, 0, 0, 0, loc), Status: store.StatusSuccess, CostUSD: 16, Provider: claude},
		// Never started: counted in the totals, but it cannot open the window.
		{RunID: "6", Status: store.StatusFailed, Provider: codex},
	}
	tests := []struct {
		name        string
		win         statsWindow
		days        int
		chartDays   int
		periodRuns  int
		periodCost  float64
		providerIDs []string // busiest first; nil when there is nothing to split
	}{
		{name: "one day", win: statsWindow{Days: 1}, days: 1, chartDays: 1, periodRuns: 1, periodCost: 1},
		{name: "7 days", win: statsWindow{Days: 7}, days: 7, chartDays: 7, periodRuns: 2, periodCost: 3, providerIDs: []string{"claude", "codex"}},
		{name: "default", win: statsWindow{Days: defaultStatsDays}, days: 14, chartDays: 14, periodRuns: 3, periodCost: 7, providerIDs: []string{"codex", "claude"}},
		{name: "90 days", win: statsWindow{Days: 90}, days: 90, chartDays: 90, periodRuns: 4, periodCost: 15, providerIDs: []string{"codex", "claude"}},
		// 2024-10-01 to 2026-10-19 inclusive; the chart keeps the newest year.
		{name: "all", win: statsWindow{All: true}, days: 749, chartDays: maxStatsDays, periodRuns: 6, periodCost: 31, providerIDs: []string{"codex", "claude"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := computeStats(runs, now, tt.win)
			if s.Days != tt.days || s.All != tt.win.All {
				t.Fatalf("days = %d, all = %v; want %d, %v", s.Days, s.All, tt.days, tt.win.All)
			}
			if len(s.PerDay) != tt.chartDays {
				t.Fatalf("per_day has %d days, want %d", len(s.PerDay), tt.chartDays)
			}
			if first, last := s.PerDay[0].Date, s.PerDay[len(s.PerDay)-1].Date; last != "2026-10-19" ||
				first != now.AddDate(0, 0, -(tt.chartDays-1)).Format("2006-01-02") {
				t.Fatalf("per_day spans %s..%s, want %d days ending today", first, last, tt.chartDays)
			}
			if s.Period.Runs != tt.periodRuns || s.Period.CostUSD != tt.periodCost {
				t.Fatalf("period = %+v, want %d runs costing %v", s.Period, tt.periodRuns, tt.periodCost)
			}
			var chartRuns int
			for _, d := range s.PerDay {
				chartRuns += d.Runs
			}
			// Only the never-started run and the one before the chart's year
			// are missing from the chart.
			if want := min(tt.periodRuns, 4); chartRuns != want {
				t.Fatalf("chart holds %d runs, want %d", chartRuns, want)
			}
			var ids []string
			for _, b := range s.ByProvider {
				ids = append(ids, b.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tt.providerIDs, ",") {
				t.Fatalf("by_provider = %v, want %v", ids, tt.providerIDs)
			}
			if s.Totals.Runs != len(runs) || s.Totals.CostUSD != 31 {
				t.Fatalf("totals = %+v, want every run whatever the window", s.Totals)
			}
			// The week is 7×24h back from now, so it reaches the evening seven
			// calendar days ago that the 7-day window leaves out.
			if s.Last7d.Runs != 3 {
				t.Fatalf("last_7d = %+v, want the rolling week whatever the window", s.Last7d)
			}
		})
	}
}

// TestStatsAllCountsCalendarDaysAcrossDST: the night the clocks go back is 25
// hours long, and the window still counts it as one day.
func TestStatsAllCountsCalendarDaysAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}
	now := time.Date(2026, 10, 26, 0, 30, 0, 0, loc)
	runs := []store.Run{{RunID: "1", StartedAt: time.Date(2026, 10, 24, 23, 30, 0, 0, loc), Status: store.StatusSuccess}}
	s := computeStats(runs, now, statsWindow{All: true})
	if s.Days != 3 || len(s.PerDay) != 3 || s.PerDay[0].Date != "2026-10-24" || s.PerDay[0].Runs != 1 {
		t.Fatalf("days = %d, per_day = %+v; want Oct 24..26 with the run on the first", s.Days, s.PerDay)
	}
}

func TestStatsAllWithoutRunsIsToday(t *testing.T) {
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	s := computeStats(nil, now, statsWindow{All: true})
	if s.Days != 1 || len(s.PerDay) != 1 || s.PerDay[0].Date != "2026-10-19" {
		t.Fatalf("days = %d, per_day = %+v; want today alone", s.Days, s.PerDay)
	}
}

func TestGetStatsDays(t *testing.T) {
	srv, st := newServer(t, nil)
	if err := st.AppendRun(store.Run{RunID: "r1", TaskID: "a", StartedAt: time.Now(), Status: store.StatusSuccess, CostUSD: 1}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}
	tests := []struct {
		query  string
		status int
		days   int
	}{
		{query: "", status: http.StatusOK, days: defaultStatsDays},
		{query: "?days=30", status: http.StatusOK, days: 30},
		{query: "?days=all", status: http.StatusOK, days: 1},
		{query: "?days=0", status: http.StatusBadRequest},
		{query: "?days=abc", status: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := do(t, srv, http.MethodGet, "/api/stats"+tt.query, nil)
			if r.Status != tt.status {
				t.Fatalf("status = %d, want %d (%s)", r.Status, tt.status, r.Body)
			}
			if tt.status != http.StatusOK {
				var e map[string]string
				r.into(t, &e)
				if !strings.Contains(e["error"], "'days'") {
					t.Fatalf("error = %q, want it to name the parameter", e["error"])
				}
				return
			}
			var s Stats
			r.into(t, &s)
			if s.Days != tt.days || len(s.PerDay) != tt.days || s.Period.Runs != 1 {
				t.Fatalf("days = %d, per_day = %d, period = %+v; want %d days holding the run", s.Days, len(s.PerDay), s.Period, tt.days)
			}
		})
	}
}
