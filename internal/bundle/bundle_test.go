package bundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

func fullTask() task.Task {
	return task.Task{
		ID: "nightly-sweep", Name: "Nightly sweep",
		Prompt:     "# Brief\n\nLook at *everything*.\n\n- with lists\n- and ümlauts\r\nand CRLF\n",
		WorkingDir: "/Users/someone/repo",
		Trigger:    task.TriggerCron, Cron: "0 3 * * 1-5",
		FixedAt:  time.Date(2026, 9, 1, 2, 0, 0, 0, time.UTC),
		Parallel: true, Enabled: false,
		Model: "opus", Permissions: task.PermissionsSkip, NotifyOnResult: true,
	}
}

// build assembles a zip with the given entries, for hand-crafted bundles.
func build(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func settingsJSON(t *testing.T, mutate func(m map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"format": Format, "format_version": Version,
		"task": map[string]any{"id": "x", "name": "X", "working_dir": "/r", "trigger": "asap", "enabled": true, "permissions": "default"},
	}
	if mutate != nil {
		mutate(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRoundTripKeepsEveryField(t *testing.T) {
	want := fullTask()
	var buf bytes.Buffer
	if err := Write(&buf, want, time.Date(2026, 9, 7, 10, 30, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !got.FixedAt.Equal(want.FixedAt) {
		t.Errorf("fixed_at = %v, want %v", got.FixedAt, want.FixedAt)
	}
	got.FixedAt, want.FixedAt = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("round trip changed the task:\n got %+v\nwant %+v", got, want)
	}
}

func TestWriteLayout(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, fullTask(), time.Date(2026, 9, 7, 10, 30, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("entries = %d, want 2 (task.json + prompt.md)", len(zr.File))
	}

	meta, err := entry(zr, settingsName)
	if err != nil {
		t.Fatalf("task.json: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(meta, &raw); err != nil {
		t.Fatalf("task.json is not JSON: %v", err)
	}
	if string(raw["format"]) != `"claudeq-task"` || string(raw["format_version"]) != "1" {
		t.Errorf("envelope = %s", meta)
	}
	if string(raw["exported_at"]) != `"2026-09-07T10:30:00Z"` {
		t.Errorf("exported_at = %s", raw["exported_at"])
	}
	var ts map[string]any
	if err := json.Unmarshal(raw["task"], &ts); err != nil {
		t.Fatalf("task object: %v", err)
	}
	if _, has := ts["prompt"]; has {
		t.Errorf("task.json must not carry the prompt: %s", raw["task"])
	}
	for _, key := range []string{"id", "name", "working_dir", "trigger", "cron", "parallel", "enabled", "model", "permissions", "notify_on_result"} {
		if _, has := ts[key]; !has {
			t.Errorf("task.json lacks %q", key)
		}
	}

	prompt, err := entry(zr, promptName)
	if err != nil {
		t.Fatalf("prompt.md: %v", err)
	}
	if string(prompt) != fullTask().Prompt {
		t.Errorf("prompt.md = %q", prompt)
	}
}

func TestReadRejectsBrokenBundles(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"not a zip", []byte("hello"), "not a zip archive"},
		{"missing task.json", build(t, map[string]string{promptName: "p"}), "missing task.json"},
		{"missing prompt.md", build(t, map[string]string{settingsName: settingsJSON(t, nil)}), "missing prompt.md"},
		{"empty prompt", build(t, map[string]string{settingsName: settingsJSON(t, nil), promptName: ""}), "prompt.md is empty"},
		{"nested two levels", build(t, map[string]string{"a/b/" + settingsName: settingsJSON(t, nil), "a/b/" + promptName: "p"}), "missing task.json"},
		{"malformed json", build(t, map[string]string{settingsName: "{", promptName: "p"}), "task.json"},
		{"wrong format", build(t, map[string]string{
			settingsName: settingsJSON(t, func(m map[string]any) { m["format"] = "something-else" }), promptName: "p",
		}), `format "something-else"`},
		{"newer version", build(t, map[string]string{
			settingsName: settingsJSON(t, func(m map[string]any) { m["format_version"] = 2 }), promptName: "p",
		}), "format_version 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Read(c.data)
			if err == nil {
				t.Fatal("Read succeeded, want an error")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error %v is not ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

// A hand-edited task.json with a "prompt" key must not win over prompt.md,
// and unknown entries in the archive are ignored.
func TestReadPromptComesFromPromptMD(t *testing.T) {
	data := build(t, map[string]string{
		settingsName: settingsJSON(t, func(m map[string]any) { m["task"].(map[string]any)["prompt"] = "stray" }),
		promptName:   "the real prompt",
		"README.txt": "ignored",
	})
	got, err := Read(data)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Prompt != "the real prompt" || got.ID != "x" || got.Trigger != task.TriggerASAP {
		t.Errorf("got %+v", got)
	}
}

func TestReadKeepsFileAsIs(t *testing.T) {
	data := build(t, map[string]string{
		settingsName: settingsJSON(t, func(m map[string]any) {
			ts := m["task"].(map[string]any)
			delete(ts, "id")
			delete(ts, "permissions")
			ts["enabled"] = false
		}),
		promptName: "p",
	})
	got, err := Read(data)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.ID != "" || got.Permissions != "" || got.Enabled {
		t.Errorf("Read invented values: %+v", got)
	}
}

// A bundle unpacked with Finder and re-zipped with "Compress" carries a folder
// around the entries plus __MACOSX resource forks; it must still read.
func TestReadAcceptsFinderRezip(t *testing.T) {
	data := build(t, map[string]string{
		"sweep/":                     "",
		"sweep/" + settingsName:      settingsJSON(t, nil),
		"sweep/" + promptName:        "nested prompt",
		"__MACOSX/sweep/._task.json": "junk",
		"__MACOSX/sweep/._prompt.md": "junk",
		"__MACOSX/sweep/.DS_Store":   "junk",
	})
	got, err := Read(data)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.ID != "x" || got.Prompt != "nested prompt" {
		t.Errorf("got %+v", got)
	}
	// With both a root and a nested entry, the root one wins.
	data = build(t, map[string]string{
		settingsName: settingsJSON(t, nil), promptName: "root prompt",
		"old/" + settingsName: settingsJSON(t, nil), "old/" + promptName: "nested prompt",
	})
	if got, err := Read(data); err != nil || got.Prompt != "root prompt" {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestReadRejectsOversizedInput(t *testing.T) {
	if _, err := Read(make([]byte, MaxSize+1)); err == nil || !errors.Is(err, ErrInvalid) {
		t.Errorf("oversized bundle: %v", err)
	}
	// An entry that inflates past the limit is rejected from its header,
	// before any inflating happens; the error names the entry.
	big := build(t, map[string]string{settingsName: settingsJSON(t, nil), promptName: strings.Repeat("x", MaxSize+1)})
	if len(big) > 64<<10 {
		t.Fatalf("test bundle should compress well, got %d bytes", len(big))
	}
	_, err := Read(big)
	if err == nil || !strings.Contains(err.Error(), "prompt.md exceeds") {
		t.Errorf("oversized entry: %v", err)
	}
}

func TestEnsureExt(t *testing.T) {
	cases := []struct {
		in, want string
		appended bool
	}{
		{"x.claudeq", "x.claudeq", false},
		{"X.CLAUDEQ", "X.CLAUDEQ", false},
		{"x", "x.claudeq", true},
		{"x.zip", "x.zip.claudeq", true},
	}
	for _, c := range cases {
		if got, appended := EnsureExt(c.in); got != c.want || appended != c.appended {
			t.Errorf("EnsureExt(%q) = (%q, %v), want (%q, %v)", c.in, got, appended, c.want, c.appended)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "t.claudeq")
	var buf bytes.Buffer
	if err := Write(&buf, fullTask(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, buf.Bytes(), false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Save(p, []byte("x"), false); !errors.Is(err, fs.ErrExist) {
		t.Errorf("second Save without overwrite: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != fullTask().ID || got.Prompt != fullTask().Prompt {
		t.Errorf("Load returned %+v", got)
	}
	if err := Save(p, buf.Bytes(), true); err != nil {
		t.Errorf("Save with overwrite: %v", err)
	}

	if _, err := Load(filepath.Join(dir, "missing.claudeq")); err == nil {
		t.Error("Load of a missing file succeeded")
	}
	huge := filepath.Join(dir, "huge.claudeq")
	if err := os.WriteFile(huge, make([]byte, MaxSize+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(huge); err == nil || !errors.Is(err, ErrInvalid) {
		t.Errorf("Load of an oversized file: %v", err)
	}
}

func TestFileName(t *testing.T) {
	cases := map[string]string{"nightly": "nightly.claudeq", "": "task.claudeq", "a/b:c": "a-b-c.claudeq", "  ": "task.claudeq"}
	for id, want := range cases {
		if got := FileName(task.Task{ID: id}); got != want {
			t.Errorf("FileName(%q) = %q, want %q", id, got, want)
		}
	}
}
