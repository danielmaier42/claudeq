package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func bundleOf(t *testing.T, tk task.Task) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := bundle.Write(&buf, tk, time.Now()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newServerDeps is newServer for tests that need more than a store and runner.
func newServerDeps(t *testing.T, d Deps) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(Handler(d))
	t.Cleanup(srv.Close)
	return srv
}

func upload(t *testing.T, srv *httptest.Server, data []byte) resp {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/tasks/import", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/zip")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Body.Close() }()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r.Body)
	return resp{Status: r.StatusCode, Body: buf.Bytes()}
}

func TestImportTaskEndpointReadsWithoutQueueing(t *testing.T) {
	srv, st := newServer(t, nil)
	dir := t.TempDir()

	shared := task.Task{ID: "shared", Name: "Shared", Prompt: "p", WorkingDir: dir,
		Trigger: task.TriggerCron, Cron: "0 4 * * *", Model: "opus", Permissions: task.PermissionsSkip, Enabled: true}

	r := upload(t, srv, bundleOf(t, shared))
	if r.Status != http.StatusOK {
		t.Fatalf("import: %d %s", r.Status, r.Body)
	}
	var got app.ImportDraft
	r.into(t, &got)
	if got.Task != shared {
		t.Errorf("draft %+v, want %+v", got.Task, shared)
	}
	if got.MissingWorkingDir != "" {
		t.Errorf("missing_working_dir = %q for an existing folder", got.MissingWorkingDir)
	}

	// The file is only read: adding the task is the sheet's job afterwards.
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Errorf("import queued %d tasks", len(cfg.Tasks))
	}
}

// A working directory from the exporter's machine that is not here must not
// reach the sheet: the field stays empty so the importer picks a real folder.
func TestImportTaskEndpointDropsMissingWorkingDir(t *testing.T) {
	srv, _ := newServer(t, nil)
	gone := filepath.Join(t.TempDir(), "no-such-dir")

	r := upload(t, srv, bundleOf(t, task.Task{ID: "shared", Name: "Shared", Prompt: "p",
		WorkingDir: gone, Trigger: task.TriggerASAP, Permissions: task.PermissionsDefault}))
	if r.Status != http.StatusOK {
		t.Fatalf("import: %d %s", r.Status, r.Body)
	}
	var got app.ImportDraft
	r.into(t, &got)
	if got.Task.WorkingDir != "" {
		t.Errorf("working_dir = %q, want empty", got.Task.WorkingDir)
	}
	if got.MissingWorkingDir != gone {
		t.Errorf("missing_working_dir = %q, want %q", got.MissingWorkingDir, gone)
	}
	if got.Task.ID != "shared" || got.Task.Prompt != "p" {
		t.Errorf("rest of the draft lost: %+v", got.Task)
	}
}

func TestImportTaskEndpointRejectsBadFiles(t *testing.T) {
	srv, st := newServer(t, nil)
	cases := map[string][]byte{
		"not a zip":    []byte("nope"),
		"invalid task": bundleOf(t, task.Task{ID: "x", Prompt: "p", WorkingDir: "/r", Trigger: "never"}),
		"unsafe id":    bundleOf(t, task.Task{ID: "team/nightly", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP}),
	}
	for name, data := range cases {
		if r := upload(t, srv, data); r.Status != http.StatusBadRequest {
			t.Errorf("%s: status %d (%s)", name, r.Status, r.Body)
		}
	}
	if r := upload(t, srv, bytes.Repeat([]byte("x"), bundle.MaxSize+1)); r.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized: status %d", r.Status)
	}
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Errorf("bad files were stored: %+v", cfg.Tasks)
	}
}

func TestExportTaskEndpoint(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig := sampleTask("nightly")
	orig.Name = "Nightly “quoted”"
	if err := app.AddTask(st, orig); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var gotPrompt, gotDefault string
	dialog := func(_ context.Context, prompt, defaultName string) (string, bool, error) {
		gotPrompt, gotDefault = prompt, defaultName
		return filepath.Join(dir, "chosen.claudeq"), true, nil
	}
	srv := newServerDeps(t, Deps{Store: st, SaveFile: dialog})

	r := do(t, srv, http.MethodPost, "/api/tasks/nightly/export", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("export: %d %s", r.Status, r.Body)
	}
	var out map[string]string
	r.into(t, &out)
	if out["path"] != filepath.Join(dir, "chosen.claudeq") {
		t.Errorf("path = %q", out["path"])
	}
	if gotDefault != "nightly.claudeq" || !strings.Contains(gotPrompt, orig.Name) {
		t.Errorf("dialog got prompt %q default %q", gotPrompt, gotDefault)
	}
	data, err := os.ReadFile(out["path"])
	if err != nil {
		t.Fatal(err)
	}
	read, err := bundle.Read(data)
	if err != nil {
		t.Fatalf("written file is not a bundle: %v", err)
	}
	if read != orig {
		t.Errorf("file holds %+v, want %+v", read, orig)
	}

	if r := do(t, srv, http.MethodPost, "/api/tasks/missing/export", nil); r.Status != http.StatusNotFound {
		t.Errorf("unknown task: %d", r.Status)
	}
}

func TestExportTaskEndpointDialogOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		dialog SaveFileDialog
		want   int
	}{
		{"no dialog", nil, http.StatusServiceUnavailable},
		{"cancelled", func(context.Context, string, string) (string, bool, error) { return "", false, nil }, http.StatusNoContent},
		{"dialog error", func(context.Context, string, string) (string, bool, error) { return "", false, errors.New("boom") }, http.StatusBadGateway},
		{"unwritable path", func(context.Context, string, string) (string, bool, error) {
			return filepath.Join(t.TempDir(), "gone", "x.claudeq"), true, nil
		}, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := app.AddTask(st, sampleTask("a")); err != nil {
				t.Fatal(err)
			}
			srv := newServerDeps(t, Deps{Store: st, SaveFile: c.dialog})
			if r := do(t, srv, http.MethodPost, "/api/tasks/a/export", nil); r.Status != c.want {
				t.Errorf("status %d, want %d (%s)", r.Status, c.want, r.Body)
			}
		})
	}
}

func TestExportTaskEndpointDoesNotClobberUnseenName(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.AddTask(st, sampleTask("a")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain.claudeq"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The panel returned "plain" (no extension): "plain.claudeq" exists but
	// was never shown, so it must survive and the user gets told.
	srv := newServerDeps(t, Deps{Store: st, SaveFile: func(context.Context, string, string) (string, bool, error) {
		return filepath.Join(dir, "plain"), true, nil
	}})
	r := do(t, srv, http.MethodPost, "/api/tasks/a/export", nil)
	if r.Status != http.StatusInternalServerError || !strings.Contains(string(r.Body), "already exists") {
		t.Errorf("status %d body %s", r.Status, r.Body)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "plain.claudeq")); string(data) != "keep" {
		t.Errorf("unseen file was overwritten: %q", data)
	}
	// The panel returned the full name: it confirmed replacing, so overwrite.
	srv = newServerDeps(t, Deps{Store: st, SaveFile: func(context.Context, string, string) (string, bool, error) {
		return filepath.Join(dir, "plain.claudeq"), true, nil
	}})
	if r := do(t, srv, http.MethodPost, "/api/tasks/a/export", nil); r.Status != http.StatusOK {
		t.Fatalf("status %d body %s", r.Status, r.Body)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "plain.claudeq"))
	if _, err := bundle.Read(data); err != nil {
		t.Errorf("file was not replaced with a bundle: %v", err)
	}
}

func TestOSAScriptSaveFileDialog(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		err     error
		path    string
		chosen  bool
		wantErr bool
	}{
		{"chosen", "/Users/me/Desktop/nightly.claudeq\n", nil, "/Users/me/Desktop/nightly.claudeq", true, false},
		{"cancelled", "execution error: User canceled. (-128)", errors.New("exit 1"), "", false, false},
		{"failure", "execution error: something else (-1700)", errors.New("exit 1"), "", false, true},
		{"empty", "", nil, "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{out: []byte(c.out), err: c.err}
			path, chosen, err := OSAScriptSaveFileDialog(fr)(context.Background(), `Export "quoted" task`, "nightly.claudeq")
			if (err != nil) != c.wantErr || chosen != c.chosen || path != c.path {
				t.Errorf("got (%q, %v, %v)", path, chosen, err)
			}
			if fr.name != "osascript" {
				t.Fatalf("ran %q", fr.name)
			}
			// The user strings travel as run arguments, verbatim, never
			// interpolated into the script text.
			n := len(fr.args)
			if n < 2 || fr.args[n-2] != `Export "quoted" task` || fr.args[n-1] != "nightly.claudeq" {
				t.Errorf("args = %q", fr.args)
			}
			script := strings.Join(fr.args[:n-2], "\n")
			if !strings.Contains(script, "on run argv") || !strings.Contains(script, "choose file name with prompt (item 1 of argv) default name (item 2 of argv)") {
				t.Errorf("script = %s", script)
			}
			if strings.Contains(script, "quoted") {
				t.Errorf("user string leaked into the script text: %s", script)
			}
		})
	}
}
