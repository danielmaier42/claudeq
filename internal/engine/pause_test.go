package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// savePaused stores tasks with the global pause switch on.
func savePaused(t *testing.T, st *store.Store, tasks ...task.Task) {
	t.Helper()
	if err := st.SaveConfig(store.Config{
		Settings: store.Settings{Paused: true},
		Tasks:    tasks,
	}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
}

func TestPausedTickStartsNothing(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	savePaused(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	e.WaitIdle()

	if got := len(r.requests()); got != 0 {
		t.Fatalf("paused queue started %d run(s)", got)
	}
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Fatalf("paused queue recorded history: %+v", runs)
	}
	// Nothing may be marked as started either, so the task is still due the
	// moment the switch goes off.
	state, _ := st.LoadState()
	if _, ok := state.LastStart("a"); ok {
		t.Fatal("paused tick recorded a start")
	}
}

func TestUnpausingRunsTheTaskThatCameDueWhilePaused(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	savePaused(t, st, asapTask("a", false))

	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick paused: %v", err)
	}
	e.WaitIdle()

	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Settings.Paused = false
		return nil
	}); err != nil {
		t.Fatalf("unpause: %v", err)
	}
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("Tick resumed: %v", err)
	}
	e.WaitIdle()

	if got := len(r.requests()); got != 1 {
		t.Fatalf("expected 1 run after resuming, got %d", got)
	}
}

func TestPausedRunTaskNowIsRefused(t *testing.T) {
	fc := clock.NewFake(time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC))
	r := &stub{}
	e, st := newTestEngine(t, r, fc)
	savePaused(t, st, asapTask("a", false))

	err := e.RunTaskNow(context.Background(), "a")
	if !errors.Is(err, store.ErrPaused) {
		t.Fatalf("RunTaskNow error = %v, want store.ErrPaused", err)
	}
	if got := len(r.requests()); got != 0 {
		t.Fatalf("refused run-now still started %d run(s)", got)
	}
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Fatalf("refused run-now recorded history: %+v", runs)
	}
}

func TestPausedPlanWakeIgnoresTasks(t *testing.T) {
	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFake(now)
	e, st := newTestEngine(t, &stub{}, fc)

	fixed := task.Task{
		ID: "f", Name: "f", Prompt: "p", WorkingDir: "/r",
		Trigger: task.TriggerFixed, FixedAt: now.Add(30 * time.Minute),
		Enabled: true, Permissions: task.PermissionsDefault,
	}
	savePaused(t, st, fixed)

	w := &fakeWaker{}
	e.SetWaker(w)
	if err := e.planWake(context.Background()); err != nil {
		t.Fatalf("planWake: %v", err)
	}
	if len(w.scheduled) != 1 {
		t.Fatalf("expected 1 wake scheduled, got %d", len(w.scheduled))
	}
	// Only the heartbeat is left: waking for a task that cannot run is pointless.
	if want := now.Add(store.DefaultHeartbeatMinutes * time.Minute); !w.scheduled[0].Equal(want) {
		t.Fatalf("scheduled %v, want the %v heartbeat", w.scheduled[0], want)
	}
}
