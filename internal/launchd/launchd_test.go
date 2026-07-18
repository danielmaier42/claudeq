package launchd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlistContainsProgramAndFlags(t *testing.T) {
	p, err := Plist(Config{
		Label: "ag.dc.claudeq", BinPath: "/usr/local/bin/claudeqd", Args: []string{"run"},
		StdoutPath: "/tmp/out.log", StderrPath: "/tmp/err.log",
	})
	if err != nil {
		t.Fatalf("Plist: %v", err)
	}
	for _, want := range []string{
		"<string>ag.dc.claudeq</string>",
		"<string>/usr/local/bin/claudeqd</string>",
		"<string>run</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"/tmp/out.log",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("plist missing %q\n%s", want, p)
		}
	}
}

func TestPlistAssociatedBundleID(t *testing.T) {
	// Set: the association keys are present so Login Items shows the app.
	with, _ := Plist(Config{Label: "ag.dc.claudeq", BinPath: "/x", AssociatedBundleID: "ag.dc.claudeq"})
	if !strings.Contains(with, "<key>AssociatedBundleIdentifiers</key>") ||
		!strings.Contains(with, "<string>ag.dc.claudeq</string>") {
		t.Fatalf("plist missing AssociatedBundleIdentifiers:\n%s", with)
	}
	// Unset: the key is omitted (e.g. a dev run outside a bundle).
	without, _ := Plist(Config{Label: "ag.dc.claudeq", BinPath: "/x"})
	if strings.Contains(without, "AssociatedBundleIdentifiers") {
		t.Fatalf("plist should omit AssociatedBundleIdentifiers when unset:\n%s", without)
	}
}

type recordRunner struct{ calls [][]string }

func (r *recordRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil, nil
}

func TestInstallWritesPlistAndBootstraps(t *testing.T) {
	dir := t.TempDir()
	r := &recordRunner{}
	a := Agent{Runner: r, Dir: dir, Label: "ag.dc.claudeq", UID: 501}

	if err := a.Install(context.Background(), "PLISTDATA"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	path := filepath.Join(dir, "ag.dc.claudeq.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if string(data) != "PLISTDATA" {
		t.Fatalf("plist content = %q", string(data))
	}

	if len(r.calls) != 2 {
		t.Fatalf("expected bootout + bootstrap, got %v", r.calls)
	}
	if got := strings.Join(r.calls[0], " "); !strings.Contains(got, "bootout gui/501/ag.dc.claudeq") {
		t.Fatalf("first call = %q, want bootout", got)
	}
	if got := strings.Join(r.calls[1], " "); !strings.Contains(got, "bootstrap gui/501 "+path) {
		t.Fatalf("second call = %q, want bootstrap", got)
	}
}

func TestUninstallRemovesPlist(t *testing.T) {
	dir := t.TempDir()
	r := &recordRunner{}
	a := Agent{Runner: r, Dir: dir, Label: "ag.dc.claudeq", UID: 501}
	if err := a.Install(context.Background(), "X"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ag.dc.claudeq.plist")); !os.IsNotExist(err) {
		t.Fatal("plist should be removed")
	}
}

func TestUninstallMissingPlistIsOK(t *testing.T) {
	a := Agent{Runner: &recordRunner{}, Dir: t.TempDir(), UID: 501}
	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall on missing plist should be nil, got %v", err)
	}
}
