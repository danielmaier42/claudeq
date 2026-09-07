package app

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// ErrNotFound is returned when no task has the requested id.
var ErrNotFound = errors.New("task not found")

// ExportTask writes the task with the given id to w as a .claudeq bundle.
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
	if err := bundle.Write(w, t, now); err != nil {
		return task.Task{}, fmt.Errorf("export task %q: %w", id, err)
	}
	return t, nil
}

// ImportTask adds a task read from a .claudeq bundle to the queue, settings
// as they are in the file. Only what the file cannot decide is filled in: a
// missing id is derived from the name, a missing name from the id, missing
// permissions mean "default", and an id already in use gets a numeric suffix
// (nightly, nightly-2, nightly-3, …) so importing never clobbers an existing
// task. Any scheduling state left behind by an earlier task with the final
// id is dropped, so the import starts fresh. The stored task is returned.
func ImportTask(s *store.Store, t task.Task) (task.Task, error) {
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
	err := s.UpdateConfig(func(cfg *store.Config) error {
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
