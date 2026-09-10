package task

import (
	"strings"
	"testing"
	"time"
)

func TestCheckCronAccepts(t *testing.T) {
	for _, expr := range []string{
		"0 20 * * *",
		"*/15 * * * *",
		"30 4 * * 1",
		"0 9-17 * * 1-5",
		"0 0 1,15 * *",
		"0 6 * JAN-MAR MON",
		"  0 20 * * *  ",              // surrounding whitespace is not the user's mistake
		"TZ=Europe/Berlin 0 20 * * *", // the parser reads a zone ahead of the five fields
		"CRON_TZ=UTC 0 20 * * *",
	} {
		if err := CheckCron(expr); err != nil {
			t.Errorf("CheckCron(%q) = %v, want nil", expr, err)
		}
	}
}

func TestCheckCronRejects(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want []string // substrings the message must contain
	}{
		{"empty", "", []string{"missing cron schedule", "0 20 * * *"}},
		{"blank", "   ", []string{"missing cron schedule"}},
		{"shorthand", "@daily", []string{"invalid cron", "not supported", "five fields"}},
		{"too few fields", "0 20 * *", []string{"exactly 5 fields", "found 4"}},
		{"too many fields", "0 20 * * * *", []string{"exactly 5 fields", "found 6"}},
		{"hour out of range", "0 99 * * *", []string{"hour", `"99"`, "0-23"}},
		{"minute out of range", "60 * * * *", []string{"minute", `"60"`, "0-59"}},
		{"day out of range", "0 0 32 * *", []string{"day-of-month", `"32"`, "1-31"}},
		{"month out of range", "0 0 * 13 *", []string{"month", `"13"`, "1-12"}},
		{"weekday out of range", "0 0 * * 9", []string{"weekday", `"9"`, "0-6"}},
		{"not a number", "x * * * *", []string{"minute", `"x"`}},
		{"zero step", "*/0 * * * *", []string{"minute", `"*/0"`}},
		{"garbage", "not a cron", []string{"invalid cron"}},
		{"unknown zone", "TZ=Nowhere 0 20 * * *", []string{"invalid cron"}},
		{"zone without a schedule", "TZ=UTC 0 20 * *", []string{"exactly 5 fields", "found 4"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckCron(tc.expr)
			if err == nil {
				t.Fatalf("CheckCron(%q) = nil, want an error", tc.expr)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CheckCron(%q) = %q, want it to mention %q", tc.expr, err, want)
				}
			}
		})
	}
}

// A rejected field is named, not just the whole line: the message must not
// blame a field that is perfectly fine.
func TestCheckCronNamesOnlyTheBadField(t *testing.T) {
	err := CheckCron("0 99 * * *")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "minute") {
		t.Errorf("message blames the minute field: %q", err)
	}
}

func TestCronNext(t *testing.T) {
	from := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	got, err := CronNext("0 20 * * *", from, 3)
	if err != nil {
		t.Fatalf("CronNext: %v", err)
	}
	want := []time.Time{
		time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 2, 20, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 3, 20, 0, 0, 0, time.UTC),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d occurrences, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("occurrence %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestCronNextRejectsBadExpression(t *testing.T) {
	if _, err := CronNext("0 99 * * *", time.Now(), 3); err == nil {
		t.Fatal("want an error for an invalid expression")
	}
}

// A schedule that never comes round again yields what exists, not a hang.
func TestCronNextStopsWhenNoOccurrenceFollows(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	got, err := CronNext("0 0 30 2 *", from, 3) // 30 February
	if err != nil {
		t.Fatalf("CronNext: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no occurrence", got)
	}
}
