package uninstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeRunner records every command and answers from a table keyed by the
// command name. A command missing from the table succeeds with no output.
type fakeRunner struct {
	calls  []string
	answer map[string]func(args []string) ([]byte, error)
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	if fn, ok := f.answer[name]; ok {
		return fn(args)
	}
	return nil, nil
}

func (f *fakeRunner) called(prefix string) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func (f *fakeRunner) index(prefix string) int {
	return slices.IndexFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

var errExit = errors.New("exit status 1")

func fail([]string) ([]byte, error) { return nil, errExit }

// quietRunner is a Mac with no receipt and no ClaudeQ processes left running.
func quietRunner() *fakeRunner {
	return &fakeRunner{answer: map[string]func([]string) ([]byte, error){
		"pkgutil": fail,
		"pgrep":   fail,
	}}
}

func writeApp(t *testing.T, dir, name, bundleID string) string {
	t.Helper()
	app := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := "<plist><dict><key>CFBundleIdentifier</key><string>" + bundleID + "</string></dict></plist>"
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newMachine lays out a home folder with a user-owned ClaudeQ.app, a
// LaunchAgent plist and a data directory, all inside a temporary directory.
func newMachine(t *testing.T, r *fakeRunner) (Machine, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	app := writeApp(t, filepath.Join(root, "Applications"), "ClaudeQ.app", BundleID)
	data := filepath.Join(home, "Library", "Application Support", "claudeq")
	touch(t, filepath.Join(data, "config.toml"))
	touch(t, filepath.Join(home, "Library", "LaunchAgents", BundleID+".plist"))
	return Machine{
		Runner: r, UID: os.Getuid(), Home: home, DataDir: data,
		Apps:    []string{app},
		Sudoers: filepath.Join(root, "sudoers.d", "claudeq"),
		Sleep:   func(time.Duration) {},
	}, app
}

func TestPlanFindsOnlyClaudeQBundlesOnce(t *testing.T) {
	m, app := newMachine(t, quietRunner())
	other := writeApp(t, filepath.Dir(app), "Other.app", "com.example.other")
	m.Apps = []string{app, app + "/", other, "relative/ClaudeQ.app", filepath.Join(filepath.Dir(app), "Gone.app")}

	p, err := m.Plan(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Apps, []string{app}) {
		t.Fatalf("Apps = %v, want only %s", p.Apps, app)
	}
	if p.Admin || p.Receipt || p.Sudoers != "" {
		t.Fatalf("a user-owned install needs no administrator: %+v", p)
	}
	if p.Agent == "" {
		t.Fatal("the LaunchAgent plist was not found")
	}
	if len(p.Data) != 0 {
		t.Fatalf("no purge, but Data = %v", p.Data)
	}
}

func TestPlanNeedsAdminForSystemParts(t *testing.T) {
	t.Run("sudoers entry", func(t *testing.T) {
		m, _ := newMachine(t, quietRunner())
		touch(t, m.Sudoers)
		p, err := m.Plan(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		if p.Sudoers != m.Sudoers || !p.Admin {
			t.Fatalf("Sudoers=%q Admin=%v, want %q and true", p.Sudoers, p.Admin, m.Sudoers)
		}
	})
	t.Run("package receipt", func(t *testing.T) {
		r := quietRunner()
		delete(r.answer, "pkgutil") // --pkg-info succeeds: the receipt exists
		m, _ := newMachine(t, r)
		p, err := m.Plan(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Receipt || !p.Admin {
			t.Fatalf("Receipt=%v Admin=%v, want both", p.Receipt, p.Admin)
		}
		if !r.called("pkgutil --pkg-info " + BundleID) {
			t.Fatalf("receipt not looked up: %v", r.calls)
		}
	})
}

func TestPlanPurgeCollectsDataAndLibraryFolders(t *testing.T) {
	m, _ := newMachine(t, quietRunner())
	webkit := filepath.Join(m.Home, "Library", "WebKit", BundleID)
	prefs := filepath.Join(m.Home, "Library", "Preferences", BundleID+".plist")
	touch(t, filepath.Join(webkit, "x"))
	touch(t, prefs)

	p, err := m.Plan(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{m.DataDir, webkit, prefs}
	if !slices.Equal(p.Data, want) {
		t.Fatalf("Data = %v, want %v", p.Data, want)
	}
}

func TestPlanPurgeRefusesForeignDirectories(t *testing.T) {
	tests := []struct {
		name string
		data func(m Machine) string
		ok   bool
	}{
		{"claudeq data dir", func(m Machine) string { return m.DataDir }, true},
		{"missing dir", func(m Machine) string { return filepath.Join(m.Home, "nope") }, true},
		{"home itself", func(m Machine) string { return m.Home }, false},
		{"parent of home", func(m Machine) string { return filepath.Dir(m.Home) }, false},
		{"root", func(Machine) string { return "/" }, false},
		{"relative", func(Machine) string { return "claudeq" }, false},
		{"empty", func(Machine) string { return "" }, false},
		{"folder without claudeq files", func(m Machine) string {
			dir := filepath.Join(m.Home, "Documents")
			touch(t, filepath.Join(dir, "thesis.pages"))
			return dir
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newMachine(t, quietRunner())
			m.DataDir = tc.data(m)
			_, err := m.Plan(context.Background(), true)
			if (err == nil) != tc.ok {
				t.Fatalf("Plan(purge) with DataDir %q: err = %v, want ok=%v", m.DataDir, err, tc.ok)
			}
		})
	}
}

func TestRunRemovesEverythingInOrder(t *testing.T) {
	r := quietRunner()
	m, app := newMachine(t, r)
	dataAtBootout := false
	r.answer["launchctl"] = func([]string) ([]byte, error) {
		dataAtBootout = exists(m.DataDir)
		return nil, nil
	}
	p, err := m.Plan(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	for _, gone := range []string{app, m.agentPlist(), m.DataDir} {
		if exists(gone) {
			t.Errorf("%s still exists", gone)
		}
	}
	if r.called("osascript") {
		t.Errorf("a user-owned install asked for the administrator password: %v", r.calls)
	}
	if !r.called("launchctl bootout gui/") {
		t.Errorf("LaunchAgent not booted out: %v", r.calls)
	}
	if !r.called("defaults delete " + BundleID) {
		t.Errorf("preferences not deleted through cfprefsd: %v", r.calls)
	}
	// The data has to outlive the daemon, or it writes it back.
	if !dataAtBootout {
		t.Error("data was deleted before the LaunchAgent was stopped")
	}
	if a, b := r.index("pgrep -x claudeqapp"), r.index("launchctl bootout"); a < 0 || a > b {
		t.Errorf("the window must be closed before the agent goes: %v", r.calls)
	}
	if a, b := r.index("launchctl bootout"), r.index("pgrep -x claudeqd"); a < 0 || a > b {
		t.Errorf("stray daemons must be stopped after the bootout: %v", r.calls)
	}
}

// A user-owned app goes after the agent, so a failed delete never leaves the
// agent registered for a binary that is gone.
func TestRunRemovesUserOwnedAppAfterTheAgent(t *testing.T) {
	r := quietRunner()
	m, app := newMachine(t, r)
	appAtBootout := false
	r.answer["launchctl"] = func([]string) ([]byte, error) {
		appAtBootout = exists(app)
		return nil, nil
	}
	p, err := m.Plan(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !appAtBootout || exists(app) {
		t.Fatalf("app at bootout = %v, after = %v; want there, then gone", appAtBootout, exists(app))
	}
}

// A CLAUDEQ_HOME outside the default location may be a folder the user keeps
// other things in: a purge takes ClaudeQ's entries and leaves the rest.
func TestPurgeOfSharedDataDirKeepsForeignFiles(t *testing.T) {
	for _, foreign := range []bool{true, false} {
		m, _ := newMachine(t, quietRunner())
		m.DataDir = filepath.Join(m.Home, "automation")
		for _, name := range []string{".lock", "config.toml", "history.jsonl", "runs/r1.log", "claudeqd.err.log", ".tmp-123"} {
			touch(t, filepath.Join(m.DataDir, name))
		}
		if foreign {
			touch(t, filepath.Join(m.DataDir, "deploy.sh"))
		}
		p, err := m.Plan(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		if p.SharedDir != m.DataDir || slices.Contains(p.Data, m.DataDir) {
			t.Fatalf("SharedDir = %q, Data = %v: the folder itself must not be deleted", p.SharedDir, p.Data)
		}
		if err := m.Run(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		left, _ := os.ReadDir(m.DataDir)
		var names []string
		for _, e := range left {
			names = append(names, e.Name())
		}
		switch {
		case foreign && !slices.Equal(names, []string{"deploy.sh"}):
			t.Fatalf("left %v, want only deploy.sh", names)
		case !foreign && exists(m.DataDir):
			t.Fatalf("an emptied CLAUDEQ_HOME was left behind: %v", names)
		}
	}
}

func TestRemovableByUser(t *testing.T) {
	m, app := newMachine(t, quietRunner())
	if !removableByUser(app, m.UID) {
		t.Fatal("a bundle the user owns needs no administrator")
	}
	if removableByUser(app, m.UID+1) {
		t.Fatal("a bundle owned by someone else counted as removable")
	}
	// A folder the user cannot write to stops a delete halfway.
	res := filepath.Join(app, "Contents", "Resources")
	touch(t, filepath.Join(res, "icon.icns"))
	if err := os.Chmod(res, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(res, 0o755) })
	if removableByUser(app, m.UID) {
		t.Fatal("a read-only folder deep in the bundle was missed")
	}
}

func TestRunWithoutPurgeKeepsData(t *testing.T) {
	m, _ := newMachine(t, quietRunner())
	p, err := m.Plan(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(m.DataDir, "config.toml")) {
		t.Fatal("the data directory was deleted without --purge")
	}
}

func TestRunAsAdmin(t *testing.T) {
	r := quietRunner()
	var script, prompt string
	r.answer["osascript"] = func(args []string) ([]byte, error) {
		script, prompt = args[len(args)-2], args[len(args)-1]
		return nil, nil
	}
	m, app := newMachine(t, r)
	touch(t, m.Sudoers)
	p, err := m.Plan(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Run(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	want := "/bin/rm -f -- '" + m.Sudoers + "' && /bin/rm -rf -- '" + app + "'"
	if script != want {
		t.Fatalf("admin script = %q\nwant %q", script, want)
	}
	if prompt != adminPrompt {
		t.Fatalf("prompt = %q", prompt)
	}
	// Root's part is the administrator script's job, not the user's.
	if !exists(app) {
		t.Fatal("the app was removed outside the administrator script")
	}
	if !r.called("launchctl bootout") {
		t.Fatalf("the rest did not run after the administrator step: %v", r.calls)
	}
}

func TestRunStopsWhenAdminStepFails(t *testing.T) {
	tests := []struct {
		name   string
		out    string
		cancel bool
	}{
		{"user dismisses the prompt", "execution error: User canceled. (-128)", true},
		{"command fails", "execution error: rm: Operation not permitted (1)", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := quietRunner()
			r.answer["osascript"] = func([]string) ([]byte, error) { return []byte(tc.out), errExit }
			m, app := newMachine(t, r)
			touch(t, m.Sudoers)
			p, err := m.Plan(context.Background(), true)
			if err != nil {
				t.Fatal(err)
			}
			err = m.Run(context.Background(), p)
			if got := errors.Is(err, ErrCancelled); got != tc.cancel || err == nil {
				t.Fatalf("err = %v, want cancelled=%v", err, tc.cancel)
			}
			if r.called("launchctl") || r.called("pkill") || r.called("defaults") {
				t.Fatalf("kept going after the administrator step failed: %v", r.calls)
			}
			for _, kept := range []string{app, m.agentPlist(), m.DataDir} {
				if !exists(kept) {
					t.Fatalf("%s was removed", kept)
				}
			}
		})
	}
}

func TestStopEscalatesOnlyWhenNeeded(t *testing.T) {
	t.Run("exits on TERM", func(t *testing.T) {
		alive := true
		r := &fakeRunner{answer: map[string]func([]string) ([]byte, error){
			"pgrep": func([]string) ([]byte, error) {
				if alive {
					return []byte("42"), nil
				}
				return nil, errExit
			},
			"pkill": func([]string) ([]byte, error) { alive = false; return nil, nil },
		}}
		m := Machine{Runner: r, Sleep: func(time.Duration) {}}
		m.stop(context.Background(), "claudeqd", time.Second)
		if r.called("pkill -9") {
			t.Fatalf("killed a process that exited on TERM: %v", r.calls)
		}
	})
	t.Run("ignores TERM", func(t *testing.T) {
		r := &fakeRunner{}
		slept := time.Duration(0)
		m := Machine{Runner: r, Sleep: func(d time.Duration) { slept += d }}
		m.stop(context.Background(), "claudeqd", time.Second)
		if !r.called("pkill -9 -x claudeqd") {
			t.Fatalf("no SIGKILL for a process that ignored TERM: %v", r.calls)
		}
		// The grace period, then the wait for the killed process to be gone.
		if slept < time.Second+killWait-2*pollInterval {
			t.Fatalf("waited %v, want about the grace period plus %v", slept, killWait)
		}
		if last := r.calls[len(r.calls)-1]; last != "pgrep -x claudeqd" {
			t.Fatalf("did not look again after SIGKILL: %v", r.calls)
		}
	})
	t.Run("not running", func(t *testing.T) {
		r := quietRunner()
		m := Machine{Runner: r, Sleep: func(time.Duration) {}}
		m.stop(context.Background(), "claudeqapp", time.Second)
		if r.called("pkill") {
			t.Fatalf("signalled a process that is not running: %v", r.calls)
		}
	})
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/Users/o'neil/ClaudeQ.app"); got != `'/Users/o'\''neil/ClaudeQ.app'` {
		t.Fatalf("shellQuote = %s", got)
	}
}

func TestAlertPassesTextAsArguments(t *testing.T) {
	r := &fakeRunner{}
	if err := Alert(context.Background(), r, `Say "hi"`, "body", true); err != nil {
		t.Fatal(err)
	}
	want := `osascript -e on run argv -e display alert (item 1 of argv) message (item 2 of argv) as critical -e end run Say "hi" body`
	if r.calls[0] != want {
		t.Fatalf("call = %q\nwant %q", r.calls[0], want)
	}
}
