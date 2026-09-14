package claudecode

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectBinaryFindsCommonInstallLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX install layout")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	bin := filepath.Join(home, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := DetectBinary(); got != bin {
		t.Fatalf("DetectBinary() = %q, want %q", got, bin)
	}
}

func TestDetectBinaryIgnoresNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A non-executable file at the candidate path must not be picked.
	bin := filepath.Join(home, ".local", "bin", "claude")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("not exec"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectBinary(); got == bin {
		t.Fatalf("DetectBinary() picked a non-executable candidate %q", got)
	}
}

func TestViaLoginShellKeepsAPathPrintedBeforeTheWaitFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only fake shell")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A shell that prints the answer, then exits leaving a background child
	// holding stdout open. cmd.Output therefore ends in ErrWaitDelay even though
	// the path was already delivered — the case that used to discard it.
	shell := filepath.Join(dir, "shell")
	script := "#!/bin/sh\necho " + bin + "\n(sleep 30) &\nexit 0\n"
	if err := os.WriteFile(shell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)

	if got := viaLoginShell(); got != bin {
		t.Fatalf("viaLoginShell() = %q, want %q", got, bin)
	}
}
