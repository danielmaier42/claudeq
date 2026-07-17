package api

import (
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

// Stats is the consumption summary shown in the dashboard.
type Stats struct {
	Totals Bucket      `json:"totals"`
	Last7d Bucket      `json:"last_7d"`
	PerDay []DayBucket `json:"per_day"` // last 14 calendar days, oldest first
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

	for _, r := range runs {
		add(&s.Totals, r)
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
