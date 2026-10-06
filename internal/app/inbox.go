package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/danielmaier42/claudeq/internal/store"
)

// ErrNotificationNotFound is returned when the inbox holds no entry with the
// given id: it was pruned, or the notification predates the inbox.
var ErrNotificationNotFound = errors.New("notification not found")

// MarkNotificationRead marks one inbox entry read: the operator clicked the
// macOS notification or the row in the Notifications view. An unknown id is
// reported as ErrNotificationNotFound, so the caller can tell a stale click
// from a recorded one.
func MarkNotificationRead(s *store.Store, id string) error {
	found := false
	err := s.UpdateInbox(func(list *[]store.InboxEntry) error {
		for i := range *list {
			if (*list)[i].ID == id {
				(*list)[i].Read = true
				found = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("notification %q: %w", id, ErrNotificationNotFound)
	}
	return nil
}

// MarkAllNotificationsRead marks every inbox entry read.
func MarkAllNotificationsRead(s *store.Store) error {
	return markInboxReadWhere(s, func(store.InboxEntry) bool { return true })
}

// carryRead reports a failure to carry a read flag over to the inbox. The
// carry-over is best-effort: the run or artifact is already marked read, and a
// request that did what it was asked must not come back as a failure because
// a second file could not be written.
func carryRead(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudeq: notifications not marked read:", err)
	}
}

// markInboxReadWhere marks every unread inbox entry that match accepts as
// read. A notification is a pointer to something — an artifact, a run — and
// once the operator has looked at that thing, the pointer has nothing left to
// show; the read flags of artifacts and runs therefore carry over to the
// notifications about them. The inbox is read first so the common case,
// nothing to change, costs no write.
func markInboxReadWhere(s *store.Store, match func(store.InboxEntry) bool) error {
	entries, err := s.Inbox()
	if err != nil {
		return err
	}
	if !anyUnread(entries, match) {
		return nil
	}
	return s.UpdateInbox(func(list *[]store.InboxEntry) error {
		for i := range *list {
			if !(*list)[i].Read && match((*list)[i]) {
				(*list)[i].Read = true
			}
		}
		return nil
	})
}

func anyUnread(entries []store.InboxEntry, match func(store.InboxEntry) bool) bool {
	for _, e := range entries {
		if !e.Read && match(e) {
			return true
		}
	}
	return false
}

// readArtifactNotifications marks the notifications about the given artifacts
// read; an empty id set means every artifact.
func readArtifactNotifications(s *store.Store, ids map[string]bool) error {
	return markInboxReadWhere(s, func(e store.InboxEntry) bool {
		return e.ArtifactID != "" && (len(ids) == 0 || ids[e.ArtifactID])
	})
}

// readRunNotifications marks the notifications about the given runs read; an
// empty id set means every run.
func readRunNotifications(s *store.Store, ids map[string]bool) error {
	return markInboxReadWhere(s, func(e store.InboxEntry) bool {
		return e.RunID != "" && (len(ids) == 0 || ids[e.RunID])
	})
}
