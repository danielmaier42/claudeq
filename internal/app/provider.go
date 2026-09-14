package app

// Provider instances: reading, adding, editing and removing the configured
// harnesses, plus which of them is the default. The rules live here so the CLI
// and the HTTP API cannot disagree about what a valid change is.

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// Providers returns the configured provider instances and which one is the
// default. The store seeds them, so a fresh installation reports the Claude
// Code instance rather than an empty list.
func Providers(s *store.Store) (provider.Set, error) {
	cfg, err := s.LoadConfig()
	if err != nil {
		return provider.Set{}, err
	}
	return provider.FromConfig(cfg)
}

// EnsureRunnable refuses to file work for a provider that cannot run it: an id
// nothing is configured under, or a configured instance that is switched off or
// not ready. Filing it anyway would put a job in the queue that is known in
// advance to fail at three in the morning, which is the one thing an unattended
// queue must not do. An empty id means the default provider.
func EnsureRunnable(ctx context.Context, set provider.Set, ch *provider.Checker, providerID string) error {
	resolved, err := set.Resolve(provider.Selection{ProviderID: providerID})
	if err != nil {
		return err
	}
	if h := ch.Check(ctx, resolved.Instance); !h.Ready() {
		return fmt.Errorf("provider %q is not ready: %s", resolved.Instance.ID, h.Reason)
	}
	return nil
}

// AddProvider stores a new provider instance. The id must be free and the whole
// configuration must still be valid afterwards.
func AddProvider(s *store.Store, reg *provider.Registry, inst provider.Instance) error {
	if err := provider.Validate(reg, inst); err != nil {
		return err
	}
	return s.UpdateConfig(func(cfg *store.Config) error {
		if providerIndex(cfg.Providers, inst.ID) >= 0 {
			return fmt.Errorf("provider %q already exists", inst.ID)
		}
		cfg.Providers = append(cfg.Providers, inst.Stored())
		return nil
	})
}

// EditProvider applies a change to one configured instance in place. The id is
// fixed: a task, a run's history and the pending resumes all name it, so apply
// must not change it.
func EditProvider(s *store.Store, reg *provider.Registry, id string, apply func(*provider.Instance) error) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		idx := providerIndex(cfg.Providers, id)
		if idx < 0 {
			return unknownProvider(cfg, id)
		}
		edited := provider.InstanceOf(cfg.Providers[idx])
		if err := apply(&edited); err != nil {
			return err
		}
		if edited.ID != id {
			return fmt.Errorf("provider id cannot be changed (%q -> %q)", id, edited.ID)
		}
		if err := provider.Validate(reg, edited); err != nil {
			return err
		}
		cfg.Providers[idx] = edited.Stored()
		return nil
	})
}

// SetProviderEnabled switches an instance on or off without removing it. A
// disabled instance keeps its configuration and its tasks; those tasks stay
// queued and report why they are blocked.
func SetProviderEnabled(s *store.Store, id string, enabled bool) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		idx := providerIndex(cfg.Providers, id)
		if idx < 0 {
			return unknownProvider(cfg, id)
		}
		cfg.Providers[idx].Enabled = enabled
		return nil
	})
}

// RemoveProvider deletes a provider instance. It refuses while anything still
// names it and lists every reference, because silently re-pointing that work at
// another provider is exactly the substitution claudeq never makes.
func RemoveProvider(s *store.Store, id string) error {
	err := s.UpdateConfig(func(cfg *store.Config) error {
		idx := providerIndex(cfg.Providers, id)
		if idx < 0 {
			return unknownProvider(cfg, id)
		}
		if len(cfg.Providers) == 1 {
			return fmt.Errorf("provider %q is the only one configured; add another before removing it", id)
		}
		if refs := providerRefs(*cfg, id); len(refs) > 0 {
			return fmt.Errorf("provider %q is still used by %s; change those first",
				id, strings.Join(refs, ", "))
		}
		cfg.Providers = append(cfg.Providers[:idx], cfg.Providers[idx+1:]...)
		return nil
	})
	if err != nil {
		return err
	}
	return s.UpdateState(func(st *store.State) error {
		st.ForgetProvider(id)
		return nil
	})
}

// SetDefaultProvider chooses the instance a task runs on when it names none.
func SetDefaultProvider(s *store.Store, id string) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		if providerIndex(cfg.Providers, id) < 0 {
			return unknownProvider(cfg, id)
		}
		cfg.Settings.DefaultProvider = id
		return nil
	})
}

// providerRefs lists, in words the operator can act on, everything that names a
// provider id.
func providerRefs(cfg store.Config, id string) []string {
	var refs []string
	if cfg.Settings.DefaultProvider == id {
		refs = append(refs, "the default provider setting")
	}
	var tasks []string
	for _, t := range cfg.Tasks {
		if t.Provider == id {
			tasks = append(tasks, t.ID)
		}
	}
	if len(tasks) > 0 {
		refs = append(refs, "task "+strings.Join(tasks, ", "))
	}
	return refs
}

func providerIndex(ps []store.Provider, id string) int {
	for i, p := range ps {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// unknownProvider names the ids that do exist, so a typo is answered with the
// answer rather than with another lookup.
func unknownProvider(cfg *store.Config, id string) error {
	known := make([]string, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		known = append(known, p.ID)
	}
	if len(known) == 0 {
		return fmt.Errorf("%w %q", provider.ErrUnknownProvider, id)
	}
	return fmt.Errorf("%w %q (configured: %s)", provider.ErrUnknownProvider, id, strings.Join(known, ", "))
}
