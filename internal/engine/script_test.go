package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// scriptJob is a job the queue runs as a program rather than as a prompt.
func scriptJob(id string) task.Task {
	t := asapTask(id, false)
	t.Kind = task.KindScript
	t.Prompt = "echo " + id
	return t
}

func TestScriptJobRunsWhileTheProviderIsRateLimited(t *testing.T) {
	// The point of a script watcher: the allowance is gone, every agent job is
	// waiting for the gate — and the watcher keeps ticking, so the work it finds
	// is already queued when the limit reopens.
	now := time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC)
	fc := clock.NewFake(now)
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	e.gates.For(store.DefaultProviderID).BlockFor(2 * time.Hour)
	saveTasks(t, st, asapTask("agent", false), scriptJob("watch"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected only the script job to run, got %d runs", len(reqs))
	}
	if reqs[0].Task.ID != "watch" {
		t.Fatalf("ran %q, want the script job", reqs[0].Task.ID)
	}
}

func TestScriptJobRunsWhileTheProviderIsUnready(t *testing.T) {
	// No harness installed, nobody logged in: a script job needs none of it.
	fc := clock.NewFake(time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st, ad := newTestEngineWithProvider(t, r, fc)
	ad.set(provider.Health{State: provider.HealthNotInstalled, Reason: "no CLI here"})
	// The second script job is switched off, so it is still in the queue after
	// the tick and "run now" has something to act on.
	manual := scriptJob("manual")
	manual.Enabled = false
	saveTasks(t, st, asapTask("agent", false), scriptJob("watch"), manual)

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 1 || reqs[0].Task.ID != "watch" {
		t.Fatalf("expected only the script job to run, got %+v", reqs)
	}
	// The manual run of an agent job would be refused with the provider's
	// reason; a script job has no provider to be refused by.
	if err := e.RunTaskNow(context.Background(), "manual"); err != nil {
		t.Fatalf("RunTaskNow on a script job: %v", err)
	}
	if err := e.RunTaskNow(context.Background(), "agent"); err == nil {
		t.Fatal("the agent job must be refused while its provider cannot run")
	}
}

func TestScriptRunRecordsNoProviderAndNoSession(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, scriptJob("watch"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(reqs))
	}
	if reqs[0].SessionID != "" || reqs[0].Resume {
		t.Fatalf("script run got session %q (resume %v), want none", reqs[0].SessionID, reqs[0].Resume)
	}
	if reqs[0].Provider.ID != "" {
		t.Fatalf("script run got provider %q, want none", reqs[0].Provider.ID)
	}

	runs, _ := st.Runs()
	if len(runs) != 1 {
		t.Fatalf("expected 1 run in history, got %d", len(runs))
	}
	if got := runs[0].Provider; !got.IsScript() || got.Name != "Script" {
		t.Fatalf("recorded provider = %+v, want the script identity", got)
	}
	if runs[0].Provider.ID != "" || runs[0].Provider.Model != "" {
		t.Fatalf("recorded provider = %+v, want no account and no model", runs[0].Provider)
	}
}

func TestScriptJobReadsDependencyResultsOnStdin(t *testing.T) {
	// A join written as a script gets the same digest an agent job gets, only
	// where a program can read it: prepending it to the script would break it.
	fc := clock.NewFake(time.Date(2026, 9, 22, 2, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)

	join := scriptJob("join")
	join.DependsOn = []string{"first"}
	join.IncludeResults = true
	saveTasks(t, st, join)

	// The job it waited for has finished and left the queue, the way a one-shot
	// does; its answer lives in history.
	finished := fc.Now()
	if err := st.AppendRun(store.Run{
		RunID: "r1", TaskID: "first", TaskName: "first",
		Status: store.StatusSuccess, FinalOutput: "the answer from first",
		StartedAt: fc.Now(), FinishedAt: &finished,
	}); err != nil {
		t.Fatalf("AppendRun: %v", err)
	}

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	var joined *executorRequest
	for _, req := range r.requests() {
		if req.Task.ID == "join" {
			joined = &executorRequest{stdin: req.Stdin, prompt: req.Task.Prompt}
		}
	}
	if joined == nil {
		t.Fatal("the script join did not run")
	}
	if joined.prompt != "echo join" {
		t.Fatalf("the script was rewritten to %q, want it untouched", joined.prompt)
	}
	if !strings.Contains(joined.stdin, "the answer from first") {
		t.Fatalf("stdin = %q, want the dependency digest", joined.stdin)
	}
}

// executorRequest is the part of a run request this file asserts on.
type executorRequest struct {
	stdin  string
	prompt string
}
