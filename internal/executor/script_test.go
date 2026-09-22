package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// scriptTask is a job whose prompt is the program to run.
func scriptTask(src string) task.Task {
	t := sampleTask()
	t.Kind = task.KindScript
	t.Prompt = src
	return t
}

// runScriptTask runs src as a script job and returns the result and everything
// that reached the run log.
func runScriptTask(t *testing.T, e *Executor, req Request) (result, log string, status store.RunStatus, code int) {
	t.Helper()
	var buf bytes.Buffer
	req.Log = &buf
	res, err := e.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return res.FinalOutput, buf.String(), res.Status, res.ExitCode
}

func TestScriptRunsWithoutAProvider(t *testing.T) {
	// No registry, no provider instance: a script job must not need either.
	e := &Executor{}
	out, log, status, code := runScriptTask(t, e, Request{Task: scriptTask("echo hello")})
	if status != store.StatusSuccess || code != 0 {
		t.Fatalf("status/exit = %v/%d, want success/0", status, code)
	}
	if strings.TrimSpace(out) != "hello" {
		t.Fatalf("final output = %q, want %q", out, "hello")
	}
	if !strings.Contains(log, "hello") {
		t.Fatalf("run log = %q, want it to carry the script's output", log)
	}
}

func TestScriptFailureIsTheExitCode(t *testing.T) {
	e := &Executor{}
	_, log, status, code := runScriptTask(t, e, Request{Task: scriptTask("echo to stderr >&2; exit 3")})
	if status != store.StatusFailed {
		t.Fatalf("status = %v, want failed", status)
	}
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
	if !strings.Contains(log, "to stderr") {
		t.Fatalf("run log = %q, want it to carry stderr too", log)
	}
}

func TestScriptAnswerIsStdoutOnly(t *testing.T) {
	// A script that reports its progress on stderr must not have that turn into
	// the answer a notification quotes or a dependent job consolidates.
	e := &Executor{}
	out, _, _, _ := runScriptTask(t, e, Request{Task: scriptTask("echo noise >&2; echo answer")})
	if strings.TrimSpace(out) != "answer" {
		t.Fatalf("final output = %q, want only stdout", out)
	}
}

func TestScriptHonoursItsShebang(t *testing.T) {
	e := &Executor{}
	src := "#!/bin/sh\necho \"$0 ran\"\n"
	out, _, status, _ := runScriptTask(t, e, Request{Task: scriptTask(src)})
	if status != store.StatusSuccess {
		t.Fatalf("status = %v, want success", status)
	}
	// With a shebang the file itself is executed, so $0 is the script's path —
	// not the interpreter claudeq would otherwise have picked.
	if !strings.Contains(out, "ran") || strings.Contains(out, defaultShell+" ran") {
		t.Fatalf("output = %q, want the script to have been executed directly", out)
	}
}

func TestScriptRunsInTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	tk := scriptTask("pwd")
	tk.WorkingDir = dir
	e := &Executor{}
	out, _, _, _ := runScriptTask(t, e, Request{Task: tk})
	// macOS resolves the temp dir through /private, so compare the suffix.
	if !strings.HasSuffix(strings.TrimSpace(out), strings.TrimPrefix(dir, "/private")) {
		t.Fatalf("pwd = %q, want the task's working directory %q", strings.TrimSpace(out), dir)
	}
}

func TestScriptGetsTheSelfQueueEnvironment(t *testing.T) {
	// A script job exists to file agent work, so it needs exactly what an agent
	// run needs to call `claudeq queue`.
	e := &Executor{Home: "/tmp/home", QueueBin: "/bin/claudeq"}
	req := Request{
		Task:       scriptTask(`printf '%s|%s|%s|%s\n' "$CLAUDEQ_HOME" "$CLAUDEQ_BIN" "$CLAUDEQ_RUN_ID" "$CLAUDEQ_TASK_ID"; echo "$CLAUDEQ_PARENT_TASK"`),
		RunID:      "run-1",
		WorkflowID: "wf-1",
	}
	out, _, _, _ := runScriptTask(t, e, req)
	head, rest, _ := strings.Cut(out, "\n")
	if head != "/tmp/home|/bin/claudeq|run-1|t1" {
		t.Fatalf("environment = %q, want home, bin, run id and task id", head)
	}
	var parent task.Task
	if err := json.Unmarshal([]byte(strings.TrimSpace(rest)), &parent); err != nil {
		t.Fatalf("parent task is not JSON: %v (%q)", err, rest)
	}
	if parent.ID != "t1" || !parent.IsScript() {
		t.Fatalf("parent task = %+v, want the script job itself", parent)
	}
	if parent.Provider != "" {
		t.Fatalf("parent provider = %q, want none: a script runs on no provider", parent.Provider)
	}
}

func TestScriptReadsDependencyResultsOnStdin(t *testing.T) {
	e := &Executor{}
	out, _, _, _ := runScriptTask(t, e, Request{Task: scriptTask("cat"), Stdin: "what the other jobs said"})
	if strings.TrimSpace(out) != "what the other jobs said" {
		t.Fatalf("stdin = %q, want the dependency digest", out)
	}
}

func TestScriptWithoutStdinIsNotLeftWaiting(t *testing.T) {
	// An unattended run must never block on input nobody will type: stdin is
	// closed immediately, so `cat` ends by itself.
	e := &Executor{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, status, _ := runScriptTask(t, e, Request{Task: scriptTask("cat")})
		if status != store.StatusSuccess {
			t.Errorf("status = %v, want success", status)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the script is still waiting for input")
	}
}

func TestScriptAnswerKeepsTheEnd(t *testing.T) {
	// A script says what matters last, so an answer longer than a run record may
	// carry is cut at the front.
	e := &Executor{}
	src := "awk 'BEGIN{ for(i=0;i<3000;i++) print \"line\" i }'; echo THEEND"
	out, _, _, _ := runScriptTask(t, e, Request{Task: scriptTask(src)})
	if len(out) > store.MaxFinalOutput {
		t.Fatalf("final output is %d bytes, want at most %d", len(out), store.MaxFinalOutput)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "THEEND") {
		t.Fatalf("final output ends with %q, want the last line", out[max(0, len(out)-20):])
	}
	if strings.Contains(out, "line0\n") {
		t.Fatal("final output kept the start, want the end")
	}
}

func TestScriptStoppedWhenItProducesNothing(t *testing.T) {
	e := &Executor{}
	var buf bytes.Buffer
	res, err := e.Run(context.Background(), Request{
		Task:        scriptTask("sleep 30"),
		IdleTimeout: 1200 * time.Millisecond,
		Log:         &buf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != store.StatusFailed {
		t.Fatalf("status = %v, want failed", res.Status)
	}
	if !strings.Contains(res.Message, "looked hung") {
		t.Fatalf("message = %q, want it to say the run looked hung", res.Message)
	}
}

func TestScriptCanceledWithItsContext(t *testing.T) {
	e := &Executor{}
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	res, err := e.Run(ctx, Request{Task: scriptTask("sleep 30"), Log: &buf})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != store.StatusFailed {
		t.Fatalf("status = %v, want failed after the run was stopped", res.Status)
	}
}

func TestTailBufferCutsOnARuneBoundary(t *testing.T) {
	b := &tailBuffer{max: 5}
	if _, err := b.Write([]byte("ähnlich")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := b.String(); got != "lich" && got != "nlich" {
		t.Fatalf("String() = %q, want the tail without half a character", got)
	}
	for _, r := range b.String() {
		if r == '�' {
			t.Fatalf("String() = %q, want valid UTF-8", b.String())
		}
	}
}
