package codex

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// runFixture feeds one of the spike's captured streams through the parser and
// collector exactly as the executor does, and returns the run's outcome. The
// fixtures are sanitized recordings of the real CLI at codex-cli 0.154.0; no
// Codex process is started here or anywhere else in the test suite.
func runFixture(t *testing.T, name string, exitCode int) provider.Result {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	p := (&Adapter{}).NewParser()
	c := provider.NewCollector("assigned-session", "Codex")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		c.AddAll(p.Parse(sc.Bytes()))
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return c.Result(exitCode)
}

func TestParseCapturedRuns(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		exitCode    int
		wantStatus  store.RunStatus
		wantOutput  string
		wantMessage string // a fragment the operator-facing message must carry
		wantInput   int
		wantOutTok  int
	}{
		{
			name: "a successful run", fixture: "success.jsonl", exitCode: 0,
			wantStatus: store.StatusSuccess, wantOutput: "SPIKE_OK",
			wantInput: 15967, wantOutTok: 7,
		},
		{
			name: "a resumed run", fixture: "resume.jsonl", exitCode: 0,
			wantStatus: store.StatusSuccess, wantOutput: "RESUME_OK",
			wantInput: 32176, wantOutTok: 14,
		},
		{
			// Codex coordinates its own subagents; claudeq counts the whole thing
			// as one job and reads only the parent's answer and totals.
			name: "a run with provider-owned subagents", fixture: "subagent.jsonl", exitCode: 0,
			wantStatus: store.StatusSuccess, wantOutput: "PARENT_OK",
			wantInput: 36585, wantOutTok: 62,
		},
		{
			name: "no credentials", fixture: "auth-error.jsonl", exitCode: 1,
			wantStatus: store.StatusAuthError, wantMessage: "401",
		},
		{
			name: "a model the account cannot use", fixture: "model-error.jsonl", exitCode: 1,
			wantStatus: store.StatusFailed, wantMessage: "not supported",
		},
		{
			name: "too many requests", fixture: "rate-limit.jsonl", exitCode: 1,
			wantStatus: store.StatusRateLimited,
		},
		{
			name: "a ChatGPT-plan usage limit", fixture: "usage-limit.jsonl", exitCode: 1,
			wantStatus: store.StatusRateLimited,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runFixture(t, tc.fixture, tc.exitCode)
			if got.Status != tc.wantStatus {
				t.Fatalf("status = %q (%s), want %q", got.Status, got.Message, tc.wantStatus)
			}
			if got.SessionID != "<THREAD_ID>" {
				t.Fatalf("session = %q, want the thread the CLI reported", got.SessionID)
			}
			if tc.wantOutput != "" && got.FinalOutput != tc.wantOutput {
				t.Fatalf("final output = %q, want %q", got.FinalOutput, tc.wantOutput)
			}
			if tc.wantMessage != "" && !strings.Contains(got.Message, tc.wantMessage) {
				t.Fatalf("message = %q, want it to carry %q", got.Message, tc.wantMessage)
			}
			if tc.wantInput == 0 {
				return
			}
			if got.Metrics == nil {
				t.Fatal("a completed turn must report its usage")
			}
			if got.Metrics.InputTokens != tc.wantInput || got.Metrics.OutputTokens != tc.wantOutTok {
				t.Fatalf("usage = %+v, want %d in / %d out", got.Metrics, tc.wantInput, tc.wantOutTok)
			}
			// Codex reports no money, and claudeq does not invent any.
			if got.Metrics.CostUSD != 0 {
				t.Fatalf("cost = %v, want none — Codex reports none", got.Metrics.CostUSD)
			}
		})
	}
}

// TestRateLimitCarriesNoInventedTiming: the captured 429 preserved neither a
// reset time nor Retry-After, so the adapter reports none and the engine falls
// back to its own backoff. Inventing one would resume into a closed window.
func TestRateLimitCarriesNoInventedTiming(t *testing.T) {
	got := runFixture(t, "rate-limit.jsonl", 1)
	if got.RetryAfter != 0 || !got.ResetAt.IsZero() {
		t.Fatalf("timing = %v / %v, want none reported", got.RetryAfter, got.ResetAt)
	}
}

// TestUsageLimitReportsItsOwnResetTime: unlike the transport 429, a ChatGPT-plan
// usage limit names its own reset in the message, and the adapter reads it
// instead of falling back to a blind backoff that would retry for days.
func TestUsageLimitReportsItsOwnResetTime(t *testing.T) {
	got := runFixture(t, "usage-limit.jsonl", 1)
	want := time.Date(2026, time.September, 19, 12, 25, 0, 0, time.Local)
	if !got.ResetAt.Equal(want) {
		t.Fatalf("reset = %v, want %v", got.ResetAt, want)
	}
}

// TestParseToleratesNoise covers what a real stream carries besides its records:
// blank lines, stderr text, half-written JSON, and record types this version of
// claudeq has never heard of. None of it may change the outcome.
func TestParseToleratesNoise(t *testing.T) {
	p := (&Adapter{}).NewParser()
	c := provider.NewCollector("assigned", "Codex")
	for _, line := range []string{
		``,
		`   `,
		`Reading additional input from stdin...`,
		`{"type":"thread.started","thread_id":"t-1"}`,
		`{"type":"item.completed","item":{"id":"i","type":"todo_list","items":[]}}`,
		`{"type":"something.new","detail":"a record from a later CLI"}`,
		`{"type":"item.completed","item":{"id":"i2","type":"agent_message","text":"done"}`, // truncated
		`{"type":"item.completed","item":{"id":"i3","type":"agent_message","text":"the answer"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":2}}`,
	} {
		c.AddAll(p.Parse([]byte(line)))
	}
	got := c.Result(0)
	if got.Status != store.StatusSuccess {
		t.Fatalf("status = %q (%s), want success", got.Status, got.Message)
	}
	if got.SessionID != "t-1" || got.FinalOutput != "the answer" {
		t.Fatalf("result = %+v, want the thread and the last agent message", got)
	}
}

// TestMissingTerminalEvent: the process stopped without ever completing a turn,
// which is a failure however it exited.
func TestMissingTerminalEvent(t *testing.T) {
	p := (&Adapter{}).NewParser()
	c := provider.NewCollector("assigned", "Codex")
	c.AddAll(p.Parse([]byte(`{"type":"thread.started","thread_id":"t-1"}`)))
	c.AddAll(p.Parse([]byte(`{"type":"turn.started"}`)))
	if got := c.Result(0); got.Status != store.StatusFailed {
		t.Fatalf("status = %q, want failed without a terminal event", got.Status)
	}
}

// TestNonZeroExitAfterAnApparentSuccess: Codex said the turn completed, then the
// process exited badly. The exit status decides, because whatever went wrong
// happened after the answer and the run cannot be called successful.
func TestNonZeroExitAfterAnApparentSuccess(t *testing.T) {
	got := runFixture(t, "success.jsonl", 1)
	if got.Status != store.StatusFailed {
		t.Fatalf("status = %q, want failed on a non-zero exit", got.Status)
	}
	if got.FinalOutput != "SPIKE_OK" {
		t.Fatalf("final output = %q, want what the run managed to say", got.FinalOutput)
	}
}

// TestLastFailureWins: Codex narrates a failure as it unfolds — a transport
// warning first, the status that actually ended it last. The auth fixture ends
// on a 401 that arrives after a "Reconnecting…" line, and that is the one the
// operator needs to read.
func TestLastFailureWins(t *testing.T) {
	got := runFixture(t, "auth-error.jsonl", 1)
	if strings.Contains(got.Message, "Reconnecting") {
		t.Fatalf("message = %q, want the failure that ended the run, not the first warning", got.Message)
	}
}

func TestFailureClassification(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    provider.EventType
	}{
		{name: "unauthorized", message: "unexpected status 401 Unauthorized", want: provider.EventAuthFailed},
		{name: "too many requests", message: "exceeded retry limit, last status: 429 Too Many Requests", want: provider.EventRateLimited},
		{
			name:    "a ChatGPT-plan usage limit",
			message: "You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Sep 19th, 2026 12:25 PM.",
			want:    provider.EventRateLimited,
		},
		{
			name:    "an unsupported model",
			message: `{"status":400,"error":{"type":"invalid_request_error","message":"The 'x' model is not supported"}}`,
			want:    provider.EventModelRejected,
		},
		{
			// A 400 on its own is not a model problem; plenty of requests are
			// rejected for other reasons, and calling them all model errors would
			// send the operator to change a setting that is fine.
			name:    "some other bad request",
			message: `{"status":400,"error":{"message":"the working directory does not exist"}}`,
			want:    provider.EventFailed,
		},
		{name: "anything else", message: "the sandbox denied a write", want: provider.EventFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &parser{}
			p.noteFailure(tc.message)
			if got := p.terminalFailure(); got.Type != tc.want {
				t.Fatalf("type = %q, want %q", got.Type, tc.want)
			}
		})
	}
}
