package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
)

// TestBackfillReportedModels: finished runs recorded before dispatchq kept the
// reported model get it read back out of their logs; anything else is left as
// it was.
func TestBackfillReportedModels(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	fake := newFakeAdapter("fake")
	fake.events = []Event{{Type: EventSessionStarted, SessionID: "s", Model: "fake-full-id"}}
	if err := reg.Register(fake); err != nil {
		t.Fatal(err)
	}
	logOf := func(id string) string {
		p := filepath.Join(home, id+".log")
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	runs := []store.Run{
		{RunID: "old", Status: store.StatusSuccess, LogPath: logOf("old"),
			Provider: store.RunProvider{Kind: "fake", Model: "alias"}},
		{RunID: "known", Status: store.StatusSuccess, LogPath: logOf("known"),
			Provider: store.RunProvider{Kind: "fake", ReportedModel: "kept"}},
		{RunID: "running", Status: store.StatusRunning, LogPath: logOf("running"),
			Provider: store.RunProvider{Kind: "fake"}},
		{RunID: "nolog", Status: store.StatusFailed, LogPath: filepath.Join(home, "gone.log"),
			Provider: store.RunProvider{Kind: "fake"}},
		{RunID: "unreadable", Status: store.StatusSuccess, LogPath: home,
			Provider: store.RunProvider{Kind: "fake"}},
		{RunID: "after", Status: store.StatusSuccess, LogPath: logOf("after"),
			Provider: store.RunProvider{Kind: "fake"}},
		{RunID: "script", Status: store.StatusSuccess, LogPath: logOf("script"),
			Provider: store.RunProvider{Kind: store.ScriptRunKind}},
	}
	for _, r := range runs {
		if err := st.AppendRun(r); err != nil {
			t.Fatal(err)
		}
	}

	// The unreadable log (a directory) is reported without holding up the
	// run after it.
	n, err := BackfillReportedModels(st, reg)
	if err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("err = %v, want the unreadable run reported", err)
	}
	if n != 2 {
		t.Fatalf("filled in %d runs, want 2", n)
	}
	got, err := st.Runs()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"old": "fake-full-id", "known": "kept", "running": "", "nolog": "",
		"unreadable": "", "after": "fake-full-id", "script": ""}
	for _, r := range got {
		if r.Provider.ReportedModel != want[r.RunID] {
			t.Errorf("run %s reported model = %q, want %q", r.RunID, r.Provider.ReportedModel, want[r.RunID])
		}
	}
	if got[0].Provider.Model != "alias" {
		t.Errorf("requested model = %q, want it kept", got[0].Provider.Model)
	}

	// A second start finds nothing left to do.
	if n, _ := BackfillReportedModels(st, reg); n != 0 {
		t.Fatalf("second pass filled %d (err %v), want 0", n, err)
	}
}

// TestCollectorKeepsTheFirstReportedModel: the run's own model is named first;
// a later one belongs to something the run started.
func TestCollectorKeepsTheFirstReportedModel(t *testing.T) {
	c := NewCollector("", "x")
	c.Add(Event{Type: EventSessionStarted, SessionID: "s"})
	c.Add(Event{Type: EventSessionStarted, SessionID: "s", Model: "first"})
	c.Add(Event{Type: EventSessionStarted, SessionID: "s", Model: "second"})
	if got := c.Result(0).Model; got != "first" {
		t.Fatalf("model = %q, want first", got)
	}
}
