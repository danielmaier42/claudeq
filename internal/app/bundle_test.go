package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

func TestExportImportRoundTrip(t *testing.T) {
	src, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orig := task.Task{
		ID: "nightly", Name: "Nightly sweep", Prompt: "do the thing\n", WorkingDir: "/repo",
		Trigger: task.TriggerCron, Cron: "0 3 * * *", Parallel: true, Enabled: true,
		Model: "opus", Permissions: task.PermissionsSkip, NotifyOnResult: true,
	}
	if err := AddTask(src, orig); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	exported, err := ExportTask(src, "nightly", &buf, time.Now())
	if err != nil {
		t.Fatalf("ExportTask: %v", err)
	}
	if exported != orig {
		t.Errorf("ExportTask returned %+v", exported)
	}
	if _, err := ExportTask(src, "nope", &buf, time.Now()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("unknown id: err = %v", err)
	}

	dst, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	read, _, err := bundle.Read(buf.Bytes())
	if err != nil {
		t.Fatalf("bundle.Read: %v", err)
	}
	got, err := ImportTask(dst, read)
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	if got != orig {
		t.Errorf("imported task differs:\n got %+v\nwant %+v", got, orig)
	}
	cfg, err := dst.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tasks) != 1 || cfg.Tasks[0] != orig {
		t.Errorf("stored tasks = %+v", cfg.Tasks)
	}
}

func TestImportTaskSuffixesTakenIDs(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := task.Task{ID: "nightly", Name: "N", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP, Permissions: task.PermissionsDefault}
	var ids []string
	for i := 0; i < 3; i++ {
		got, err := ImportTask(s, base)
		if err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
		ids = append(ids, got.ID)
	}
	if want := "nightly nightly-2 nightly-3"; strings.Join(ids, " ") != want {
		t.Errorf("ids = %q, want %q", strings.Join(ids, " "), want)
	}
}

func TestImportTaskFillsOnlyTheGaps(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ImportTask(s, task.Task{Name: "Weekly Report", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP, Enabled: false})
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	if got.ID != "weekly-report" || got.Permissions != task.PermissionsDefault || got.Enabled {
		t.Errorf("got %+v", got)
	}
	got, err = ImportTask(s, task.Task{ID: "only-id", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP})
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	if got.Name != "only-id" {
		t.Errorf("name = %q, want the id", got.Name)
	}
}

func TestImportTaskRejectsInvalid(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]task.Task{
		"no prompt":      {ID: "a", WorkingDir: "/r", Trigger: task.TriggerASAP},
		"no working dir": {ID: "a", Prompt: "p", Trigger: task.TriggerASAP},
		"bad trigger":    {ID: "a", Prompt: "p", WorkingDir: "/r", Trigger: "sometimes"},
		"bad cron":       {ID: "a", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerCron, Cron: "nope"},
		"unsafe id":      {ID: "team/nightly", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP},
		"dot id":         {ID: "..", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP},
	}
	for name, in := range cases {
		if _, err := ImportTask(s, in); err == nil {
			t.Errorf("%s: import succeeded", name)
		}
	}
	cfg, _ := s.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Errorf("invalid imports were stored: %+v", cfg.Tasks)
	}
}

// A re-imported id must not inherit a dead task's scheduling state: a
// completed-once flag would keep the new one-shot task from ever running, and
// a stale cron anchor would fire it immediately.
func TestImportTaskStartsFromCleanState(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stale := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := s.UpdateState(func(st *store.State) error {
		st.MarkCompletedOnce("nightly")
		st.RecordStart("nightly", stale)
		st.SetPendingResume("nightly", "sess", "claude")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ImportTask(s, task.Task{ID: "nightly", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP})
	if err != nil {
		t.Fatalf("ImportTask: %v", err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	_, resuming := st.PendingResume(got.ID)
	if _, ok := st.LastStart(got.ID); ok || st.IsCompletedOnce(got.ID) || resuming {
		t.Errorf("stale state survived the import: %+v", st)
	}
}

func TestReadImportKeepsAnExistingWorkingDir(t *testing.T) {
	st := openStore(t)
	dir := t.TempDir()
	in := task.Task{Name: "Shared", Prompt: "p", WorkingDir: dir, Trigger: task.TriggerASAP}
	d, err := ReadImport(st, in, bundle.ProviderHint{})
	if err != nil {
		t.Fatal(err)
	}
	if d.MissingWorkingDir != "" {
		t.Errorf("missing = %q for an existing folder", d.MissingWorkingDir)
	}
	// The gaps a file can leave open are filled, everything else is untouched.
	want := in
	want.ID, want.Permissions = "shared", task.PermissionsDefault
	if d.Task != want {
		t.Errorf("draft %+v, want %+v", d.Task, want)
	}
}

func TestReadImportDropsAWorkingDirThatIsNotHere(t *testing.T) {
	st := openStore(t)
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"gone":     filepath.Join(t.TempDir(), "no-such-dir"),
		"a file":   file,
		"below it": filepath.Join(file, "sub"),
	}
	for name, dir := range cases {
		d, err := ReadImport(st, task.Task{ID: "shared", Prompt: "p", WorkingDir: dir, Trigger: task.TriggerASAP}, bundle.ProviderHint{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d.Task.WorkingDir != "" || d.MissingWorkingDir != dir {
			t.Errorf("%s: working_dir = %q, missing = %q", name, d.Task.WorkingDir, d.MissingWorkingDir)
		}
	}
}

// The file itself is validated as strictly as on a real import — only the
// working directory may fall away.
func TestReadImportRejectsInvalid(t *testing.T) {
	st := openStore(t)
	cases := map[string]task.Task{
		"no prompt":      {ID: "a", WorkingDir: "/r", Trigger: task.TriggerASAP},
		"no working dir": {ID: "a", Prompt: "p", Trigger: task.TriggerASAP},
		"bad cron":       {ID: "a", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerCron, Cron: "nope"},
		"unsafe id":      {ID: "team/nightly", Prompt: "p", WorkingDir: "/r", Trigger: task.TriggerASAP},
	}
	for name, in := range cases {
		if _, err := ReadImport(st, in, bundle.ProviderHint{}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestExportCarriesAProviderHintNotAnInstance: a provider id names one account
// on one Mac, so it cannot travel. What the file says instead is which harness
// the task was written for.
func TestExportCarriesAProviderHintNotAnInstance(t *testing.T) {
	st := openStore(t)
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Providers[0].Name = "Claude (personal)"
		cfg.Providers[0].DefaultModel = "opus"
		cfg.Tasks = []task.Task{{
			ID: "nightly", Name: "Nightly", Prompt: "p", WorkingDir: "/repo",
			Trigger: task.TriggerASAP, Enabled: true, Permissions: task.PermissionsDefault,
			Provider: store.DefaultProviderID,
		}}
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	var buf bytes.Buffer
	if _, err := ExportTask(st, "nightly", &buf, time.Now()); err != nil {
		t.Fatalf("ExportTask: %v", err)
	}
	if strings.Contains(buf.String(), store.DefaultProviderID+`"`) {
		t.Error("the bundle names a local provider instance")
	}
	_, hint, err := bundle.Read(buf.Bytes())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := bundle.ProviderHint{Kind: store.DefaultProviderKind, Model: "opus", ProviderName: "Claude (personal)"}
	if hint != want {
		t.Fatalf("hint = %+v, want %+v", hint, want)
	}
}

// TestImportResolvesTheHintToTheOneLocalInstance: one instance of that kind is
// an unambiguous answer, so the import takes it — with the model, which belongs
// to that kind.
func TestImportResolvesTheHintToTheOneLocalInstance(t *testing.T) {
	st := openStore(t)
	dir := t.TempDir()
	in := task.Task{ID: "shared", Prompt: "p", WorkingDir: dir, Trigger: task.TriggerASAP}
	hint := bundle.ProviderHint{Kind: store.DefaultProviderKind, Model: "opus", ProviderName: "Claude (theirs)"}

	d, err := ReadImport(st, in, hint)
	if err != nil {
		t.Fatalf("ReadImport: %v", err)
	}
	if d.UnresolvedProvider != "" {
		t.Fatalf("unresolved = %q, want the one local instance to be taken", d.UnresolvedProvider)
	}
	if d.Task.Provider != store.DefaultProviderID || d.Task.Model != "opus" {
		t.Fatalf("draft provider = %q model = %q", d.Task.Provider, d.Task.Model)
	}
}

// TestImportLeavesAnAmbiguousProviderToTheOperator: two accounts of the same
// kind are not interchangeable — separate allowances, separate logins, often
// separate employers — so claudeq does not choose one on their behalf.
func TestImportLeavesAnAmbiguousProviderToTheOperator(t *testing.T) {
	st := openStore(t)
	if err := st.UpdateConfig(func(cfg *store.Config) error {
		cfg.Providers = append(cfg.Providers, store.Provider{
			ID: "claude-work", Kind: store.DefaultProviderKind, Name: "Claude (work)", Enabled: true,
		})
		return nil
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	dir := t.TempDir()
	in := task.Task{ID: "shared", Prompt: "p", WorkingDir: dir, Trigger: task.TriggerASAP}
	hint := bundle.ProviderHint{Kind: store.DefaultProviderKind, Model: "opus", ProviderName: "Claude (theirs)"}

	d, err := ReadImport(st, in, hint)
	if err != nil {
		t.Fatalf("ReadImport: %v", err)
	}
	if d.UnresolvedProvider == "" {
		t.Fatal("two matching accounts were resolved to one anyway")
	}
	if d.Task.Provider != "" {
		t.Fatalf("provider = %q, want the choice left open", d.Task.Provider)
	}
	if d.Task.Model != "" {
		t.Fatalf("model = %q, want it dropped with the provider it belonged to", d.Task.Model)
	}
}

// TestImportOfAnUnknownHarnessSaysSo: a task written for a harness this Mac has
// no provider for cannot be placed, and says which one it wants.
func TestImportOfAnUnknownHarnessSaysSo(t *testing.T) {
	st := openStore(t)
	dir := t.TempDir()
	in := task.Task{ID: "shared", Prompt: "p", WorkingDir: dir, Trigger: task.TriggerASAP}

	d, err := ReadImport(st, in, bundle.ProviderHint{Kind: "codex", ProviderName: "Codex"})
	if err != nil {
		t.Fatalf("ReadImport: %v", err)
	}
	if !strings.Contains(d.UnresolvedProvider, "codex") {
		t.Fatalf("unresolved = %q, want it to name the harness", d.UnresolvedProvider)
	}
}

// TestImportOfAHintlessBundleChangesNothing: a file from before hints existed
// keeps the task exactly as it is and runs on the default provider.
func TestImportOfAHintlessBundleChangesNothing(t *testing.T) {
	st := openStore(t)
	dir := t.TempDir()
	in := task.Task{ID: "shared", Prompt: "p", WorkingDir: dir, Trigger: task.TriggerASAP, Model: "opus"}

	d, err := ReadImport(st, in, bundle.ProviderHint{})
	if err != nil {
		t.Fatalf("ReadImport: %v", err)
	}
	if d.UnresolvedProvider != "" || d.Task.Provider != "" || d.Task.Model != "opus" {
		t.Fatalf("draft = %+v, want the task untouched", d.Task)
	}
}
