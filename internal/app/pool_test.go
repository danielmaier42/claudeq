package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
)

func TestPoolLifecycle(t *testing.T) {
	s := openStore(t)
	if err := AddProvider(s, registry(), provider.Instance{
		ID: "team", Kind: provider.KindClaudeCode, Name: "Team", ConfigDir: "/tmp/team", Enabled: true,
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	p := provider.Pool{ID: "claude-pool", Name: "Claude", Members: []provider.PoolMember{
		{ProviderID: provider.DefaultInstanceID}, {ProviderID: "team", Weight: 5},
	}}
	if err := AddPool(s, p); err != nil {
		t.Fatalf("AddPool: %v", err)
	}
	if err := AddPool(s, p); err == nil {
		t.Fatal("a second pool with the same id must be refused")
	}
	// A provider may not take a pool's id, nor a pool a provider's.
	if err := AddProvider(s, registry(), provider.Instance{ID: "claude-pool", Kind: provider.KindClaudeCode, Enabled: true}); err == nil {
		t.Fatal("a provider with a pool's id must be refused")
	}

	if err := EditPool(s, "claude-pool", func(p *provider.Pool) error { p.Name = "Both"; p.Members = p.Members[:1]; return nil }); err != nil {
		t.Fatalf("EditPool: %v", err)
	}
	set, err := Providers(s)
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	got, ok := set.LookupPool("claude-pool")
	if !ok || got.Name != "Both" || len(got.Members) != 1 {
		t.Fatalf("pool = %+v, want the edit applied", got)
	}
	if err := EditPool(s, "claude-pool", func(p *provider.Pool) error { p.ID = "other"; return nil }); err == nil {
		t.Fatal("the pool id must not change")
	}
	if err := EditPool(s, "nope", func(*provider.Pool) error { return nil }); !errors.Is(err, provider.ErrUnknownPool) {
		t.Fatalf("err = %v, want ErrUnknownPool", err)
	}

	// A member cannot be removed from under its pool.
	if err := RemoveProvider(s, provider.DefaultInstanceID); err == nil || !strings.Contains(err.Error(), "pool claude-pool") {
		t.Fatalf("err = %v, want the pool named", err)
	}
	// Nor a pool from under its tasks.
	tk := mk("a")
	tk.Pool = "claude-pool"
	if err := AddTask(s, tk); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if err := RemovePool(s, "claude-pool"); err == nil || !strings.Contains(err.Error(), "task a") {
		t.Fatalf("err = %v, want the task named", err)
	}
	if err := RemoveTask(s, "a"); err != nil {
		t.Fatalf("RemoveTask: %v", err)
	}
	if err := RemovePool(s, "claude-pool"); err != nil {
		t.Fatalf("RemovePool: %v", err)
	}
}

func TestEnsurePoolRunnable(t *testing.T) {
	s := openStore(t)
	if err := AddProvider(s, registry(), provider.Instance{
		ID: "team", Kind: provider.KindClaudeCode, Name: "Team", ConfigDir: "/tmp/team", Enabled: true,
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	if err := AddPool(s, provider.Pool{ID: "p", Members: []provider.PoolMember{
		{ProviderID: provider.DefaultInstanceID}, {ProviderID: "team"},
	}}); err != nil {
		t.Fatalf("AddPool: %v", err)
	}
	set, err := Providers(s)
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	ctx := context.Background()
	if err := EnsureTaskTarget(ctx, set, readinessChecker(provider.Health{State: provider.HealthReady}), "", "p"); err != nil {
		t.Fatalf("ready pool refused: %v", err)
	}
	err = EnsureTaskTarget(ctx, set, readinessChecker(provider.Health{State: provider.HealthNotAuthenticated, Reason: "not logged in"}), "", "p")
	if err == nil || !strings.Contains(err.Error(), "no member of pool") || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v, want every member's reason", err)
	}
	if err := EnsureTaskTarget(ctx, set, readinessChecker(provider.Health{State: provider.HealthReady}), "", "nope"); !errors.Is(err, provider.ErrUnknownPool) {
		t.Fatalf("err = %v, want ErrUnknownPool", err)
	}
}
