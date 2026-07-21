package store

import (
	"path/filepath"
	"testing"
	"time"
)

func sampleArtifact(id string) Artifact {
	return Artifact{
		ID:          id,
		Title:       "Report " + id,
		FileName:    "report.html",
		RelPath:     id + "/report.html",
		Size:        123,
		ContentType: "text/html; charset=utf-8",
		TaskID:      "t-" + id,
		TaskName:    "Task " + id,
		RunID:       "r-" + id,
		PublishedAt: time.Date(2026, 7, 21, 3, 0, 0, 0, time.UTC),
	}
}

func TestArtifactsMissingReturnsEmpty(t *testing.T) {
	s := openTemp(t)
	arts, err := s.Artifacts()
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	if len(arts) != 0 {
		t.Fatalf("expected no artifacts, got %d", len(arts))
	}
}

func TestUpdateArtifactsRoundTripPreservesOrder(t *testing.T) {
	s := openTemp(t)
	for _, id := range []string{"a", "b", "c"} {
		art := sampleArtifact(id)
		if err := s.UpdateArtifacts(func(list *[]Artifact) error {
			*list = append(*list, art)
			return nil
		}); err != nil {
			t.Fatalf("UpdateArtifacts(%s): %v", id, err)
		}
	}
	arts, err := s.Artifacts()
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	if len(arts) != 3 || arts[0].ID != "a" || arts[2].ID != "c" {
		t.Fatalf("unexpected order/content: %+v", arts)
	}
	if arts[1].ContentType != "text/html; charset=utf-8" || arts[1].TaskName != "Task b" {
		t.Fatalf("fields did not round-trip: %+v", arts[1])
	}
}

func TestArtifactContentPath(t *testing.T) {
	s := openTemp(t)
	a := sampleArtifact("a")
	want := filepath.Join(s.Home(), "artifacts", "a", "report.html")
	if got := s.ArtifactContentPath(a); got != want {
		t.Fatalf("ArtifactContentPath = %q, want %q", got, want)
	}
	if got := s.ArtifactDir("a"); got != filepath.Join(s.Home(), "artifacts", "a") {
		t.Fatalf("ArtifactDir = %q", got)
	}
}

func TestArtifactReadState(t *testing.T) {
	s := openTemp(t)
	if err := s.UpdateState(func(st *State) error {
		st.MarkArtifactRead("a")
		return nil
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !st.IsArtifactRead("a") {
		t.Fatal("artifact a should be read")
	}
	if st.IsArtifactRead("b") {
		t.Fatal("artifact b should be unread")
	}
	st.ForgetArtifact("a")
	if st.IsArtifactRead("a") {
		t.Fatal("forgotten artifact should be unread again")
	}
}
