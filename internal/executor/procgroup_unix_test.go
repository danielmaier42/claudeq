//go:build unix

package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestKillTreeTakesDescendantsInTheirOwnGroups is the case the Codex spike
// measured: a harness starts a tool process in a *different* process group, so
// killing the harness's own group leaves that tool running, reparented to PID 1.
// A cancelled run that leaves work behind is worse than one that never started.
func TestKillTreeTakesDescendantsInTheirOwnGroups(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	// A stand-in harness: it starts a long sleep in a new session (and therefore
	// a new process group), records its pid, and then waits itself. macOS has no
	// setsid(1), so perl — which it does ship — makes the call.
	perl, err := exec.LookPath("perl")
	if err != nil {
		t.Skip("no perl here to put a child in a process group of its own")
	}
	script := filepath.Join(dir, "harness.sh")
	body := "#!/bin/sh\n" +
		perl + " -e 'use POSIX; POSIX::setsid(); exec(\"sleep\", \"300\")' & echo $! > " + pidFile + "\n" +
		"sleep 300\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil { //nolint:gosec // a test stand-in that must be executable
		t.Fatalf("write harness: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, script)
	configureProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start harness: %v", err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })

	grandchild := waitForPID(t, pidFile)
	if grandchild == cmd.Process.Pid {
		t.Fatal("the test needs the sleep in a process of its own")
	}
	// The pid is recorded the moment the shell forks, which is before the child
	// has called setsid — so wait for the group to actually change rather than
	// reading it too early and concluding the case cannot be tested.
	if !waitForOwnGroup(grandchild, cmd.Process.Pid) {
		t.Skip("this system kept the child in the harness's process group; " +
			"the descendant-group case cannot be exercised here")
	}

	cancel()
	_ = cmd.Wait()

	if alive(grandchild) {
		_ = syscall.Kill(grandchild, syscall.SIGKILL) // do not leave it behind either way
		t.Fatalf("process %d survived the cancel in its own process group", grandchild)
	}
}

// waitForOwnGroup reports whether pid ended up in a process group of its own,
// which is the situation this test exists for.
func waitForOwnGroup(pid, harness int) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pgid, err := syscall.Getpgid(pid)
		if err != nil {
			return false
		}
		if pgid != harness {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// waitForPID reads the pid the harness recorded, waiting for it to appear.
func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the harness never recorded its child's pid")
	return 0
}

// alive reports whether a process still exists, giving the kill a moment to
// land — signals are delivered asynchronously.
func alive(pid int) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

func TestDescendantsGroupsByDepth(t *testing.T) {
	// 100 → 200 → 300, plus an unrelated 400.
	snapshot := []procEntry{
		{pid: 100, ppid: 1, pgid: 100},
		{pid: 200, ppid: 100, pgid: 200},
		{pid: 300, ppid: 200, pgid: 300},
		{pid: 400, ppid: 1, pgid: 400},
	}
	got := descendants(snapshot, 100)
	if len(got) != 2 || len(got[0]) != 1 || got[0][0].pid != 200 || len(got[1]) != 1 || got[1][0].pid != 300 {
		t.Fatalf("generations = %+v, want [[200] [300]]", got)
	}
}

// TestDescendantsToleratesACycle: a reparented process whose ppid was reused can
// make the table look circular, and the teardown must still return.
func TestDescendantsToleratesACycle(t *testing.T) {
	snapshot := []procEntry{
		{pid: 100, ppid: 200, pgid: 100},
		{pid: 200, ppid: 100, pgid: 200},
	}
	if got := descendants(snapshot, 100); len(got) != 1 || got[0][0].pid != 200 {
		t.Fatalf("generations = %+v, want the cycle walked once", got)
	}
}

func TestDescendantsWithoutASnapshot(t *testing.T) {
	if got := descendants(nil, 100); got != nil {
		t.Fatalf("generations = %+v, want none without a process table", got)
	}
}
