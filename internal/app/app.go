// Package app implements the store-level operations behind the claudeq CLI and
// HTTP API: task CRUD, reordering (priority), and run read-status. Mutations go
// through the store's atomic update helpers so concurrent callers (CLI + API)
// never lose each other's changes.
package app

import (
	"fmt"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// AddTask appends a task and persists the config. IDs must be unique.
func AddTask(s *store.Store, t task.Task) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		if indexOf(cfg.Tasks, t.ID) >= 0 {
			return fmt.Errorf("task %q already exists", t.ID)
		}
		cfg.Tasks = append(cfg.Tasks, t)
		return nil
	})
}

// RemoveTask deletes a task by id, along with its scheduling state, so the id
// can be reused (by a re-import, say) without inheriting the old task's
// completed-once flag or cron anchor.
func RemoveTask(s *store.Store, id string) error {
	err := s.UpdateConfig(func(cfg *store.Config) error {
		idx := indexOf(cfg.Tasks, id)
		if idx < 0 {
			return fmt.Errorf("task %q not found", id)
		}
		cfg.Tasks = append(cfg.Tasks[:idx], cfg.Tasks[idx+1:]...)
		return nil
	})
	if err != nil {
		return err
	}
	return forgetTaskState(s, id)
}

// forgetTaskState clears the scheduling bookkeeping kept for a task id.
func forgetTaskState(s *store.Store, id string) error {
	return s.UpdateState(func(st *store.State) error {
		st.ForgetTask(id)
		return nil
	})
}

// EditTask applies a change to one existing task in place. The mutation runs
// inside the store's atomic update, so a concurrent edit from the app or another
// CLI call is never clobbered. The task id is fixed: apply must not change it.
func EditTask(s *store.Store, id string, apply func(*task.Task) error) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		idx := indexOf(cfg.Tasks, id)
		if idx < 0 {
			return fmt.Errorf("task %q not found", id)
		}
		edited := cfg.Tasks[idx]
		if err := apply(&edited); err != nil {
			return err
		}
		if edited.ID != id {
			return fmt.Errorf("task id cannot be changed (%q -> %q)", id, edited.ID)
		}
		cfg.Tasks[idx] = edited
		return nil
	})
}

// SetEnabled activates or pauses a task without deleting it (FA-17).
func SetEnabled(s *store.Store, id string, enabled bool) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		idx := indexOf(cfg.Tasks, id)
		if idx < 0 {
			return fmt.Errorf("task %q not found", id)
		}
		cfg.Tasks[idx].Enabled = enabled
		return nil
	})
}

// SetPaused flips the global pause switch: while it is on, the daemon starts no
// run at all (see store.Settings.Paused). Only that one field is touched, so a
// concurrent settings change is not clobbered.
func SetPaused(s *store.Store, paused bool) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		cfg.Settings.Paused = paused
		return nil
	})
}

// Move changes a task's position in the list, which is its priority: index 0 is
// highest (FA-11). The target index is clamped to the valid range.
func Move(s *store.Store, id string, to int) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		from := indexOf(cfg.Tasks, id)
		if from < 0 {
			return fmt.Errorf("task %q not found", id)
		}
		if to < 0 {
			to = 0
		}
		if to > len(cfg.Tasks)-1 {
			to = len(cfg.Tasks) - 1
		}
		moved := cfg.Tasks[from]
		cfg.Tasks = append(cfg.Tasks[:from], cfg.Tasks[from+1:]...)
		cfg.Tasks = append(cfg.Tasks, task.Task{})
		copy(cfg.Tasks[to+1:], cfg.Tasks[to:])
		cfg.Tasks[to] = moved
		return nil
	})
}

// MarkRead marks a single run as read (FA-23).
func MarkRead(s *store.Store, runID string) error {
	return s.UpdateState(func(st *store.State) error {
		st.MarkRead(runID)
		return nil
	})
}

// MarkAllRead marks every recorded run as read (FA-24).
func MarkAllRead(s *store.Store) error {
	runs, err := s.Runs()
	if err != nil {
		return err
	}
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.RunID
	}
	return s.UpdateState(func(st *store.State) error {
		st.MarkAllRead(ids)
		return nil
	})
}

func indexOf(tasks []task.Task, id string) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}
