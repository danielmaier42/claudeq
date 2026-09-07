package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Notification is a message a task asked ClaudeQ to send on its behalf
// (`claudeq notify`). The CLI drops it into the outbox file; the daemon, which
// owns the notification channels, delivers it on its next tick and removes it.
type Notification struct {
	// ID is unique per queued notification.
	ID string `json:"id"`
	// Title and Message are the notification as the task wrote it.
	Title   string `json:"title"`
	Message string `json:"message"`
	// URL is an optional link the notification points at (opened on click).
	URL string `json:"url,omitempty"`
	// TaskID/TaskName/RunID attribute the notification to the run that sent it
	// (empty when sent outside a run, e.g. manually).
	TaskID   string `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	// QueuedAt is when the task asked for the notification.
	QueuedAt time.Time `json:"queued_at"`
}

const notificationsFile = "notifications.json"

// notificationList is the on-disk outbox ({"pending": [...]}). The file exists
// only while something is waiting, so the daemon's per-tick check is a plain
// ENOENT almost always.
var notificationList = jsonList[Notification]{file: notificationsFile, field: "pending", dropWhenEmpty: true}

// PendingNotifications returns the queued, not yet delivered notifications in
// the order they were queued. A missing outbox yields an empty list.
func (s *Store) PendingNotifications() ([]Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return notificationList.load(s)
}

// QueueNotification appends n to the outbox for the daemon to deliver.
func (s *Store) QueueNotification(n Notification) error {
	if n.ID == "" {
		return fmt.Errorf("queue notification: missing id")
	}
	return notificationList.update(s, func(list *[]Notification) error {
		for _, q := range *list {
			if q.ID == n.ID {
				return fmt.Errorf("notification %q already queued", n.ID)
			}
		}
		*list = append(*list, n)
		return nil
	})
}

// TakeNotifications empties the outbox and returns what it held, oldest first.
// Taking is atomic across processes, so a notification is delivered by exactly
// one daemon pass; one queued during the take lands in the next pass. An empty
// outbox — the overwhelmingly common case — is answered by a plain read
// without touching the write lock.
func (s *Store) TakeNotifications() ([]Notification, error) {
	pending, err := s.PendingNotifications()
	if err != nil {
		// A damaged outbox must not wedge every notification from now on: set
		// the file aside (kept for inspection) so the next queue starts afresh.
		aside := s.path(notificationsFile + ".corrupt")
		if renameErr := os.Rename(s.path(notificationsFile), aside); renameErr != nil {
			return nil, fmt.Errorf("%w (could not set it aside: %w)", err, renameErr)
		}
		return nil, fmt.Errorf("%w (set aside as %s)", err, filepath.Base(aside))
	}
	if len(pending) == 0 {
		return nil, nil
	}
	var taken []Notification
	err = notificationList.update(s, func(list *[]Notification) error {
		taken = *list
		*list = nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return taken, nil
}
