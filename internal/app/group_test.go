package app

import (
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func groups(s *store.Store, t *testing.T) []string {
	t.Helper()
	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	out := make([]string, len(cfg.Tasks))
	for i, tk := range cfg.Tasks {
		out[i] = tk.ID + ":" + tk.Group
	}
	return out
}

func TestMoveToGroup(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		group string
		to    int
		want  []string
	}{
		{"into a new group", "a", "Nightly", 2, []string{"b:", "c:", "a:Nightly"}},
		{"into a new group, index clamped", "a", "Nightly", 99, []string{"b:", "c:", "a:Nightly"}},
		{"to the top, still ungrouped", "c", "", 0, []string{"c:", "a:", "b:"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := openStore(t)
			for _, id := range []string{"a", "b", "c"} {
				if err := AddTask(s, mk(id)); err != nil {
					t.Fatalf("AddTask: %v", err)
				}
			}
			if err := MoveToGroup(s, tc.id, tc.group, tc.to); err != nil {
				t.Fatalf("MoveToGroup: %v", err)
			}
			got := groups(s, t)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A drop inside a group has to keep the group's tasks together, whatever index
// the dashboard computed: the stored order is what the queue renders.
func TestMoveToGroupKeepsGroupsContiguous(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := AddTask(s, mk(id)); err != nil {
			t.Fatalf("AddTask: %v", err)
		}
	}
	if err := MoveToGroup(s, "a", "G", 3); err != nil {
		t.Fatalf("MoveToGroup: %v", err)
	}
	if err := MoveToGroup(s, "c", "G", 1); err != nil { // lands between b and d
		t.Fatalf("MoveToGroup: %v", err)
	}
	want := "b:,d:,c:G,a:G"
	if got := strings.Join(groups(s, t), ","); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMoveToGroupRejectsBadName(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	if err := MoveToGroup(s, "a", strings.Repeat("x", task.MaxGroupLen+1), 0); err == nil {
		t.Fatal("expected an invalid-group error")
	}
	if err := MoveToGroup(s, "missing", "G", 0); err == nil {
		t.Fatal("expected a not-found error")
	}
}

// A group name is trimmed on the way in, so "Nightly" and " Nightly " are not
// two sections that look identical.
func TestMoveToGroupTrimsName(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	if err := MoveToGroup(s, "a", "  Nightly  ", 0); err != nil {
		t.Fatalf("MoveToGroup: %v", err)
	}
	if got := groups(s, t); got[0] != "a:Nightly" {
		t.Fatalf("got %v, want a:Nightly", got)
	}
}

func TestAddTaskJoinsItsGroup(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	grouped := mk("b")
	grouped.Group = "G"
	_ = AddTask(s, grouped)
	_ = AddTask(s, mk("c"))
	want := "a:,c:,b:G"
	if got := strings.Join(groups(s, t), ","); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGroupCollapsedRoundTrip(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	if err := MoveToGroup(s, "a", "G", 0); err != nil {
		t.Fatalf("MoveToGroup: %v", err)
	}
	if err := SetGroupCollapsed(s, "G", true); err != nil {
		t.Fatalf("SetGroupCollapsed: %v", err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !st.GroupCollapsed("G") {
		t.Fatal("group G should be collapsed")
	}
	if err := SetGroupCollapsed(s, "", true); err == nil {
		t.Fatal("expected an error for an empty group name")
	}
}

// The fold state belongs to a group that exists. Once the last task leaves it,
// the memory of it goes too, so reusing the name later does not start collapsed.
func TestEmptyGroupLosesItsFoldState(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	_ = MoveToGroup(s, "a", "G", 0)
	_ = SetGroupCollapsed(s, "G", true)
	if err := MoveToGroup(s, "a", "", 0); err != nil {
		t.Fatalf("MoveToGroup: %v", err)
	}
	st, _ := s.LoadState()
	if st.GroupCollapsed("G") {
		t.Fatal("fold state of an empty group should be gone")
	}
}

func TestDeletingTheLastTaskClearsItsGroup(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	_ = MoveToGroup(s, "a", "G", 0)
	_ = SetGroupCollapsed(s, "G", true)
	if err := RemoveTask(s, "a"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}
	st, _ := s.LoadState()
	if st.GroupCollapsed("G") {
		t.Fatal("fold state should be gone with the last task")
	}
}

// Editing a task into another group re-files it without touching the order
// inside either group.
func TestEditTaskRefilesIntoGroup(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"a", "b"} {
		_ = AddTask(s, mk(id))
	}
	_ = MoveToGroup(s, "a", "G", 1)
	if err := EditTask(s, "b", func(tk *task.Task) error { tk.Group = "G"; return nil }); err != nil {
		t.Fatalf("EditTask: %v", err)
	}
	// b sat above a, and it keeps that place: re-filing a task changes which
	// section it is in, not its priority relative to the rest.
	want := "b:G,a:G"
	if got := strings.Join(groups(s, t), ","); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSetGroupCollapsedRejectsBadName(t *testing.T) {
	s := openStore(t)
	if err := SetGroupCollapsed(s, strings.Repeat("x", task.MaxGroupLen+1), true); err == nil {
		t.Fatal("expected an invalid-group error")
	}
	if err := SetGroupCollapsed(s, "two\nlines", true); err == nil {
		t.Fatal("expected an invalid-group error")
	}
}

func TestMoveGroupUnknown(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	_ = MoveToGroup(s, "a", "G", 0)
	before := "H"
	if err := MoveGroup(s, "G", &before); err == nil {
		t.Fatal("expected an error for an unknown target group")
	}
	if err := MoveGroup(s, "", nil); err == nil {
		t.Fatal("expected an error for an empty group name")
	}
	// Dropped on itself: nothing to do, and nothing to complain about.
	self := "G"
	if err := MoveGroup(s, "G", &self); err != nil {
		t.Fatalf("MoveGroup onto itself: %v", err)
	}
}

func TestRenameGroup(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"a", "b", "c"} {
		_ = AddTask(s, mk(id))
	}
	_ = MoveToGroup(s, "a", "Nightly", 2)
	_ = MoveToGroup(s, "b", "Nightly", 2)
	_ = SetGroupCollapsed(s, "Nightly", true)

	if err := RenameGroup(s, "Nightly", "  Overnight  "); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if got, want := strings.Join(groups(s, t), ","), "c:,a:Overnight,b:Overnight"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	st, _ := s.LoadState()
	if !st.GroupCollapsed("Overnight") || st.GroupCollapsed("Nightly") {
		t.Fatalf("fold state did not travel with the name: %v", st.CollapsedGroups)
	}
}

func TestRenameGroupMerges(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"a", "b"} {
		_ = AddTask(s, mk(id))
	}
	_ = MoveToGroup(s, "a", "Nightly", 1)
	_ = MoveToGroup(s, "b", "Weekly", 1)
	_ = SetGroupCollapsed(s, "Nightly", true)

	if err := RenameGroup(s, "Nightly", "Weekly"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	// The tasks join the section that was already there: it keeps its place in
	// the queue, and its own task stays ahead of the one that moved in.
	if got, want := strings.Join(groups(s, t), ","), "b:Weekly,a:Weekly"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// The section that was already there keeps its own open/folded state.
	st, _ := s.LoadState()
	if st.GroupCollapsed("Weekly") || len(st.CollapsedGroups) != 0 {
		t.Fatalf("collapsed groups = %v, want empty", st.CollapsedGroups)
	}
}

func TestRenameGroupRejects(t *testing.T) {
	s := openStore(t)
	_ = AddTask(s, mk("a"))
	_ = MoveToGroup(s, "a", "Nightly", 0)
	for _, tc := range []struct{ name, from, to string }{
		{"unknown group", "Weekly", "Overnight"},
		{"no name", "", "Overnight"},
		{"no new name", "Nightly", "   "},
		{"invalid new name", "Nightly", "two\nlines"},
		{"too long", "Nightly", strings.Repeat("x", task.MaxGroupLen+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := RenameGroup(s, tc.from, tc.to); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	// Renaming to the same name is a no-op, not an error.
	if err := RenameGroup(s, "Nightly", "Nightly"); err != nil {
		t.Fatalf("RenameGroup onto itself: %v", err)
	}
	if got := groups(s, t); got[0] != "a:Nightly" {
		t.Fatalf("got %v, want a:Nightly", got)
	}
}

// A merge must not promote the section it merges into: priority is the order of
// the list, and everything between the two groups keeps its place.
func TestRenameGroupMergeKeepsPriority(t *testing.T) {
	s := openStore(t)
	for _, id := range []string{"a", "b", "c"} {
		_ = AddTask(s, mk(id))
	}
	_ = MoveToGroup(s, "a", "Nightly", 0)
	_ = MoveToGroup(s, "b", "Other", 2)
	_ = MoveToGroup(s, "c", "Weekly", 2)
	if got, want := strings.Join(groups(s, t), ","), "a:Nightly,b:Other,c:Weekly"; got != want {
		t.Fatalf("setup %q, want %q", got, want)
	}
	if err := RenameGroup(s, "Nightly", "Weekly"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if got, want := strings.Join(groups(s, t), ","), "b:Other,c:Weekly,a:Weekly"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
