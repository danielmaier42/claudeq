package schedule

import (
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

func base(id string, tr task.Trigger) task.Task {
	return task.Task{
		ID: id, Name: id, Prompt: "p", WorkingDir: "/r",
		Trigger: tr, Enabled: true, Permissions: task.PermissionsDefault,
	}
}

func TestDueASAP(t *testing.T) {
	now := time.Now()
	tk := base("a", task.TriggerASAP)

	if due, _ := Due(tk, Inputs{Now: now}); !due {
		t.Fatal("fresh asap task should be due")
	}
	if due, _ := Due(tk, Inputs{Now: now, CompletedOnce: true}); due {
		t.Fatal("completed asap task should not be due")
	}
	if due, _ := Due(tk, Inputs{Now: now, Running: true}); due {
		t.Fatal("running task should not be due again")
	}
	tk.Enabled = false
	if due, _ := Due(tk, Inputs{Now: now}); due {
		t.Fatal("disabled task should not be due")
	}
}

func TestDueFixed(t *testing.T) {
	at := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	tk := base("f", task.TriggerFixed)
	tk.FixedAt = at

	if due, _ := Due(tk, Inputs{Now: at.Add(-time.Minute)}); due {
		t.Fatal("fixed task should not be due before its time")
	}
	if due, _ := Due(tk, Inputs{Now: at}); !due {
		t.Fatal("fixed task should be due at its time")
	}
	if due, _ := Due(tk, Inputs{Now: at.Add(time.Hour), CompletedOnce: true}); due {
		t.Fatal("completed fixed task should not re-run")
	}
}

func TestDueCron(t *testing.T) {
	tk := base("c", task.TriggerCron)
	tk.Cron = "0 20 * * *" // daily 20:00
	anchor := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)

	// Before 20:00, next occurrence after anchor is 20:00 today -> not due.
	if due, _ := Due(tk, Inputs{Now: time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC), CronAnchor: anchor}); due {
		t.Fatal("cron should not be due before its occurrence")
	}
	// At 20:00 -> due.
	if due, _ := Due(tk, Inputs{Now: time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC), CronAnchor: anchor}); !due {
		t.Fatal("cron should be due at its occurrence")
	}
	// Anchor already past today's occurrence -> next is tomorrow -> not due now.
	justRan := time.Date(2026, 7, 17, 20, 0, 1, 0, time.UTC)
	if due, _ := Due(tk, Inputs{Now: time.Date(2026, 7, 17, 21, 0, 0, 0, time.UTC), CronAnchor: justRan}); due {
		t.Fatal("cron should wait for the next occurrence after the last run")
	}
}

func TestSelectExclusiveRunsAlone(t *testing.T) {
	due := []task.Task{base("hi", task.TriggerASAP)} // non-parallel by default
	got := Select(due, Running{})
	if len(got) != 1 || got[0].ID != "hi" {
		t.Fatalf("expected [hi], got %v", ids(got))
	}
}

func TestSelectNothingWhenExclusiveRunning(t *testing.T) {
	due := []task.Task{base("a", task.TriggerASAP)}
	if got := Select(due, Running{NonParallel: true}); got != nil {
		t.Fatalf("expected nothing while exclusive running, got %v", ids(got))
	}
}

func TestSelectBatchesParallelInPriorityOrder(t *testing.T) {
	p1 := base("p1", task.TriggerASAP)
	p1.Parallel = true
	p2 := base("p2", task.TriggerASAP)
	p2.Parallel = true
	excl := base("e", task.TriggerASAP)

	// [p1, p2, e]: both parallels start; the exclusive waits behind them.
	got := Select([]task.Task{p1, p2, excl}, Running{})
	if want := []string{"p1", "p2"}; !equal(ids(got), want) {
		t.Fatalf("got %v, want %v", ids(got), want)
	}
}

func TestSelectExclusiveFirstBlocksLowerParallel(t *testing.T) {
	excl := base("e", task.TriggerASAP) // highest priority, non-parallel
	p1 := base("p1", task.TriggerASAP)
	p1.Parallel = true

	// [e, p1]: e must run alone; p1 waits despite being parallel.
	got := Select([]task.Task{excl, p1}, Running{})
	if want := []string{"e"}; !equal(ids(got), want) {
		t.Fatalf("got %v, want %v", ids(got), want)
	}
}

func TestSelectAddsParallelWhileParallelRunning(t *testing.T) {
	p := base("p", task.TriggerASAP)
	p.Parallel = true
	excl := base("e", task.TriggerASAP)

	got := Select([]task.Task{p, excl}, Running{Parallel: true})
	if want := []string{"p"}; !equal(ids(got), want) {
		t.Fatalf("got %v, want %v (exclusive must wait for parallels to drain)", ids(got), want)
	}
}

func ids(ts []task.Task) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
