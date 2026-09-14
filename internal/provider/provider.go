// Package provider is claudeq's harness abstraction: the scheduler and the
// store talk about *provider instances* and *capabilities*, never about a
// particular CLI. Everything a single harness does differently — how its
// process is invoked, how its output is read, how its configuration directory
// is passed — lives in an [Adapter] behind this contract.
//
// The pieces fit together like this:
//
//	Kind        an adapter implementation for one CLI harness ("claude-code")
//	Instance    a configured adapter instance, chosen by its stable ID ("claude")
//	Registry    maps a Kind to the Adapter that implements it
//	Set         the configured instances plus which one is the default
//	Adapter     builds the [Command] for a run and parses its output into [Event]s
//	Collector   folds those events into one normalized [Result]
//
// Instance ID and Kind are deliberately separate: two Claude subscriptions are
// two instances of the same kind, each with its own configuration directory,
// sessions and rate-limit state.
package provider

import (
	"errors"
	"fmt"
	"time"

	"github.com/danielmaier42/claudeq/internal/store"
)

// Kind identifies an adapter implementation for one CLI harness. It is the
// registry key; a configured [Instance] names the kind it uses.
type Kind string

// KindClaudeCode is the adapter for the Claude Code CLI.
const KindClaudeCode Kind = "claude-code"

// Instance is a configured adapter instance — the thing a task selects by ID.
// The struct tags are the on-disk shape the provider configuration will take;
// today the instances are derived from the existing settings (see [FromSettings]).
type Instance struct {
	// ID is the stable identifier a task selects ("claude").
	ID string `toml:"id" json:"id"`
	// Kind names the adapter that runs this instance.
	Kind Kind `toml:"kind" json:"kind"`
	// Name is the human-readable label shown in the UI and in run messages.
	Name string `toml:"name" json:"name"`
	// BinaryPath is an absolute path to the harness CLI. Empty lets the adapter
	// detect it.
	BinaryPath string `toml:"binary_path" json:"binary_path"`
	// ConfigDir selects the harness's configuration/account directory, which is
	// how a second subscription of the same kind stays separate. Empty leaves
	// the choice to the CLI and the daemon's environment, which is what a
	// single-account setup wants; an instance that must be isolated names one.
	ConfigDir string `toml:"config_dir" json:"config_dir"`
	// DefaultModel is used when neither the task nor the caller names a model.
	DefaultModel string `toml:"default_model" json:"default_model"`
	// Enabled turns the instance off without removing it.
	Enabled bool `toml:"enabled" json:"enabled"`
}

// AccessMode is claudeq's provider-neutral expression of how much authority a
// run gets. Each adapter validates and maps only the modes it can actually
// enforce, so the application never promises a restriction a harness cannot
// deliver.
type AccessMode string

const (
	// AccessProviderDefault leaves the harness's own permission handling alone.
	AccessProviderDefault AccessMode = "provider-default"
	// AccessReadOnly forbids any modification of the working tree.
	AccessReadOnly AccessMode = "read-only"
	// AccessWorkspaceWrite allows writes inside the working directory only.
	AccessWorkspaceWrite AccessMode = "workspace-write"
	// AccessFullAccess bypasses the harness's permission prompts entirely.
	AccessFullAccess AccessMode = "full-access"
)

// OrDefault resolves the zero value to AccessProviderDefault. An unset access
// mode must never be read as "anything goes": a request that says nothing about
// authority gets the harness's own permission handling.
func (m AccessMode) OrDefault() AccessMode {
	if m == "" {
		return AccessProviderDefault
	}
	return m
}

// Capabilities describe what an adapter kind can do, so callers can ask about a
// capability instead of branching on a provider name.
type Capabilities struct {
	// SessionResume: the harness can continue a previous session non-interactively.
	SessionResume bool
	// InteractiveResume: the harness can reopen a session in a terminal.
	InteractiveResume bool
	// StructuredOutput: the harness emits machine-readable run output.
	StructuredOutput bool
	// ModelDiscovery: the harness can list the models it accepts.
	ModelDiscovery bool
	// ReasoningEffort: the harness accepts a reasoning-effort setting.
	ReasoningEffort bool
	// UsageMetrics: the harness reports token and turn counts.
	UsageMetrics bool
	// CostMetrics: the harness reports a monetary cost.
	CostMetrics bool
	// RateLimitResume: a rate-limited session can be picked up again later.
	RateLimitResume bool
	// Subagents: the harness runs its own subagents inside one job.
	Subagents bool
	// AccessModes lists the access modes this adapter can actually enforce.
	AccessModes []AccessMode
}

// SupportsAccess reports whether the adapter can enforce mode.
func (c Capabilities) SupportsAccess(mode AccessMode) bool {
	for _, m := range c.AccessModes {
		if m == mode {
			return true
		}
	}
	return false
}

// Request is one run, expressed without reference to any particular CLI. The
// adapter turns it into a [Command].
type Request struct {
	// Prompt is the instruction for the harness.
	Prompt string
	// WorkingDir is the directory the harness runs in.
	WorkingDir string
	// Model is the already-resolved effective model. Empty means the harness's
	// own default.
	Model string
	// SessionID identifies the session to start or continue.
	SessionID string
	// Resume continues SessionID instead of starting a new session.
	Resume bool
	// AccessMode is the requested authority for this run. The zero value means
	// AccessProviderDefault (see AccessMode.OrDefault).
	AccessMode AccessMode
	// SystemPrompt is claudeq's own run guidance, to be delivered alongside the
	// harness's built-in instructions rather than replacing them.
	SystemPrompt string
}

// Command is the process invocation an adapter produced for a [Request].
type Command struct {
	// Path is the resolved executable, invoked directly — never through a shell.
	Path string
	// Args are the arguments after the executable.
	Args []string
	// Env holds the adapter's own environment additions (for example a
	// configuration-directory variable). They are appended to the daemon's
	// environment, so a later entry wins.
	Env []string
}

// EventType is a normalized application event. Adapters translate their
// harness's output into these, so the executor never reads provider syntax.
type EventType string

const (
	// EventSessionStarted reports the session ID the harness is using.
	EventSessionStarted EventType = "session_started"
	// EventCompleted is the harness's terminal result for the run.
	EventCompleted EventType = "completed"
	// EventRateLimited reports that the provider refused the work for allowance
	// reasons; the run can be resumed once the window reopens.
	EventRateLimited EventType = "rate_limited"
	// EventAuthFailed reports a login/authentication problem.
	EventAuthFailed EventType = "authentication_failed"
	// EventModelRejected reports that the provider does not accept the model.
	EventModelRejected EventType = "model_rejected"
	// EventFailed reports a harness-level failure with a reason.
	EventFailed EventType = "failed"
)

// Event is one normalized observation from a run's output.
type Event struct {
	// Type selects which of the fields below carry meaning.
	Type EventType
	// SessionID is set on EventSessionStarted.
	SessionID string
	// FinalOutput is the harness's final response, set on EventCompleted.
	FinalOutput string
	// IsError marks an EventCompleted that reports a failure.
	IsError bool
	// Detail is a short human-readable reason, for the failure-ish events.
	Detail string
	// RetryAfter is a relative wait reported with EventRateLimited.
	RetryAfter time.Duration
	// ResetAt is the absolute time the allowance window reopens, reported with
	// EventRateLimited.
	ResetAt time.Time
	// Metrics are the run's usage figures, reported with EventCompleted.
	Metrics *Metrics
}

// Metrics are the usage figures a harness reports for a run. Values it does not
// report stay zero — claudeq never infers a missing cost from a price table.
type Metrics struct {
	CostUSD      float64
	InputTokens  int
	OutputTokens int
	NumTurns     int
	DurationMS   int64
}

// Result is the normalized outcome of a run.
type Result struct {
	// Status is the classified outcome.
	Status store.RunStatus
	// SessionID is the session the run actually used.
	SessionID string
	// ExitCode is the harness process's exit status.
	ExitCode int
	// RetryAfter is how long to wait after a rate limit, when reported.
	RetryAfter time.Duration
	// ResetAt is the absolute rate-limit reset time, when reported.
	ResetAt time.Time
	// Message is a short human-readable detail for a non-success outcome.
	Message string
	// FinalOutput is the harness's final response — the answer, as opposed to
	// the raw run log.
	FinalOutput string
	// Metrics are the reported usage figures, nil when the harness reported none.
	Metrics *Metrics
}

// Parser reads one run's output. Adapters return a fresh parser per run, so a
// parser may keep run-local state (a rate-limit reset time seen earlier, say).
type Parser interface {
	// Parse translates one line of harness output into zero or more events.
	Parse(line []byte) []Event
}

// Adapter implements one CLI harness. Everything provider-specific —
// arguments, environment, output syntax — lives behind this interface.
type Adapter interface {
	// Kind is the registry key this adapter implements.
	Kind() Kind
	// Capabilities describes what the harness can do.
	Capabilities() Capabilities
	// DetectBinary locates the harness CLI, returning "" when it cannot be found.
	DetectBinary() string
	// Command builds the invocation for a run on the given instance.
	Command(inst Instance, req Request) (Command, error)
	// NewParser returns a parser for one run's output.
	NewParser() Parser
}

// ErrUnsupported is returned by an adapter asked for something its harness
// cannot do (an access mode it cannot enforce, a resume it does not support).
var ErrUnsupported = errors.New("not supported by this provider")

// UnsupportedAccessError reports an access mode an adapter cannot enforce.
func UnsupportedAccessError(inst Instance, mode AccessMode) error {
	return fmt.Errorf("%w: %s cannot enforce %q access", ErrUnsupported, instanceLabel(inst), mode)
}

// instanceLabel is how an instance is named in an error the operator reads: its
// display name when it has one, its ID otherwise.
func instanceLabel(inst Instance) string {
	if inst.Name != "" {
		return inst.Name
	}
	return inst.ID
}
