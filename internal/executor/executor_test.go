package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestArgsAppendsSelfQueueSystemPrompt(t *testing.T) {
	e := &Executor{}
	tk := sampleTask()
	args := e.Args(Request{Task: tk, SessionID: "SID"})

	// The prompt must remain the final positional argument, immediately preceded
	// by the --append-system-prompt flag and its (single) value.
	if args[len(args)-1] != tk.Prompt {
		t.Fatalf("prompt must be the last arg, got %q", args[len(args)-1])
	}
	if args[len(args)-3] != "--append-system-prompt" {
		t.Fatalf("expected --append-system-prompt before the prompt, got %v", args[len(args)-3:])
	}
	if got := args[len(args)-2]; got != builtinSystemPrompt {
		t.Fatalf("append-system-prompt value = %q, want the built-in prompt", got)
	}
	if !strings.Contains(selfQueueSystemPrompt, "queue --prompt") {
		t.Fatal("self-queue prompt must document the `queue --prompt` command")
	}
	if !strings.Contains(builtinSystemPrompt, "publish --file") {
		t.Fatal("built-in prompt must document the `publish --file` command")
	}
}

func TestArgsAppendsCustomSystemPrompt(t *testing.T) {
	e := &Executor{}
	tk := sampleTask()
	const custom = "Always run the tests before finishing."
	args := e.Args(Request{Task: tk, SessionID: "SID", CustomSystemPrompt: custom})

	// Still a single --append-system-prompt value, right before the prompt.
	if args[len(args)-1] != tk.Prompt {
		t.Fatalf("prompt must be the last arg, got %q", args[len(args)-1])
	}
	if args[len(args)-3] != "--append-system-prompt" {
		t.Fatalf("expected --append-system-prompt before the prompt, got %v", args[len(args)-3:])
	}
	got := args[len(args)-2]
	// Built-in first, custom last.
	if !strings.HasPrefix(got, builtinSystemPrompt) {
		t.Fatal("built-in prompt must come first")
	}
	if !strings.HasSuffix(got, custom) {
		t.Fatalf("custom prompt must come last, got %q", got)
	}
	if !strings.Contains(got, customSystemPromptIntro) {
		t.Fatal("custom prompt must be introduced by customSystemPromptIntro")
	}
}

func TestSystemPromptBlankCustomIsUnchanged(t *testing.T) {
	// A blank or whitespace-only custom prompt must not alter the built-in value,
	// so runs without one behave exactly as before.
	for _, custom := range []string{"", "   ", "\n\t "} {
		if got := systemPrompt(custom); got != builtinSystemPrompt {
			t.Fatalf("systemPrompt(%q) = %q, want the unmodified built-in prompt", custom, got)
		}
	}
}

func TestRunEnvCarriesSelfQueueContext(t *testing.T) {
	e := &Executor{Home: "/data/home", QueueBin: "/opt/claudeq"}
	tk := sampleTask()
	tk.Model = "claude-opus-4-8"
	tk.Permissions = task.PermissionsSkip
	tk.Parallel = true

	env := e.runEnv(Request{Task: tk})
	got := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			got[k] = v // later duplicates win, matching exec semantics
		}
	}

	if got[store.EnvHome] != "/data/home" {
		t.Fatalf("%s = %q, want /data/home", store.EnvHome, got[store.EnvHome])
	}
	if got[EnvQueueBin] != "/opt/claudeq" {
		t.Fatalf("%s = %q, want /opt/claudeq", EnvQueueBin, got[EnvQueueBin])
	}
	var parent task.Task
	if err := json.Unmarshal([]byte(got[EnvParentTask]), &parent); err != nil {
		t.Fatalf("parent task env is not valid JSON: %v", err)
	}
	if parent.Model != "claude-opus-4-8" || parent.Permissions != task.PermissionsSkip || !parent.Parallel {
		t.Fatalf("parent task did not round-trip inheritable settings: %+v", parent)
	}
}

func TestRunEnvCarriesRunAndTaskIDs(t *testing.T) {
	e := &Executor{}
	tk := sampleTask() // sampleTask sets a non-empty ID
	env := e.runEnv(Request{Task: tk, RunID: "20260721T030000-abcd"})
	got := map[string]string{}
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			got[k] = v
		}
	}
	if got[EnvRunID] != "20260721T030000-abcd" {
		t.Fatalf("%s = %q, want the run id", EnvRunID, got[EnvRunID])
	}
	if got[EnvTaskID] != tk.ID {
		t.Fatalf("%s = %q, want %q", EnvTaskID, got[EnvTaskID], tk.ID)
	}
}

func TestRunEnvOmitsUnsetRunID(t *testing.T) {
	e := &Executor{}
	env := e.runEnv(Request{Task: sampleTask()}) // no RunID
	if strings.Contains(strings.Join(env, "\n"), EnvRunID+"=") {
		t.Fatalf("%s must be omitted when RunID is unset", EnvRunID)
	}
}

func TestRunEnvOmitsUnsetPaths(t *testing.T) {
	e := &Executor{} // no Home / QueueBin configured
	env := e.runEnv(Request{Task: sampleTask()})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, store.EnvHome+"=") {
		t.Fatal("CLAUDEQ_HOME must be omitted when Home is unset")
	}
	if strings.Contains(joined, EnvQueueBin+"=") {
		t.Fatal("CLAUDEQ_BIN must be omitted when QueueBin is unset")
	}
	if !strings.Contains(joined, EnvParentTask+"=") {
		t.Fatal("the parent task should always be exported")
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

func TestRunIdleTimeoutKillsHungRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only fake")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	// Produces no output and sleeps well past the idle timeout: a hung run.
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 10\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := &Executor{Bin: path}
	tk := sampleTask()
	tk.WorkingDir = t.TempDir()
	var log bytes.Buffer
	start := time.Now()
	res, err := e.Run(context.Background(), Request{
		Task: tk, SessionID: "sid", IdleTimeout: 150 * time.Millisecond, Log: &log,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != store.StatusFailed {
		t.Fatalf("status = %q, want failed", res.Status)
	}
	if !strings.Contains(res.Message, "no output") {
		t.Fatalf("message = %q, want an inactivity note", res.Message)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("watchdog took %s, expected to kill quickly", d)
	}
}

func TestRunNoIdleTimeoutCompletesNormally(t *testing.T) {
	// A quick run with the watchdog enabled must still succeed (no false kill).
	e := &Executor{Bin: fakeClaude(t, `{"type":"result","is_error":false,"result":"ok","session_id":"s"}`, 0)}
	tk := sampleTask()
	tk.WorkingDir = t.TempDir()
	var log bytes.Buffer
	res, err := e.Run(context.Background(), Request{Task: tk, SessionID: "sid", IdleTimeout: time.Hour, Log: &log})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != store.StatusSuccess {
		t.Fatalf("status = %q, want success", res.Status)
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
