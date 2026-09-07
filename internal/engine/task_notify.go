package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
)

// deliverTaskNotifications sends every notification a task queued with
// `claudeq notify` since the last pass. The CLI cannot deliver itself: the
// daemon owns the channels (and the Pushover credentials), and a macOS
// notification must come from the process the operator authorized.
//
// The outbox is emptied *before* anything is sent, in one atomic take: a broken
// channel then costs one message instead of re-firing on every tick.
func (e *Engine) deliverTaskNotifications() {
	if e.notifier == nil {
		return
	}
	pending, err := e.takeTaskNotifications()
	if err != nil {
		// Log a given failure once — this runs on every tick.
		if msg := err.Error(); msg != e.lastNotifyErr {
			fmt.Fprintln(os.Stderr, "claudeqd: task notification failed:", err)
			e.lastNotifyErr = msg
		}
		return
	}
	e.lastNotifyErr = ""
	for _, n := range pending {
		// Deliberately not the loop's context: a notification queued moments
		// before shutdown would otherwise be taken and never sent.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = e.notifier.Notify(ctx, notify.Notification{
			Title:   n.Title,
			Message: taskNotificationBody(n),
			URL:     n.URL,
		})
		cancel()
	}
}

// takeTaskNotifications empties the outbox and returns its contents. An empty
// outbox is the overwhelmingly common case and is answered by a plain read;
// the write lock is only taken when there is something to take.
func (e *Engine) takeTaskNotifications() ([]store.Notification, error) {
	pending, err := e.store.PendingNotifications()
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, nil
	}
	return e.store.TakeNotifications()
}

// taskNotificationBody renders the body: the task's message, attributed to the
// task that sent it so the operator can tell which job is talking.
func taskNotificationBody(n store.Notification) string {
	if n.TaskName == "" {
		return n.Message
	}
	return n.Message + "\n· from " + n.TaskName
}
