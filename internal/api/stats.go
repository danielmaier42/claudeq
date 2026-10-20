package api

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

// Bucket aggregates run metrics over a period.
type Bucket struct {
	Runs         int     `json:"runs"`
	Success      int     `json:"success"`
	Failed       int     `json:"failed"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	DurationMS   int64   `json:"duration_ms"`
}

// DayBucket is one calendar day's totals.
type DayBucket struct {
	Date    string  `json:"date"` // YYYY-MM-DD (local)
	Runs    int     `json:"runs"`
	CostUSD float64 `json:"cost_usd"`
	Tokens  int     `json:"tokens"`
}

// ProviderBucket is what one provider instance consumed. Several providers mean
// several allowances, and a total that adds them together answers a question
// nobody has: it is the split that tells you which account the month went on.
type ProviderBucket struct {
	// ID is the provider instance as the runs recorded it. Empty groups the runs
	// from before dispatchq wrote one down.
	ID string `json:"id"`
	// Name is what that provider was called when the runs happened. A provider
	// renamed since keeps the name its history was written under, because that
	// is what the run says.
	Name string `json:"name"`
	Bucket
	// Models splits the provider's runs per model, busiest first. One provider
	// often runs several, and the model decides what a run costs.
	Models []ModelBucket `json:"models,omitempty"`
}

// ModelBucket is what one model consumed on one provider. Model is the one the
// harness reported running where it did, else the one the task asked for;
// empty groups the runs where neither is known.
type ModelBucket struct {
	Model string `json:"model"`
	Bucket
}

// Stats is the consumption summary shown in the dashboard.
type Stats struct {
	Totals Bucket `json:"totals"`
	Last7d Bucket `json:"last_7d"`
	// Period sums the runs of the chosen window: the last Days calendar days,
	// today included, or everything recorded when All is set.
	Period Bucket `json:"period"`
	// Days is the length of the window actually used. For All it counts from the
	// first recorded run's day to today, so a caller can say "over 213 days".
	Days int  `json:"days"`
	All  bool `json:"all,omitempty"`
	// PerDay is one entry per calendar day of the window, oldest first. A long
	// history is cut to the last maxStatsDays days: a chart of years of days is
	// unreadable, and the window's totals stay in Period regardless.
	PerDay []DayBucket `json:"per_day"`
	// ByProvider splits the period per provider instance, busiest first, and
	// each provider per model. It is omitted while every run in it came from the
	// same provider and model: a breakdown of one row is not a breakdown.
	ByProvider []ProviderBucket `json:"by_provider,omitempty"`
}

const (
	// defaultStatsDays is the window of a plain GET /api/stats, which is what
	// the dashboard has always shown.
	defaultStatsDays = 14
	// maxStatsDays caps a window at a year (a leap year's worth): enough for
	// any question the usage view asks, small enough to stay a cheap response.
	maxStatsDays = 366
)

// statsWindow is the period a stats request asks about. All means everything
// recorded; Days is ignored then.
type statsWindow struct {
	Days int
	All  bool
}

// parseStatsWindow reads the optional `days` query parameter: a number of
// calendar days, or "all". Absent means the default window, so a client that
// knows nothing about the parameter gets the answer it always got.
func parseStatsWindow(v string) (statsWindow, error) {
	switch v {
	case "":
		return statsWindow{Days: defaultStatsDays}, nil
	case "all":
		return statsWindow{All: true}, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return statsWindow{}, fmt.Errorf("query param 'days' must be a number of days or 'all', got %q", v)
	}
	if n < 1 || n > maxStatsDays {
		return statsWindow{}, fmt.Errorf("query param 'days' must be between 1 and %d, got %d", maxStatsDays, n)
	}
	return statsWindow{Days: n}, nil
}

// computeStats aggregates the run history over the window. now anchors the
// relative windows, and its location decides which calendar day a run falls
// on. win is what parseStatsWindow accepted. Only terminal runs count toward
// success/failed; metrics sum wherever present.
func computeStats(runs []store.Run, now time.Time, win statsWindow) Stats {
	loc := now.Location()
	dayOf := func(t time.Time) string { return t.In(loc).Format("2006-01-02") }
	today := now.In(loc)
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)

	days := win.Days
	if win.All {
		days = 1
		// The window opens on the first run's day. A run without a start time
		// never ran and says nothing about when the history began.
		for _, r := range runs {
			if r.StartedAt.IsZero() {
				continue
			}
			t := r.StartedAt.In(loc)
			first := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
			// Calendar days, not 24h steps: a DST change makes one day 23 hours.
			if n := calendarDaysBetween(first, todayStart) + 1; n > days {
				days = n
			}
		}
	}
	s := Stats{Days: days, All: win.All}
	weekAgo := now.Add(-7 * 24 * time.Hour)
	// The window's first day as a key; keys sort like the days they name.
	since := todayStart.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	// Prepare the per-day skeleton for the window, cut to the newest days.
	chartDays := min(days, maxStatsDays)
	dayIdx := map[string]int{}
	s.PerDay = make([]DayBucket, chartDays)
	for i := range chartDays {
		key := todayStart.AddDate(0, 0, -(chartDays - 1 - i)).Format("2006-01-02")
		s.PerDay[i] = DayBucket{Date: key}
		dayIdx[key] = i
	}

	byProvider := map[string]*ProviderBucket{}
	byModel := map[string]map[string]*ModelBucket{}
	var order []string
	for _, r := range runs {
		add(&s.Totals, r)
		if !r.StartedAt.Before(weekAgo) {
			add(&s.Last7d, r)
		}
		key := dayOf(r.StartedAt)
		if !win.All && key < since {
			continue
		}
		add(&s.Period, r)
		if i, ok := dayIdx[key]; ok {
			s.PerDay[i].Runs++
			s.PerDay[i].CostUSD += r.CostUSD
			s.PerDay[i].Tokens += r.InputTokens + r.OutputTokens
		}
		// A script run went to no account and spent no allowance, so it has no
		// place in a split that answers "which account did the month go on".
		if r.Provider.IsScript() {
			continue
		}
		b, ok := byProvider[r.Provider.ID]
		if !ok {
			b = &ProviderBucket{ID: r.Provider.ID, Name: r.Provider.Name}
			byProvider[r.Provider.ID] = b
			byModel[r.Provider.ID] = map[string]*ModelBucket{}
			order = append(order, r.Provider.ID)
		}
		// The newest run's name wins, so a renamed provider reads as it is
		// called now while its id keeps the history together.
		if r.Provider.Name != "" {
			b.Name = r.Provider.Name
		}
		add(&b.Bucket, r)
		model := r.Provider.ModelName()
		m, ok := byModel[r.Provider.ID][model]
		if !ok {
			m = &ModelBucket{Model: model}
			byModel[r.Provider.ID][model] = m
		}
		add(&m.Bucket, r)
	}
	if len(order) > 1 || (len(order) == 1 && len(byModel[order[0]]) > 1) {
		s.ByProvider = make([]ProviderBucket, 0, len(order))
		for _, id := range order {
			b := *byProvider[id]
			for _, m := range byModel[id] {
				b.Models = append(b.Models, *m)
			}
			// Map order is random; the name breaks ties so the rows hold still.
			sort.Slice(b.Models, func(i, j int) bool {
				if b.Models[i].Runs != b.Models[j].Runs {
					return b.Models[i].Runs > b.Models[j].Runs
				}
				return b.Models[i].Model < b.Models[j].Model
			})
			s.ByProvider = append(s.ByProvider, b)
		}
		// Busiest first: the account the work actually went to is the one worth
		// reading, not whichever happens to sort first.
		sort.SliceStable(s.ByProvider, func(i, j int) bool {
			return s.ByProvider[i].Runs > s.ByProvider[j].Runs
		})
	}
	return s
}

// calendarDaysBetween counts the midnights from a to b, both local midnights.
// Rounding absorbs the hour a DST change adds or takes away.
func calendarDaysBetween(a, b time.Time) int {
	return int((b.Sub(a) + 12*time.Hour) / (24 * time.Hour))
}

func add(b *Bucket, r store.Run) {
	b.Runs++
	switch r.Status {
	case store.StatusSuccess:
		b.Success++
	case store.StatusFailed, store.StatusAuthError:
		b.Failed++
	}
	b.CostUSD += r.CostUSD
	b.InputTokens += r.InputTokens
	b.OutputTokens += r.OutputTokens
	b.DurationMS += r.DurationMS
}
