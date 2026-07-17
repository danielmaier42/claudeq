package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const usageFile = "usage.json"

// Usage is the most recent rate-limit / usage snapshot reported by the Claude
// Code CLI (from a run's stream-json `rate_limit_info`). It is the closest
// programmatic equivalent to the interactive /usage panel.
type Usage struct {
	// Utilization is the fraction of the allowance used (1.0 == 100%).
	Utilization float64 `json:"utilization"`
	// Status is the CLI's status string (e.g. "allowed", "allowed_warning").
	Status string `json:"status"`
	// LimitType is the rate-limit type (e.g. "overage").
	LimitType string `json:"limit_type"`
	// IsUsingOverage indicates the account is currently in overage.
	IsUsingOverage bool `json:"is_using_overage"`
	// ResetsAt is when the current window resets (zero if unknown).
	ResetsAt time.Time `json:"resets_at"`
	// CapturedAt is when this snapshot was recorded.
	CapturedAt time.Time `json:"captured_at"`
}

// SaveUsage atomically writes the latest usage snapshot.
func (s *Store) SaveUsage(u Usage) error {
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return fmt.Errorf("encode usage: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(s.path(usageFile), data)
}

// LoadUsage returns the latest usage snapshot and whether one exists.
func (s *Store) LoadUsage() (Usage, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(usageFile))
	if errors.Is(err, os.ErrNotExist) {
		return Usage{}, false, nil
	}
	if err != nil {
		return Usage{}, false, fmt.Errorf("read usage: %w", err)
	}
	var u Usage
	if err := json.Unmarshal(data, &u); err != nil {
		return Usage{}, false, fmt.Errorf("parse usage: %w", err)
	}
	return u, true, nil
}
