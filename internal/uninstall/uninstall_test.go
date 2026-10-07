package uninstall

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct{ calls []string }

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return nil, nil
}

// writePkg puts an uninstaller into a bundle's Contents/Resources.
func writePkg(t *testing.T, app string) string {
	t.Helper()
	res := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(res, PkgName)
	if err := os.WriteFile(pkg, []byte("xar!"), 0o644); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestLocate(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "Own", "ClaudeQ.app")
	def := filepath.Join(dir, "Applications", "ClaudeQ.app")
	defPkg := writePkg(t, def)
	exe := filepath.Join(own, "Contents", "MacOS", "claudeq")

	// The running bundle has none: the one in /Applications is used.
	if got, err := locate(exe, def); err != nil || got != defPkg {
		t.Fatalf("locate = %q, %v; want %q", got, err, defPkg)
	}
	// The running bundle's own uninstaller wins.
	ownPkg := writePkg(t, own)
	if got, err := locate(exe, def); err != nil || got != ownPkg {
		t.Fatalf("locate = %q, %v; want %q", got, err, ownPkg)
	}
	// Outside any bundle and with nothing installed: a clear error.
	if _, err := locate(filepath.Join(dir, "bin", "claudeq"), filepath.Join(dir, "Nope.app")); err == nil || !strings.Contains(err.Error(), PkgName) {
		t.Fatalf("err = %v, want one naming %s", err, PkgName)
	}
}

func TestStageCopiesOutOfTheBundle(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	pkg := writePkg(t, filepath.Join(t.TempDir(), "ClaudeQ.app"))
	staged, err := Stage(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(staged, filepath.Dir(pkg)) || filepath.Base(staged) != PkgName {
		t.Fatalf("staged at %s", staged)
	}
	if b, err := os.ReadFile(staged); err != nil || string(b) != "xar!" {
		t.Fatalf("staged copy = %q, %v", b, err)
	}
	// Asking again reuses the place, so the Installer gets the same document.
	if again, err := Stage(pkg); err != nil || again != staged {
		t.Fatalf("second stage = %q, %v; want %q", again, err, staged)
	}
}

func TestOpenHandsTheStagedCopyToTheInstaller(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	app := filepath.Join(t.TempDir(), "ClaudeQ.app")
	writePkg(t, app)
	r := &fakeRunner{}
	if err := Open(context.Background(), r, filepath.Join(app, "Contents", "MacOS", "claudeqd")); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || !strings.HasPrefix(r.calls[0], "open ") || strings.Contains(r.calls[0], app) {
		t.Fatalf("calls = %v, want one open of a copy outside the bundle", r.calls)
	}
}

func TestChoiceChangesSelectsTheDataChoice(t *testing.T) {
	x := string(ChoiceChanges())
	for _, want := range []string{"<string>selected</string>", "<string>" + DataChoice + "</string>", "<integer>1</integer>"} {
		if !strings.Contains(x, want) {
			t.Fatalf("choice changes lack %q:\n%s", want, x)
		}
	}
}
