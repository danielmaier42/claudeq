package adapters

import (
	"reflect"
	"testing"

	"github.com/danielmaier42/claudeq/internal/provider"
)

func TestDefaultRegistersEveryShippedAdapter(t *testing.T) {
	got := Default().Kinds()
	want := []provider.Kind{provider.KindClaudeCode}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
}

func TestDefaultResolvesTheClaudeInstanceKind(t *testing.T) {
	// The instance a migrated configuration produces must find its adapter.
	if _, err := Default().Lookup(provider.KindClaudeCode); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
}
