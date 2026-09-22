package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// ErrNotFound is returned when no task has the requested id.
var ErrNotFound = errors.New("task not found")

// ExportTask writes the task with the given id to w as a .claudeq bundle.
//
// The bundle carries a hint about the harness the task was written for, never
// the local provider instance: a hint travels, an instance is one account on
// one Mac.
func ExportTask(s *store.Store, id string, w io.Writer, now time.Time) (task.Task, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return task.Task{}, err
	}
	idx := indexOf(cfg.Tasks, id)
	if idx < 0 {
		return task.Task{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	t := cfg.Tasks[idx]
	if err := bundle.Write(w, t, exportHint(cfg, t), now); err != nil {
		return task.Task{}, fmt.Errorf("export task %q: %w", id, err)
	}
	return t, nil
}

// exportHint describes what the task runs on here, in terms another machine can
// use. A configuration it cannot resolve yields no hint rather than a guess.
func exportHint(cfg store.Config, t task.Task) bundle.ProviderHint {
	// A script job runs on no harness, so there is nothing to tell the other
	// machine about — and a hint would make it import as an agent job's.
	if t.IsScript() {
		return bundle.ProviderHint{}
	}
	set, err := provider.FromConfig(cfg)
	if err != nil {
		return bundle.ProviderHint{}
	}
	res, err := set.Resolve(provider.Selection{ProviderID: t.Provider, Model: t.Model})
	if err != nil {
		return bundle.ProviderHint{}
	}
	return bundle.ProviderHint{
		Kind:         string(res.Instance.Kind),
		Model:        res.Model,
		ProviderName: res.Instance.Label(),
	}
}

// ImportTask adds a task read from a .claudeq bundle to the queue, settings
// as they are in the file. Only what the file cannot decide is filled in: a
// missing id is derived from the name, a missing name from the id, missing
// permissions mean "default", and an id already in use gets a numeric suffix
// (nightly, nightly-2, nightly-3, …) so importing never clobbers an existing
// task. Any scheduling state left behind by an earlier task with the final
// id is dropped, so the import starts fresh. The stored task is returned.
func ImportTask(s *store.Store, t task.Task) (task.Task, error) {
	t, err := completeImport(t)
	if err != nil {
		return task.Task{}, err
	}
	err = s.UpdateConfig(func(cfg *store.Config) error {
		t.ID = uniqueID(cfg.Tasks, t.ID)
		cfg.Tasks = append(cfg.Tasks, t)
		return nil
	})
	if err != nil {
		return task.Task{}, err
	}
	if err := forgetTaskState(s, t.ID); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

// uniqueID returns want if no task uses it, else the first free want-N (N ≥ 2).
func uniqueID(tasks []task.Task, want string) string {
	if indexOf(tasks, want) < 0 {
		return want
	}
	for n := 2; ; n++ {
		if id := want + "-" + strconv.Itoa(n); indexOf(tasks, id) < 0 {
			return id
		}
	}
}

// ImportDraft is a task read from a .claudeq bundle, prepared for review before
// anything is queued: the app prefills its task sheet with it so the importer
// can adjust the prompt and the paths, which come from the exporter's machine.
type ImportDraft struct {
	Task task.Task `json:"task"`
	// MissingWorkingDir is the working directory named in the file when there
	// is no such directory here. Task.WorkingDir is empty in that case, so the
	// importer has to point the task at a folder that exists on this machine.
	MissingWorkingDir string `json:"missing_working_dir,omitempty"`
	// UnresolvedProvider names the harness the file was written for when this
	// Mac has no single obvious instance of it. Task.Provider is empty then, so
	// the importer has to choose before the task can be queued.
	UnresolvedProvider string `json:"unresolved_provider,omitempty"`
}

// ReadImport turns a task from a bundle into a draft. The file is validated as
// strictly as an actual import, so a broken file is refused before it reaches
// the sheet; only the working directory and the provider are allowed to fall
// away — both are properties of the machine, not of the task.
func ReadImport(s *store.Store, t task.Task, hint bundle.ProviderHint) (ImportDraft, error) {
	t, err := completeImport(t)
	if err != nil {
		return ImportDraft{}, err
	}
	d := ImportDraft{Task: t}
	if !DirExists(t.WorkingDir) {
		d.MissingWorkingDir = t.WorkingDir
		d.Task.WorkingDir = ""
	}
	// A file with no hint was written before providers existed, or by a claudeq
	// that had nothing to say about them. It keeps the task exactly as it is and
	// runs on the default provider, which is what such a file always did. A
	// script job has no provider to resolve at all.
	if hint.Kind == "" || t.IsScript() {
		return d, nil
	}
	if match, ok := ResolveHint(s, hint); ok {
		d.Task.Provider, d.Task.Model = match.Provider, match.Model
	} else {
		d.UnresolvedProvider = hintLabel(hint)
		d.Task.Model = "" // a model belongs to the harness it was named for
	}
	return d, nil
}

// HintMatch is the local provider a bundle's hint resolved to.
type HintMatch struct {
	// Provider is the instance id to run on. Empty means the default provider,
	// which is what a bundle without a hint gets.
	Provider string
	// Model is the model to carry over, kept only because the resolved provider
	// is the kind the model was named for.
	Model string
}

// ResolveHint finds the local provider a bundle was written for, and reports
// whether the answer is unambiguous.
//
// One enabled instance of that kind is the answer. Several is not: claudeq does
// not pick an account on the operator's behalf, because the accounts are not
// interchangeable — they have separate allowances, separate logins and often
// separate employers. None is not either. In both cases the importer chooses.
//
// A bundle with no hint at all (written before this existed) resolves to the
// default provider, which is what such a file has always imported as.
func ResolveHint(s *store.Store, hint bundle.ProviderHint) (HintMatch, bool) {
	if hint.Kind == "" {
		return HintMatch{}, true
	}
	set, err := Providers(s)
	if err != nil {
		return HintMatch{}, false
	}
	var found []provider.Instance
	for _, inst := range set.All() {
		if inst.Enabled && string(inst.Kind) == hint.Kind {
			found = append(found, inst)
		}
	}
	if len(found) != 1 {
		return HintMatch{}, false
	}
	return HintMatch{Provider: found[0].ID, Model: hint.Model}, true
}

// hintLabel is how an unresolved provider is named to the operator: what the
// exporter called it, with the kind that is actually being looked for.
func hintLabel(hint bundle.ProviderHint) string {
	if hint.ProviderName != "" && hint.ProviderName != hint.Kind {
		return hint.ProviderName + " (" + hint.Kind + ")"
	}
	return hint.Kind
}

// completeImport fills in what a bundle cannot decide and validates the result:
// a missing id is derived from the name, a missing name from the id, and
// missing permissions mean "default".
func completeImport(t task.Task) (task.Task, error) {
	if t.ID == "" {
		t.ID = task.Slug(t.Name)
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	if t.Permissions == "" {
		t.Permissions = task.PermissionsDefault
	}
	if err := task.CheckID(t.ID); err != nil {
		return task.Task{}, err
	}
	if err := t.Validate(); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

// DirExists reports whether path is a directory on this machine. A path we are
// not allowed to look at counts as existing: the daemon may well lack access to
// a folder that is perfectly real, and dropping it then would be wrong.
func DirExists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, fs.ErrPermission)
	}
	return fi.IsDir()
}
