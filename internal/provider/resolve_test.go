package provider

import (
	"errors"
	"testing"

	"github.com/danielmaier42/claudeq/internal/store"
)

func testSet(t *testing.T) Set {
	t.Helper()
	s, err := NewSet("claude", []Instance{
		{ID: "claude", Kind: KindClaudeCode, Name: "Claude Code", DefaultModel: "sonnet", Enabled: true},
		{ID: "fake", Kind: "fake", Name: "Fake", DefaultModel: "fake-default", Enabled: true},
		{ID: "off", Kind: "fake", Name: "Off", DefaultModel: "irrelevant", Enabled: false},
		{ID: "nodefault", Kind: "fake", Name: "No default", Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	return s
}

// TestResolveFollowsTheInheritanceTable walks the documented resolution table
// row by row. The crucial row is the third: naming only a provider must take
// that provider's own default model, never carry one over from the old one.
func TestResolveFollowsTheInheritanceTable(t *testing.T) {
	tests := []struct {
		name      string
		sel       Selection
		wantID    string
		wantModel string
	}{
		{
			name:      "neither specified: default provider and its default model",
			sel:       Selection{},
			wantID:    "claude",
			wantModel: "sonnet",
		},
		{
			name:      "only a model: default provider, explicit model",
			sel:       Selection{Model: "opus"},
			wantID:    "claude",
			wantModel: "opus",
		},
		{
			name:      "only a provider: that provider's default model",
			sel:       Selection{ProviderID: "fake"},
			wantID:    "fake",
			wantModel: "fake-default",
		},
		{
			name:      "both: exactly what was asked for",
			sel:       Selection{ProviderID: "fake", Model: "fake-pro"},
			wantID:    "fake",
			wantModel: "fake-pro",
		},
		{
			name:      "a provider without a default model leaves the harness to choose",
			sel:       Selection{ProviderID: "nodefault"},
			wantID:    "nodefault",
			wantModel: "",
		},
	}

	set := testSet(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := set.Resolve(tc.sel)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.Instance.ID != tc.wantID {
				t.Fatalf("provider = %q, want %q", got.Instance.ID, tc.wantID)
			}
			if got.Model != tc.wantModel {
				t.Fatalf("model = %q, want %q", got.Model, tc.wantModel)
			}
		})
	}
}

func TestResolveNeverCarriesAModelIntoAnotherProvider(t *testing.T) {
	set := testSet(t)
	// What the parent ran with.
	parent, err := set.Resolve(Selection{Model: "opus"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if parent.Model != "opus" {
		t.Fatalf("parent model = %q, want opus", parent.Model)
	}
	// Switching provider without naming a model must land on the new
	// provider's default, not on "opus", which the new provider knows nothing
	// about.
	child, err := set.Resolve(Selection{ProviderID: "fake"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if child.Model != "fake-default" {
		t.Fatalf("model = %q, want the new provider's default", child.Model)
	}
}

func TestResolveRefusesInsteadOfSubstituting(t *testing.T) {
	set := testSet(t)
	tests := []struct {
		name    string
		sel     Selection
		wantErr error
	}{
		{name: "unknown provider", sel: Selection{ProviderID: "codex-work"}, wantErr: ErrUnknownProvider},
		{name: "disabled provider", sel: Selection{ProviderID: "off"}, wantErr: ErrProviderDisabled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := set.Resolve(tc.sel)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got.Instance.ID != "" {
				t.Fatalf("a failed resolution must not name a provider, got %q", got.Instance.ID)
			}
		})
	}
}

func TestResolveWithoutAnyProviderConfigured(t *testing.T) {
	empty, err := NewSet("", nil)
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	if _, err := empty.Resolve(Selection{}); !errors.Is(err, ErrNoProviders) {
		t.Fatalf("err = %v, want ErrNoProviders", err)
	}
}

func TestNewSetRejectsAnUnusableInstanceList(t *testing.T) {
	tests := []struct {
		name      string
		defaultID string
		instances []Instance
	}{
		{name: "empty id", instances: []Instance{{Kind: KindClaudeCode}}},
		{
			name:      "duplicate id",
			instances: []Instance{{ID: "claude", Kind: KindClaudeCode}, {ID: "claude", Kind: KindClaudeCode}},
		},
		{
			name:      "default names nothing",
			defaultID: "missing",
			instances: []Instance{{ID: "claude", Kind: KindClaudeCode}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSet(tc.defaultID, tc.instances); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestFromSettingsMigratesThePreProviderConfiguration is the compatibility
// case: an existing claudeq configuration becomes the `claude` instance, and
// every task that names no provider keeps running exactly as before.
func TestFromSettingsMigratesThePreProviderConfiguration(t *testing.T) {
	set := FromSettings(store.Settings{ClaudePath: "/opt/claude", DefaultModel: "opus"})

	all := set.All()
	if len(all) != 1 {
		t.Fatalf("got %d instances, want exactly the migrated one", len(all))
	}
	inst := all[0]
	if inst.ID != DefaultInstanceID || inst.Kind != KindClaudeCode {
		t.Fatalf("instance = %+v, want id %q of kind %q", inst, DefaultInstanceID, KindClaudeCode)
	}
	if inst.BinaryPath != "/opt/claude" {
		t.Fatalf("binary path = %q, want the configured claude_path", inst.BinaryPath)
	}
	if inst.DefaultModel != "opus" {
		t.Fatalf("default model = %q, want the configured default_model", inst.DefaultModel)
	}
	if !inst.Enabled {
		t.Fatal("the migrated instance must be enabled")
	}
	if inst.ConfigDir != "" {
		t.Fatalf("config dir = %q, want the CLI's own configuration", inst.ConfigDir)
	}

	res, err := set.Resolve(Selection{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Instance.ID != DefaultInstanceID || res.Model != "opus" {
		t.Fatalf("resolved %+v, want the migrated instance on the configured model", res)
	}
}

func TestFromSettingsIsIdempotent(t *testing.T) {
	// Migration runs on every load, so applying it twice must produce exactly
	// the same instance.
	s := store.Settings{ClaudePath: "/opt/claude", DefaultModel: "opus"}
	first, second := FromSettings(s), FromSettings(s)
	if first.DefaultID() != second.DefaultID() {
		t.Fatal("default provider changed between migrations")
	}
	a, b := first.All(), second.All()
	if len(a) != len(b) || a[0] != b[0] {
		t.Fatalf("migration is not idempotent: %+v vs %+v", a, b)
	}
}

func TestFromSettingsOnAFreshInstallation(t *testing.T) {
	// Nothing configured yet: the claude provider still exists, with no binary
	// path and no model, so the harness picks its own default.
	set := FromSettings(store.Settings{})
	res, err := set.Resolve(Selection{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Instance.ID != DefaultInstanceID || res.Instance.BinaryPath != "" || res.Model != "" {
		t.Fatalf("resolved %+v, want the claude instance with nothing configured", res)
	}
}

func TestLookupAndAllDoNotAliasTheSet(t *testing.T) {
	set := testSet(t)
	all := set.All()
	all[0].DefaultModel = "tampered"
	again, ok := set.Lookup("claude")
	if !ok {
		t.Fatal("claude should be configured")
	}
	if again.DefaultModel != "sonnet" {
		t.Fatalf("the set was mutated through All(): %q", again.DefaultModel)
	}
}
