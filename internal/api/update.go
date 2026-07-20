package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/update"
	"github.com/danielmaier42/claudeq/internal/version"
)

// updateStatus is the update state the dashboard renders (the yellow banner and
// the Settings badge). It combines the running build's version, the cached
// GitHub check, and the user's dismissed version.
type updateStatus struct {
	// Current is the running build's normalized version, e.g. "0.1.4".
	Current string `json:"current"`
	// Supported is false for a development build (version "dev"), where updates
	// cannot be compared and nothing is offered.
	Supported bool `json:"supported"`
	// Available is true when a newer, non-dismissed release with an installer is
	// ready to download.
	Available bool `json:"available"`
	// Latest is the latest release's version, "" if unknown / not yet checked.
	Latest string `json:"latest"`
	// Tag is the latest release's git tag, e.g. "v0.1.5".
	Tag string `json:"tag,omitempty"`
	// Notes is the combined notes of every release newer than the current build
	// (each prefixed with its version), so a user who skipped versions sees all
	// the changes in between — not just the newest release.
	Notes string `json:"notes,omitempty"`
	// SkippedCount is how many releases are newer than the current build (i.e.
	// how many versions' notes Notes aggregates).
	SkippedCount int `json:"skipped_count,omitempty"`
	// HTMLURL is the latest release's page on GitHub.
	HTMLURL string `json:"html_url,omitempty"`
	// AllReleasesURL is the GitHub page listing every release, so the user can
	// always browse the full history.
	AllReleasesURL string `json:"all_releases_url,omitempty"`
	// HasInstaller reports whether the latest release ships a `.pkg` asset.
	HasInstaller bool `json:"has_installer"`
	// PublishedAt is when the latest release was published.
	PublishedAt *time.Time `json:"published_at,omitempty"`
	// Dismissed is the version the user dismissed, "" if none.
	Dismissed string `json:"dismissed,omitempty"`
	// Checking / Downloading reflect in-flight background work.
	Checking    bool `json:"checking"`
	Downloading bool `json:"downloading"`
	// LastChecked is when the cache was last refreshed.
	LastChecked *time.Time `json:"last_checked,omitempty"`
	// Error is the last check error, "" if healthy.
	Error string `json:"error,omitempty"`
}

// buildUpdateStatus assembles the response from the current version, the
// dismissed version, and the cached check snapshot.
func buildUpdateStatus(current, dismissed string, snap update.Snapshot) updateStatus {
	cur := update.Normalize(current)
	st := updateStatus{
		Current:        cur,
		Supported:      update.IsReleaseVersion(current),
		Dismissed:      dismissed,
		Checking:       snap.Checking,
		Downloading:    snap.Downloading,
		Error:          snap.Err,
		AllReleasesURL: update.ReleasesPageURL(""),
	}
	if !snap.CheckedAt.IsZero() {
		t := snap.CheckedAt
		st.LastChecked = &t
	}
	rel := snap.Latest()
	if rel == nil {
		return st
	}
	st.Latest = rel.Version
	st.Tag = rel.Tag
	st.HTMLURL = rel.HTMLURL
	st.HasInstaller = rel.PkgURL != ""
	if !rel.PublishedAt.IsZero() {
		p := rel.PublishedAt
		st.PublishedAt = &p
	}

	// Show the notes of every release the user skipped, newest first. For a
	// dev/unknown current version NewerThan returns nothing, so fall back to the
	// latest release's own notes for display.
	skipped := update.NewerThan(snap.Releases, cur)
	st.SkippedCount = len(skipped)
	st.Notes = aggregateNotes(skipped, rel)

	// An update is offered only when we can compare versions, the release is
	// strictly newer, it ships an installer, and the user hasn't dismissed
	// exactly this version.
	if st.Supported && rel.PkgURL != "" &&
		update.IsNewer(rel.Version, cur) &&
		rel.Version != update.Normalize(dismissed) {
		st.Available = true
	}
	return st
}

// aggregateNotes joins the notes of the skipped releases (newest first), each
// under a "ClaudeQ <version>" heading. With none skipped (e.g. a dev build) it
// falls back to the latest release's plain notes.
func aggregateNotes(skipped []update.Release, latest *update.Release) string {
	if len(skipped) == 0 {
		return latest.Notes
	}
	var b strings.Builder
	for i, r := range skipped {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("ClaudeQ ")
		b.WriteString(r.Version)
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(r.Notes))
	}
	return b.String()
}

// currentStatus reads the dismissed version from the store and combines it with
// the update service snapshot.
func (s *server) currentStatus() updateStatus {
	snap := s.d.Updates.Snapshot()
	dismissed := ""
	if st, err := s.d.Store.LoadState(); err == nil {
		dismissed = st.DismissedUpdate()
	}
	return buildUpdateStatus(version.String(), dismissed, snap)
}

func (s *server) getUpdate(w http.ResponseWriter, _ *http.Request) {
	if s.d.Updates == nil {
		writeJSON(w, http.StatusOK, updateStatus{Current: update.Normalize(version.String())})
		return
	}
	writeJSON(w, http.StatusOK, s.currentStatus())
}

// checkUpdate forces an immediate GitHub check, then returns the fresh status.
func (s *server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	if s.d.Updates == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("update checks are not available"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	// Ignore the error here: a failed check records itself in the snapshot, so
	// currentStatus() surfaces it (Error field) rather than returning a bare
	// HTTP error the UI can't explain.
	_, _ = s.d.Updates.CheckNow(ctx)
	writeJSON(w, http.StatusOK, s.currentStatus())
}

// dismissUpdate records the dismissed version so this release stops prompting;
// a newer release later supersedes it.
func (s *server) dismissUpdate(w http.ResponseWriter, r *http.Request) {
	if s.d.Updates == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("update checks are not available"))
		return
	}
	var in struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if in.Version == "" {
		writeErr(w, http.StatusBadRequest, errors.New("version is required"))
		return
	}
	err := s.d.Store.UpdateState(func(st *store.State) error {
		st.DismissUpdate(update.Normalize(in.Version))
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.currentStatus())
}

// downloadUpdate downloads the latest installer and opens it. It blocks until
// the download finishes (a package is ~16 MB), so the client shows a
// "Downloading…" state meanwhile.
func (s *server) downloadUpdate(w http.ResponseWriter, r *http.Request) {
	if s.d.Updates == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("update downloads are not available"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	path, err := s.d.Updates.Download(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}
