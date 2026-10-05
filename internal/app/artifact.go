package app

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// PublishInput describes a file to publish as an artifact. ID is a caller-
// generated unique id (its directory name under artifacts/); the caller retries
// with a fresh id on the rare collision.
type PublishInput struct {
	ID          string
	SourcePath  string
	Title       string
	Description string
	TaskID      string
	TaskName    string
	RunID       string
	Group       string
	OriginID    string
	OriginName  string
	Now         time.Time
}

// PublishArtifact copies the input file into the store as a permanent snapshot
// (artifacts/<id>/<name>) and records it in the artifact list, returning the
// stored artifact. The original file is left untouched. On any failure the
// partially-created directory is cleaned up so no orphan files linger.
func PublishArtifact(s *store.Store, in PublishInput) (store.Artifact, error) {
	if in.ID == "" {
		return store.Artifact{}, fmt.Errorf("missing artifact id")
	}
	src := strings.TrimSpace(in.SourcePath)
	if src == "" {
		return store.Artifact{}, fmt.Errorf("missing file to publish")
	}
	info, err := os.Stat(src)
	if err != nil {
		return store.Artifact{}, fmt.Errorf("stat %q: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return store.Artifact{}, fmt.Errorf("%q is not a regular file", src)
	}
	name := filepath.Base(src)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return store.Artifact{}, fmt.Errorf("cannot determine a file name for %q", src)
	}

	destDir := s.ArtifactDir(in.ID)
	if _, err := os.Stat(destDir); err == nil {
		return store.Artifact{}, fmt.Errorf("artifact id %q already exists", in.ID)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return store.Artifact{}, fmt.Errorf("create artifact dir: %w", err)
	}

	destPath := filepath.Join(destDir, name)
	size, err := copyFile(src, destPath)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return store.Artifact{}, err
	}

	art := store.Artifact{
		ID:          in.ID,
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		FileName:    name,
		RelPath:     filepath.ToSlash(filepath.Join(in.ID, name)),
		Size:        size,
		ContentType: detectContentType(name, destPath),
		TaskID:      in.TaskID,
		TaskName:    in.TaskName,
		RunID:       in.RunID,
		Group:       in.Group,
		OriginID:    in.OriginID,
		OriginName:  in.OriginName,
		PublishedAt: in.Now,
	}
	if art.Title == "" {
		art.Title = name
	}

	if err := s.UpdateArtifacts(func(list *[]store.Artifact) error {
		*list = append(*list, art)
		return nil
	}); err != nil {
		_ = os.RemoveAll(destDir)
		return store.Artifact{}, err
	}
	return art, nil
}

// ResolveArtifactSources fills in the group and origin the Artifacts view
// filters by, in place. An artifact keeps what it recorded when it was
// published; one published before claudeq recorded either is traced back
// through run history: its run's task gives the group, and the origin its run
// recorded, else the first run of its workflow, else the publishing job itself.
// A guessed origin (the last two) only stands when it is a job still in the
// queue, and then as that job's own origin:
// history rarely reaches back to the watcher behind a one-off review job, and
// taking that job as its own origin would offer every one of them as a parent.
// For the same reason a job that recorded itself as its origin loses it once it
// has left the queue: a one-off job is not a parent anyone filters by.
//
// An origin still in the queue then lends its current name, and its group when
// it has one, so renaming a watcher or moving it to another group takes its
// artifacts along instead of splitting them under an old label.
func ResolveArtifactSources(arts []store.Artifact, tasks []task.Task, runs []store.Run) {
	queued := make(map[string]task.Task, len(tasks))
	for _, t := range tasks {
		queued[t.ID] = t
	}
	byRun := make(map[string]store.Run, len(runs))
	for _, r := range runs {
		byRun[r.RunID] = r
	}
	for i := range arts {
		a := &arts[i]
		if a.OriginID == "" {
			traceArtifactOrigin(a, queued, byRun)
		}
		o, ok := queued[a.OriginID]
		if !ok && a.OriginID == a.TaskID {
			a.OriginID, a.OriginName = "", ""
		}
		if a.OriginID == "" {
			continue
		}
		if ok {
			a.OriginName = o.Name
			if o.Group != "" {
				a.Group = o.Group
			}
		}
		if a.OriginName == "" {
			a.OriginName = a.OriginID
		}
	}
}

// traceArtifactOrigin works out the group and origin of an artifact published
// before either was recorded.
func traceArtifactOrigin(a *store.Artifact, queued map[string]task.Task, byRun map[string]store.Run) {
	root := a.TaskID
	if run, ok := byRun[a.RunID]; ok && run.Task != nil {
		if a.Group == "" {
			a.Group = run.Task.Group
		}
		switch {
		case run.Task.OriginID != "":
			a.OriginID, a.OriginName = run.Task.OriginID, run.Task.OriginName
		case run.WorkflowID != "" && run.WorkflowID != run.RunID:
			if first, ok := byRun[run.WorkflowID]; ok && first.Task != nil {
				root = first.Task.ID
			}
		}
	}
	if a.Group == "" {
		a.Group = queued[a.TaskID].Group
	}
	if a.OriginID == "" {
		if t, ok := queued[root]; ok {
			a.OriginID, a.OriginName = t.Origin()
		}
	}
}

// copyFile copies src to dst and returns the number of bytes written.
func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("open %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("create %q: %w", dst, err)
	}
	n, copyErr := io.Copy(out, in)
	if closeErr := out.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return 0, fmt.Errorf("copy to %q: %w", dst, copyErr)
	}
	return n, nil
}

// detectContentType resolves a MIME type from the file extension, falling back
// to sniffing the file's leading bytes.
func detectContentType(name, path string) string {
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); ct != "" {
		return ct
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n]) // "application/octet-stream" for empty
}

// MarkArtifactRead marks a single artifact as read (FA-A2).
func MarkArtifactRead(s *store.Store, id string) error {
	return s.UpdateState(func(st *store.State) error {
		st.MarkArtifactRead(id)
		return nil
	})
}

// MarkAllArtifactsRead marks every published artifact as read.
func MarkAllArtifactsRead(s *store.Store) error {
	arts, err := s.Artifacts()
	if err != nil {
		return err
	}
	ids := make([]string, len(arts))
	for i, a := range arts {
		ids[i] = a.ID
	}
	return s.UpdateState(func(st *store.State) error {
		st.MarkAllArtifactsRead(ids)
		return nil
	})
}

// DeleteArtifact removes an artifact's record, its stored file(s), and its
// read-status.
func DeleteArtifact(s *store.Store, id string) error {
	found := false
	if err := s.UpdateArtifacts(func(list *[]store.Artifact) error {
		kept := (*list)[:0]
		for _, a := range *list {
			if a.ID == id {
				found = true
				continue
			}
			kept = append(kept, a)
		}
		*list = kept
		return nil
	}); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("artifact %q not found", id)
	}
	_ = os.RemoveAll(s.ArtifactDir(id))
	_ = s.UpdateState(func(st *store.State) error {
		st.ForgetArtifact(id)
		return nil
	})
	return nil
}
