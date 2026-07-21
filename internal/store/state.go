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
	// LastStarted maps a task id to the start time of its most recent run,
	// used to compute the next cron occurrence (FA-18).
	LastStarted map[string]time.Time `json:"last_started"`
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
	if s.LastStarted == nil {
		s.LastStarted = map[string]time.Time{}
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

// ForgetArtifact drops an artifact's read-status (used when it is deleted).
func (s *State) ForgetArtifact(id string) { delete(s.ReadArtifacts, id) }

// RecordStart records that a task started at t.
func (s *State) RecordStart(taskID string, t time.Time) {
	s.LastStarted[taskID] = t
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

// DismissUpdate records that the user dismissed the update prompt for a version.
func (s *State) DismissUpdate(version string) { s.DismissedUpdateVersion = version }

// DismissedUpdate returns the last dismissed update version, or "" if none.
func (s *State) DismissedUpdate() string { return s.DismissedUpdateVersion }
