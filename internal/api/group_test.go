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
// PUT that says nothing about it must not quietly take the task out. A payload
// that does name one (an empty name included) has its say.
func TestUpdateTaskKeepsTheGroupUnlessTold(t *testing.T) {
	srv, st := newServer(t, nil)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	do(t, srv, "POST", "/api/tasks/a/move?to=0&group=Nightly", nil)
	form := map[string]any{"name": "renamed", "prompt": "p", "working_dir": "/r",
		"trigger": "asap", "enabled": true, "permissions": "default"}

	if r := do(t, srv, "PUT", "/api/tasks/a", form); r.Status != http.StatusOK {
		t.Fatalf("update status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Tasks[0].Group != "Nightly" || cfg.Tasks[0].Name != "renamed" {
		t.Fatalf("got %+v, want the rename with the group kept", cfg.Tasks[0])
	}

	form["group"] = ""
	if r := do(t, srv, "PUT", "/api/tasks/a", form); r.Status != http.StatusOK {
		t.Fatalf("ungroup status = %d (%s)", r.Status, r.Body)
	}
	cfg, _ = st.LoadConfig()
	if cfg.Tasks[0].Group != "" {
		t.Fatalf("group = %q, want it cleared", cfg.Tasks[0].Group)
	}
	// The group is empty now, so nothing may be left remembering it.
	state, _ := st.LoadState()
	if state.GroupCollapsed("Nightly") || len(state.CollapsedGroups) != 0 {
		t.Fatalf("collapsed groups = %v, want empty", state.CollapsedGroups)
	}
}

func TestMoveGroup(t *testing.T) {
	srv, st := newServer(t, nil)
	for _, id := range []string{"a", "b", "c"} {
		do(t, srv, "POST", "/api/tasks", sampleTask(id))
	}
	do(t, srv, "POST", "/api/tasks/b/move?to=2&group=Nightly", nil)
	do(t, srv, "POST", "/api/tasks/c/move?to=2&group=Weekly", nil)
	order := func() string {
		cfg, _ := st.LoadConfig()
		out := make([]string, len(cfg.Tasks))
		for i, tk := range cfg.Tasks {
			out[i] = tk.ID + ":" + tk.Group
		}
		return strings.Join(out, ",")
	}
	if got, want := order(), "a:,b:Nightly,c:Weekly"; got != want {
		t.Fatalf("setup order %q, want %q", got, want)
	}

	// Weekly in front of Nightly.
	if r := do(t, srv, "POST", "/api/groups/move", map[string]any{"name": "Weekly", "before": "Nightly"}); r.Status != http.StatusNoContent {
		t.Fatalf("status = %d (%s)", r.Status, r.Body)
	}
	if got, want := order(), "a:,c:Weekly,b:Nightly"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}

	// In front of the ungrouped section.
	do(t, srv, "POST", "/api/groups/move", map[string]any{"name": "Nightly", "before": ""})
	if got, want := order(), "b:Nightly,a:,c:Weekly"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}

	// No "before" at all means the end of the queue.
	do(t, srv, "POST", "/api/groups/move", map[string]any{"name": "Nightly"})
	if got, want := order(), "a:,c:Weekly,b:Nightly"; got != want {
		t.Fatalf("order %q, want %q", got, want)
	}

	if r := do(t, srv, "POST", "/api/groups/move", map[string]any{"name": "Nope", "before": "Weekly"}); r.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown group", r.Status)
	}
}

func TestRenameGroupEndpoint(t *testing.T) {
	srv, st := newServer(t, nil)
	do(t, srv, "POST", "/api/tasks", sampleTask("a"))
	do(t, srv, "POST", "/api/tasks/a/move?to=0&group=Nightly", nil)
	do(t, srv, "POST", "/api/groups/collapse", map[string]any{"name": "Nightly", "collapsed": true})

	if r := do(t, srv, "POST", "/api/groups/rename", map[string]any{"name": "Nightly", "to": "Overnight"}); r.Status != http.StatusNoContent {
		t.Fatalf("status = %d (%s)", r.Status, r.Body)
	}
	var groups []groupView
	do(t, srv, "GET", "/api/groups", nil).into(t, &groups)
	if len(groups) != 1 || groups[0].Name != "Overnight" || !groups[0].Collapsed {
		t.Fatalf("groups = %+v, want one folded Overnight", groups)
	}
	cfg, _ := st.LoadConfig()
	if cfg.Tasks[0].Group != "Overnight" {
		t.Fatalf("task group = %q, want Overnight", cfg.Tasks[0].Group)
	}

	if r := do(t, srv, "POST", "/api/groups/rename", map[string]any{"name": "Nope", "to": "X"}); r.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown group", r.Status)
	}
}
