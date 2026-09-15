package adapters

import (
	"reflect"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
)

func TestDefaultRegistersEveryShippedAdapter(t *testing.T) {
	got := Default().Kinds()
	want := []provider.Kind{provider.KindClaudeCode, provider.KindCodex, provider.KindOpencode}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
}

// TestBetaAdaptersAreRegisteredUnconditionally is the Beta rule in one test:
// a beta adapter is part of the build, whatever the app is currently willing
// to show. Hiding setup controls must never hide the ability to run.
func TestBetaAdaptersAreRegisteredUnconditionally(t *testing.T) {
	for _, kind := range []provider.Kind{provider.KindCodex, provider.KindOpencode} {
		if _, err := Default().Lookup(kind); err != nil {
			t.Fatalf("Lookup %q: %v", kind, err)
		}
	}
}

func TestDefaultResolvesTheClaudeInstanceKind(t *testing.T) {
	// The instance a migrated configuration produces must find its adapter.
	if _, err := Default().Lookup(provider.KindClaudeCode); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
}

// TestEveryAdapterDescribesItself: the app names and explains a provider from
// what the adapter says, so a harness that says nothing would show up blank.
func TestEveryAdapterDescribesItself(t *testing.T) {
	for _, kind := range Default().Kinds() {
		ad, err := Default().Lookup(kind)
		if err != nil {
			t.Fatalf("Lookup %q: %v", kind, err)
		}
		desc := ad.Describe()
		if desc.Name == "" {
			t.Errorf("adapter %q has no display name", kind)
		}
		if desc.DefaultConfigDir == "" {
			t.Errorf("adapter %q does not say where its CLI keeps its configuration", kind)
		}
	}
}
