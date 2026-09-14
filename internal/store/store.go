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
	"bytes"
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

	cfg, _, err := s.readConfig()
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// readConfig parses config.toml and applies the migrations, reporting whether
// they changed anything. The caller holds the appropriate lock.
func (s *Store) readConfig() (Config, bool, error) {
	data, err := os.ReadFile(s.path(configFile))
	if errors.Is(err, os.ErrNotExist) {
		// A fresh installation still has the Claude Code provider — its Settings
		// card reports what is actually wrong (not installed, not logged in)
		// rather than the app pretending no harness exists. Nothing is written:
		// this is the default a first save will persist.
		fresh := Config{}
		fresh.migrate()
		return fresh, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, fmt.Errorf("parse config: %w", err)
	}
	return cfg, cfg.migrate(), nil
}

// migrate rewrites configs written by older versions and reports whether it
// changed anything. Every load applies it in memory, so an old config behaves
// correctly even before it is rewritten; MigrateConfig writes the result back.
// It must stay idempotent.
func (c *Config) migrate() bool {
	changed := false

	// The global skip-permissions default is gone: a task that relied on it
	// keeps its authority by carrying the setting itself.
	if c.Settings.LegacySkipPermissions {
		for i := range c.Tasks {
			if c.Tasks[i].Permissions == task.PermissionsDefault {
				c.Tasks[i].Permissions = task.PermissionsSkip
			}
		}
		c.Settings.LegacySkipPermissions = false
		changed = true
	}

	// Runs go through a configured provider instance now. A configuration
	// written before that has none, so the Claude Code binary and the global
	// default model become the `claude` instance every existing task then runs
	// on — same CLI, same model, same schedule.
	if len(c.Providers) == 0 {
		c.Providers = seedProviders(c.Settings)
		changed = true
	}
	if c.Settings.DefaultProvider == "" {
		c.Settings.DefaultProvider = c.Providers[0].ID
		changed = true
	}
	// The instances own these two values now; leaving copies behind would give
	// the file two answers to the same question.
	if c.Settings.LegacyClaudePath != "" || c.Settings.LegacyDefaultModel != "" {
		c.Settings.LegacyClaudePath, c.Settings.LegacyDefaultModel = "", ""
		changed = true
	}
	return changed
}

// MigrateConfig persists the migrations LoadConfig applies in memory, so the
// file on disk says what the daemon actually does. It reports whether the file
// was rewritten and is a no-op for an already-current config.
func (s *Store) MigrateConfig() (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	changed := false
	err := s.withWriteLock(func() error {
		cfg, migrated, err := s.readConfig()
		if err != nil || !migrated {
			return err
		}
		changed = true
		return s.SaveConfig(cfg)
	})
	return changed, err
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
	if err := cfg.checkProviders(); err != nil {
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
	return s.withWriteLock(func() error {
		cfg, err := s.LoadConfig()
		if err != nil {
			return err
		}
		if err := fn(&cfg); err != nil {
			return err
		}
		return s.SaveConfig(cfg)
	})
}

// UpdateState atomically applies fn to the state, serialized with other
// updates. fn should mutate only the fields it owns so concurrent writers do
// not clobber each other's keys (e.g. the daemon must not overwrite read-status
// set via the API).
func (s *Store) UpdateState(fn func(*State) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.withWriteLock(func() error {
		st, err := s.LoadState()
		if err != nil {
			return err
		}
		if err := fn(st); err != nil {
			return err
		}
		return s.SaveState(st)
	})
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

// ReconcileRunningRuns marks any run still recorded as "running" as failed. It
// is meant to run at startup: a fresh process means nothing is actually running,
// so such entries are leftovers from a hard stop (crash, power loss, SIGKILL)
// that never got to record an outcome. Returns how many it reconciled.
func (s *Store) ReconcileRunningRuns(now time.Time) (int, error) {
	runs, err := s.Runs()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range runs {
		if r.Status != StatusRunning {
			continue
		}
		r.Status = StatusFailed
		fin := now
		r.FinishedAt = &fin
		if r.Error == "" {
			r.Error = "interrupted before completing (daemon stopped unexpectedly)"
		}
		if err := s.AppendRun(r); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// BackfillLastRuns fills in State.LastRunAt from run history for tasks that
// have no entry yet. It exists for installs that predate the field: without it
// a recurring task would show no last execution until it next runs. Tasks
// already recorded are left untouched. Returns how many entries it added.
func (s *Store) BackfillLastRuns() (int, error) {
	runs, err := s.Runs()
	if err != nil {
		return 0, err
	}
	latest := map[string]time.Time{}
	for _, r := range runs {
		if r.TaskID == "" {
			continue
		}
		if prev, ok := latest[r.TaskID]; !ok || r.StartedAt.After(prev) {
			latest[r.TaskID] = r.StartedAt
		}
	}
	n := 0
	if err := s.UpdateState(func(cur *State) error {
		n = 0
		for id, at := range latest {
			if _, ok := cur.LastRun(id); ok {
				continue
			}
			cur.RecordRun(id, at)
			n++
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return n, nil
}

// PruneHistory keeps only the most recent `limit` runs: it compacts
// history.jsonl to one latest event per kept run and deletes the log files of
// the dropped runs. limit <= 0 keeps everything.
func (s *Store) PruneHistory(limit int) error {
	if limit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path(historyFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open history: %w", err)
	}
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
			_ = f.Close()
			return fmt.Errorf("parse history line: %w", err)
		}
		if _, seen := latest[r.RunID]; !seen {
			order = append(order, r.RunID)
		}
		latest[r.RunID] = r
	}
	scErr := sc.Err()
	_ = f.Close()
	if scErr != nil {
		return fmt.Errorf("read history: %w", scErr)
	}

	if len(order) <= limit {
		return nil
	}
	drop := order[:len(order)-limit]
	keep := order[len(order)-limit:]

	var buf bytes.Buffer
	for _, id := range keep {
		b, err := json.Marshal(latest[id])
		if err != nil {
			return fmt.Errorf("marshal run: %w", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := writeAtomic(s.path(historyFile), buf.Bytes()); err != nil {
		return fmt.Errorf("rewrite history: %w", err)
	}
	for _, id := range drop {
		p := latest[id].LogPath
		if p == "" {
			p = s.LogPath(id)
		}
		_ = os.Remove(p)
	}
	return nil
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

// Config is the persisted configuration: global settings, the configured
// provider instances, plus the ordered task list. List order defines priority —
// index 0 is highest (PLAN.md FA-11).
type Config struct {
	Settings Settings `toml:"settings"`
	// Providers are the configured provider instances, in the order they were
	// added. The domain type built from them lives in internal/provider; this is
	// only their on-disk shape.
	Providers []Provider  `toml:"providers,omitempty"`
	Tasks     []task.Task `toml:"tasks"`
}

// Identity of the provider instance every claudeq configuration has: the Claude
// Code CLI, seeded on migration and on a fresh installation (see seedProviders).
const (
	// DefaultProviderID is the stable id of that instance — what a task names.
	DefaultProviderID = "claude"
	// DefaultProviderKind is the adapter it runs on.
	DefaultProviderKind = "claude-code"
	// DefaultProviderName is its display name.
	DefaultProviderName = "Claude Code"
)

// Provider is one configured provider instance as config.toml holds it: a
// stable id, the adapter kind that runs it, and the instance's own settings.
// Two subscriptions of the same harness are two entries of the same kind with
// separate configuration directories, so nothing here assumes one instance per
// kind.
//
// The store keeps the file shape only. What an instance can do, and how it is
// resolved, validated and probed, lives in internal/provider.
type Provider struct {
	// ID is the stable identifier a task selects ("claude").
	ID string `toml:"id" json:"id"`
	// Kind names the adapter implementation that runs this instance.
	Kind string `toml:"kind" json:"kind"`
	// Name is the human-readable label shown in the app and in run messages.
	Name string `toml:"name" json:"name"`
	// BinaryPath is an absolute path to the harness CLI. Empty lets the adapter
	// detect it.
	BinaryPath string `toml:"binary_path" json:"binary_path"`
	// ConfigDir selects the harness's configuration (and therefore account)
	// directory. Empty uses the CLI's own default.
	ConfigDir string `toml:"config_dir" json:"config_dir"`
	// DefaultModel is used when neither the task nor the caller names a model.
	DefaultModel string `toml:"default_model" json:"default_model"`
	// Enabled turns the instance off without removing it.
	Enabled bool `toml:"enabled" json:"enabled"`
}

// seedProviders builds the provider list for a configuration that has none: the
// Claude Code instance, carrying over the pre-provider global settings so a
// migrated installation invokes exactly the same CLI with exactly the same
// model as before.
func seedProviders(s Settings) []Provider {
	return []Provider{{
		ID:           DefaultProviderID,
		Kind:         DefaultProviderKind,
		Name:         DefaultProviderName,
		BinaryPath:   s.LegacyClaudePath,
		DefaultModel: s.LegacyDefaultModel,
		Enabled:      true,
	}}
}

// checkProviders guards what the file format itself has to guarantee: every
// instance is addressable by a unique id, and the default names one of them.
// Whether a kind exists and a path is usable is the provider layer's business
// (internal/provider), which the store must not depend on.
func (c Config) checkProviders() error {
	seen := map[string]struct{}{}
	for _, p := range c.Providers {
		if p.ID == "" {
			return fmt.Errorf("provider with empty id")
		}
		if _, dup := seen[p.ID]; dup {
			return fmt.Errorf("duplicate provider id %q", p.ID)
		}
		seen[p.ID] = struct{}{}
	}
	if id := c.Settings.DefaultProvider; id != "" {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("default provider %q is not configured", id)
		}
	}
	return nil
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
	// DefaultProvider is the id of the provider instance a task runs on when it
	// names none. Empty falls back to the first configured instance.
	DefaultProvider string `toml:"default_provider" json:"default_provider"`
	// LegacyDefaultModel is the retired global default model. Providers carry
	// their own default model now; this is only read to migrate old configs
	// (see migrate) and never written back or exposed over the API.
	LegacyDefaultModel string `toml:"default_model,omitempty" json:"-"`
	// LegacySkipPermissions is the removed global "may do anything" default.
	// It is only read to migrate old configs (see migrate) and never written
	// back or exposed over the API; permissions live on the task now.
	LegacySkipPermissions bool `toml:"skip_permissions_default,omitempty" json:"-"`
	// HeartbeatMinutes is the safety-net wake interval in minutes (PLAN.md D8).
	// Zero means use the default (see HeartbeatOrDefault).
	HeartbeatMinutes int `toml:"heartbeat_minutes" json:"heartbeat_minutes"`
	// Pushover holds mobile-notification credentials (FA-41). Used from Phase 4.
	Pushover Pushover `toml:"pushover" json:"pushover"`
	// Ntfy holds the ntfy push channel: a topic on ntfy.sh or a self-hosted
	// instance.
	Ntfy Ntfy `toml:"ntfy" json:"ntfy"`
	// Webhook holds the generic JSON webhook channel — the one that covers a
	// service claudeq does not know about (Slack, Discord, Home Assistant, n8n).
	Webhook Webhook `toml:"webhook" json:"webhook"`
	// LegacyClaudePath is the retired global path to the Claude Code binary. The
	// `claude` provider instance carries it now; this is only read to migrate
	// old configs (see migrate) and never written back or exposed over the API.
	LegacyClaudePath string `toml:"claude_path,omitempty" json:"-"`
	// IdleTimeoutMinutes kills a run that produces no output for this many
	// minutes — a hung/deadlocked process. A working run keeps streaming events,
	// so it is not affected. 0 = use the default; negative = never kill.
	IdleTimeoutMinutes int `toml:"idle_timeout_minutes" json:"idle_timeout_minutes"`
	// MaxRunHistory caps how many runs are kept; older runs (and their log files)
	// are pruned. 0 = use the default; negative = keep everything.
	MaxRunHistory int `toml:"max_run_history" json:"max_run_history"`
	// SystemPrompt is the operator's custom system prompt, appended to claudeq's
	// built-in one on every run (built-in first, this last). Empty means none.
	SystemPrompt string `toml:"system_prompt" json:"system_prompt"`
	// Paused is the global stop switch: while it is true no run starts at all —
	// neither a due task nor a manual "run now" — and the machine is no longer
	// woken for scheduled work. Runs already in flight are left alone.
	Paused bool `toml:"paused" json:"paused"`
	// PromptReviewDisabled turns off the prompt review that checks a task's
	// prompt against this machine before it is queued. The zero value keeps the
	// review on, so an existing config gains the feature without being edited.
	PromptReviewDisabled bool `toml:"prompt_review_disabled" json:"prompt_review_disabled"`
	// PromptReviewModel is the model used for that review. Empty means "the same
	// model as everything else", i.e. the reviewing provider's default model.
	PromptReviewModel string `toml:"prompt_review_model,omitempty" json:"prompt_review_model"`
}

// ErrPaused is what a refused run carries while Settings.Paused is on. A pause
// a manual "run now" could step around would not be a pause, so such a request
// is refused with this instead of quietly starting a run.
var ErrPaused = errors.New("all runs are paused (Settings → Pause all runs)")

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

// DefaultIdleTimeoutMinutes is the no-output kill threshold when unset.
const DefaultIdleTimeoutMinutes = 30

// IdleTimeout returns the no-output duration after which a run is killed, or 0
// when disabled. Unset (0) uses the default; a negative setting disables it.
func (s Settings) IdleTimeout() time.Duration {
	m := s.IdleTimeoutMinutes
	if m == 0 {
		m = DefaultIdleTimeoutMinutes
	}
	if m < 0 {
		return 0
	}
	return time.Duration(m) * time.Minute
}

// DefaultMaxRunHistory is the retained-run count when unset.
const DefaultMaxRunHistory = 500

// RunHistoryLimit returns how many runs to keep, or 0 for unlimited. Unset (0)
// uses the default; a negative setting keeps everything.
func (s Settings) RunHistoryLimit() int {
	n := s.MaxRunHistory
	if n == 0 {
		n = DefaultMaxRunHistory
	}
	if n < 0 {
		return 0
	}
	return n
}

// Pushover holds Pushover API credentials and whether the channel is enabled.
type Pushover struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Token   string `toml:"token" json:"token"`
	UserKey string `toml:"user_key" json:"user_key"`
}

// Ntfy holds the ntfy channel's configuration and whether it is enabled. Server
// empty means the public ntfy.sh; Token is only needed for a protected topic.
type Ntfy struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Server  string `toml:"server" json:"server"`
	Topic   string `toml:"topic" json:"topic"`
	Token   string `toml:"token" json:"token"`
}

// Webhook holds the generic webhook channel's configuration and whether it is
// enabled. Template empty means claudeq's own JSON body (see
// notify.DefaultWebhookTemplate).
type Webhook struct {
	Enabled  bool   `toml:"enabled" json:"enabled"`
	URL      string `toml:"url" json:"url"`
	Template string `toml:"template" json:"template"`
}
