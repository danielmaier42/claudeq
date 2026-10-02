package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// EnsurePoolRunnable is EnsureRunnable for a task that names a pool: the pool
// must exist and at least one member must be able to run. A pool with one
// working account still gets its work done, so one logged-out member is no
// reason to refuse.
func EnsurePoolRunnable(ctx context.Context, set provider.Set, ch *provider.Checker, poolID string) error {
	p, ok := set.LookupPool(poolID)
	if !ok {
		return unknownPool(set, poolID)
	}
	var reasons []string
	for _, inst := range set.PoolMembers(p) {
		if !inst.Enabled {
			reasons = append(reasons, inst.Label()+" is switched off")
			continue
		}
		h := ch.Check(ctx, inst)
		if !h.KnownUnready() {
			return nil
		}
		reasons = append(reasons, inst.Label()+": "+h.ReasonOr("it cannot run tasks right now"))
	}
	return fmt.Errorf("no member of pool %q can run tasks (%s)", poolID, strings.Join(reasons, "; "))
}

// EnsureTaskTarget is EnsureRunnable or EnsurePoolRunnable, whichever the task
// names.
func EnsureTaskTarget(ctx context.Context, set provider.Set, ch *provider.Checker, providerID, poolID string) error {
	if poolID != "" {
		return EnsurePoolRunnable(ctx, set, ch, poolID)
	}
	return EnsureRunnable(ctx, set, ch, providerID)
}

// AddPool stores a new pool.
func AddPool(s *store.Store, p provider.Pool) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		if poolIndex(cfg.Pools, p.ID) >= 0 {
			return fmt.Errorf("pool %q already exists", p.ID)
		}
		if err := validatePool(*cfg, p); err != nil {
			return err
		}
		cfg.Pools = append(cfg.Pools, p.Stored())
		return nil
	})
}

// EditPool applies a change to one pool in place. The id is fixed, because the
// tasks name it.
func EditPool(s *store.Store, id string, apply func(*provider.Pool) error) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		idx := poolIndex(cfg.Pools, id)
		if idx < 0 {
			return unknownPool(poolSet(*cfg), id)
		}
		edited := provider.PoolOf(cfg.Pools[idx])
		if err := apply(&edited); err != nil {
			return err
		}
		if edited.ID != id {
			return fmt.Errorf("pool id cannot be changed (%q -> %q)", id, edited.ID)
		}
		if err := validatePool(*cfg, edited); err != nil {
			return err
		}
		cfg.Pools[idx] = edited.Stored()
		return nil
	})
}

// RemovePool deletes a pool. It refuses while a task names it, for the same
// reason a provider cannot be removed from under its tasks.
func RemovePool(s *store.Store, id string) error {
	return s.UpdateConfig(func(cfg *store.Config) error {
		idx := poolIndex(cfg.Pools, id)
		if idx < 0 {
			return unknownPool(poolSet(*cfg), id)
		}
		var tasks []string
		for _, t := range cfg.Tasks {
			if t.Pool == id {
				tasks = append(tasks, t.ID)
			}
		}
		if len(tasks) > 0 {
			return fmt.Errorf("pool %q is still used by task %s; change those first", id, strings.Join(tasks, ", "))
		}
		cfg.Pools = append(cfg.Pools[:idx], cfg.Pools[idx+1:]...)
		return nil
	})
}

func validatePool(cfg store.Config, p provider.Pool) error {
	set, err := provider.FromConfig(cfg)
	if err != nil {
		return err
	}
	return provider.ValidatePool(set, p)
}

// poolSet is the configuration's provider set, for naming the configured pools
// in an error; a configuration that cannot be read has none to name.
func poolSet(cfg store.Config) provider.Set {
	set, err := provider.FromConfig(cfg)
	if err != nil {
		return provider.Set{}
	}
	return set
}

func poolIndex(ps []store.Pool, id string) int {
	for i, p := range ps {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// unknownPool names the pools that do exist.
func unknownPool(set provider.Set, id string) error {
	pools := set.Pools()
	if len(pools) == 0 {
		return fmt.Errorf("%w %q (none is configured)", provider.ErrUnknownPool, id)
	}
	known := make([]string, len(pools))
	for i, p := range pools {
		known[i] = p.ID
	}
	return fmt.Errorf("%w %q (configured: %s)", provider.ErrUnknownPool, id, strings.Join(known, ", "))
}
