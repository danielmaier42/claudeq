package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/danielmaier42/claudeq/internal/notify"
	"github.com/danielmaier42/claudeq/internal/store"
)

// artifactNotifyTitle is the title of the "a task published something for you"
// notification. The artifact's own title goes into the body, so the operator can
// tell at a glance what kind of notification arrived.
const artifactNotifyTitle = "ClaudeQ 📄 New artifact"

// artifactNotifyBodyLimit caps the notification body; a long description is cut
// rather than pushed at the operator in full.
const artifactNotifyBodyLimit = 300

// notifyNewArtifacts sends one notification per artifact published since the
// last pass (FA-A4). The macOS notification carries the artifact id, so clicking
// it opens that artifact in the ClaudeQ window (cmd/claudeqapp).
//
// Artifacts are recorded as notified *before* the notification is sent, and in a
// single state write: a broken notification channel then costs one message
// instead of re-firing on every tick.
func (e *Engine) notifyNewArtifacts() {
	if e.notifier == nil {
		return
	}
	pending, err := e.takeUnnotifiedArtifacts()
	if err != nil {
		// Log a given failure once — this runs on every tick.
		if msg := err.Error(); msg != e.lastArtifactErr {
			fmt.Fprintln(os.Stderr, "claudeqd: artifact notification failed:", err)
			e.lastArtifactErr = msg
		}
		return
	}
	e.lastArtifactErr = ""
	for _, a := range pending {
		// Deliberately not the loop's context: a publish moments before shutdown
		// would otherwise be marked notified and never announced.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_ = e.notifier.Notify(ctx, notify.Notification{
			Title:      artifactNotifyTitle,
			Message:    artifactNotifyBody(a),
			ArtifactID: a.ID,
		})
		cancel()
	}
}

// takeUnnotifiedArtifacts marks every not-yet-notified artifact as notified and
// returns those artifacts (oldest first).
//
// On the very first pass it instead takes stock of the artifacts that already
// exist and notifies for none of them: a state file written before this feature
// existed — or one wiped while artifacts remained — must not produce a burst of
// notifications for old news. Priming happens even when there is no artifact
// yet, so the first artifact published afterwards does notify.
//
// Nothing new is the overwhelmingly common case, and it is answered by a plain
// read: every write goes through an fsync and a cross-process lock, which has no
// business happening on every tick of an idle daemon.
func (e *Engine) takeUnnotifiedArtifacts() ([]store.Artifact, error) {
	arts, err := e.store.Artifacts()
	if err != nil {
		return nil, err
	}
	st, err := e.store.LoadState()
	if err != nil {
		return nil, err
	}
	if st.IsArtifactNotifyPrimed() && !hasUnnotified(st, arts) {
		return nil, nil
	}

	// Something to record: redo the decision inside the write lock, where the
	// state is authoritative even if a publish or a delete landed in between.
	var pending []store.Artifact
	if err := e.store.UpdateState(func(st *store.State) error {
		pending = nil
		if !st.IsArtifactNotifyPrimed() {
			ids := make([]string, len(arts))
			for i, a := range arts {
				ids[i] = a.ID
			}
			st.PrimeArtifactNotify(ids)
			return nil
		}
		for _, a := range arts {
			if st.IsArtifactNotified(a.ID) {
				continue
			}
			st.MarkArtifactNotified(a.ID)
			pending = append(pending, a)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return pending, nil
}

// hasUnnotified reports whether any artifact still needs a notification.
func hasUnnotified(st *store.State, arts []store.Artifact) bool {
	for _, a := range arts {
		if !st.IsArtifactNotified(a.ID) {
			return true
		}
	}
	return false
}

// artifactNotifyBody renders the notification body: what was published, by which
// task, and its description if it has one.
func artifactNotifyBody(a store.Artifact) string {
	body := a.Title
	if body == "" {
		body = a.FileName
	}
	if a.TaskName != "" {
		body += " · from " + a.TaskName
	}
	if a.Description != "" {
		body += "\n" + a.Description
	}
	if r := []rune(body); len(r) > artifactNotifyBodyLimit {
		body = string(r[:artifactNotifyBodyLimit]) + "…"
	}
	return body
}
