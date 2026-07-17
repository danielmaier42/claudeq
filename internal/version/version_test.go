package version

import "testing"

func TestStringReportsOverriddenVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	Version = "v1.2.3"
	if got, want := String(), "v1.2.3"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestStringNeverEmpty(t *testing.T) {
	if String() == "" {
		t.Fatal("String() returned an empty version")
	}
}
