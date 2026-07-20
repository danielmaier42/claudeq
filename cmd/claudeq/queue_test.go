package main

import (
	"encoding/json"
	"testing"
	"time"

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
		Permissions: task.PermissionsSkip, NotifyOnResult: true,
	}
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	got, err := buildQueuedTask(parentJSON(t, parent), "q-1", queueOpts{prompt: "optimize the widget"}, now)
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
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
