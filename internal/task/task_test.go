package task

import (
	"errors"
	"testing"
	"time"
)

func valid() Task {
	return Task{
		ID:          "t1",
		Name:        "example",
		Prompt:      "do the thing",
		WorkingDir:  "/repo",
		Trigger:     TriggerASAP,
		Enabled:     true,
		Permissions: PermissionsDefault,
	}
}

func TestValidateAcceptsValidTasks(t *testing.T) {
	cases := map[string]func(Task) Task{
		"asap":  func(t Task) Task { t.Trigger = TriggerASAP; return t },
		"fixed": func(t Task) Task { t.Trigger = TriggerFixed; t.FixedAt = time.Now(); return t },
		"cron":  func(t Task) Task { t.Trigger = TriggerCron; t.Cron = "0 20 * * *"; return t },
		"skip-perms": func(t Task) Task {
			t.Permissions = PermissionsSkip
			return t
		},
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mod(valid()).Validate(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateRejectsBadTasks(t *testing.T) {
	cases := map[string]func(Task) Task{
		"no id":          func(t Task) Task { t.ID = ""; return t },
		"no prompt":      func(t Task) Task { t.Prompt = ""; return t },
		"no working dir": func(t Task) Task { t.WorkingDir = ""; return t },
		"fixed w/o time": func(t Task) Task { t.Trigger = TriggerFixed; return t },
		"cron w/o expr":  func(t Task) Task { t.Trigger = TriggerCron; return t },
		"bad cron":       func(t Task) Task { t.Trigger = TriggerCron; t.Cron = "not a cron"; return t },
		"unknown trig":   func(t Task) Task { t.Trigger = "weekly"; return t },
		"unknown perms":  func(t Task) Task { t.Permissions = "yolo"; return t },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			err := mod(valid()).Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !errors.Is(err, ErrInvalidTask) {
				t.Fatalf("error %v is not ErrInvalidTask", err)
			}
		})
	}
}

func TestCronScheduleNext(t *testing.T) {
	task := valid()
	task.Trigger = TriggerCron
	task.Cron = "0 20 * * *" // daily at 20:00

	sched, err := task.CronSchedule()
	if err != nil {
		t.Fatalf("CronSchedule: %v", err)
	}

	from := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	next := sched.Next(from)
	want := time.Date(2026, 7, 17, 20, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("Next(%v) = %v, want %v", from, next, want)
	}
}
