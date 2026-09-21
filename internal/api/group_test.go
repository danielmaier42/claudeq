package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/task"
)

func TestMoveTaskIntoGroup(t *testing.T) {
	srv, st := newServer(t, nil)
	for _, id := range []string{"a", "b", "c"} {
		if r := do(t, srv, "POST", "/api/tasks", sampleTask(id)); r.Status != http.StatusCreated {
			t.Fatalf("add %s: %d", id, r.Status)
		}
	}
	if r := do(t, srv, "POST", "/api/tasks/a/move?to=2&group=Nightly", nil); r.Status != http.StatusNoContent {
		t.Fatalf("move status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	var got []string
	for _, tk := range cfg.Tasks {
		got = append(got, tk.ID+":"+tk.Group)
	}
	if want := "b:,c:,a:Nightly"; strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %q", got, want)
	}

	// No group in the query means "leave the group alone" — a plain reorder.
	if r := do(t, srv, "POST", "/api/tasks/a/move?to=0", nil); r.Status != http.StatusNoContent {
		t.Fatalf("reorder status = %d", r.Status)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Tasks[0].ID != "a" || cfg.Tasks[0].Group != "Nightly" {
		t.Fatalf("plain reorder should move the task and keep its group: %+v", cfg.Tasks)
	}

	// An empty group is a value, not a missing one: it takes the task out again.
	if r := do(t, srv, "POST", "/api/tasks/a/move?to=0&group=", nil); r.Status != http.StatusNoContent {
		t.Fatalf("ungroup status = %d", r.Status)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Tasks[0].ID != "a" || cfg.Tasks[0].Group != "" {
		t.Fatalf("task was not taken out of its group: %+v", cfg.Tasks)
	}
}

func TestMoveTaskRejectsBadGroup(t *testing.T) {
	srv, _ := newServer(t, nil)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	long := strings.Repeat("x", task.MaxGroupLen+1)
	if r := do(t, srv, "POST", "/api/tasks/a/move?to=0&group="+long, nil); r.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", r.Status)
	}
}

func TestGroupsListAndCollapse(t *testing.T) {
	srv, _ := newServer(t, nil)
	for _, id := range []string{"a", "b", "c"} {
		do(t, srv, "POST", "/api/tasks", sampleTask(id))
	}
	do(t, srv, "POST", "/api/tasks/a/move?to=2&group=Nightly", nil)
	do(t, srv, "POST", "/api/tasks/b/move?to=2&group=Nightly", nil)

	var groups []groupView
	do(t, srv, "GET", "/api/groups", nil).into(t, &groups)
	if len(groups) != 1 || groups[0].Name != "Nightly" || groups[0].Count != 2 || groups[0].Collapsed {
		t.Fatalf("groups = %+v, want one open Nightly with 2 tasks", groups)
	}

	if r := do(t, srv, "POST", "/api/groups/collapse", map[string]any{"name": "Nightly", "collapsed": true}); r.Status != http.StatusNoContent {
		t.Fatalf("collapse status = %d (%s)", r.Status, r.Body)
	}
	groups = nil
	do(t, srv, "GET", "/api/groups", nil).into(t, &groups)
	if len(groups) != 1 || !groups[0].Collapsed {
		t.Fatalf("groups = %+v, want Nightly collapsed", groups)
	}

	if r := do(t, srv, "POST", "/api/groups/collapse", map[string]any{"name": "", "collapsed": true}); r.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a nameless group", r.Status)
	}
}

// The task form does not show the group — it is set by dragging the row — so a
// PUT that says nothing about it must not quietly take the task out.
func TestUpdateTaskKeepsTheGroup(t *testing.T) {
	srv, st := newServer(t, nil)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	do(t, srv, "POST", "/api/tasks/a/move?to=0&group=Nightly", nil)

	upd := sampleTask("a")
	upd.Name = "renamed"
	if r := do(t, srv, "PUT", "/api/tasks/a", upd); r.Status != http.StatusOK {
		t.Fatalf("update status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Tasks[0].Group != "Nightly" || cfg.Tasks[0].Name != "renamed" {
		t.Fatalf("got %+v, want the rename with the group kept", cfg.Tasks[0])
	}

	// A payload that does name a group re-files the task.
	if r := do(t, srv, "PUT", "/api/tasks/a", map[string]any{
		"name": "renamed", "prompt": "p", "working_dir": "/r", "trigger": "asap",
		"enabled": true, "permissions": "default", "group": "",
	}); r.Status != http.StatusOK {
		t.Fatalf("ungroup status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Tasks[0].Group != "" {
		t.Fatalf("group = %q, want it cleared", cfg.Tasks[0].Group)
	}
}
