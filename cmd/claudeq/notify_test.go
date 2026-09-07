package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildNotification(t *testing.T) {
	now := time.Date(2026, 9, 7, 8, 15, 0, 0, time.UTC)
	src := runSource{taskID: "watch", taskName: "Prod watch", runID: "20260907T081500-abcd"}

	got, err := buildNotification("  Prod drifted ", "3 commits behind\n", " https://example.com/deploys?env=prod ", src, now)
	if err != nil {
		t.Fatalf("buildNotification: %v", err)
	}
	if got.Title != "Prod drifted" || got.Message != "3 commits behind" || got.URL != "https://example.com/deploys?env=prod" {
		t.Fatalf("fields not trimmed/kept: %+v", got)
	}
	if got.TaskID != "watch" || got.TaskName != "Prod watch" || got.RunID != src.runID {
		t.Fatalf("attribution lost: %+v", got)
	}
	if !got.QueuedAt.Equal(now) || !strings.HasPrefix(got.ID, "n-20260907T081500-") {
		t.Fatalf("id/time = %q/%v", got.ID, got.QueuedAt)
	}

	// Standalone (no run in the environment): no attribution, still valid.
	plain, err := buildNotification("T", "M", "", runSource{}, now)
	if err != nil || plain.TaskID != "" || plain.TaskName != "" || plain.RunID != "" || plain.URL != "" {
		t.Fatalf("standalone notification = %+v, err %v", plain, err)
	}
}

func TestBuildNotificationRejectsBadInput(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name                 string
		title, message, link string
		wantErr              string
	}{
		{"missing title", "", "M", "", "--title is required"},
		{"blank title", "   ", "M", "", "--title is required"},
		{"missing message", "T", "", "", "--message is required"},
		{"relative url", "T", "M", "status/page", "http or https"},
		{"file url", "T", "M", "file:///etc/passwd", "http or https"},
		{"custom scheme", "T", "M", "x-apple.systempreferences:com.apple.preference", "http or https"},
		{"scheme without host", "T", "M", "https://", "http or https"},
		{"unparseable", "T", "M", "http://exa mple.com/%zz", "invalid --url"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := buildNotification(c.title, c.message, c.link, runSource{}, now)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want it to mention %q", err, c.wantErr)
			}
		})
	}
}

func TestCallingRunReadsTheDaemonEnvironment(t *testing.T) {
	t.Setenv("CLAUDEQ_TASK_ID", "watch")
	t.Setenv("CLAUDEQ_RUN_ID", "r-1")
	t.Setenv("CLAUDEQ_PARENT_TASK", `{"id":"watch","name":"Prod watch"}`)
	if got := callingRun(); got != (runSource{taskID: "watch", taskName: "Prod watch", runID: "r-1"}) {
		t.Fatalf("callingRun = %+v", got)
	}

	// Without the parent JSON the name falls back to the id.
	t.Setenv("CLAUDEQ_PARENT_TASK", "")
	if got := callingRun(); got.taskName != "watch" {
		t.Fatalf("name fallback = %+v", got)
	}

	// Outside a run everything is empty.
	t.Setenv("CLAUDEQ_TASK_ID", "")
	t.Setenv("CLAUDEQ_RUN_ID", "")
	if got := callingRun(); got != (runSource{}) {
		t.Fatalf("standalone = %+v", got)
	}
}
