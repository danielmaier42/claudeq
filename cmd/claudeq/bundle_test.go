package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/bundle"
)

func TestCmdExportImportRoundTrip(t *testing.T) {
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
	if read, err := bundle.Read(data); err != nil || read != orig {
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
	if got != orig {
		t.Errorf("imported %+v, want %+v", got, orig)
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
