package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func baseTask() task.Task {
	return task.Task{
		ID: "nightly", Name: "Nightly sweep", Prompt: "old prompt",
		WorkingDir: "/repo", Trigger: task.TriggerCron, Cron: "0 3 * * *",
		Enabled: true, Permissions: task.PermissionsDefault,
	}
}

func failingRead(string) ([]byte, error) { return nil, fmt.Errorf("should not be read") }

func TestTaskPatchApply(t *testing.T) {
	fixed := time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		args []string
		want func(task.Task) task.Task
	}{
		{
			name: "prompt only leaves the schedule alone",
			args: []string{"--prompt", "new prompt"},
			want: func(t task.Task) task.Task { t.Prompt = "new prompt"; return t },
		},
		{
			name: "cron implies the cron trigger",
			args: []string{"--cron", "30 4 * * 1"},
			want: func(t task.Task) task.Task { t.Cron = "30 4 * * 1"; return t },
		},
		{
			name: "at implies the fixed trigger and clears the cron",
			args: []string{"--at", "2026-09-01T02:00:00Z"},
			want: func(t task.Task) task.Task {
				t.Trigger = task.TriggerFixed
				t.Cron = ""
				t.FixedAt = fixed
				return t
			},
		},
		{
			name: "switching to asap clears the cron",
			args: []string{"--trigger", "asap"},
			want: func(t task.Task) task.Task {
				t.Trigger = task.TriggerASAP
				t.Cron = ""
				return t
			},
		},
		{
			name: "explicit trigger wins over the implied one",
			args: []string{"--trigger", "cron", "--cron", "0 5 * * *"},
			want: func(t task.Task) task.Task { t.Cron = "0 5 * * *"; return t },
		},
		{
			name: "toggles and overrides",
			args: []string{"--parallel=true", "--enabled=false", "--notify=true", "--model", "opus", "--name", "Renamed"},
			want: func(t task.Task) task.Task {
				t.Parallel, t.Enabled, t.NotifyOnResult = true, false, true
				t.Model, t.Name = "opus", "Renamed"
				return t
			},
		},
		{
			name: "skip-permissions maps onto the permissions field",
			args: []string{"--skip-permissions=true"},
			want: func(t task.Task) task.Task { t.Permissions = task.PermissionsSkip; return t },
		},
		{
			name: "skip-permissions=false goes back to default",
			args: []string{"--skip-permissions=false"},
			want: func(t task.Task) task.Task { t.Permissions = task.PermissionsDefault; return t },
		},
		{
			name: "empty model clears the override",
			args: []string{"--model", ""},
			want: func(t task.Task) task.Task { t.Model = ""; return t },
		},
		{
			name: "empty name falls back to the id",
			args: []string{"--name", ""},
			want: func(t task.Task) task.Task { t.Name = t.ID; return t },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start := baseTask()
			start.Model = "sonnet"
			p, err := parseTaskPatch(tc.args, failingRead)
			if err != nil {
				t.Fatalf("parseTaskPatch: %v", err)
			}
			got, err := p.apply(start)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			want := tc.want(start)
			if got != want {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("result does not validate: %v", err)
			}
		})
	}
}

func TestParseTaskPatchErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flags", nil, "no changes given"},
		{"prompt and prompt-file", []string{"--prompt", "x", "--prompt-file", "p.md"}, "either --prompt or --prompt-file"},
		{"stray argument", []string{"--name", "x", "leftover"}, "unexpected argument"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTaskPatch(tc.args, failingRead)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestParseTaskPatchPromptFile(t *testing.T) {
	p, err := parseTaskPatch([]string{"--prompt-file", "brief.md"}, func(path string) ([]byte, error) {
		if path != "brief.md" {
			t.Fatalf("read path = %q", path)
		}
		return []byte("multi\nline\nbrief"), nil
	})
	if err != nil {
		t.Fatalf("parseTaskPatch: %v", err)
	}
	got, err := p.apply(baseTask())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got.Prompt != "multi\nline\nbrief" {
		t.Errorf("prompt = %q", got.Prompt)
	}
}

func TestTaskPatchApplyInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"bad time", []string{"--at", "tomorrow"}, "invalid --at time"},
		{"bad cron", []string{"--cron", "not a cron"}, "invalid cron"},
		{"unknown trigger", []string{"--trigger", "sometimes"}, "unknown trigger"},
		{"empty prompt", []string{"--prompt", ""}, "missing prompt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseTaskPatch(tc.args, failingRead)
			if err != nil {
				t.Fatalf("parseTaskPatch: %v", err)
			}
			got, err := p.apply(baseTask())
			if err == nil {
				err = got.Validate()
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestTaskDocRoundTrip(t *testing.T) {
	orig := baseTask()
	orig.Prompt = "Line one.\n\nA quote: \"like this\", a triple: \"\"\" and a backslash \\.\nDone.\n"
	orig.Model = "opus"
	orig.NotifyOnResult = true
	orig.Trigger = task.TriggerFixed
	orig.Cron = ""
	orig.FixedAt = time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC)

	data, err := encodeTaskDoc(orig)
	if err != nil {
		t.Fatalf("encodeTaskDoc: %v", err)
	}
	got, err := decodeTaskDoc(data, orig)
	if err != nil {
		t.Fatalf("decodeTaskDoc: %v", err)
	}
	if got.Prompt != orig.Prompt {
		t.Errorf("prompt round-trip:\ngot  %q\nwant %q", got.Prompt, orig.Prompt)
	}
	if !got.FixedAt.Equal(orig.FixedAt) {
		t.Errorf("fixed_at = %v, want %v", got.FixedAt, orig.FixedAt)
	}
	got.FixedAt = orig.FixedAt // compared above; times differ by location only
	if got != orig {
		t.Errorf("round trip:\ngot  %+v\nwant %+v", got, orig)
	}
	// The document is meant to be edited by hand: every setting must be visible.
	for _, field := range []string{"id", "name", "enabled", "working_dir", "trigger",
		"fixed_at", "cron", "parallel", "model", "permissions", "notify_on_result", "prompt"} {
		if !strings.Contains(string(data), "\n"+field+" ") {
			t.Errorf("document is missing the %q field:\n%s", field, data)
		}
	}
}

func TestDecodeTaskDoc(t *testing.T) {
	orig := baseTask()
	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			name:    "renamed id is rejected",
			doc:     "id = 'other'\nprompt = 'p'\nworking_dir = '/repo'\ntrigger = 'asap'\n",
			wantErr: "id cannot be changed",
		},
		{
			name:    "invalid toml",
			doc:     "id = 'nightly\n",
			wantErr: "parse task",
		},
		{
			name:    "invalid fixed_at",
			doc:     "id = 'nightly'\nprompt = 'p'\nworking_dir = '/repo'\ntrigger = 'fixed'\nfixed_at = 'soon'\n",
			wantErr: "invalid fixed_at",
		},
		{
			name:    "invalid cron",
			doc:     "id = 'nightly'\nprompt = 'p'\nworking_dir = '/repo'\ntrigger = 'cron'\ncron = 'nope'\n",
			wantErr: "invalid cron",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeTaskDoc([]byte(tc.doc), orig)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestDecodeTaskDocDefaults(t *testing.T) {
	orig := baseTask()
	got, err := decodeTaskDoc([]byte("id = 'nightly'\nprompt = 'p'\nworking_dir = '/repo'\ntrigger = 'asap'\n"), orig)
	if err != nil {
		t.Fatalf("decodeTaskDoc: %v", err)
	}
	if got.Permissions != task.PermissionsDefault {
		t.Errorf("permissions = %q, want %q", got.Permissions, task.PermissionsDefault)
	}
	if got.Name != "nightly" {
		t.Errorf("name = %q, want the id as fallback", got.Name)
	}
}

func TestSplitTaskID(t *testing.T) {
	id, rest, err := splitTaskID([]string{"nightly", "--prompt", "x"})
	if err != nil || id != "nightly" || len(rest) != 2 {
		t.Fatalf("got (%q, %v, %v)", id, rest, err)
	}
	if _, _, err := splitTaskID(nil); err == nil {
		t.Error("expected an error for a missing id")
	}
	if _, _, err := splitTaskID([]string{"--prompt", "x"}); err == nil {
		t.Error("expected an error when the id is missing before the flags")
	}
}

// TestCmdEditPersists covers the whole path: flags in, config.toml changed.
func TestCmdEditPersists(t *testing.T) {
	st := newTestStore(t)
	if err := app.AddTask(st, baseTask()); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := cmdEdit(st, []string{"nightly", "--prompt", "brand new", "--enabled=false"}); err != nil {
		t.Fatalf("cmdEdit: %v", err)
	}
	got, err := findTask(st, "nightly")
	if err != nil {
		t.Fatalf("findTask: %v", err)
	}
	if got.Prompt != "brand new" || got.Enabled {
		t.Errorf("task not updated: %+v", got)
	}
	if got.Cron != "0 3 * * *" || got.Trigger != task.TriggerCron {
		t.Errorf("untouched fields changed: %+v", got)
	}
	if err := cmdEdit(st, []string{"missing", "--prompt", "x"}); err == nil {
		t.Error("expected an error for an unknown task")
	}
	// An invalid edit must leave the stored task untouched.
	if err := cmdEdit(st, []string{"nightly", "--cron", "nope"}); err == nil {
		t.Error("expected an error for an invalid cron")
	}
	if again, _ := findTask(st, "nightly"); again.Cron != "0 3 * * *" {
		t.Errorf("failed edit changed the task: %+v", again)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	home := filepath.Join(t.TempDir(), "claudeq")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	st, err := store.Open(home)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	return st
}

func TestApplyEditedDoc(t *testing.T) {
	st := newTestStore(t)
	orig := baseTask()
	if err := app.AddTask(st, orig); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	before, err := encodeTaskDoc(orig)
	if err != nil {
		t.Fatalf("encodeTaskDoc: %v", err)
	}
	after := []byte(strings.Replace(string(before), "old prompt", "edited prompt", 1))
	if bytes.Equal(before, after) {
		t.Fatal("the test document was not modified")
	}

	if err := applyEditedDoc(st, orig, before, after); err != nil {
		t.Fatalf("applyEditedDoc: %v", err)
	}
	got, err := findTask(st, orig.ID)
	if err != nil {
		t.Fatalf("findTask: %v", err)
	}
	if got.Prompt != "edited prompt" {
		t.Errorf("prompt = %q", got.Prompt)
	}

	// The stored task has moved on since `before` was handed to the editor, so
	// the same write must now be refused instead of dropping that change.
	err = applyEditedDoc(st, orig, before, after)
	if err == nil || !strings.Contains(err.Error(), "changed while your editor was open") {
		t.Fatalf("got %v, want a concurrent-change error", err)
	}
}

func TestDecodeTaskDocMissingID(t *testing.T) {
	_, err := decodeTaskDoc([]byte("# only a comment\n"), baseTask())
	if err == nil || !strings.Contains(err.Error(), "no id line") {
		t.Fatalf("got %v, want a missing-id error", err)
	}
}
