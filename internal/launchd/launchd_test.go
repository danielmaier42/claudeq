package launchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPlistContainsProgramAndFlags(t *testing.T) {
	p, err := Plist(Config{
		Label: "de.maierdaniel.dispatchq", BinPath: "/usr/local/bin/dispatchqd", Args: []string{"run"},
		StdoutPath: "/tmp/out.log", StderrPath: "/tmp/err.log",
	})
	if err != nil {
		t.Fatalf("Plist: %v", err)
	}
	for _, want := range []string{
		"<string>de.maierdaniel.dispatchq</string>",
		"<string>/usr/local/bin/dispatchqd</string>",
		"<string>run</string>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		// Throttled, so a daemon that exits immediately (another one owns the data
		// directory) is retried once a minute instead of ten times a minute.
		"<key>ThrottleInterval</key>",
		"<integer>60</integer>",
		// Not "Background": children inherit the job's scheduling policy, and a
		// throttled harness CLI misses its probe deadline on a busy machine.
		"<key>ProcessType</key>",
		"<string>Interactive</string>",
		"/tmp/out.log",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("plist missing %q\n%s", want, p)
		}
	}
}

func TestPlistAssociatedBundleID(t *testing.T) {
	// Set: the association keys are present so Login Items shows the app.
	with, _ := Plist(Config{Label: "de.maierdaniel.dispatchq", BinPath: "/x", AssociatedBundleID: "de.maierdaniel.dispatchq"})
	if !strings.Contains(with, "<key>AssociatedBundleIdentifiers</key>") ||
		!strings.Contains(with, "<string>de.maierdaniel.dispatchq</string>") {
		t.Fatalf("plist missing AssociatedBundleIdentifiers:\n%s", with)
	}
	// Unset: the key is omitted (e.g. a dev run outside a bundle).
	without, _ := Plist(Config{Label: "de.maierdaniel.dispatchq", BinPath: "/x"})
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
	a := Agent{Runner: r, Dir: dir, Label: "de.maierdaniel.dispatchq", UID: 501}

	if err := a.Install(context.Background(), "PLISTDATA"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	path := filepath.Join(dir, "de.maierdaniel.dispatchq.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("plist not written: %v", err)
	}
	if string(data) != "PLISTDATA" {
		t.Fatalf("plist content = %q", string(data))
	}

	wantCalls := [][]string{
		{"launchctl", "bootout", "gui/501/de.maierdaniel.dispatchq"},
		{"launchctl", "enable", "gui/501/de.maierdaniel.dispatchq"},
		{"launchctl", "bootstrap", "gui/501", path},
		{"launchctl", "kickstart", "gui/501/de.maierdaniel.dispatchq"},
	}
	if !reflect.DeepEqual(r.calls, wantCalls) {
		t.Fatalf("launch sequence = %v, want %v", r.calls, wantCalls)
	}
}

func TestUninstallRemovesPlist(t *testing.T) {
	dir := t.TempDir()
	r := &recordRunner{}
	a := Agent{Runner: r, Dir: dir, Label: "de.maierdaniel.dispatchq", UID: 501}
	if err := a.Install(context.Background(), "X"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "de.maierdaniel.dispatchq.plist")); !os.IsNotExist(err) {
		t.Fatal("plist should be removed")
	}
}

func TestUninstallMissingPlistIsOK(t *testing.T) {
	a := Agent{Runner: &recordRunner{}, Dir: t.TempDir(), UID: 501}
	if err := a.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall on missing plist should be nil, got %v", err)
	}
}

// Model a persisted disabled override: bootstrap is refused until enable ran,
// and every failed stage must prevent the subsequent stages from running.
type installRunner struct {
	disabled bool
	fail     string
	calls    []string
}

func (r *installRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	op := args[0]
	r.calls = append(r.calls, op)
	if op == r.fail {
		return []byte("denied"), fmt.Errorf("injected failure")
	}
	if op == "enable" {
		r.disabled = false
	}
	if op == "bootstrap" && r.disabled {
		return nil, fmt.Errorf("service disabled")
	}
	return nil, nil
}
func TestInstallRepairsDisabledServiceAndPropagatesFailures(t *testing.T) {
	for _, stage := range []string{"", "enable", "bootstrap", "kickstart"} {
		t.Run(stage, func(t *testing.T) {
			r := &installRunner{disabled: true, fail: stage}
			a := Agent{Runner: r, Dir: t.TempDir(), UID: 501}
			err := a.Install(context.Background(), "plist")
			if stage == "" {
				if err != nil || r.disabled {
					t.Fatalf("repair: disabled=%v, err=%v", r.disabled, err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "launchctl "+stage) || !strings.Contains(err.Error(), "denied") {
					t.Fatalf("lost %s failure: %v", stage, err)
				}
				if r.calls[len(r.calls)-1] != stage {
					t.Fatalf("continued after failure: %v", r.calls)
				}
			}
		})
	}
}
