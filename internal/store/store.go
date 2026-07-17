// Package store persists claudeq configuration, run history, and read-state
// on disk in human-readable formats (PLAN.md D5/D10):
//
//   - config.toml    global settings + the ordered task list (TOML)
//   - history.jsonl  append-only run event log (JSON Lines)
//   - state.json     read-status and scheduling bookkeeping (JSON)
//   - runs/<id>.log  per-run output log
//
// The data directory is ~/Library/Application Support/claudeq by default and
// can be overridden with the CLAUDEQ_HOME environment variable.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/danielmaier42/claudeq/internal/task"
)

// EnvHome is the environment variable that overrides the data directory.
const EnvHome = "CLAUDEQ_HOME"

const (
	configFile  = "config.toml"
	historyFile = "history.jsonl"
	stateFile   = "state.json"
	runsDir     = "runs"
)

// Store provides serialized access to the on-disk data directory.
type Store struct {
	home    string
	mu      sync.Mutex // guards individual file reads/writes
	writeMu sync.Mutex // serializes read-modify-write updates
}

// DefaultHome resolves the data directory: $CLAUDEQ_HOME if set, otherwise
// ~/Library/Application Support/claudeq.
func DefaultHome() (string, error) {
	if h := os.Getenv(EnvHome); h != "" {
		return h, nil
	}
	base, err := os.UserConfigDir() // ~/Library/Application Support on macOS
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(base, "claudeq"), nil
}

// Open opens (creating if necessary) the data directory at home.
func Open(home string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(home, runsDir), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{home: home}, nil
}

// Home returns the data directory path.
func (s *Store) Home() string { return s.home }

// LogPath returns the log file path for a run id.
func (s *Store) LogPath(runID string) string {
	return filepath.Join(s.home, runsDir, runID+".log")
}

func (s *Store) path(name string) string { return filepath.Join(s.home, name) }

// LoadConfig reads config.toml. A missing file yields a default, empty Config.
func (s *Store) LoadConfig() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path(configFile))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// SaveConfig atomically writes config.toml after validating every task.
func (s *Store) SaveConfig(cfg Config) error {
	for i, t := range cfg.Tasks {
		if err := t.Validate(); err != nil {
			return fmt.Errorf("task %d (%q): %w", i, t.ID, err)
		}
	}
	if err := cfg.checkUniqueIDs(); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(s.path(configFile), data)
}

// UpdateConfig atomically applies fn to the config: it serializes with other
// updates, loads the current config, applies fn, and saves the result. This
// prevents lost updates when several callers (e.g. concurrent API requests)
// modify tasks at once.
func (s *Store) UpdateConfig(fn func(*Config) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	if err := fn(&cfg); err != nil {
		return err
	}
	return s.SaveConfig(cfg)
}

// UpdateState atomically applies fn to the state, serialized with other
// updates. fn should mutate only the fields it owns so concurrent writers do
// not clobber each other's keys (e.g. the daemon must not overwrite read-status
// set via the API).
func (s *Store) UpdateState(fn func(*State) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	st, err := s.LoadState()
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	return s.SaveState(st)
}

// AppendRun appends a run event to history.jsonl. Later events for the same
// run id supersede earlier ones (see Runs).
func (s *Store) AppendRun(r Run) error {
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode run: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path(historyFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("write history: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close history: %w", err)
	}
	return nil
}

// AppendRunLog appends a line to a run's log file (used to record the final
// status/error into the log so it shows in both raw and chat views).
func (s *Store) AppendRunLog(runID string, data []byte) error {
	f, err := os.OpenFile(s.LogPath(runID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open run log: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write run log: %w", err)
	}
	return f.Close()
}

// Runs returns the run history collapsed so the latest event per run id wins,
// preserving first-seen order.
func (s *Store) Runs() ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path(historyFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open history: %w", err)
	}
	defer func() { _ = f.Close() }()

	latest := map[string]Run{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Run
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("parse history line: %w", err)
		}
		if _, seen := latest[r.RunID]; !seen {
			order = append(order, r.RunID)
		}
		latest[r.RunID] = r
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}

	out := make([]Run, 0, len(order))
	for _, id := range order {
		out = append(out, latest[id])
	}
	return out, nil
}

// LoadState reads state.json. A missing file yields a ready-to-use zero State.
func (s *Store) LoadState() (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path(stateFile))
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	st := newState()
	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	st.ensureMaps()
	return st, nil
}

// SaveState atomically writes state.json.
func (s *Store) SaveState(st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(s.path(stateFile), data)
}

// writeAtomic writes data to a temp file in the same directory and renames it
// into place, so a crash never leaves a half-written file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// Config is the persisted configuration: global settings plus the ordered task
// list. List order defines priority — index 0 is highest (PLAN.md FA-11).
type Config struct {
	Settings Settings    `toml:"settings"`
	Tasks    []task.Task `toml:"tasks"`
}

func (c Config) checkUniqueIDs() error {
	seen := map[string]struct{}{}
	for _, t := range c.Tasks {
		if _, dup := seen[t.ID]; dup {
			return fmt.Errorf("duplicate task id %q", t.ID)
		}
		seen[t.ID] = struct{}{}
	}
	return nil
}

// Settings holds global configuration.
type Settings struct {
	// DefaultModel is used for runs unless a task overrides it (FA-28).
	DefaultModel string `toml:"default_model" json:"default_model"`
	// SkipPermissionsDefault is the global "may do anything" default (FA-29).
	SkipPermissionsDefault bool `toml:"skip_permissions_default" json:"skip_permissions_default"`
	// HeartbeatMinutes is the safety-net wake interval in minutes (PLAN.md D8).
	// Zero means use the default (see HeartbeatOrDefault).
	HeartbeatMinutes int `toml:"heartbeat_minutes" json:"heartbeat_minutes"`
	// Pushover holds mobile-notification credentials (FA-41). Used from Phase 4.
	Pushover Pushover `toml:"pushover" json:"pushover"`
}

// DefaultHeartbeatMinutes is the wake safety-net interval when unset.
const DefaultHeartbeatMinutes = 60

// HeartbeatOrDefault returns the configured heartbeat, or the default if unset.
func (s Settings) HeartbeatOrDefault() time.Duration {
	m := s.HeartbeatMinutes
	if m <= 0 {
		m = DefaultHeartbeatMinutes
	}
	return time.Duration(m) * time.Minute
}

// Pushover holds Pushover API credentials and whether the channel is enabled.
type Pushover struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Token   string `toml:"token" json:"token"`
	UserKey string `toml:"user_key" json:"user_key"`
}
