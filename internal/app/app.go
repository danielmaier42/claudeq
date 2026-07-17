// Package app implements the store-level operations behind the claudeq CLI:
// task CRUD, reordering (priority), and run read-status. Keeping them here
// (rather than in main) makes them unit-testable.
package app

import (
	"fmt"

	"github.com/danielmaier42/claudeq/internal/store"
	"github.com/danielmaier42/claudeq/internal/task"
)

// AddTask appends a task and persists the config. IDs must be unique.
func AddTask(s *store.Store, t task.Task) error {
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	for _, existing := range cfg.Tasks {
		if existing.ID == t.ID {
			return fmt.Errorf("task %q already exists", t.ID)
		}
	}
	cfg.Tasks = append(cfg.Tasks, t)
	return s.SaveConfig(cfg)
}

// RemoveTask deletes a task by id.
func RemoveTask(s *store.Store, id string) error {
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	idx := indexOf(cfg.Tasks, id)
	if idx < 0 {
		return fmt.Errorf("task %q not found", id)
	}
	cfg.Tasks = append(cfg.Tasks[:idx], cfg.Tasks[idx+1:]...)
	return s.SaveConfig(cfg)
}

// SetEnabled activates or pauses a task without deleting it (FA-17).
func SetEnabled(s *store.Store, id string, enabled bool) error {
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	idx := indexOf(cfg.Tasks, id)
	if idx < 0 {
		return fmt.Errorf("task %q not found", id)
	}
	cfg.Tasks[idx].Enabled = enabled
	return s.SaveConfig(cfg)
}

// Move changes a task's position in the list, which is its priority: index 0 is
// highest (FA-11). The target index is clamped to the valid range.
func Move(s *store.Store, id string, to int) error {
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
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
	// Re-insert at the target index.
	cfg.Tasks = append(cfg.Tasks, task.Task{})
	copy(cfg.Tasks[to+1:], cfg.Tasks[to:])
	cfg.Tasks[to] = moved
	return s.SaveConfig(cfg)
}

// MarkRead marks a single run as read (FA-23).
func MarkRead(s *store.Store, runID string) error {
	st, err := s.LoadState()
	if err != nil {
		return err
	}
	st.MarkRead(runID)
	return s.SaveState(st)
}

// MarkAllRead marks every recorded run as read (FA-24).
func MarkAllRead(s *store.Store) error {
	runs, err := s.Runs()
	if err != nil {
		return err
	}
	st, err := s.LoadState()
	if err != nil {
		return err
	}
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.RunID
	}
	st.MarkAllRead(ids)
	return s.SaveState(st)
}

func indexOf(tasks []task.Task, id string) int {
	for i, t := range tasks {
		if t.ID == id {
			return i
		}
	}
	return -1
}
