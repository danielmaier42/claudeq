package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func sampleTask() task.Task {
	return task.Task{
		ID: "t1", Name: "t1", Prompt: "do the thing",
		WorkingDir: ".", Trigger: task.TriggerASAP,
		Enabled: true, Permissions: task.PermissionsDefault,
	}
}

func TestArgsFreshRun(t *testing.T) {
	e := &Executor{}
	tk := sampleTask()
	args := e.Args(Request{Task: tk, SessionID: "SID", Model: "claude-opus-4-8"})

	joined := strings.Join(args, " ")
	for _, want := range []string{"-p", "--output-format stream-json", "--model claude-opus-4-8", "--session-id SID"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if args[len(args)-1] != tk.Prompt {
		t.Fatalf("prompt must be the last arg, got %q", args[len(args)-1])
	}
	if strings.Contains(joined, "--resume") {
		t.Fatal("fresh run must not use --resume")
	}
}

func TestArgsResumeAndPermissions(t *testing.T) {
	e := &Executor{}
	tk := sampleTask()

	args := strings.Join(e.Args(Request{
		Task: tk, SessionID: "SID", Resume: true,
		Model: "claude-haiku-4-5-20251001", SkipPermissions: true,
	}), " ")
	if !strings.Contains(args, "--resume SID") {
		t.Fatalf("resume run must use --resume, got %q", args)
	}
	if strings.Contains(args, "--session-id") {
		t.Fatal("resume run must not also pass --session-id")
	}
	if !strings.Contains(args, "--dangerously-skip-permissions") {
		t.Fatalf("skip permissions not applied, got %q", args)
	}
	if !strings.Contains(args, "--model claude-haiku-4-5-20251001") {
		t.Fatalf("model override missing, got %q", args)
	}
}

func TestArgsNoModelWhenEmpty(t *testing.T) {
	e := &Executor{}
	args := strings.Join(e.Args(Request{Task: sampleTask(), SessionID: "S"}), " ")
	if strings.Contains(args, "--model") {
		t.Fatalf("no model should be passed when empty, got %q", args)
	}
	if strings.Contains(args, "--dangerously-skip-permissions") {
		t.Fatalf("skip should not be set by default, got %q", args)
	}
}

// fakeClaude writes an executable script to a temp dir that prints the given
// stdout lines and exits with exitCode, standing in for the real CLI.
func fakeClaude(t *testing.T, stdout string, exitCode int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell binary is POSIX-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat <<'EOF'\n" + stdout + "\nEOF\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}

func runFake(t *testing.T, stdout string, exitCode int) Result {
	t.Helper()
	e := &Executor{Bin: fakeClaude(t, stdout, exitCode)}
	var log bytes.Buffer
	tk := sampleTask()
	tk.WorkingDir = t.TempDir()
	res, err := e.Run(context.Background(), Request{Task: tk, SessionID: "assigned-sid", Log: &log})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if log.Len() == 0 {
		t.Fatal("expected output to be streamed to the log")
	}
	return res
}

func TestRequestBinOverridesExecutorDefault(t *testing.T) {
	// The Executor's own Bin points nowhere; the per-run Request.Bin is a working
	// fake CLI. Run must use the request override and succeed.
	e := &Executor{Bin: "/nonexistent/claude"}
	out := `{"type":"result","subtype":"success","is_error":false,"result":"OK","session_id":"real-sid"}`
	tk := sampleTask()
	tk.WorkingDir = t.TempDir()
	var log bytes.Buffer
	res, err := e.Run(context.Background(), Request{
		Task: tk, SessionID: "sid", Bin: fakeClaude(t, out, 0), Log: &log,
	})
	if err != nil {
		t.Fatalf("Run with Request.Bin override: %v", err)
	}
	if res.Status != store.StatusSuccess {
		t.Fatalf("status = %q, want success (override binary should have run)", res.Status)
	}
}

func TestRunSuccess(t *testing.T) {
	out := `{"type":"result","subtype":"success","is_error":false,"result":"OK","session_id":"real-sid"}`
	res := runFake(t, out, 0)
	if res.Status != store.StatusSuccess {
		t.Fatalf("status = %q, want success", res.Status)
	}
	if res.SessionID != "real-sid" {
		t.Fatalf("session id = %q, want the one reported by the CLI", res.SessionID)
	}
}

func TestRunRateLimited(t *testing.T) {
	out := strings.Join([]string{
		`{"type":"system","subtype":"api_retry","error_status":429,"error":"rate_limit","retry_delay_ms":5000,"session_id":"real-sid"}`,
	}, "\n")
	res := runFake(t, out, 1)
	if res.Status != store.StatusRateLimited {
		t.Fatalf("status = %q, want rate_limited_waiting", res.Status)
	}
	if res.RetryAfter.Milliseconds() != 5000 {
		t.Fatalf("retry after = %v, want 5s", res.RetryAfter)
	}
}

func TestRunAuthError(t *testing.T) {
	out := `{"type":"result","subtype":"error","is_error":true,"error":"authentication_failed"}`
	res := runFake(t, out, 1)
	if res.Status != store.StatusAuthError {
		t.Fatalf("status = %q, want auth_error", res.Status)
	}
}

func TestRunFailure(t *testing.T) {
	out := `{"type":"result","subtype":"error","is_error":true,"result":"boom"}`
	res := runFake(t, out, 2)
	if res.Status != store.StatusFailed {
		t.Fatalf("status = %q, want failed", res.Status)
	}
	if res.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", res.ExitCode)
	}
}

func TestRunCapturesMetricsAndResultText(t *testing.T) {
	out := `{"type":"result","subtype":"success","is_error":false,"result":"All done.","session_id":"s","total_cost_usd":0.012,"num_turns":2,"duration_ms":3400,"usage":{"input_tokens":1200,"output_tokens":300}}`
	res := runFake(t, out, 0)
	if res.Status != store.StatusSuccess {
		t.Fatalf("status = %q, want success", res.Status)
	}
	if res.ResultText != "All done." {
		t.Fatalf("result text = %q, want %q", res.ResultText, "All done.")
	}
	if res.Metrics == nil || res.Metrics.CostUSD != 0.012 || res.Metrics.InputTokens != 1200 || res.Metrics.OutputTokens != 300 {
		t.Fatalf("unexpected metrics: %+v", res.Metrics)
	}
}

func TestRunRateLimitWinsOverPartialResult(t *testing.T) {
	// A rate-limit event followed by an errored result must classify as
	// rate-limited (so the gate waits), not a plain failure.
	out := strings.Join([]string{
		`{"type":"system","subtype":"api_retry","error_status":429,"retry_delay_ms":1000}`,
		`{"type":"result","is_error":true,"result":"aborted"}`,
	}, "\n")
	res := runFake(t, out, 1)
	if res.Status != store.StatusRateLimited {
		t.Fatalf("status = %q, want rate_limited_waiting", res.Status)
	}
}
