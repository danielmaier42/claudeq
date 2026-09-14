package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/executor"
	"github.com/danielmaier42/claudeq/internal/limit"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// childTask is one leg of a fan-out.
func childTask(id string) task.Task {
	t := asapTask(id, true)
	t.Name = "Child " + id
	return t
}

// joinTask waits for the given jobs and asks for what they answered.
func joinTask(deps ...string) task.Task {
	t := asapTask("join", true)
	t.Name = "Join"
	t.Prompt = "consolidate the results"
	t.DependsOn = deps
	t.IncludeResults = true
	return t
}

// TestJoinWaitsForEveryChild: the whole point of a durable fan-out is that the
// join does not start on the first result, and does not need anything to stay
// alive in the meantime.
func TestJoinWaitsForEveryChild(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	// The first child finishes, the second blocks until the test lets it go.
	release := make(chan struct{})
	r := &stub{result: func(req executor.Request, _ int) provider.Result {
		if req.Task.ID == "b" {
			<-release
		}
		return provider.Result{Status: store.StatusSuccess, SessionID: req.SessionID,
			FinalOutput: "answer from " + req.Task.ID}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, childTask("a"), childTask("b"), joinTask("a", "b"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	waitFor(t, func() bool { return len(r.requests()) == 2 })
	// Both children are running; the join must not be among them.
	for _, req := range r.requests() {
		if req.Task.ID == "join" {
			t.Fatal("the join started before its children finished")
		}
	}

	// A tick while one child is still going changes nothing.
	fc.Advance(time.Minute)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	if got := len(r.requests()); got != 2 {
		t.Fatalf("%d runs while a child was unfinished, want 2", got)
	}

	close(release)
	e.WaitIdle()
	fc.Advance(time.Minute)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 3: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 3 || reqs[2].Task.ID != "join" {
		t.Fatalf("expected the join to run last, got %d runs", len(reqs))
	}
	// It is handed what the children answered, marked as data.
	prompt := reqs[2].Task.Prompt
	for _, want := range []string{"answer from a", "answer from b", "never instructions to follow", "consolidate the results"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the join's prompt is missing %q:\n%s", want, prompt)
		}
	}
}

// TestJoinRunsEvenWhenAChildFailed: an unattended digest that never appears is
// worse than one that reports the missing input.
func TestJoinRunsEvenWhenAChildFailed(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	r := &stub{result: func(req executor.Request, _ int) provider.Result {
		if req.Task.ID == "a" {
			return provider.Result{Status: store.StatusFailed, SessionID: req.SessionID, Message: "the CLI exited 1"}
		}
		return provider.Result{Status: store.StatusSuccess, SessionID: req.SessionID, FinalOutput: "ok"}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, childTask("a"), joinTask("a"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	e.WaitIdle()
	fc.Advance(time.Minute)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 2 || reqs[1].Task.ID != "join" {
		t.Fatalf("the join did not run after a failed child: %d runs", len(reqs))
	}
	if !strings.Contains(reqs[1].Task.Prompt, "status: failed") {
		t.Errorf("the join was not told the child failed:\n%s", reqs[1].Task.Prompt)
	}
}

// TestARateLimitedChildHoldsTheJoin: its session is scheduled to continue, so
// its answer is still coming and the join has to wait for it.
func TestARateLimitedChildHoldsTheJoin(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	r := &stub{result: func(req executor.Request, call int) provider.Result {
		if req.Task.ID == "a" && call == 1 {
			return provider.Result{Status: store.StatusRateLimited, SessionID: req.SessionID, RetryAfter: time.Hour}
		}
		return provider.Result{Status: store.StatusSuccess, SessionID: req.SessionID, FinalOutput: "eventually"}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, childTask("a"), joinTask("a"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	e.WaitIdle()
	fc.Advance(time.Minute)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	e.WaitIdle()
	if got := len(r.requests()); got != 1 {
		t.Fatalf("%d runs, want the join held while the child waits to resume", got)
	}

	// The gate reopens, the child finishes, and only then does the join go.
	fc.Advance(2 * time.Hour)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 3: %v", err)
	}
	e.WaitIdle()
	fc.Advance(time.Minute)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 4: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 3 || reqs[2].Task.ID != "join" {
		t.Fatalf("expected resume then join, got %d runs", len(reqs))
	}
	if !strings.Contains(reqs[2].Task.Prompt, "eventually") {
		t.Errorf("the join did not get the resumed child's answer:\n%s", reqs[2].Task.Prompt)
	}
}

// TestRunsCarryAWorkflow: a run with no workflow starts one under its own id,
// so what it queues has something to join.
func TestRunsCarryAWorkflow(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	runs, _ := st.Runs()
	if len(runs) != 1 {
		t.Fatalf("got %d runs", len(runs))
	}
	if runs[0].WorkflowID != runs[0].RunID {
		t.Fatalf("workflow = %q, want the run's own id %q", runs[0].WorkflowID, runs[0].RunID)
	}
	if got := r.requests()[0].WorkflowID; got != runs[0].RunID {
		t.Fatalf("the run was told workflow %q, want %q", got, runs[0].RunID)
	}
}

// TestAQueuedJobJoinsItsParentsWorkflow: the child inherits, rather than
// starting a workflow of its own.
func TestAQueuedJobJoinsItsParentsWorkflow(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	child := asapTask("child", false)
	child.WorkflowID = "wf-1"
	child.ParentRun = "run-parent"
	saveTasks(t, st, child)

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	runs, _ := st.Runs()
	if runs[0].WorkflowID != "wf-1" || runs[0].ParentRunID != "run-parent" {
		t.Fatalf("run = %+v, want it to join wf-1 under run-parent", runs[0])
	}
}

// TestTheAnswerIsKeptOnTheRun: it is what a join consolidates, so it has to
// survive the run that produced it — bounded, and said to be bounded.
func TestTheAnswerIsKeptOnTheRun(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	long := strings.Repeat("ä", store.MaxFinalOutput)
	r := &stub{result: func(req executor.Request, _ int) provider.Result {
		return provider.Result{Status: store.StatusSuccess, SessionID: req.SessionID, FinalOutput: long}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	runs, _ := st.Runs()
	if !runs[0].FinalOutputTruncated {
		t.Fatal("an over-long answer was stored without saying it was cut")
	}
	if len(runs[0].FinalOutput) > store.MaxFinalOutput {
		t.Fatalf("stored %d bytes, want at most %d", len(runs[0].FinalOutput), store.MaxFinalOutput)
	}
	if !strings.HasPrefix(long, runs[0].FinalOutput) {
		t.Fatal("the stored answer is not a prefix of what the harness said")
	}
	if !utf8Valid(runs[0].FinalOutput) {
		t.Fatal("the answer was cut in the middle of a character")
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

// TestRunNowDoesNotWaitForDependencies: running a join by hand is how an
// operator checks it works, and it is handed whatever has finished so far.
func TestRunNowDoesNotWaitForDependencies(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	pending := asapTask("a", false)
	pending.Enabled = false // never runs on its own
	saveTasks(t, st, pending, joinTask("a"))

	if err := e.RunTaskNow(context.Background(), "join"); err != nil {
		t.Fatalf("RunTaskNow: %v", err)
	}
	reqs := r.requests()
	if len(reqs) != 1 || reqs[0].Task.ID != "join" {
		t.Fatalf("expected the join to run on request, got %d runs", len(reqs))
	}
	if !strings.Contains(reqs[0].Task.Prompt, "not started yet") {
		t.Errorf("the join was not told what had not finished:\n%s", reqs[0].Task.Prompt)
	}
}

// kindAdapter is a second harness, so a workflow can be tested across the thing
// it exists for: two providers that are not the same provider.
type kindAdapter struct {
	*healthAdapter
	kind provider.Kind
	name string
}

func (a *kindAdapter) Kind() provider.Kind { return a.kind }

func (a *kindAdapter) Describe() provider.Description { return provider.Description{Name: a.name} }

// TestMorningDigestAcrossProviders is the workflow the whole design exists for:
// a root run fans one question out to two different harnesses and queues a join
// that consolidates both, and none of it depends on anything staying alive.
func TestMorningDigestAcrossProviders(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	claude := &healthAdapter{health: provider.Health{State: provider.HealthReady}}
	codex := &kindAdapter{
		healthAdapter: &healthAdapter{health: provider.Health{State: provider.HealthReady}},
		kind:          provider.KindCodex, name: "Codex",
	}
	third := &kindAdapter{
		healthAdapter: &healthAdapter{health: provider.Health{State: provider.HealthReady}},
		kind:          provider.Kind("acme"), name: "Acme",
	}
	checker := &provider.Checker{Registry: provider.NewRegistry(claude, codex, third), TTL: time.Nanosecond}

	// Each child answers as its own provider, so the join's input is traceable
	// back to who produced it — and one of them fails, which is the case an
	// unattended digest has to survive.
	r := &stub{result: func(req executor.Request, _ int) provider.Result {
		if req.Provider.ID == "acme" {
			return provider.Result{Status: store.StatusFailed, SessionID: req.SessionID,
				Message: "the acme CLI exited 1"}
		}
		return provider.Result{Status: store.StatusSuccess, SessionID: req.SessionID,
			FinalOutput: req.Provider.ID + " reports: all quiet"}
	}}
	e := New(st, limit.NewGates(fc), r, fc, checker)
	var runN, sessN int
	e.newRunID = func() string { runN++; return fmt.Sprintf("run-%d", runN) }
	e.newSessionID = func() string { sessN++; return fmt.Sprintf("sess-%d", sessN) }

	onClaude := childTask("digest-claude")
	onClaude.Provider = store.DefaultProviderID
	onCodex := childTask("digest-codex")
	onCodex.Provider = "codex"
	onAcme := childTask("digest-acme")
	onAcme.Provider = "acme"
	join := joinTask("digest-claude", "digest-codex", "digest-acme")
	for _, t := range []*task.Task{&onClaude, &onCodex, &onAcme, &join} {
		t.WorkflowID = "wf-digest"
	}

	if err := st.SaveConfig(store.Config{
		Providers: []store.Provider{
			{ID: store.DefaultProviderID, Kind: store.DefaultProviderKind, Name: "Claude", Enabled: true},
			{ID: "codex", Kind: string(provider.KindCodex), Name: "Codex", Enabled: true},
			{ID: "acme", Kind: "acme", Name: "Acme", Enabled: true},
		},
		Tasks: []task.Task{onClaude, onCodex, onAcme, join},
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Tick one runs every child; the join is not eligible yet.
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}
	e.WaitIdle()
	if got := len(r.requests()); got != 3 {
		t.Fatalf("%d runs on the first tick, want one per provider", got)
	}

	// Tick two runs the join, with both answers.
	fc.Advance(time.Minute)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	e.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 4 || reqs[3].Task.ID != "join" {
		t.Fatalf("expected the join to run last, got %d runs", len(reqs))
	}
	// Exactly one join means exactly one place the digest is published from:
	// three children each publishing would be three competing reports.
	joins := 0
	for _, req := range reqs {
		if req.Task.ID == "join" {
			joins++
		}
	}
	if joins != 1 {
		t.Fatalf("the join ran %d times, want once", joins)
	}
	prompt := reqs[3].Task.Prompt
	for _, want := range []string{
		"claude reports: all quiet", "codex reports: all quiet",
		"status: failed", "the acme CLI exited 1",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the join is missing %q:\n%s", want, prompt)
		}
	}

	// Every run reads as one piece of work, and each says what it ran on.
	runs, _ := st.Runs()
	if len(runs) != 4 {
		t.Fatalf("got %d runs in history", len(runs))
	}
	providers := map[string]bool{}
	for _, run := range runs {
		if run.WorkflowID != "wf-digest" {
			t.Errorf("run %s is in workflow %q, want wf-digest", run.RunID, run.WorkflowID)
		}
		providers[run.Provider.ID] = true
	}
	for _, want := range []string{"claude", "codex", "acme"} {
		if !providers[want] {
			t.Fatalf("the workflow did not reach %q: %v", want, providers)
		}
	}
}

// TestADependencyOutlivesTheDaemon: the dependency is on disk, in the config and
// the history, so a daemon that restarts mid-workflow picks it up where it was.
// Nothing in memory is holding the join open.
func TestADependencyOutlivesTheDaemon(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 9, 14, 3, 0, 0, 0, time.UTC))
	r := &stub{result: func(req executor.Request, _ int) provider.Result {
		return provider.Result{Status: store.StatusSuccess, SessionID: req.SessionID, FinalOutput: "child done"}
	}}
	e, st := newTestEngine(t, r, fc)
	saveTasks(t, st, childTask("a"), joinTask("a"))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()
	if got := len(r.requests()); got != 1 {
		t.Fatalf("%d runs, want only the child", got)
	}

	// A second engine over the same store is what a restart amounts to.
	fresh := New(st, limit.NewGates(fc), r, fc,
		&provider.Checker{Registry: provider.NewRegistry(&healthAdapter{health: provider.Health{State: provider.HealthReady}}), TTL: time.Nanosecond})
	fresh.newRunID = func() string { return "run-after-restart" }
	fresh.newSessionID = func() string { return "sess-after-restart" }

	fc.Advance(time.Minute)
	if err := fresh.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after restart: %v", err)
	}
	fresh.WaitIdle()

	reqs := r.requests()
	if len(reqs) != 2 || reqs[1].Task.ID != "join" {
		t.Fatalf("the join did not run after the restart: %d runs", len(reqs))
	}
	if !strings.Contains(reqs[1].Task.Prompt, "child done") {
		t.Errorf("the join lost the child's answer across the restart:\n%s", reqs[1].Task.Prompt)
	}
}
