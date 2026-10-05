package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func TestPublishArtifactCopiesAndRecords(t *testing.T) {
	s := openStore(t)
	src := writeTempFile(t, "report.html", "<h1>hi</h1>")

	art, err := PublishArtifact(s, PublishInput{
		ID: "a-1", SourcePath: src, Description: "a summary",
		TaskID: "t1", TaskName: "Nightly", RunID: "r1",
		Group: "dc AG", OriginID: "watch", OriginName: "Watch",
		Now: time.Date(2026, 7, 21, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PublishArtifact: %v", err)
	}
	if art.Title != "report.html" { // defaults to file name
		t.Fatalf("Title = %q, want the file name", art.Title)
	}
	if art.Size != int64(len("<h1>hi</h1>")) {
		t.Fatalf("Size = %d", art.Size)
	}
	if art.ContentType == "" || art.ContentType[:9] != "text/html" {
		t.Fatalf("ContentType = %q, want text/html…", art.ContentType)
	}
	if art.TaskName != "Nightly" || art.RunID != "r1" ||
		art.Group != "dc AG" || art.OriginID != "watch" || art.OriginName != "Watch" {
		t.Fatalf("attribution not recorded: %+v", art)
	}

	// The file is copied into the store as a snapshot.
	copied := s.ArtifactContentPath(art)
	data, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(data) != "<h1>hi</h1>" {
		t.Fatalf("copied content = %q", data)
	}

	// Editing the original afterwards must not change the snapshot.
	if err := os.WriteFile(src, []byte("changed"), 0o644); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}
	data, _ = os.ReadFile(copied)
	if string(data) != "<h1>hi</h1>" {
		t.Fatalf("snapshot changed with the source: %q", data)
	}

	arts, err := s.Artifacts()
	if err != nil || len(arts) != 1 {
		t.Fatalf("Artifacts: %v (n=%d)", err, len(arts))
	}
}

func TestPublishArtifactRejectsMissingAndDir(t *testing.T) {
	s := openStore(t)
	if _, err := PublishArtifact(s, PublishInput{ID: "a-1", SourcePath: "/no/such/file"}); err == nil {
		t.Fatal("expected error for missing file")
	}
	if _, err := PublishArtifact(s, PublishInput{ID: "a-2", SourcePath: t.TempDir()}); err == nil {
		t.Fatal("expected error for a directory")
	}
}

func TestPublishArtifactDuplicateID(t *testing.T) {
	s := openStore(t)
	src := writeTempFile(t, "a.txt", "x")
	if _, err := PublishArtifact(s, PublishInput{ID: "dup", SourcePath: src, Now: time.Now()}); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if _, err := PublishArtifact(s, PublishInput{ID: "dup", SourcePath: src, Now: time.Now()}); err == nil {
		t.Fatal("expected duplicate-id error")
	}
}

func TestDeleteArtifactRemovesFileAndRecord(t *testing.T) {
	s := openStore(t)
	src := writeTempFile(t, "a.txt", "x")
	art, err := PublishArtifact(s, PublishInput{ID: "a-1", SourcePath: src, Now: time.Now()})
	if err != nil {
		t.Fatalf("PublishArtifact: %v", err)
	}
	if err := MarkArtifactRead(s, art.ID); err != nil {
		t.Fatalf("MarkArtifactRead: %v", err)
	}

	if err := DeleteArtifact(s, art.ID); err != nil {
		t.Fatalf("DeleteArtifact: %v", err)
	}
	if _, err := os.Stat(s.ArtifactDir(art.ID)); !os.IsNotExist(err) {
		t.Fatalf("artifact dir should be gone, err=%v", err)
	}
	arts, _ := s.Artifacts()
	if len(arts) != 0 {
		t.Fatalf("expected no artifacts after delete, got %d", len(arts))
	}
	if err := DeleteArtifact(s, art.ID); err == nil {
		t.Fatal("deleting a missing artifact should error")
	}
}

func TestMarkAllArtifactsRead(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"a-1", "a-2"} {
		src := writeTempFile(t, "f.txt", "x")
		if _, err := PublishArtifact(s, PublishInput{ID: id, SourcePath: src, Now: time.Now()}); err != nil {
			t.Fatalf("publish %s: %v", id, err)
		}
	}
	if err := MarkAllArtifactsRead(s); err != nil {
		t.Fatalf("MarkAllArtifactsRead: %v", err)
	}
	st, _ := s.LoadState()
	if !st.IsArtifactRead("a-1") || !st.IsArtifactRead("a-2") {
		t.Fatal("all artifacts should be read")
	}
}

func TestResolveArtifactSources(t *testing.T) {
	watcher := task.Task{ID: "adr-watch", Name: "dc: ADR Wiki Watch", Group: "dc AG"}
	digest := task.Task{ID: "digest", Name: "Morning Digest", Group: "dc AG"}
	runs := []store.Run{
		// A digest run, the join it queued, and a run with no snapshot at all.
		{RunID: "r-digest", WorkflowID: "r-digest", Task: &task.Task{ID: "digest", Name: "Morning Digest"}},
		{RunID: "r-join", WorkflowID: "r-digest", Task: &task.Task{ID: "q-join", Name: "Join", Group: "Old group"}},
		{RunID: "r-bare"},
		// A review job's run and its follow-up, long after its watcher's
		// quiet runs left history.
		{RunID: "r-review", WorkflowID: "r-review", Task: &task.Task{ID: "q-review", Name: "Review #1"}},
		{RunID: "r-fix", WorkflowID: "r-review", Task: &task.Task{ID: "q-fix", Name: "Fix #1"}},
	}
	rollout := task.Task{ID: "rollout", Name: "Rollout", OriginID: "adr-watch", OriginName: "ADR Watch"}
	tasks := []task.Task{watcher, digest, rollout, {ID: "loner", Name: "Loner"}}

	cases := []struct {
		name              string
		in                store.Artifact
		group, oID, oName string
	}{
		{"recorded origin takes the origin's current name and group",
			store.Artifact{TaskID: "q-1", OriginID: "adr-watch", OriginName: "ADR Watch (old name)", Group: "Elsewhere"},
			"dc AG", "adr-watch", "dc: ADR Wiki Watch"},
		{"recorded origin no longer queued keeps what it recorded",
			store.Artifact{TaskID: "q-1", OriginID: "gone", OriginName: "Gone watcher", Group: "GP3D"},
			"GP3D", "gone", "Gone watcher"},
		{"legacy artifact traced to its workflow's first run",
			store.Artifact{TaskID: "q-join", TaskName: "Join", RunID: "r-join"},
			"dc AG", "digest", "Morning Digest"},
		{"legacy artifact of a root job is its own origin",
			store.Artifact{TaskID: "digest", TaskName: "Morning Digest", RunID: "r-digest"},
			"dc AG", "digest", "Morning Digest"},
		{"legacy artifact whose run is gone has no parent",
			store.Artifact{TaskID: "q-gone", TaskName: "Review #1", RunID: "r-pruned"},
			"", "", ""},
		{"legacy artifact of a one-off job no longer queued has no parent",
			store.Artifact{TaskID: "q-review", TaskName: "Review #1", RunID: "r-review"},
			"", "", ""},
		{"legacy artifact traced to a first run no longer queued has no parent",
			store.Artifact{TaskID: "q-fix", TaskName: "Fix #1", RunID: "r-fix"},
			"", "", ""},
		{"legacy artifact of a queued job another job created takes that job's origin",
			store.Artifact{TaskID: "rollout", TaskName: "Rollout"},
			"dc AG", "adr-watch", "dc: ADR Wiki Watch"},
		{"recorded self-origin of a job no longer queued has no parent",
			store.Artifact{TaskID: "q-3", OriginID: "q-3", OriginName: "One-off", Group: "GP3D"},
			"GP3D", "", ""},
		{"recorded self-origin of a queued job stays",
			store.Artifact{TaskID: "digest", OriginID: "digest", OriginName: "Digest"},
			"dc AG", "digest", "Morning Digest"},
		{"ungrouped origin keeps the publisher's group",
			store.Artifact{TaskID: "q-2", OriginID: "loner", Group: "GP3D"},
			"GP3D", "loner", "Loner"},
		{"run without a snapshot uses the queued publisher",
			store.Artifact{TaskID: "digest", TaskName: "Digest", RunID: "r-bare"},
			"dc AG", "digest", "Morning Digest"},
		{"published outside any run has no source",
			store.Artifact{}, "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			arts := []store.Artifact{c.in}
			ResolveArtifactSources(arts, tasks, runs)
			got := arts[0]
			if got.Group != c.group || got.OriginID != c.oID || got.OriginName != c.oName {
				t.Fatalf("got group=%q origin=%q/%q, want %q %q/%q",
					got.Group, got.OriginID, got.OriginName, c.group, c.oID, c.oName)
			}
		})
	}
}
