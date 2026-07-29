package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}

	cases := []struct {
		name  string
		start string
		want  string
	}{
		{"existing directory is kept", dir, dir},
		{"trailing slash", dir + "/", dir},
		{"deleted directory falls back to its parent", filepath.Join(dir, "gone"), dir},
		{"deeply deleted path walks up", filepath.Join(dir, "gone", "deeper", "still"), dir},
		{"a file resolves to its directory", file, dir},
		{"empty start falls back to home", "", home},
		{"blank start falls back to home", "   ", home},
		{"relative path falls back to home", "some/relative/dir", home},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := existingDir(c.start); got != c.want {
				t.Errorf("existingDir(%q) = %q, want %q", c.start, got, c.want)
			}
		})
	}
}

// A folder the daemon may not stat (macOS TCC protects Documents/Desktop/…)
// must be kept as-is: it exists, and the GUI dialog has its own access.
func TestExistingDirKeepsUnreadablePath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	inner := filepath.Join(locked, "project")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if got := existingDir(inner); got != inner {
		t.Errorf("existingDir(%q) = %q, want the path unchanged", inner, got)
	}
}

func TestOSAScriptFolderChooserSkipsMissingStartDir(t *testing.T) {
	dir := t.TempDir()
	gone := filepath.Join(dir, "deleted-project")

	fr := &fakeRunner{out: []byte(dir + "\n")}
	path, chosen, err := OSAScriptFolderChooser(fr)(context.Background(), gone)
	if err != nil || !chosen || path != dir {
		t.Fatalf("chooser = (%q, %v, %v), want (%q, true, nil)", path, chosen, err, dir)
	}
	script := strings.Join(fr.args, " ")
	// The dialog must open at the nearest existing ancestor: `choose folder`
	// errors out without showing anything when the default location is gone.
	if strings.Contains(script, gone) {
		t.Fatalf("script must not point at the missing directory: %s", script)
	}
	if !strings.Contains(script, `default location (POSIX file "`+dir+`")`) {
		t.Fatalf("script should default to the existing parent, got: %s", script)
	}
}

func TestOSAScriptFolderChooserCancel(t *testing.T) {
	fr := &fakeRunner{out: []byte("execution error: User canceled. (-128)"), err: errors.New("exit status 1")}
	path, chosen, err := OSAScriptFolderChooser(fr)(context.Background(), "")
	if err != nil {
		t.Fatalf("cancel should not be an error, got: %v", err)
	}
	if chosen || path != "" {
		t.Fatalf("chooser = (%q, %v), want (\"\", false)", path, chosen)
	}
}

func TestOSAScriptFolderChooserError(t *testing.T) {
	fr := &fakeRunner{out: []byte("execution error: Not authorized to send Apple events"), err: errors.New("exit status 1")}
	_, _, err := OSAScriptFolderChooser(fr)(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "Not authorized") {
		t.Fatalf("error should carry osascript output, got: %v", err)
	}
}
