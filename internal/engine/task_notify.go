package engine

import (
	"fmt"
	"os"
	"time"

	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
)

// Channel limits for a task-authored notification (Pushover's, the stricter
// channel): a longer title or body is cut rather than rejected.
const (
	taskNotifyTitleLimit = 250
	taskNotifyBodyLimit  = 1024
)

// taskNotifyMaxAge is how long a queued notification stays deliverable. The
// outbox survives the daemon being down; an alert about a condition from days
// ago would arrive looking current, so older entries are dropped with a log
// line instead.
const taskNotifyMaxAge = 24 * time.Hour

// deliverTaskNotifications sends every notification a task queued with
// `claudeq notify` since the last pass. The CLI cannot deliver itself: the
// daemon owns the channels (and the Pushover credentials), and a macOS
// notification must come from the process the operator authorized.
//
// The outbox is emptied *before* anything is sent, in one atomic take: a broken
// channel then costs the batch instead of re-firing on every tick. Sending
// happens off the scheduler goroutine so a slow channel never delays a tick.
func (e *Engine) deliverTaskNotifications() {
	if e.notifier == nil {
		return
	}
	pending, err := e.store.TakeNotifications()
	noteErr(&e.lastNotifyErr, "task notification failed", err)
	if len(pending) == 0 {
		return
	}
	now := e.clock.Now()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for _, n := range pending {
			if age := now.Sub(n.QueuedAt); age > taskNotifyMaxAge {
				fmt.Fprintf(os.Stderr, "claudeqd: dropped stale notification %q (queued %s ago)\n",
					n.Title, age.Round(time.Minute))
				continue
			}
			e.send(taskNotification(n))
		}
	}()
}

// taskNotification renders an outbox entry for the channels: title and body
// cut to what they accept, the body attributed to the task that sent it so the
// operator can tell which job is talking, and the link re-checked here at the
// point of use (the CLI validates it too, but the outbox is a plain file).
func taskNotification(n store.Notification) notify.Notification {
	body := n.Message
	if n.TaskName != "" {
		body += "\n· from " + n.TaskName
	}
	link := n.URL
	if link != "" && !notify.IsWebURL(link) {
		fmt.Fprintf(os.Stderr, "claudeqd: notification %q: ignoring link %q (not an http(s) URL)\n", n.Title, link)
		link = ""
	}
	return notify.Notification{
		Title:   truncateRunes(n.Title, taskNotifyTitleLimit),
		Message: truncateRunes(body, taskNotifyBodyLimit),
		URL:     link,
	}
}
