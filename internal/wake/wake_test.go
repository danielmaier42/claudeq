package wake

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNextWakeTimePicksEarliestFutureCandidate(t *testing.T) {
	now := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	cands := []time.Time{
		now.Add(-time.Hour), // past, ignored
		now.Add(3 * time.Hour),
		now.Add(90 * time.Minute),
	}
	got, ok := NextWakeTime(now, cands, time.Hour)
	if !ok {
		t.Fatal("expected a wake time")
	}
	// Heartbeat is now+1h; earliest future candidate is now+90m; heartbeat wins.
	if want := now.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNextWakeTimeCandidateBeatsHeartbeat(t *testing.T) {
	now := time.Date(2026, 7, 17, 22, 0, 0, 0, time.UTC)
	got, ok := NextWakeTime(now, []time.Time{now.Add(10 * time.Minute)}, time.Hour)
	if !ok || !got.Equal(now.Add(10*time.Minute)) {
		t.Fatalf("got %v ok=%v, want %v", got, ok, now.Add(10*time.Minute))
	}
}

func TestNextWakeTimeNothingToSchedule(t *testing.T) {
	now := time.Now()
	if _, ok := NextWakeTime(now, nil, 0); ok {
		t.Fatal("expected no wake when no candidates and no heartbeat")
	}
}

// recordRunner captures commands for assertions.
type recordRunner struct {
	calls [][]string
	err   error
}

func (r *recordRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil, r.err
}

func TestScheduleCancelsPreviousAndUsesSudo(t *testing.T) {
	r := &recordRunner{}
	s := &Scheduler{Runner: r, Sudo: true}
	ctx := context.Background()

	t1 := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 18, 7, 0, 0, 0, time.UTC)
	if err := s.Schedule(ctx, t1); err != nil {
		t.Fatalf("Schedule t1: %v", err)
	}
	if err := s.Schedule(ctx, t2); err != nil {
		t.Fatalf("Schedule t2: %v", err)
	}

	if len(r.calls) != 3 {
		t.Fatalf("expected 3 pmset calls (wake, cancel, wake), got %d: %v", len(r.calls), r.calls)
	}
	joined := strings.Join(r.calls[0], " ")
	if !strings.HasPrefix(joined, "sudo -n pmset schedule wake ") {
		t.Fatalf("first call not a sudo pmset wake: %q", joined)
	}
	if got := strings.Join(r.calls[1], " "); !strings.Contains(got, "schedule cancel wake") {
		t.Fatalf("second call should cancel the previous wake: %q", got)
	}
}

func TestScheduleSameTimeIsNoop(t *testing.T) {
	r := &recordRunner{}
	s := &Scheduler{Runner: r}
	ctx := context.Background()
	at := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	_ = s.Schedule(ctx, at)
	_ = s.Schedule(ctx, at)
	if len(r.calls) != 1 {
		t.Fatalf("scheduling the same time twice should be a no-op, got %d calls", len(r.calls))
	}
}

func TestScheduleWithinToleranceIsNoop(t *testing.T) {
	r := &recordRunner{}
	s := &Scheduler{Runner: r, MinReschedule: time.Minute}
	ctx := context.Background()
	at := time.Date(2026, 7, 18, 6, 0, 0, 0, time.UTC)
	_ = s.Schedule(ctx, at)
	_ = s.Schedule(ctx, at.Add(30*time.Second)) // within tolerance -> no-op
	if len(r.calls) != 1 {
		t.Fatalf("near-identical wake should not reschedule, got %d calls", len(r.calls))
	}
	_ = s.Schedule(ctx, at.Add(2*time.Minute)) // beyond tolerance -> cancel + schedule
	if len(r.calls) != 3 {
		t.Fatalf("wake beyond tolerance should reschedule, got %d calls", len(r.calls))
	}
}

func TestScheduleReturnsErrorFromRunner(t *testing.T) {
	r := &recordRunner{err: errors.New("no sudo")}
	s := &Scheduler{Runner: r}
	if err := s.Schedule(context.Background(), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected error to propagate from the runner")
	}
}
