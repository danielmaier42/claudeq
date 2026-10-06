package store

import (
	"fmt"
	"time"
)

// InboxEntry is one notification ClaudeQ sent, kept so the operator can find
// it again in the app's Notifications view once the macOS banner is gone: what
// it said, what it points at, and whether it has been looked at. The daemon
// writes an entry for every notification before handing it to the channels, so
// the inbox also shows what macOS or Pushover swallowed.
type InboxEntry struct {
	// ID is unique per sent notification. The macOS notification carries it,
	// so a click can mark exactly this entry read.
	ID string `json:"id"`
	// Kind says what the notification announced (see the InboxKind constants);
	// the view picks its glyph and offers it as a filter.
	Kind string `json:"kind"`
	// Title and Message are what the channels showed.
	Title   string `json:"title"`
	Message string `json:"message"`
	// ArtifactID, RunID and URL are the click target, at most one of them set:
	// the artifact to open in the viewer, the run whose log to open, or the
	// link to open in the browser. None set means the notification itself is
	// the destination (the Notifications view).
	ArtifactID string `json:"artifact_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	URL        string `json:"url,omitempty"`
	// TaskID/TaskName attribute the notification to the task it is about or
	// that sent it (empty for a provider notification).
	TaskID   string `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`
	// SentAt is when the daemon sent it.
	SentAt time.Time `json:"sent_at"`
	// Read is set once the operator clicked the notification or marked it read
	// in the app.
	Read bool `json:"read,omitempty"`
}

// The kinds of notification the inbox records.
const (
	// InboxKindSuccess is a run that finished well (sent when the task opted in).
	InboxKindSuccess = "success"
	// InboxKindFailure is a run that failed or hit an authentication problem.
	InboxKindFailure = "failure"
	// InboxKindArtifact is a file a task published.
	InboxKindArtifact = "artifact"
	// InboxKindProvider is a provider that stopped or resumed running tasks.
	InboxKindProvider = "provider"
	// InboxKindTask is a message a task sent itself with `claudeq notify`.
	InboxKindTask = "task"
)

// InboxLimit is how many notifications the inbox keeps; the oldest make room
// for new ones. Two hundred is weeks of a busy queue, and a notification older
// than that has nothing left to say.
const InboxLimit = 200

const inboxFile = "inbox.json"

// inboxList is the on-disk inbox ({"notifications": [...]}), oldest first.
var inboxList = jsonList[InboxEntry]{file: inboxFile, field: "notifications"}

// Inbox returns the sent notifications, oldest first. A missing file yields an
// empty list.
func (s *Store) Inbox() ([]InboxEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return inboxList.load(s)
}

// AddInboxEntry appends e to the inbox, dropping the oldest entries beyond
// InboxLimit.
func (s *Store) AddInboxEntry(e InboxEntry) error {
	if e.ID == "" {
		return fmt.Errorf("add inbox entry: missing id")
	}
	return inboxList.update(s, func(list *[]InboxEntry) error {
		for _, q := range *list {
			if q.ID == e.ID {
				return fmt.Errorf("notification %q already in the inbox", e.ID)
			}
		}
		*list = append(*list, e)
		if extra := len(*list) - InboxLimit; extra > 0 {
			*list = (*list)[extra:]
		}
		return nil
	})
}

// UpdateInbox atomically applies fn to the inbox, serialized with other
// updates (and cross-process via the write lock).
func (s *Store) UpdateInbox(fn func(*[]InboxEntry) error) error {
	return inboxList.update(s, fn)
}
