package main

import (
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/task"
)

func TestEditTurnsAJobIntoAScript(t *testing.T) {
	st := newTestStore(t)
	agent := baseTask()
	agent.Provider = "claude"
	agent.Model = "opus"
	agent.ReasoningEffort = "high"
	agent.Permissions = task.PermissionsSkip
	if err := app.AddTask(st, agent); err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	// Switching the kind drops what only a model has, rather than refusing the
	// edit over settings the job no longer uses.
	if err := cmdEdit(st, []string{"nightly", "--kind", "script", "--prompt", "#!/bin/sh\necho hi"}); err != nil {
		t.Fatalf("cmdEdit: %v", err)
	}
	got, err := findTask(st, "nightly")
	if err != nil {
		t.Fatalf("findTask: %v", err)
	}
	if !got.IsScript() {
		t.Fatalf("task kind = %q, want script", got.Kind)
	}
	if got.Provider != "" || got.Model != "" || got.ReasoningEffort != "" || got.Permissions != task.PermissionsDefault {
		t.Fatalf("model settings survived the switch: %+v", got)
	}
	if got.Cron != "0 3 * * *" {
		t.Fatalf("schedule changed: %+v", got)
	}

	// And back: an agent job is the default, stored as no kind at all.
	if err := cmdEdit(st, []string{"nightly", "--kind", "agent"}); err != nil {
		t.Fatalf("cmdEdit back: %v", err)
	}
	if again, _ := findTask(st, "nightly"); again.Kind != "" {
		t.Fatalf("kind = %q, want the default stored as absent", again.Kind)
	}
}

func TestEditRefusesAProviderOnAScript(t *testing.T) {
	st := newTestStore(t)
	s := baseTask()
	s.Kind = task.KindScript
	if err := app.AddTask(st, s); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	err := cmdEdit(st, []string{"nightly", "--provider", "claude"})
	if err == nil || !strings.Contains(err.Error(), "script job") {
		t.Fatalf("error = %v, want it to say a script job has no provider", err)
	}
}

func TestQueuedJobIsAnAgentJobUnlessAsked(t *testing.T) {
	// The watcher rebuild depends on this: a script queues the work that needs a
	// model, and must not hand its own kind down.
	parent := task.Task{
		ID: "watch", Name: "watch", Kind: task.KindScript, Prompt: "echo hi",
		WorkingDir: "/repo", Trigger: task.TriggerCron, Cron: "*/5 * * * *",
		Enabled: true, Permissions: task.PermissionsDefault, QuietHistory: true,
	}
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

	o, err := parseQueueOpts([]string{"--prompt", "review the new item"})
	if err != nil {
		t.Fatalf("parseQueueOpts: %v", err)
	}
	got, err := buildQueuedTask(parentJSON(t, parent), "q-1", o, now)
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
	}
	if got.IsScript() {
		t.Fatalf("queued job = %+v, want an agent job", got)
	}
	if got.WorkingDir != "/repo" {
		t.Fatalf("working dir = %q, want the script's own", got.WorkingDir)
	}

	// A script that wants another script still says so.
	o, err = parseQueueOpts([]string{"--prompt", "echo again", "--kind", "script"})
	if err != nil {
		t.Fatalf("parseQueueOpts: %v", err)
	}
	got, err = buildQueuedTask(parentJSON(t, parent), "q-2", o, now)
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
	}
	if !got.IsScript() {
		t.Fatalf("queued job = %+v, want a script job", got)
	}
}

func TestQueuedScriptDropsInheritedModelSettings(t *testing.T) {
	parent := task.Task{
		ID: "nightly", Name: "nightly", Prompt: "do", WorkingDir: "/repo",
		Trigger: task.TriggerASAP, Enabled: true, Provider: "claude", Model: "opus",
		ReasoningEffort: "high", Permissions: task.PermissionsSkip,
	}
	o, err := parseQueueOpts([]string{"--prompt", "echo hi", "--kind", "script"})
	if err != nil {
		t.Fatalf("parseQueueOpts: %v", err)
	}
	got, err := buildQueuedTask(parentJSON(t, parent), "q-1", o, time.Now())
	if err != nil {
		t.Fatalf("buildQueuedTask: %v", err)
	}
	if got.Provider != "" || got.Model != "" || got.ReasoningEffort != "" || got.Permissions != task.PermissionsDefault {
		t.Fatalf("a queued script inherited model settings: %+v", got)
	}
}

func TestTaskDocCarriesTheKind(t *testing.T) {
	s := baseTask()
	s.Kind = task.KindScript
	s.Permissions = task.PermissionsDefault
	doc, err := encodeTaskDoc(s)
	if err != nil {
		t.Fatalf("encodeTaskDoc: %v", err)
	}
	if !strings.Contains(string(doc), `kind = 'script'`) && !strings.Contains(string(doc), `kind = "script"`) {
		t.Fatalf("document does not carry the kind:\n%s", doc)
	}
	back, err := decodeTaskDoc(doc, s)
	if err != nil {
		t.Fatalf("decodeTaskDoc: %v", err)
	}
	if !back.IsScript() {
		t.Fatalf("round trip lost the kind: %+v", back)
	}
}
