package store

import "time"

// State holds machine-managed bookkeeping kept separate from the
// human-editable config: run read-status (FA-26) and per-task scheduling
// history. It is not hand-edited.
type State struct {
	// ReadRuns maps a run id to true once it has been read (FA-23/24). A run
	// absent from this map is unread (FA-22).
	ReadRuns map[string]bool `json:"read_runs"`
	// ReadArtifacts maps an artifact id to true once it has been read. An
	// artifact absent from this map is unread (FA-A2).
	ReadArtifacts map[string]bool `json:"read_artifacts"`
	// NotifiedArtifacts maps an artifact id to true once its "new artifact"
	// notification has been sent, so the daemon notifies exactly once per
	// published artifact even though it re-reads the list on every tick.
	NotifiedArtifacts map[string]bool `json:"notified_artifacts"`
	// ArtifactNotifyPrimed records that the daemon has taken stock of the
	// artifacts that already existed when it first started watching. Without it
	// an upgrade (or a fresh state file) would notify for the whole backlog.
	ArtifactNotifyPrimed bool `json:"artifact_notify_primed"`
	// LastStarted maps a task id to the start time of its most recent run,
	// used to compute the next cron occurrence (FA-18). A freshly added cron
	// task is anchored here without having run, so this is not a run history.
	LastStarted map[string]time.Time `json:"last_started"`
	// LastRunAt maps a task id to the start time of its most recent actual run.
	// Unlike LastStarted it is only written when a run is really launched, so
	// the dashboard can show when a recurring task last executed.
	LastRunAt map[string]time.Time `json:"last_run_at"`
	// CompletedOnce marks one-shot tasks (asap/fixed) that have already run so
	// they are not re-enqueued (PLAN.md §7).
	CompletedOnce map[string]bool `json:"completed_once"`
	// PendingResumes maps a task id to the Claude Code session id to resume
	// after a rate-limit wait (PLAN.md D4/V1).
	PendingResumes map[string]string `json:"pending_resumes"`
	// DismissedUpdateVersion is the release version the user dismissed in the
	// update prompt. While the latest release equals it, no "update available"
	// prompt is shown; a newer release supersedes it and prompts again.
	DismissedUpdateVersion string `json:"dismissed_update_version"`
}

func newState() *State {
	s := &State{}
	s.ensureMaps()
	return s
}

func (s *State) ensureMaps() {
	if s.ReadRuns == nil {
		s.ReadRuns = map[string]bool{}
	}
	if s.ReadArtifacts == nil {
		s.ReadArtifacts = map[string]bool{}
	}
	if s.NotifiedArtifacts == nil {
		s.NotifiedArtifacts = map[string]bool{}
	}
	if s.LastStarted == nil {
		s.LastStarted = map[string]time.Time{}
	}
	if s.LastRunAt == nil {
		s.LastRunAt = map[string]time.Time{}
	}
	if s.CompletedOnce == nil {
		s.CompletedOnce = map[string]bool{}
	}
	if s.PendingResumes == nil {
		s.PendingResumes = map[string]string{}
	}
}

// IsRead reports whether a run has been read.
func (s *State) IsRead(runID string) bool { return s.ReadRuns[runID] }

// MarkRead marks a single run as read.
func (s *State) MarkRead(runID string) { s.ReadRuns[runID] = true }

// MarkAllRead marks every given run id as read.
func (s *State) MarkAllRead(runIDs []string) {
	for _, id := range runIDs {
		s.ReadRuns[id] = true
	}
}

// IsArtifactRead reports whether an artifact has been read.
func (s *State) IsArtifactRead(id string) bool { return s.ReadArtifacts[id] }

// MarkArtifactRead marks a single artifact as read.
func (s *State) MarkArtifactRead(id string) { s.ReadArtifacts[id] = true }

// MarkAllArtifactsRead marks every given artifact id as read.
func (s *State) MarkAllArtifactsRead(ids []string) {
	for _, id := range ids {
		s.ReadArtifacts[id] = true
	}
}

// ForgetArtifact drops an artifact's read- and notified-status (used when it is
// deleted), so state.json does not accumulate entries for artifacts that are
// long gone.
func (s *State) ForgetArtifact(id string) {
	delete(s.ReadArtifacts, id)
	delete(s.NotifiedArtifacts, id)
}

// IsArtifactNotified reports whether the "new artifact" notification for an
// artifact has already been sent.
func (s *State) IsArtifactNotified(id string) bool { return s.NotifiedArtifacts[id] }

// MarkArtifactNotified records that an artifact has been notified about.
func (s *State) MarkArtifactNotified(id string) { s.NotifiedArtifacts[id] = true }

// PrimeArtifactNotify marks the given artifacts as already notified without
// sending anything and flips ArtifactNotifyPrimed. It is called once, the first
// time the daemon looks at the artifact list, so the artifacts published before
// this feature existed stay silent while every later one notifies.
func (s *State) PrimeArtifactNotify(ids []string) {
	for _, id := range ids {
		s.NotifiedArtifacts[id] = true
	}
	s.ArtifactNotifyPrimed = true
}

// IsArtifactNotifyPrimed reports whether the pre-existing artifacts have been
// taken stock of (see PrimeArtifactNotify).
func (s *State) IsArtifactNotifyPrimed() bool { return s.ArtifactNotifyPrimed }

// RecordStart records that a task started at t.
func (s *State) RecordStart(taskID string, t time.Time) {
	s.LastStarted[taskID] = t
}

// RecordRun records that a task actually began a run at t. Callers that only
// anchor a cron schedule use RecordStart alone.
func (s *State) RecordRun(taskID string, t time.Time) {
	if s.LastRunAt == nil {
		s.LastRunAt = map[string]time.Time{}
	}
	s.LastRunAt[taskID] = t
}

// LastRun returns the start time of a task's most recent actual run and
// whether one exists.
func (s *State) LastRun(taskID string) (time.Time, bool) {
	t, ok := s.LastRunAt[taskID]
	return t, ok
}

// LastStart returns the last start time for a task and whether one exists.
func (s *State) LastStart(taskID string) (time.Time, bool) {
	t, ok := s.LastStarted[taskID]
	return t, ok
}

// MarkCompletedOnce records that a one-shot task has run.
func (s *State) MarkCompletedOnce(taskID string) { s.CompletedOnce[taskID] = true }

// IsCompletedOnce reports whether a one-shot task has already run.
func (s *State) IsCompletedOnce(taskID string) bool { return s.CompletedOnce[taskID] }

// PendingResume returns the session id a task should resume, or "" if none.
func (s *State) PendingResume(taskID string) string { return s.PendingResumes[taskID] }

// SetPendingResume records that a task should resume the given session.
func (s *State) SetPendingResume(taskID, sessionID string) {
	s.PendingResumes[taskID] = sessionID
}

// ClearPendingResume clears any pending resume for a task.
func (s *State) ClearPendingResume(taskID string) { delete(s.PendingResumes, taskID) }

// ForgetTask drops everything recorded about a task id — last start,
// completed-once flag, pending resume — so a task created later under the
// same id starts from a clean slate instead of inheriting a dead task's
// scheduling history.
func (s *State) ForgetTask(taskID string) {
	delete(s.LastStarted, taskID)
	delete(s.LastRunAt, taskID)
	delete(s.CompletedOnce, taskID)
	delete(s.PendingResumes, taskID)
}

// DismissUpdate records that the user dismissed the update prompt for a version.
func (s *State) DismissUpdate(version string) { s.DismissedUpdateVersion = version }

// DismissedUpdate returns the last dismissed update version, or "" if none.
func (s *State) DismissedUpdate() string { return s.DismissedUpdateVersion }
