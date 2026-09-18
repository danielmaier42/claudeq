package provider

import (
	"errors"
	"fmt"

	"github.com/danielmaier42/claudeq/internal/store"
)

// DefaultInstanceID is the ID of the Claude Code instance every claudeq
// configuration has. It is the migration target for the pre-provider settings
// and the default provider for a task that names none.
const DefaultInstanceID = store.DefaultProviderID

// DefaultInstanceName is that instance's display name. It appears in run
// messages, so it reads as the product the operator installed.
const DefaultInstanceName = store.DefaultProviderName

// Errors a selection can fail with. None of them is ever answered by silently
// substituting another provider, account or model.
var (
	// ErrUnknownProvider means no configured instance has that ID.
	ErrUnknownProvider = errors.New("unknown provider")
	// ErrProviderDisabled means the instance exists but is switched off.
	ErrProviderDisabled = errors.New("provider is disabled")
	// ErrNoProviders means nothing is configured to run at all.
	ErrNoProviders = errors.New("no provider is configured")
)

// Selection is what a task or a queue override asked for. An empty field
// inherits, following the resolution table in Set.Resolve.
type Selection struct {
	// ProviderID names a configured instance. Empty selects the default.
	ProviderID string
	// Model names a model for that provider. Empty selects the provider's own
	// default model.
	Model string
}

// Resolved is the effective execution identity for one run.
type Resolved struct {
	// Instance is the provider instance that will run the job.
	Instance Instance
	// Model is the effective model; empty means the harness's own default.
	Model string
	// FallbackFrom is the instance the selection actually named, set only when
	// its allowance was used up and its fallback took the job. It is what the
	// run's log says happened; an empty value means nothing was substituted.
	FallbackFrom Instance
}

// Substituted reports whether the run is going somewhere other than the
// provider it named.
func (r Resolved) Substituted() bool { return r.FallbackFrom.ID != "" }

// Set is the configured provider instances plus which of them is the default.
type Set struct {
	instances []Instance
	defaultID string
}

// NewSet builds a set. It rejects an instance list that could not be resolved
// unambiguously: a missing or duplicate ID, or a default that names no member.
func NewSet(defaultID string, instances []Instance) (Set, error) {
	seen := make(map[string]struct{}, len(instances))
	for _, inst := range instances {
		if inst.ID == "" {
			return Set{}, fmt.Errorf("provider set: instance with empty id")
		}
		if _, dup := seen[inst.ID]; dup {
			return Set{}, fmt.Errorf("provider set: duplicate provider id %q", inst.ID)
		}
		seen[inst.ID] = struct{}{}
	}
	if defaultID != "" {
		if _, ok := seen[defaultID]; !ok {
			return Set{}, fmt.Errorf("provider set: default provider %q is not configured", defaultID)
		}
	} else if len(instances) > 0 {
		// No default named: the first configured instance is it. Leaving the
		// default empty would make every task that names no provider fail, which
		// is a worse answer than the only one there can be.
		defaultID = instances[0].ID
	}
	out := make([]Instance, len(instances))
	copy(out, instances)
	return Set{instances: out, defaultID: defaultID}, nil
}

// FromConfig reads the configured provider instances out of a stored
// configuration. The store seeds and migrates the entries (see
// store.Config.migrate), so every configuration it hands out already has the
// Claude Code instance; a Config assembled without it is rejected rather than
// silently given an invented provider.
func FromConfig(cfg store.Config) (Set, error) {
	if len(cfg.Providers) == 0 {
		return Set{}, ErrNoProviders
	}
	insts := make([]Instance, len(cfg.Providers))
	for i, p := range cfg.Providers {
		insts[i] = InstanceOf(p)
	}
	return NewSet(cfg.Settings.DefaultProvider, insts)
}

// All returns the configured instances in order.
func (s Set) All() []Instance {
	out := make([]Instance, len(s.instances))
	copy(out, s.instances)
	return out
}

// DefaultID returns the ID of the default provider, or "" when none is set.
func (s Set) DefaultID() string { return s.defaultID }

// Lookup returns the instance with the given ID.
func (s Set) Lookup(id string) (Instance, bool) {
	for _, inst := range s.instances {
		if inst.ID == id {
			return inst, true
		}
	}
	return Instance{}, false
}

// Resolve applies the resolution table:
//
//	neither given        default provider, that provider's default model
//	only a model given   default provider, the given model
//	only a provider      that provider, that provider's default model
//	both given           that provider, the given model
//
// Because the model default always comes from the resolved instance, changing
// the provider without naming a model can never carry a model from the old
// provider into the new one. There is no fallback to another provider: a
// selection that cannot be honoured returns an error saying why.
func (s Set) Resolve(sel Selection) (Resolved, error) {
	id := sel.ProviderID
	if id == "" {
		id = s.defaultID
	}
	if id == "" {
		return Resolved{}, ErrNoProviders
	}
	inst, ok := s.Lookup(id)
	if !ok {
		return Resolved{}, fmt.Errorf("%w %q", ErrUnknownProvider, id)
	}
	if !inst.Enabled {
		return Resolved{}, fmt.Errorf("%w: %q", ErrProviderDisabled, id)
	}
	model := sel.Model
	if model == "" {
		model = inst.DefaultModel
	}
	return Resolved{Instance: inst, Model: model}, nil
}

// ResolveAvailable resolves a selection and then, when the chosen instance is
// out of allowance, follows the fallback it was given.
//
// available answers "can this instance take work right now" — in the daemon,
// whether its rate-limit gate is open. A nil available makes this exactly
// [Set.Resolve]: nothing is ever substituted for a provider that can run.
//
// The chain is followed until an available instance is found, and every hop
// must be configured, switched on and not already visited. A chain that is
// broken or blocked end to end resolves to the provider the selection named,
// so the caller still sees the account whose allowance ran out rather than an
// error about a fallback nobody asked about.
//
// This is the one substitution claudeq makes, and only this one: a rate limit
// is a pause, not a verdict on the work. A provider that is missing, logged
// out or switched off is still never answered by running somewhere else.
func (s Set) ResolveAvailable(sel Selection, available func(id string) bool) (Resolved, error) {
	res, err := s.Resolve(sel)
	if err != nil || available == nil || available(res.Instance.ID) {
		return res, err
	}
	origin := res.Instance
	seen := map[string]struct{}{origin.ID: {}}
	for cur := origin; cur.FallbackProvider != ""; {
		next, ok := s.Lookup(cur.FallbackProvider)
		if _, visited := seen[cur.FallbackProvider]; !ok || visited || !next.Enabled {
			break
		}
		seen[next.ID] = struct{}{}
		if available(next.ID) {
			return Resolved{Instance: next, Model: fallbackModel(origin, cur, next, sel.Model), FallbackFrom: origin}, nil
		}
		cur = next
	}
	return res, nil
}

// fallbackModel decides which model the substitute runs. The provider that
// handed the work over has the first word: whoever configured "when my limit is
// reached, go to Codex" may well know which model should answer there, and a
// named one is not second-guessed.
//
// Without that, a model name means something to one harness only: it travels to
// another account of the same kind — a second Claude subscription still runs the
// Opus the task asked for — and is dropped for a different harness, which falls
// back to that instance's own default rather than being handed a name it does
// not know.
func fallbackModel(origin, from, next Instance, want string) string {
	if from.FallbackModel != "" {
		return from.FallbackModel
	}
	if want != "" && next.Kind == origin.Kind {
		return want
	}
	return next.DefaultModel
}
