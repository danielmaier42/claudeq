package api

import (
	"sort"
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
	// from before claudeq wrote one down.
	ID string `json:"id"`
	// Name is what that provider was called when the runs happened. A provider
	// renamed since keeps the name its history was written under, because that
	// is what the run says.
	Name string `json:"name"`
	Bucket
}

// Stats is the consumption summary shown in the dashboard.
type Stats struct {
	Totals Bucket      `json:"totals"`
	Last7d Bucket      `json:"last_7d"`
	PerDay []DayBucket `json:"per_day"` // last 14 calendar days, oldest first
	// ByProvider splits the totals per provider instance, busiest first. It is
	// omitted while every run came from the same one: a breakdown of one row is
	// not a breakdown.
	ByProvider []ProviderBucket `json:"by_provider,omitempty"`
}

const perDayDays = 14

// computeStats aggregates the run history. now anchors the relative windows.
// Only terminal runs count toward success/failed; metrics sum wherever present.
func computeStats(runs []store.Run, now time.Time) Stats {
	var s Stats
	weekAgo := now.Add(-7 * 24 * time.Hour)

	// Prepare the per-day skeleton for the last perDayDays days.
	dayIdx := map[string]int{}
	s.PerDay = make([]DayBucket, perDayDays)
	for i := range perDayDays {
		day := now.AddDate(0, 0, -(perDayDays - 1 - i))
		key := day.Format("2006-01-02")
		s.PerDay[i] = DayBucket{Date: key}
		dayIdx[key] = i
	}

	byProvider := map[string]*ProviderBucket{}
	var order []string
	for _, r := range runs {
		add(&s.Totals, r)
		// A script run went to no account and spent no allowance, so it has no
		// place in a split that answers "which account did the month go on".
		if !r.Provider.IsScript() {
			b, ok := byProvider[r.Provider.ID]
			if !ok {
				b = &ProviderBucket{ID: r.Provider.ID, Name: r.Provider.Name}
				byProvider[r.Provider.ID] = b
				order = append(order, r.Provider.ID)
			}
			// The newest run's name wins, so a renamed provider reads as it is
			// called now while its id keeps the history together.
			if r.Provider.Name != "" {
				b.Name = r.Provider.Name
			}
			add(&b.Bucket, r)
		}
		if !r.StartedAt.Before(weekAgo) {
			add(&s.Last7d, r)
		}
		key := r.StartedAt.Local().Format("2006-01-02")
		if i, ok := dayIdx[key]; ok {
			s.PerDay[i].Runs++
			s.PerDay[i].CostUSD += r.CostUSD
			s.PerDay[i].Tokens += r.InputTokens + r.OutputTokens
		}
	}
	if len(order) > 1 {
		s.ByProvider = make([]ProviderBucket, 0, len(order))
		for _, id := range order {
			s.ByProvider = append(s.ByProvider, *byProvider[id])
		}
		// Busiest first: the account the work actually went to is the one worth
		// reading, not whichever happens to sort first.
		sort.SliceStable(s.ByProvider, func(i, j int) bool {
			return s.ByProvider[i].Runs > s.ByProvider[j].Runs
		})
	}
	return s
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
