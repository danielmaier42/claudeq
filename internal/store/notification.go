package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

// notificationsDoc is the on-disk container for the outbox.
type notificationsDoc struct {
	Pending []Notification `json:"pending"`
}

// PendingNotifications returns the queued, not yet delivered notifications in
// the order they were queued. A missing outbox yields an empty list.
func (s *Store) PendingNotifications() ([]Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadNotificationsLocked()
}

// loadNotificationsLocked reads notifications.json. The caller must hold s.mu.
func (s *Store) loadNotificationsLocked() ([]Notification, error) {
	data, err := os.ReadFile(s.path(notificationsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	var doc notificationsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse notifications: %w", err)
	}
	return doc.Pending, nil
}

// saveNotificationsLocked atomically writes notifications.json. The caller must
// hold s.mu.
func (s *Store) saveNotificationsLocked(list []Notification) error {
	if list == nil {
		list = []Notification{}
	}
	data, err := json.MarshalIndent(notificationsDoc{Pending: list}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode notifications: %w", err)
	}
	return writeAtomic(s.path(notificationsFile), data)
}

// updateNotifications atomically applies fn to the outbox, serialized with
// other updates (and cross-process via the write lock) so a queueing CLI
// process and the delivering daemon never clobber each other's changes.
func (s *Store) updateNotifications(fn func(*[]Notification) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.withWriteLock(func() error {
		s.mu.Lock()
		list, err := s.loadNotificationsLocked()
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if err := fn(&list); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.saveNotificationsLocked(list)
	})
}

// QueueNotification appends n to the outbox for the daemon to deliver.
func (s *Store) QueueNotification(n Notification) error {
	if n.ID == "" {
		return fmt.Errorf("queue notification: missing id")
	}
	return s.updateNotifications(func(list *[]Notification) error {
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
// one daemon pass; one queued during the take lands in the next pass.
func (s *Store) TakeNotifications() ([]Notification, error) {
	var taken []Notification
	err := s.updateNotifications(func(list *[]Notification) error {
		taken = *list
		*list = nil
		return nil
	})
	if err != nil {
		return nil, err
	}
	return taken, nil
}
