package api

import (
	"net/http"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

// cronCheck is the answer to "is this schedule usable, and when would it run?".
// It is what the task sheet shows under the cron field while it is being typed,
// so the schedule is confirmed (or rejected) before the task is saved.
type cronCheck struct {
	// Valid is true when the expression is a schedule claudeq can run.
	Valid bool `json:"valid"`
	// Error explains the problem in the words of whoever typed it (empty when valid).
	Error string `json:"error,omitempty"`
	// Next are the upcoming occurrences (RFC3339, local zone), for a preview.
	Next []time.Time `json:"next,omitempty"`
}

// cronPreviewRuns is how many upcoming occurrences the preview shows.
const cronPreviewRuns = 3

// checkCron validates a cron expression without saving anything. The dashboard
// calls it as the user types, so the answer is always HTTP 200 with valid=false
// for a bad expression — a rejected keystroke is not a failed request.
func (s *server) checkCron(w http.ResponseWriter, r *http.Request) {
	expr := r.URL.Query().Get("expr")
	next, err := task.CronNext(expr, time.Now(), cronPreviewRuns)
	if err != nil {
		writeJSON(w, http.StatusOK, cronCheck{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cronCheck{Valid: true, Next: next})
}
