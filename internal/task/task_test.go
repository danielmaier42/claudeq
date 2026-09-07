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

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Nightly sweep":                        "nightly-sweep",
		"  Ünïcode & Symbols!":                 "ncode-symbols",
		"":                                     "task",
		"---":                                  "task",
		"a_very_long_name_that_goes_on_and_on": "a-very-long-name-that-go",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckID(t *testing.T) {
	ok := []string{"nightly", "nightly-sweep-2", "q-20260907T100943-abc123", "a.b_c", "7up"}
	for _, id := range ok {
		if err := CheckID(id); err != nil {
			t.Errorf("CheckID(%q) = %v, want nil", id, err)
		}
	}
	bad := []string{"", "team/nightly", "..", ".hidden", "-lead", "with space", "ümlaut", "a?b", "a#b"}
	for _, id := range bad {
		if err := CheckID(id); !errors.Is(err, ErrInvalidTask) {
			t.Errorf("CheckID(%q) = %v, want ErrInvalidTask", id, err)
		}
	}
}

func TestPermissionsFor(t *testing.T) {
	if got := PermissionsFor(true); got != PermissionsSkip {
		t.Fatalf("PermissionsFor(true) = %q, want skip", got)
	}
	if got := PermissionsFor(false); got != PermissionsDefault {
		t.Fatalf("PermissionsFor(false) = %q, want default", got)
	}
}
