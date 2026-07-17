package engine

import (
	"context"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/clock"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

type fakeWaker struct{ scheduled []time.Time }

func (f *fakeWaker) Schedule(_ context.Context, at time.Time) error {
	f.scheduled = append(f.scheduled, at)
	return nil
}

func TestPlanWakeUsesEarliestFixedBeforeHeartbeat(t *testing.T) {
	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFake(now)
	r := &stub{}
	e, st := newTestEngine(t, r, fc)

	fixed := task.Task{
		ID: "f", Name: "f", Prompt: "p", WorkingDir: "/r",
		Trigger: task.TriggerFixed, FixedAt: now.Add(30 * time.Minute),
		Enabled: true, Permissions: task.PermissionsDefault,
	}
	if err := st.SaveConfig(store.Config{Tasks: []task.Task{fixed}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	w := &fakeWaker{}
	e.SetWaker(w)
	if err := e.planWake(context.Background()); err != nil {
		t.Fatalf("planWake: %v", err)
	}
	if len(w.scheduled) != 1 {
		t.Fatalf("expected 1 wake scheduled, got %d", len(w.scheduled))
	}
	if want := now.Add(30 * time.Minute); !w.scheduled[0].Equal(want) {
		t.Fatalf("scheduled %v, want %v (fixed start beats 60m heartbeat)", w.scheduled[0], want)
	}
}

func TestPlanWakeFallsBackToHeartbeat(t *testing.T) {
	now := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	fc := clock.NewFake(now)
	e, st := newTestEngine(t, &stub{}, fc)

	// An asap task has no concrete time; only the heartbeat applies.
	if err := st.SaveConfig(store.Config{Tasks: []task.Task{asapTask("a", false)}}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	w := &fakeWaker{}
	e.SetWaker(w)
	if err := e.planWake(context.Background()); err != nil {
		t.Fatalf("planWake: %v", err)
	}
	if want := now.Add(60 * time.Minute); len(w.scheduled) != 1 || !w.scheduled[0].Equal(want) {
		t.Fatalf("scheduled %v, want heartbeat %v", w.scheduled, want)
	}
}
