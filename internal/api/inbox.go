package api

import (
	"errors"
	"net/http"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
)

// notificationView is a sent notification plus its unread flag, the shape the
// run and artifact lists use too. The entry's own read flag is cleared before
// it is sent (its tag omits a false value), so the list says it one way.
type notificationView struct {
	store.InboxEntry
	Unread bool `json:"unread"`
}

func viewOf(e store.InboxEntry) notificationView {
	v := notificationView{InboxEntry: e, Unread: !e.Read}
	v.Read = false
	return v
}

// listNotifications answers with the inbox, newest first.
func (s *server) listNotifications(w http.ResponseWriter, _ *http.Request) {
	entries, err := s.d.Store.Inbox()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	views := make([]notificationView, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		views = append(views, viewOf(entries[i]))
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *server) readNotification(w http.ResponseWriter, r *http.Request) {
	if err := app.MarkNotificationRead(s.d.Store, r.PathValue("id")); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, app.ErrNotificationNotFound) {
			code = http.StatusNotFound
		}
		writeErr(w, code, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) readAllNotifications(w http.ResponseWriter, _ *http.Request) {
	if err := app.MarkAllNotificationsRead(s.d.Store); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
