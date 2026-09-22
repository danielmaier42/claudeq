package provider

import (
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

// TestCollectorClassifiesEveryOutcome pins the precedence rules that decide a
// run's status. They are provider-neutral: whichever harness produced the
// events, the same combination yields the same outcome.
func TestCollectorClassifiesEveryOutcome(t *testing.T) {
	reset := time.Unix(1784655600, 0)

	tests := []struct {
		name        string
		events      []Event
		exitCode    int
		wantStatus  store.RunStatus
		wantMessage string
	}{
		{
			name:       "completed cleanly",
			events:     []Event{{Type: EventCompleted, FinalOutput: "done"}},
			wantStatus: store.StatusSuccess,
		},
		{
			name:        "no terminal result at all",
			exitCode:    1,
			wantStatus:  store.StatusFailed,
			wantMessage: "run failed (exit 1)",
		},
		{
			name:        "a clean result but a non-zero exit is not a success",
			events:      []Event{{Type: EventCompleted}},
			exitCode:    3,
			wantStatus:  store.StatusFailed,
			wantMessage: "run failed (exit 3)",
		},
		{
			name:        "terminated by a signal",
			exitCode:    -1,
			wantStatus:  store.StatusFailed,
			wantMessage: "run was interrupted before completing (the process was terminated — e.g. the daemon stopped)",
		},
		{
			name:        "rate limited without a result",
			events:      []Event{{Type: EventRateLimited, ResetAt: reset}},
			exitCode:    1,
			wantStatus:  store.StatusRateLimited,
			wantMessage: "rate limit hit; waiting for reset",
		},
		{
			name: "rate limited with a failed result still waits rather than failing",
			events: []Event{
				{Type: EventRateLimited, RetryAfter: 5 * time.Second},
				{Type: EventCompleted, IsError: true, FinalOutput: "aborted"},
			},
			exitCode:   1,
			wantStatus: store.StatusRateLimited,
		},
		{
			name: "a pause that named its reason says that instead of the generic limit",
			events: []Event{
				{Type: EventRateLimited, RetryAfter: time.Hour, Detail: "subscription access is switched off for this organisation"},
				{Type: EventCompleted, IsError: true},
			},
			exitCode:    1,
			wantStatus:  store.StatusRateLimited,
			wantMessage: "subscription access is switched off for this organisation",
		},
		{
			name: "a warning that did not stop the run keeps the success",
			events: []Event{
				{Type: EventCompleted, FinalOutput: "OK"},
			},
			wantStatus: store.StatusSuccess,
		},
		{
			name: "authentication outranks a rate limit",
			events: []Event{
				{Type: EventRateLimited},
				{Type: EventAuthFailed},
				{Type: EventCompleted, IsError: true},
			},
			exitCode:    1,
			wantStatus:  store.StatusAuthError,
			wantMessage: "Fake Provider reported an authentication problem",
		},
		{
			name:        "an authentication reason from the harness is kept",
			events:      []Event{{Type: EventAuthFailed, Detail: "token expired"}},
			exitCode:    1,
			wantStatus:  store.StatusAuthError,
			wantMessage: "token expired",
		},
		{
			name:        "a rejected model is a failure that names the model",
			events:      []Event{{Type: EventModelRejected, Detail: `unknown model "gpt-9"`}},
			exitCode:    1,
			wantStatus:  store.StatusFailed,
			wantMessage: `unknown model "gpt-9"`,
		},
		{
			name:        "a rejected model without a reason still says what happened",
			events:      []Event{{Type: EventModelRejected}},
			exitCode:    1,
			wantStatus:  store.StatusFailed,
			wantMessage: "Fake Provider rejected the selected model",
		},
		{
			name:        "an explicit harness failure carries its reason",
			events:      []Event{{Type: EventFailed, Detail: "sandbox denied the write"}},
			exitCode:    1,
			wantStatus:  store.StatusFailed,
			wantMessage: "sandbox denied the write",
		},
		{
			name: "an explicit failure does not override a result that succeeded",
			events: []Event{
				{Type: EventCompleted, FinalOutput: "OK"},
			},
			wantStatus: store.StatusSuccess,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCollector("assigned-sid", "Fake Provider")
			c.AddAll(tc.events)
			got := c.Result(tc.exitCode)
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if tc.wantMessage != "" && got.Message != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got.Message, tc.wantMessage)
			}
			if tc.wantStatus == store.StatusSuccess && got.Message != "" {
				t.Fatalf("a successful run should carry no message, got %q", got.Message)
			}
		})
	}
}

func TestCollectorKeepsTheAssignedSessionUntilTheHarnessReportsOne(t *testing.T) {
	c := NewCollector("assigned-sid", "Fake Provider")
	if got := c.SessionID(); got != "assigned-sid" {
		t.Fatalf("session = %q, want the id claudeq assigned", got)
	}
	c.Add(Event{Type: EventSessionStarted, SessionID: "real-sid"})
	if got := c.SessionID(); got != "real-sid" {
		t.Fatalf("session = %q, want the harness's own id", got)
	}
	// An event without an id must not wipe it.
	c.Add(Event{Type: EventSessionStarted})
	if got := c.SessionID(); got != "real-sid" {
		t.Fatalf("session = %q, want it kept", got)
	}
	if got := c.Result(0).SessionID; got != "real-sid" {
		t.Fatalf("result session = %q, want real-sid", got)
	}
}

func TestCollectorCarriesRateLimitTimingAndMetrics(t *testing.T) {
	reset := time.Unix(1784655600, 0)
	c := NewCollector("sid", "Fake Provider")
	c.AddAll([]Event{
		{Type: EventRateLimited, RetryAfter: 5 * time.Second, ResetAt: reset},
		{Type: EventCompleted, IsError: true, Metrics: &Metrics{InputTokens: 10, OutputTokens: 2}},
	})
	got := c.Result(1)
	if got.RetryAfter != 5*time.Second {
		t.Fatalf("retry after = %v, want 5s", got.RetryAfter)
	}
	if !got.ResetAt.Equal(reset) {
		t.Fatalf("reset at = %v, want %v", got.ResetAt, reset)
	}
	if got.Metrics == nil || got.Metrics.InputTokens != 10 {
		t.Fatalf("metrics = %+v, want the reported usage", got.Metrics)
	}
	if got.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", got.ExitCode)
	}
}

func TestCollectorKeepsTheFirstReasonGiven(t *testing.T) {
	// A later event elaborating on a failure must not replace the cause the
	// harness named first.
	c := NewCollector("sid", "Fake Provider")
	c.AddAll([]Event{
		{Type: EventFailed, Detail: "the real cause"},
		{Type: EventFailed, Detail: "a follow-on symptom"},
	})
	if got := c.Result(1).Message; got != "the real cause" {
		t.Fatalf("message = %q, want the first reason", got)
	}
}

func TestCollectorWithoutAProviderNameStillReads(t *testing.T) {
	c := NewCollector("sid", "")
	c.Add(Event{Type: EventAuthFailed})
	if got := c.Result(1).Message; got != "the provider reported an authentication problem" {
		t.Fatalf("message = %q", got)
	}
}
