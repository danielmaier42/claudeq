package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

func TestCmdPoolLifecycle(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerReady)
	if err := cmdProvider(st, []string{"add", "team", "--kind", string(provider.KindClaudeCode), "--config-dir", "/tmp/team"}); err != nil {
		t.Fatalf("provider add: %v", err)
	}
	if err := cmdPool(st, []string{"add", "claude-pool", "--name", "Claude", "--member", "claude", "--member", "team=5"}); err != nil {
		t.Fatalf("pool add: %v", err)
	}
	out := captureStdout(t, func() error { return cmdPool(st, []string{"list", "--json"}) })
	var pools []poolListView
	if err := json.Unmarshal([]byte(out), &pools); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(pools) != 1 || len(pools[0].Members) != 2 || pools[0].Members[1] != (store.PoolMember{Provider: "team", Weight: 5}) {
		t.Fatalf("pools = %+v", pools)
	}
	if err := cmdPool(st, []string{"add", "bad", "--member", "claude=x"}); err == nil {
		t.Fatal("a weight that is not a number must be refused")
	}

	// A task goes on the pool; naming a provider later moves it off again.
	if err := cmdAdd(st, []string{"--id", "a", "--prompt", "p", "--dir", t.TempDir(), "--pool", "claude-pool"}); err != nil {
		t.Fatalf("add --pool: %v", err)
	}
	if err := cmdEdit(st, []string{"a", "--provider", "team"}); err != nil {
		t.Fatalf("edit --provider: %v", err)
	}
	tk, err := findTask(st, "a")
	if err != nil {
		t.Fatal(err)
	}
	if tk.Pool != "" || tk.Provider != "team" {
		t.Fatalf("task = %+v, want it moved to the provider", tk)
	}
	if err := cmdEdit(st, []string{"a", "--pool", "claude-pool"}); err != nil {
		t.Fatalf("edit --pool: %v", err)
	}
	if tk, _ = findTask(st, "a"); tk.Pool != "claude-pool" || tk.Provider != "" {
		t.Fatalf("task = %+v, want it back on the pool", tk)
	}
	if err := cmdPool(st, []string{"rm", "claude-pool"}); err == nil || !strings.Contains(err.Error(), "task a") {
		t.Fatalf("rm in use: %v", err)
	}

	if err := cmdPool(st, []string{"edit", "claude-pool", "--member", "team"}); err != nil {
		t.Fatalf("pool edit: %v", err)
	}
	set, err := app.Providers(st)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := set.LookupPool("claude-pool"); len(p.Members) != 1 || p.Name != "Claude" {
		t.Fatalf("pool = %+v, want only the members replaced", p)
	}
}

func TestCmdPoolShowRanksTheMembers(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerReady)
	withLimits(t, limitsAdapter{windows: []provider.LimitWindow{{ID: "week", Label: "Week", UsedPercent: 40}}})
	if err := cmdProvider(st, []string{"add", "team", "--kind", string(provider.KindClaudeCode), "--config-dir", "/tmp/team"}); err != nil {
		t.Fatalf("provider add: %v", err)
	}
	if err := cmdPool(st, []string{"add", "p", "--member", "claude", "--member", "team=5"}); err != nil {
		t.Fatalf("pool add: %v", err)
	}
	out := captureStdout(t, func() error { return cmdPool(st, []string{"show", "p", "--json"}) })
	var v poolShowView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	// The same reading on both: the heavier member goes first.
	if len(v.Ranking) != 2 || v.Ranking[0].ProviderID != "team" || v.Ranking[0].Weight != 5 {
		t.Fatalf("ranking = %+v", v.Ranking)
	}
}
