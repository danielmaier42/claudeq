package opencode

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// runFixture feeds one of the spike's captured streams through the parser and
// collector exactly as the executor does, and returns the run's outcome. The
// fixtures are sanitized recordings of the real CLI (opencode 1.18.30, run
// against a local LM Studio model); no opencode process is started here or
// anywhere else in the test suite.
func runFixture(t *testing.T, name string, exitCode int) provider.Result {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	p := (&Adapter{}).NewParser()
	c := provider.NewCollector("assigned-session", "opencode")
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
		name       string
		fixture    string
		exitCode   int
		wantStatus store.RunStatus
		wantOutput string
		wantInput  int
		wantOutTok int
	}{
		{
			name: "a plain successful run", fixture: "success.jsonl", exitCode: 0,
			wantStatus: store.StatusSuccess, wantOutput: "pong",
			wantInput: 7742, wantOutTok: 4,
		},
		{
			// A run that used a tool takes two steps: the tool call's
			// step_finish says "tool-calls" and is not the end of the run,
			// only the second step_finish's "stop" is — and the usage
			// reported is the second step's, not the sum of both.
			name: "a run that used a tool", fixture: "tool-use.jsonl", exitCode: 0,
			wantStatus: store.StatusSuccess, wantOutput: "The output is: `hello`",
			wantInput: 7811, wantOutTok: 10,
		},
		{
			name: "a failed run", fixture: "error.jsonl", exitCode: 1,
			wantStatus: store.StatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runFixture(t, tt.fixture, tt.exitCode)
			if res.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v (message: %q)", res.Status, tt.wantStatus, res.Message)
			}
			if tt.wantOutput != "" && res.FinalOutput != tt.wantOutput {
				t.Errorf("FinalOutput = %q, want %q", res.FinalOutput, tt.wantOutput)
			}
			if tt.wantInput != 0 {
				if res.Metrics == nil {
					t.Fatalf("Metrics is nil, want input=%d output=%d", tt.wantInput, tt.wantOutTok)
				}
				if res.Metrics.InputTokens != tt.wantInput || res.Metrics.OutputTokens != tt.wantOutTok {
					t.Errorf("Metrics = %+v, want input=%d output=%d", res.Metrics, tt.wantInput, tt.wantOutTok)
				}
			}
			if res.SessionID != "ses_test000000000000000000" {
				t.Errorf("SessionID = %q", res.SessionID)
			}
		})
	}
}

func TestParseErrorMessageIsCarried(t *testing.T) {
	res := runFixture(t, "error.jsonl", 1)
	if !strings.Contains(res.Message, "Unexpected server error") {
		t.Errorf("Message = %q, want it to carry the CLI's own error text", res.Message)
	}
}

func TestParseIgnoresNonJSONLines(t *testing.T) {
	p := (&Adapter{}).NewParser()
	if evs := p.Parse([]byte("not json at all")); evs != nil {
		t.Errorf("Parse(non-JSON) = %v, want nil", evs)
	}
}

func TestParseIgnoresUnknownEventTypes(t *testing.T) {
	p := (&Adapter{}).NewParser()
	evs := p.Parse([]byte(`{"type":"tool_use","sessionID":"s-1","part":{"type":"tool","tool":"bash"}}`))
	if len(evs) != 1 || evs[0].Type != provider.EventSessionStarted {
		t.Errorf("Parse(tool_use) = %v, want only the session-started event", evs)
	}
}
