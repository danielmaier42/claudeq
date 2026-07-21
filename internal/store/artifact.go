package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Artifact is a file a task published for the operator to review, listed in the
// app's central Artifacts view (FA-A1). The file itself is a snapshot copied
// into the data directory under artifacts/<id>/, so it survives even if the task
// later changes or deletes the original.
type Artifact struct {
	// ID is a stable unique identifier (also the name of its subdirectory under
	// artifacts/).
	ID string `json:"id"`
	// Title is a short human-readable label; defaults to the file name.
	Title string `json:"title"`
	// Description is an optional one-line summary.
	Description string `json:"description,omitempty"`
	// FileName is the original base name of the published file.
	FileName string `json:"file_name"`
	// RelPath is the stored file's path relative to the artifacts directory
	// ("<id>/<file_name>"), so the store stays relocatable (CLAUDEQ_HOME can move).
	RelPath string `json:"rel_path"`
	// Size is the stored file size in bytes.
	Size int64 `json:"size"`
	// ContentType is the MIME type, resolved at publish time from the extension
	// (falling back to content sniffing). Drives the in-app viewer choice.
	ContentType string `json:"content_type"`
	// TaskID/TaskName/RunID attribute the artifact to the run that published it
	// (empty when published outside a run, e.g. manually).
	TaskID   string `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	// PublishedAt is when the artifact was published.
	PublishedAt time.Time `json:"published_at"`
}

const (
	artifactsFile = "artifacts.json"
	artifactsDir  = "artifacts"
)

// artifactsDoc is the on-disk container for the artifact list.
type artifactsDoc struct {
	Artifacts []Artifact `json:"artifacts"`
}

// ArtifactsDir returns the directory holding all artifact files.
func (s *Store) ArtifactsDir() string { return filepath.Join(s.home, artifactsDir) }

// ArtifactDir returns the directory holding one artifact's file(s).
func (s *Store) ArtifactDir(id string) string { return filepath.Join(s.home, artifactsDir, id) }

// ArtifactContentPath returns the absolute path to an artifact's stored file.
func (s *Store) ArtifactContentPath(a Artifact) string {
	return filepath.Join(s.home, artifactsDir, filepath.FromSlash(a.RelPath))
}

// Artifacts returns the published artifacts in first-seen (oldest-first) order.
func (s *Store) Artifacts() ([]Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadArtifactsLocked()
}

// loadArtifactsLocked reads artifacts.json. A missing file yields an empty list.
// The caller must hold s.mu.
func (s *Store) loadArtifactsLocked() ([]Artifact, error) {
	data, err := os.ReadFile(s.path(artifactsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read artifacts: %w", err)
	}
	var doc artifactsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse artifacts: %w", err)
	}
	return doc.Artifacts, nil
}

// saveArtifactsLocked atomically writes artifacts.json. The caller must hold s.mu.
func (s *Store) saveArtifactsLocked(list []Artifact) error {
	if list == nil {
		list = []Artifact{}
	}
	data, err := json.MarshalIndent(artifactsDoc{Artifacts: list}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifacts: %w", err)
	}
	return writeAtomic(s.path(artifactsFile), data)
}

// UpdateArtifacts atomically applies fn to the artifact list, serialized with
// other updates (and cross-process via the write lock) so the daemon and a
// publishing CLI process never clobber each other's changes.
func (s *Store) UpdateArtifacts(fn func(*[]Artifact) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.withWriteLock(func() error {
		s.mu.Lock()
		list, err := s.loadArtifactsLocked()
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if err := fn(&list); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.saveArtifactsLocked(list)
	})
}
