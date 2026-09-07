package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/task"
)

func parentJSON(t *testing.T, p task.Task) string {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal parent: %v", err)
	}
	return string(b)
}

func TestBuildQueuedTaskInheritsParentSettings(t *testing.T) {
	parent := task.Task{
		ID: "nightly", Name: "nightly", Prompt: "check things",
		WorkingDir: "/repo", Trigger: task.TriggerCron, Cron: "0 2 * * *",
		Parallel: true, Enabled: true, Model: "claude-opus-4-8",
		Permissions: task.PermissionsSkip, NotifyOnResult: true, QuietHistory: true,
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	got, err := buildQueuedTask(parentJSON(t, parent), "q-1", queueOpts{prompt: "optimize the widget"}, now)
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
	}

	// Deliberately not inherited: a watcher's follow-up is real work.
	if got.QuietHistory {
		t.Fatal("quiet_history must not be inherited by a queued follow-up task")
	}

	// Inherited from the parent.
	if got.Model != "claude-opus-4-8" || got.Permissions != task.PermissionsSkip ||
		!got.Parallel || !got.NotifyOnResult {
		t.Fatalf("inheritable settings not carried over: %+v", got)
	}
	if got.WorkingDir != "/repo" {
		t.Fatalf("working dir = %q, want inherited /repo", got.WorkingDir)
	}
	// Reset / overridden.
	if got.ID != "q-1" || got.Prompt != "optimize the widget" || !got.Enabled {
		t.Fatalf("identity fields not applied: %+v", got)
	}
	if got.Trigger != task.TriggerASAP {
		t.Fatalf("default trigger = %q, want asap", got.Trigger)
	}
	if got.Cron != "" || !got.FixedAt.IsZero() {
		t.Fatalf("parent schedule leaked into the queued task: cron=%q fixed=%v", got.Cron, got.FixedAt)
	}
	if got.Name == "" {
		t.Fatal("expected a name derived from the prompt")
	}
}

func TestBuildQueuedTaskTiming(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	parent := task.Task{WorkingDir: "/repo", Permissions: task.PermissionsDefault}
	pj := parentJSON(t, parent)

	tests := []struct {
		name        string
		opts        queueOpts
		wantTrigger task.Trigger
		wantFixed   time.Time
		wantCron    string
	}{
		{"asap by default", queueOpts{prompt: "p"}, task.TriggerASAP, time.Time{}, ""},
		{"at fixed time", queueOpts{prompt: "p", at: "2026-07-21T03:00:00Z"}, task.TriggerFixed, time.Date(2026, 7, 21, 3, 0, 0, 0, time.UTC), ""},
		{"in duration", queueOpts{prompt: "p", in: "90m"}, task.TriggerFixed, now.Add(90 * time.Minute), ""},
		{"cron", queueOpts{prompt: "p", cron: "0 3 * * *"}, task.TriggerCron, time.Time{}, "0 3 * * *"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildQueuedTask(pj, "q-1", tc.opts, now)
			if err != nil {
				t.Fatalf("buildQueuedTask: %v", err)
			}
			if got.Trigger != tc.wantTrigger {
				t.Fatalf("trigger = %q, want %q", got.Trigger, tc.wantTrigger)
			}
			if !got.FixedAt.Equal(tc.wantFixed) {
				t.Fatalf("fixed = %v, want %v", got.FixedAt, tc.wantFixed)
			}
			if got.Cron != tc.wantCron {
				t.Fatalf("cron = %q, want %q", got.Cron, tc.wantCron)
			}
		})
	}
}

func TestBuildQueuedTaskDirOverride(t *testing.T) {
	parent := task.Task{WorkingDir: "/repo", Permissions: task.PermissionsDefault}
	got, err := buildQueuedTask(parentJSON(t, parent), "q-1",
		queueOpts{prompt: "p", dir: "/other"}, time.Now())
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
	}
	if got.WorkingDir != "/other" {
		t.Fatalf("working dir = %q, want /other", got.WorkingDir)
	}
}

func TestBuildQueuedTaskErrors(t *testing.T) {
	parent := task.Task{WorkingDir: "/repo", Permissions: task.PermissionsDefault}
	pj := parentJSON(t, parent)
	now := time.Now()

	tests := []struct {
		name string
		opts queueOpts
	}{
		{"missing prompt", queueOpts{}},
		{"two timings", queueOpts{prompt: "p", at: "2026-07-21T03:00:00Z", cron: "0 3 * * *"}},
		{"bad at", queueOpts{prompt: "p", at: "not-a-time"}},
		{"bad in", queueOpts{prompt: "p", in: "soon"}},
		{"negative in", queueOpts{prompt: "p", in: "-5m"}},
		{"bad cron", queueOpts{prompt: "p", cron: "not a cron"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildQueuedTask(pj, "q-1", tc.opts, now); err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
		})
	}
}

func TestBuildQueuedTaskStandaloneNeedsDir(t *testing.T) {
	// No parent context and no --dir: the task has no working directory, so it
	// must fail validation rather than produce an invalid task.
	if _, err := buildQueuedTask("", "q-1", queueOpts{prompt: "p"}, time.Now()); err == nil {
		t.Fatal("expected an error when neither a parent nor --dir supplies a working dir")
	}
	// With --dir it succeeds and defaults permissions.
	got, err := buildQueuedTask("", "q-1", queueOpts{prompt: "p", dir: "/x"}, time.Now())
	if err != nil {
		t.Fatalf("buildQueuedTask standalone with --dir: %v", err)
	}
	if got.Permissions != task.PermissionsDefault {
		t.Fatalf("permissions = %q, want default when standalone", got.Permissions)
	}
}

func TestNewQueueIDUnique(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newQueueID(now)
		if seen[id] {
			t.Fatalf("duplicate id %q within the same second", id)
		}
		seen[id] = true
	}
}

func TestParseQueueOptsRecordsOverrides(t *testing.T) {
	o, err := parseQueueOpts([]string{
		"--prompt", "p", "--model", "claude-opus-5", "--parallel=false",
		"--skip-permissions", "--notify=false", "--quiet-history",
	})
	if err != nil {
		t.Fatalf("parseQueueOpts: %v", err)
	}
	for _, name := range []string{"prompt", "model", "parallel", "skip-permissions", "notify", "quiet-history"} {
		if !o.has(name) {
			t.Fatalf("flag %q passed but not recorded as set: %v", name, o.set)
		}
	}
	for _, name := range []string{"at", "in", "cron", "dir", "name"} {
		if o.has(name) {
			t.Fatalf("flag %q recorded as set although it was not passed", name)
		}
	}
	if o.model != "claude-opus-5" || o.parallel || !o.skipPerms || o.notify || !o.quietHistory {
		t.Fatalf("override values not parsed: %+v", o)
	}
}

func TestParseQueueOptsRejectsStrayArgument(t *testing.T) {
	if _, err := parseQueueOpts([]string{"--prompt", "p", "stray"}); err == nil {
		t.Fatal("expected an error for a positional argument")
	}
	// A boolean written with a space is the likely mistake; the error says how
	// to write it instead of only naming the stray token.
	_, err := parseQueueOpts([]string{"--prompt", "p", "--notify", "true"})
	if err == nil || !strings.Contains(err.Error(), "--flag=true") {
		t.Fatalf("expected a hint about --flag=true, got %v", err)
	}
}

func TestBuildQueuedTaskOverridesInheritedSettings(t *testing.T) {
	// A cheap, quiet watcher with everything switched on or off in a way that
	// each override has to flip.
	parent := task.Task{
		ID: "watcher", Name: "watcher", Prompt: "watch", WorkingDir: "/repo",
		Trigger: task.TriggerCron, Cron: "*/5 * * * *", Enabled: true,
		Model: "claude-haiku-4-5-20251001", Permissions: task.PermissionsSkip,
		Parallel: true, NotifyOnResult: false, QuietHistory: true,
	}
	pj := parentJSON(t, parent)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	type want struct {
		model   string
		perms   task.Permissions
		par     bool
		notify  bool
		quiet   bool
		trigger task.Trigger
	}
	// What pure inheritance yields; each case states only its deviation.
	inherited := want{model: "claude-haiku-4-5-20251001", perms: task.PermissionsSkip, par: true, trigger: task.TriggerASAP}
	tests := []struct {
		name string
		args []string
		want func(w want) want
	}{
		{"nothing passed keeps inheriting", []string{"--prompt", "p"}, func(w want) want { return w }},
		{"model", []string{"--prompt", "p", "--model", "claude-opus-5"},
			func(w want) want { w.model = "claude-opus-5"; return w }},
		{"empty model falls back to the global default", []string{"--prompt", "p", "--model", ""},
			func(w want) want { w.model = ""; return w }},
		{"parallel off", []string{"--prompt", "p", "--parallel=false"},
			func(w want) want { w.par = false; return w }},
		{"skip-permissions off", []string{"--prompt", "p", "--skip-permissions=false"},
			func(w want) want { w.perms = task.PermissionsDefault; return w }},
		{"notify on", []string{"--prompt", "p", "--notify"},
			func(w want) want { w.notify = true; return w }},
		{"quiet history on", []string{"--prompt", "p", "--quiet-history=true"},
			func(w want) want { w.quiet = true; return w }},
		{"all at once with timing", []string{"--prompt", "p", "--in", "10m", "--model", "claude-opus-5",
			"--parallel=false", "--skip-permissions=false", "--notify=true", "--quiet-history=false"},
			func(want) want {
				return want{model: "claude-opus-5", perms: task.PermissionsDefault, notify: true, trigger: task.TriggerFixed}
			}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, err := parseQueueOpts(tc.args)
			if err != nil {
				t.Fatalf("parseQueueOpts: %v", err)
			}
			got, err := buildQueuedTask(pj, "q-1", o, now)
			if err != nil {
				t.Fatalf("buildQueuedTask: %v", err)
			}
			g := want{got.Model, got.Permissions, got.Parallel, got.NotifyOnResult, got.QuietHistory, got.Trigger}
			if w := tc.want(inherited); g != w {
				t.Fatalf("got %+v, want %+v", g, w)
			}
			// Overrides never leak into identity or scheduling.
			if got.ID != "q-1" || got.Cron != "" || got.WorkingDir != "/repo" {
				t.Fatalf("unrelated fields changed: %+v", got)
			}
		})
	}
}

func TestBuildQueuedTaskStandaloneOverrides(t *testing.T) {
	// Without a parent there is nothing to inherit; the overrides still apply
	// on top of the defaults.
	o, err := parseQueueOpts([]string{"--prompt", "p", "--dir", "/x", "--model", "claude-opus-5", "--skip-permissions"})
	if err != nil {
		t.Fatalf("parseQueueOpts: %v", err)
	}
	got, err := buildQueuedTask("", "q-1", o, time.Now())
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
	}
	if got.Model != "claude-opus-5" || got.Permissions != task.PermissionsSkip {
		t.Fatalf("overrides not applied standalone: %+v", got)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  string
	}{
		{"", 5, ""},
		{"short", 5, "short"},
		{"longer", 5, "long…"},
		{"ünïcödé nämé", 8, "ünïcödé…"},
		{"ünïcödé", 7, "ünïcödé"}, // more bytes than width, but not more runes
		{"abc", 1, "…"},
		{"abc", 0, ""},
		{"abc", -1, ""},
	}
	for _, tc := range tests {
		if got := truncate(tc.in, tc.width); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
		}
	}
}

func TestDefaultQueueNameIsBounded(t *testing.T) {
	long := strings.Repeat("x", nameWidth+10)
	got := defaultQueueName(long)
	if n := len([]rune(got)); n != nameWidth || !strings.HasSuffix(got, "…") {
		t.Fatalf("defaultQueueName(%d x) = %q (%d runes), want %d ending in an ellipsis", len(long), got, n, nameWidth)
	}
	if got := defaultQueueName("  a\n  b  "); got != "a b" {
		t.Fatalf("whitespace not collapsed: %q", got)
	}
}

func TestListCutsLongNamesButJSONKeepsThem(t *testing.T) {
	st := newTestStore(t)
	long := "A follow-up task name that is longer than the queue column\twith a tab"
	if err := app.AddTask(st, task.Task{
		ID: "long", Name: long, Prompt: "p", WorkingDir: "/repo",
		Trigger: task.TriggerASAP, Enabled: true, Permissions: task.PermissionsDefault,
	}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	table := captureStdout(t, func() error { return cmdList(st, nil) })
	if strings.Contains(table, long) || strings.Contains(table, "\t") {
		t.Fatalf("list must cut and single-line the name, got:\n%s", table)
	}
	if !strings.Contains(table, "A follow-up task name that is longer th…") {
		t.Fatalf("list should show the cut name with an ellipsis, got:\n%s", table)
	}

	asJSON := captureStdout(t, func() error { return cmdList(st, []string{"--json"}) })
	var tasks []task.Task
	if err := json.Unmarshal([]byte(asJSON), &tasks); err != nil {
		t.Fatalf("list --json: %v\n%s", err, asJSON)
	}
	if len(tasks) != 1 || tasks[0].Name != long {
		t.Fatalf("list --json must keep the full name, got %+v", tasks)
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	runErr := fn()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out := <-done
	if runErr != nil {
		t.Fatalf("command failed: %v", runErr)
	}
	return out
}
