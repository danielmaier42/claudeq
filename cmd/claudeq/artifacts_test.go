package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// seedArtifacts records three artifacts: two from review jobs a queued
// watcher created, one from a finished one-off job that has left the queue.
func seedArtifacts(t *testing.T, st *store.Store, base time.Time) {
	t.Helper()
	arts := []store.Artifact{
		{ID: "a1", Title: "Report 1", FileName: "r1.html", RelPath: "a1/r1.html",
			TaskID: "review-1", OriginID: "watch", OriginName: "Old name", PublishedAt: base},
		{ID: "a2", Title: "One-off", FileName: "x.txt", RelPath: "a2/x.txt",
			TaskID: "oneoff", OriginID: "oneoff", OriginName: "One-off", PublishedAt: base.Add(time.Hour)},
		{ID: "a3", Title: "Report 2", FileName: "r2.html", RelPath: "a3/r2.html",
			TaskID: "review-2", OriginID: "watch", OriginName: "Old name", PublishedAt: base.Add(2 * time.Hour)},
	}
	if err := st.UpdateArtifacts(func(list *[]store.Artifact) error { *list = arts; return nil }); err != nil {
		t.Fatalf("UpdateArtifacts: %v", err)
	}
	if err := st.UpdateConfig(func(c *store.Config) error {
		c.Tasks = append(c.Tasks, task.Task{ID: "watch", Name: "Watcher", Group: "ops", Prompt: "p", WorkingDir: t.TempDir(),
			Trigger: task.TriggerASAP, Enabled: true, Permissions: task.PermissionsDefault, Model: "opus"})
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if err := app.MarkArtifactRead(st, "a1"); err != nil {
		t.Fatalf("MarkArtifactRead: %v", err)
	}
}

func listArtifactsJSON(t *testing.T, st *store.Store, now time.Time, args ...string) []listedArtifact {
	t.Helper()
	out := captureStdout(t, func() error { return cmdArtifactsList(st, append([]string{"--json"}, args...), now) })
	var got []listedArtifact
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("artifacts list --json: %v\n%s", err, out)
	}
	return got
}

func artifactIDs(arts []listedArtifact) string {
	ids := make([]string, len(arts))
	for i, a := range arts {
		ids[i] = a.ID
	}
	return strings.Join(ids, ",")
}

func TestArtifactsListJSONResolvesParentPathAndUnread(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	seedArtifacts(t, st, base)

	got := listArtifactsJSON(t, st, base.Add(3*time.Hour))
	if ids := artifactIDs(got); ids != "a1,a2,a3" {
		t.Fatalf("want a1,a2,a3 oldest first, got %s", ids)
	}
	a1 := got[0]
	if a1.OriginID != "watch" || a1.OriginName != "Watcher" || a1.Group != "ops" {
		t.Errorf("a1 parent not resolved from the queue: %+v", a1)
	}
	if want := filepath.Join(st.ArtifactsDir(), "a1", "r1.html"); a1.Path != want {
		t.Errorf("a1 path = %q, want %q", a1.Path, want)
	}
	if a1.Unread || !got[1].Unread || !got[2].Unread {
		t.Errorf("unread flags wrong: %v %v %v", a1.Unread, got[1].Unread, got[2].Unread)
	}
	if got[1].OriginID != "" {
		t.Errorf("a finished one-off job is no parent, got %q", got[1].OriginID)
	}
}

func TestArtifactsListFilters(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	seedArtifacts(t, st, base)
	now := base.Add(3 * time.Hour)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"origin", []string{"--origin", "watch"}, "a1,a3"},
		{"unknown origin", []string{"--origin", "nope"}, ""},
		{"since is exclusive", []string{"--since", base.Format(time.RFC3339)}, "a2,a3"},
		{"since takes fractional seconds", []string{"--since", base.Add(time.Hour).Format(time.RFC3339Nano)}, "a3"},
		{"since as duration", []string{"--since", "90m"}, "a3"},
		{"origin and since", []string{"--origin", "watch", "--since", "2h30m"}, "a3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if ids := artifactIDs(listArtifactsJSON(t, st, now, c.args...)); ids != c.want {
				t.Errorf("got %q, want %q", ids, c.want)
			}
		})
	}
}

func TestArtifactsListJSONWithoutArtifactsIsAnEmptyList(t *testing.T) {
	st := newTestStore(t)
	out := captureStdout(t, func() error { return cmdArtifactsList(st, []string{"--json"}, time.Now()) })
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("want [], got %q", out)
	}
}

func TestArtifactsListTable(t *testing.T) {
	st := newTestStore(t)
	base := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	seedArtifacts(t, st, base)
	out := captureStdout(t, func() error { return cmdArtifactsList(st, nil, base) })
	if !strings.Contains(out, "Report 2") || !strings.Contains(out, "Watcher") || !strings.Contains(out, "2 unread") {
		t.Errorf("table misses title, parent or unread count:\n%s", out)
	}
}

func TestArtifactsListRejectsBadSince(t *testing.T) {
	st := newTestStore(t)
	for _, v := range []string{"yesterday", "-2h", "0s"} {
		if err := cmdArtifactsList(st, []string{"--since", v}, time.Now()); err == nil {
			t.Errorf("--since %q: want an error", v)
		}
	}
	if err := cmdArtifacts(st, []string{"show"}); err == nil {
		t.Error("unknown subcommand: want an error")
	}
}
