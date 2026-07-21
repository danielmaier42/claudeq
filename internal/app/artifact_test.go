package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if art.TaskName != "Nightly" || art.RunID != "r1" {
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
