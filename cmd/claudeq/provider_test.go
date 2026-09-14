package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/provider"
	"github.com/danielmaier42/claudeq/internal/store"
)

// stubAdapter answers readiness from a scripted verdict, so the CLI's provider
// rules are tested on a machine where no harness is installed.
type stubAdapter struct{ health provider.Health }

func (stubAdapter) Kind() provider.Kind                 { return provider.KindClaudeCode }
func (stubAdapter) Capabilities() provider.Capabilities { return provider.Capabilities{} }
func (stubAdapter) DetectBinary() string                { return "" }
func (stubAdapter) ResolveBinary(provider.Instance) string {
	return ""
}

func (s stubAdapter) CheckHealth(context.Context, provider.Instance, provider.Prober) provider.Health {
	return s.health
}

func (stubAdapter) Command(provider.Instance, provider.Request) (provider.Command, error) {
	return provider.Command{}, nil
}
func (stubAdapter) NewParser() provider.Parser { return nil }

// withProviderHealth makes every readiness check in this test answer h.
func withProviderHealth(t *testing.T, h provider.Health) {
	t.Helper()
	previous := newProviderChecker
	newProviderChecker = func() *provider.Checker {
		return provider.NewChecker(provider.NewRegistry(stubAdapter{health: h}))
	}
	t.Cleanup(func() { newProviderChecker = previous })
}

var (
	providerReady   = provider.Health{State: provider.HealthReady}
	providerMissing = provider.Health{
		State:  provider.HealthNotInstalled,
		Reason: "Claude Code was not found. Install the CLI, or set its path in Settings → Providers.",
	}
)

func TestCmdAddRefusesAnUnreadyProvider(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerMissing)

	err := cmdAdd(st, []string{"--id", "a", "--prompt", "do a", "--dir", t.TempDir()})
	if err == nil {
		t.Fatal("expected the task to be refused")
	}
	if !strings.Contains(err.Error(), "Claude Code was not found") {
		t.Fatalf("err = %q, want the readiness reason", err)
	}
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Fatalf("a refused task must not be stored, got %+v", cfg.Tasks)
	}
}

func TestCmdAddRefusesAnUnknownProvider(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerReady)

	err := cmdAdd(st, []string{"--id", "a", "--prompt", "do a", "--dir", t.TempDir(), "--provider", "codex-work"})
	if err == nil || !strings.Contains(err.Error(), "codex-work") {
		t.Fatalf("err = %v, want the unknown provider named", err)
	}
}

func TestCmdQueueRefusesAnUnreadyProvider(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerMissing)

	if err := cmdQueue(st, []string{"--prompt", "follow up"}); err == nil {
		t.Fatal("expected the follow-up to be refused")
	}
	cfg, _ := st.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Fatalf("a refused follow-up must not be stored, got %+v", cfg.Tasks)
	}
}

func TestCmdAddStoresTheProvider(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerReady)
	if err := app.AddProvider(st, newProviderChecker().Registry, provider.Instance{
		ID: "second", Kind: provider.KindClaudeCode, Name: "second", Enabled: true,
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	args := []string{"--id", "a", "--prompt", "do a", "--dir", t.TempDir(), "--provider", "second"}
	if err := cmdAdd(st, args); err != nil {
		t.Fatalf("cmdAdd: %v", err)
	}
	got, err := findTask(st, "a")
	if err != nil {
		t.Fatalf("findTask: %v", err)
	}
	if got.Provider != "second" {
		t.Fatalf("provider = %q, want the one passed", got.Provider)
	}
}

// TestChangingTheProviderDropsAnInheritedModel is the resolution rule that keeps
// one harness's model out of another's: with no model named, the new provider's
// own default applies.
func TestChangingTheProviderDropsAnInheritedModel(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantModel string
	}{
		{name: "provider only", args: []string{"--provider", "second"}, wantModel: ""},
		{
			name:      "provider and model",
			args:      []string{"--provider", "second", "--model", "gpt-5.6-sol"},
			wantModel: "gpt-5.6-sol",
		},
		{name: "model only", args: []string{"--model", "haiku"}, wantModel: "haiku"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			patch, err := parseTaskPatch(tc.args, nil)
			if err != nil {
				t.Fatalf("parseTaskPatch: %v", err)
			}
			got, err := patch.apply(baseTask())
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if got.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", got.Model, tc.wantModel)
			}
		})
	}
}

func TestCmdProviderLifecycle(t *testing.T) {
	st := newTestStore(t)
	withProviderHealth(t, providerReady)

	if err := cmdProvider(st, []string{"add", "second", "--kind", string(provider.KindClaudeCode),
		"--name", "Second account", "--config-dir", "/tmp/second", "--default-model", "haiku"}); err != nil {
		t.Fatalf("provider add: %v", err)
	}
	set, err := app.Providers(st)
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	inst, ok := set.Lookup("second")
	if !ok {
		t.Fatal("the added provider is missing")
	}
	if inst.Name != "Second account" || inst.ConfigDir != "/tmp/second" || inst.DefaultModel != "haiku" || !inst.Enabled {
		t.Fatalf("instance = %+v, want the flags applied", inst)
	}

	if err := cmdProvider(st, []string{"edit", "second", "--default-model", "sonnet"}); err != nil {
		t.Fatalf("provider edit: %v", err)
	}
	if err := cmdProvider(st, []string{"disable", "second"}); err != nil {
		t.Fatalf("provider disable: %v", err)
	}
	set, _ = app.Providers(st)
	inst, _ = set.Lookup("second")
	if inst.DefaultModel != "sonnet" || inst.Enabled {
		t.Fatalf("instance = %+v, want the edit and the switch-off applied", inst)
	}

	// A switched-off instance cannot become the one every task without a
	// provider runs on.
	if err := cmdProvider(st, []string{"default", "second"}); err == nil {
		t.Fatal("making a switched-off provider the default must be refused")
	}
	if err := cmdProvider(st, []string{"enable", "second"}); err != nil {
		t.Fatalf("provider enable: %v", err)
	}
	if err := cmdProvider(st, []string{"default", "second"}); err != nil {
		t.Fatalf("provider default: %v", err)
	}
	set, _ = app.Providers(st)
	if set.DefaultID() != "second" {
		t.Fatalf("default = %q, want the one just chosen", set.DefaultID())
	}

	// It is the default now, so neither removing nor switching it off may work
	// until that changes.
	if err := cmdProvider(st, []string{"rm", "second"}); err == nil {
		t.Fatal("removing a referenced provider must be refused")
	}
	if err := cmdProvider(st, []string{"disable", "second"}); err == nil {
		t.Fatal("switching off the default provider must be refused")
	}
	if err := cmdProvider(st, []string{"default", store.DefaultProviderID}); err != nil {
		t.Fatalf("provider default: %v", err)
	}
	if err := cmdProvider(st, []string{"rm", "second"}); err != nil {
		t.Fatalf("provider rm: %v", err)
	}
	set, _ = app.Providers(st)
	if _, ok := set.Lookup("second"); ok {
		t.Fatal("the provider is still configured")
	}
}

func TestCmdProviderRejectsBadInput(t *testing.T) {
	st := newTestStore(t)
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown subcommand", args: []string{"frobnicate"}},
		{name: "add without a kind", args: []string{"add", "x"}},
		{name: "add with an unimplemented kind", args: []string{"add", "x", "--kind", "opencode"}},
		{name: "add with a relative path", args: []string{"add", "x", "--kind", "claude-code", "--path", "bin/x"}},
		{name: "edit without changes", args: []string{"edit", store.DefaultProviderID}},
		{name: "edit an unknown id", args: []string{"edit", "nope", "--name", "x"}},
		{name: "show without an id", args: []string{"show"}},
		{name: "enable without an id", args: []string{"enable"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := cmdProvider(st, tc.args); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestCmdImportRefusesAnUnreadyProvider: a bundle carries no machine-local
// provider, so the imported task runs on the default one — which still has to
// be able to run it.
func TestCmdImportRefusesAnUnreadyProvider(t *testing.T) {
	src := newTestStore(t)
	if err := app.AddTask(src, baseTask()); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	out := filepath.Join(t.TempDir(), "shared.claudeq")
	withProviderHealth(t, providerReady)
	if err := cmdExport(src, []string{"nightly", "--out", out}); err != nil {
		t.Fatalf("cmdExport: %v", err)
	}

	withProviderHealth(t, providerMissing)
	dst := newTestStore(t)
	if err := cmdImport(dst, []string{out}); err == nil {
		t.Fatal("expected the import to be refused")
	}
	cfg, _ := dst.LoadConfig()
	if len(cfg.Tasks) != 0 {
		t.Fatalf("a refused import must not be stored, got %+v", cfg.Tasks)
	}
}
