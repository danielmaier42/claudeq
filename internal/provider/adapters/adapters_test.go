package adapters

import (
	"reflect"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
)

func TestDefaultRegistersEveryShippedAdapter(t *testing.T) {
	got := Default().Kinds()
	want := []provider.Kind{provider.KindClaudeCode, provider.KindCodex}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
}

// TestCodexIsRegisteredUnconditionally is the Beta rule in one test: the adapter
// is part of the build, whatever the app is currently willing to show. Hiding
// setup controls must never hide the ability to run.
func TestCodexIsRegisteredUnconditionally(t *testing.T) {
	if _, err := Default().Lookup(provider.KindCodex); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
}

func TestDefaultResolvesTheClaudeInstanceKind(t *testing.T) {
	// The instance a migrated configuration produces must find its adapter.
	if _, err := Default().Lookup(provider.KindClaudeCode); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
}
