package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func TestCmdExportImportRoundTrip(t *testing.T) {
	withProviderHealth(t, providerReady)
	src := newTestStore(t)
	orig := baseTask()
	orig.Prompt = "# Sweep\n\nMulti-line prompt with “quotes”.\n"
	orig.Model = "opus"
	orig.NotifyOnResult = true
	if err := app.AddTask(src, orig); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "shared.claudeq")
	if err := cmdExport(src, []string{"nightly", "--out", out}); err != nil {
		t.Fatalf("cmdExport: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if read, _, err := bundle.Read(data); err != nil || read != orig {
		t.Errorf("exported file holds %+v (%v), want %+v", read, err, orig)
	}

	dst := newTestStore(t)
	if err := cmdImport(dst, []string{out}); err != nil {
		t.Fatalf("cmdImport: %v", err)
	}
	got, err := findTask(dst, "nightly")
	if err != nil {
		t.Fatal(err)
	}
	// The import names the local instance the file's hint resolved to, rather
	// than leaving the task to follow whatever the default happens to be.
	want := orig
	want.Provider = store.DefaultProviderID
	if got != want {
		t.Errorf("imported %+v, want %+v", got, want)
	}

	// Importing again keeps the first task and gives the copy a suffix; --id
	// picks the id outright.
	if err := cmdImport(dst, []string{out}); err != nil {
		t.Fatalf("second cmdImport: %v", err)
	}
	if _, err := findTask(dst, "nightly-2"); err != nil {
		t.Errorf("second import: %v", err)
	}
	if err := cmdImport(dst, []string{out, "--id", "renamed"}); err != nil {
		t.Fatalf("cmdImport --id: %v", err)
	}
	if renamed, err := findTask(dst, "renamed"); err != nil || renamed.Name != orig.Name {
		t.Errorf("--id import: %+v, %v", renamed, err)
	}
}

func TestCmdExportRefusesToOverwrite(t *testing.T) {
	st := newTestStore(t)
	if err := app.AddTask(st, baseTask()); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "x.claudeq")
	if err := cmdExport(st, []string{"nightly", "--out", out}); err != nil {
		t.Fatal(err)
	}
	err := cmdExport(st, []string{"nightly", "--out", out})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second export: err = %v", err)
	}
	if err := cmdExport(st, []string{"nightly", "--out", out, "--force"}); err != nil {
		t.Errorf("--force: %v", err)
	}
	if err := cmdExport(st, []string{"missing"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown task: err = %v", err)
	}
}

func TestExportPath(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ out, want string }{
		{"", "nightly.claudeq"},
		{dir, filepath.Join(dir, "nightly.claudeq")},
		{filepath.Join(dir, "custom.claudeq"), filepath.Join(dir, "custom.claudeq")},
		{filepath.Join(dir, "custom"), filepath.Join(dir, "custom.claudeq")},
	}
	for _, c := range cases {
		if got := exportPath(c.out, "nightly.claudeq"); got != c.want {
			t.Errorf("exportPath(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}

func TestCmdImportErrors(t *testing.T) {
	withProviderHealth(t, providerReady)
	st := newTestStore(t)
	bad := filepath.Join(t.TempDir(), "bad.claudeq")
	if err := os.WriteFile(bad, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(t.TempDir(), "huge.claudeq")
	if err := os.WriteFile(huge, make([]byte, bundle.MaxSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{},
		{"a", "b"},
		{"--id", "x", bad},
		{bad, "--id", "team/x"},
		{filepath.Join(t.TempDir(), "missing.claudeq")},
		{bad},
		{huge},
	}
	for _, args := range cases {
		if err := cmdImport(st, args); err == nil {
			t.Errorf("cmdImport(%q) succeeded", args)
		}
	}
	if got, _ := findTask(st, "nightly"); got.ID != "" {
		t.Errorf("a failed import stored a task: %+v", got)
	}
}

// TestCmdImportRefusesAnUnresolvedProvider: a task written for a harness this
// Mac cannot place is not queued on a guess. Importing onto the wrong account
// spends the wrong allowance, so the command says what the file wants.
func TestCmdImportRefusesAnUnresolvedProvider(t *testing.T) {
	st := newTestStore(t)
	path := writeBundle(t, bundle.ProviderHint{Kind: "codex", ProviderName: "Codex"})

	err := cmdImport(st, []string{path})
	if err == nil {
		t.Fatal("an unresolvable provider was imported anyway")
	}
	if !strings.Contains(err.Error(), "codex") || !strings.Contains(err.Error(), "--provider") {
		t.Fatalf("error = %v, want it to name the harness and the way out", err)
	}
	if tasks, _ := st.LoadConfig(); len(tasks.Tasks) != 0 {
		t.Fatalf("a task was queued anyway: %+v", tasks.Tasks)
	}
}

// TestCmdImportProviderOverride: --provider decides where the task lands, over
// whatever the file's hint would have resolved to, and the exporter's model is
// left behind with the account it was chosen for.
func TestCmdImportProviderOverride(t *testing.T) {
	st := newTestStore(t)
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Providers = append(cfg.Providers, store.Provider{
			ID: "claude-work", Kind: store.DefaultProviderKind, Name: "Claude (work)",
			BinaryPath: "/opt/claude-work", Enabled: true,
		})
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	path := writeBundle(t, bundle.ProviderHint{Kind: store.DefaultProviderKind, Model: "opus", ProviderName: "Claude"})

	withProviderHealth(t, providerReady)
	if err := cmdImport(st, []string{path, "--provider", "claude-work"}); err != nil {
		t.Fatalf("cmdImport --provider: %v", err)
	}
	got, err := findTask(st, "shared")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "claude-work" {
		t.Fatalf("provider = %q, want the override", got.Provider)
	}
	if got.Model != "" {
		t.Fatalf("model = %q, want the exporter's model left behind with its account", got.Model)
	}
}

// writeBundle writes a one-task bundle with the given hint and returns its path.
func writeBundle(t *testing.T, hint bundle.ProviderHint) string {
	t.Helper()
	tk := task.Task{ID: "shared", Name: "Shared", Prompt: "p", WorkingDir: t.TempDir(),
		Trigger: task.TriggerASAP, Enabled: true, Permissions: task.PermissionsDefault, Model: "opus"}
	var buf bytes.Buffer
	if err := bundle.Write(&buf, tk, hint, time.Now()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shared.claudeq")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
