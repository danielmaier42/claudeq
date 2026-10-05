package engine

import (
	"fmt"
	"io"
	"maps"
	"sync"
	"time"

	"github.com/danielmaier42/claudeq/internal/schedule"
)

// HoldLogAfter is how long a due task may sit without starting before the
// daemon writes down why. Short waits are the queue working normally; a long
// one is what someone asks about afterwards, when the queue no longer shows it.
const HoldLogAfter = 5 * time.Minute

// holds remembers, per task, why the last tick did not start it although it
// was due. It has its own lock because the dashboard reads it while a tick is
// under way.
type holds struct {
	mu     sync.Mutex
	byTask map[string]schedule.Hold
	// logged is the reason last written to the log per task, so a long wait is
	// logged once, and again only when its reason changes.
	logged map[string]string
}

// update replaces the holds with one tick's verdicts. A task that keeps
// waiting keeps its start time even when the reason changes, because to the
// person looking at the queue it is still the same wait. Tasks missing from
// reasons started, or are not due any more, and are forgotten.
func (h *holds) update(reasons map[string]string, now time.Time, log io.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := make(map[string]schedule.Hold, len(reasons))
	logged := make(map[string]string, len(reasons))
	for id, reason := range reasons {
		since := now
		if prev, ok := h.byTask[id]; ok {
			since = prev.Since
		}
		next[id] = schedule.Hold{Reason: reason, Since: since}
		if last, ok := h.logged[id]; ok {
			logged[id] = last
		}
		if waited := now.Sub(since); waited >= HoldLogAfter && logged[id] != reason {
			fmt.Fprintf(log, "claudeqd: task %q has been due for %s and does not start: %s\n",
				id, waited.Round(time.Minute), reason)
			logged[id] = reason
		}
	}
	h.byTask, h.logged = next, logged
}

// snapshot returns a copy of the current holds.
func (h *holds) snapshot() map[string]schedule.Hold {
	h.mu.Lock()
	defer h.mu.Unlock()
	return maps.Clone(h.byTask)
}
