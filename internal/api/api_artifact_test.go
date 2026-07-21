package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
)

// publishTestArtifact writes a temp file and publishes it into the store.
func publishTestArtifact(t *testing.T, st *store.Store, id, name, content string) store.Artifact {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	art, err := app.PublishArtifact(st, app.PublishInput{
		ID: id, SourcePath: src, Title: name, TaskName: "T", Now: time.Now(),
	})
	if err != nil {
		t.Fatalf("PublishArtifact: %v", err)
	}
	return art
}

func TestListArtifactsNewestFirstUnread(t *testing.T) {
	srv, st := newServer(t, nil)
	publishTestArtifact(t, st, "a-1", "one.txt", "1")
	publishTestArtifact(t, st, "a-2", "two.txt", "2")

	var views []artifactView
	r := do(t, srv, "GET", "/api/artifacts", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("list status = %d", r.Status)
	}
	r.into(t, &views)
	if len(views) != 2 {
		t.Fatalf("want 2 artifacts, got %d", len(views))
	}
	if views[0].ID != "a-2" { // newest first
		t.Fatalf("want newest (a-2) first, got %q", views[0].ID)
	}
	if !views[0].Unread || !views[1].Unread {
		t.Fatal("freshly published artifacts must be unread")
	}
}

func TestArtifactContentServesFileWithSafeHeaders(t *testing.T) {
	srv, st := newServer(t, nil)
	publishTestArtifact(t, st, "a-1", "page.html", "<h1>hi</h1>")

	r := do(t, srv, "GET", "/api/artifacts/a-1/content", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("content status = %d", r.Status)
	}
	if string(r.Body) != "<h1>hi</h1>" {
		t.Fatalf("content body = %q", r.Body)
	}
}

func TestArtifactContentHeaders(t *testing.T) {
	srv, st := newServer(t, nil)
	publishTestArtifact(t, st, "a-1", "page.html", "<h1>hi</h1>")

	req, _ := http.NewRequest("GET", srv.URL+"/api/artifacts/a-1/content", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET content: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct[:9] != "text/html" {
		t.Fatalf("Content-Type = %q, want text/html…", ct)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing X-Content-Type-Options: nosniff")
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp == "" {
		t.Fatal("missing Content-Security-Policy on artifact content")
	}
}

func TestArtifactContentNotFound(t *testing.T) {
	srv, _ := newServer(t, nil)
	if r := do(t, srv, "GET", "/api/artifacts/nope/content", nil); r.Status != http.StatusNotFound {
		t.Fatalf("want 404 for unknown artifact, got %d", r.Status)
	}
}

func TestArtifactReadAndReadAll(t *testing.T) {
	srv, st := newServer(t, nil)
	publishTestArtifact(t, st, "a-1", "one.txt", "1")
	publishTestArtifact(t, st, "a-2", "two.txt", "2")

	if r := do(t, srv, "POST", "/api/artifacts/a-1/read", nil); r.Status != http.StatusNoContent {
		t.Fatalf("read status = %d", r.Status)
	}
	var views []artifactView
	do(t, srv, "GET", "/api/artifacts", nil).into(t, &views)
	for _, v := range views {
		if v.ID == "a-1" && v.Unread {
			t.Fatal("a-1 should be read")
		}
	}

	if r := do(t, srv, "POST", "/api/artifacts/read-all", nil); r.Status != http.StatusNoContent {
		t.Fatalf("read-all status = %d", r.Status)
	}
	views = nil
	do(t, srv, "GET", "/api/artifacts", nil).into(t, &views)
	for _, v := range views {
		if v.Unread {
			t.Fatalf("artifact %q should be read after read-all", v.ID)
		}
	}
}

func TestArtifactDelete(t *testing.T) {
	srv, st := newServer(t, nil)
	art := publishTestArtifact(t, st, "a-1", "one.txt", "1")

	if r := do(t, srv, "DELETE", "/api/artifacts/a-1", nil); r.Status != http.StatusNoContent {
		t.Fatalf("delete status = %d", r.Status)
	}
	if _, err := os.Stat(st.ArtifactDir(art.ID)); !os.IsNotExist(err) {
		t.Fatalf("artifact dir should be removed, err=%v", err)
	}
	if r := do(t, srv, "DELETE", "/api/artifacts/a-1", nil); r.Status != http.StatusBadRequest {
		t.Fatalf("deleting missing artifact: want 400, got %d", r.Status)
	}
}
