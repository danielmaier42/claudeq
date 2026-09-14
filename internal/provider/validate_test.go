package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateInstance(t *testing.T) {
	reg := NewRegistry(newFakeAdapter("fake"))
	tests := []struct {
		name    string
		inst    Instance
		wantErr string // a fragment the message must name; "" means it must pass
	}{
		{name: "a well-formed instance", inst: Instance{ID: "fake", Kind: "fake"}},
		{
			name: "an absolute binary path and configuration directory",
			inst: Instance{ID: "fake", Kind: "fake", BinaryPath: "/opt/fake", ConfigDir: "/tmp/fake"},
		},
		{name: "no id", inst: Instance{Kind: "fake"}, wantErr: "missing id"},
		{name: "an id with a slash", inst: Instance{ID: "a/b", Kind: "fake"}, wantErr: "may only contain"},
		{name: "an id starting with a dash", inst: Instance{ID: "-a", Kind: "fake"}, wantErr: "may only contain"},
		{name: "no kind", inst: Instance{ID: "fake"}, wantErr: "missing kind"},
		{
			name:    "a kind no adapter implements",
			inst:    Instance{ID: "oc", Kind: "opencode"},
			wantErr: "known kinds: fake",
		},
		{
			name:    "a relative binary path",
			inst:    Instance{ID: "fake", Kind: "fake", BinaryPath: "bin/fake"},
			wantErr: "must be absolute",
		},
		{
			name:    "a relative configuration directory",
			inst:    Instance{ID: "fake", Kind: "fake", ConfigDir: "cfg"},
			wantErr: "must be absolute",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(reg, tc.inst)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, ErrInvalidProvider) {
				t.Fatalf("err = %v, want it to wrap ErrInvalidProvider", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateAcceptsAPathThatIsNotThereYet keeps the check levels apart: a
// provider may be configured before its CLI is installed. Whether the binary
// exists is a readiness question, and the Settings card answers it.
func TestValidateAcceptsAPathThatIsNotThereYet(t *testing.T) {
	reg := NewRegistry(newFakeAdapter("fake"))
	inst := Instance{ID: "fake", Kind: "fake", BinaryPath: "/nowhere/at/all/fake"}
	if err := Validate(reg, inst); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory here: %v", err)
	}
	tests := []struct {
		in, want string
	}{
		{in: "", want: ""},
		{in: "~", want: home},
		{in: "~/.claude_team", want: filepath.Join(home, ".claude_team")},
		{in: "/opt/claude", want: "/opt/claude"},
		// Another user's home is not ours to resolve; the absolute-path check
		// then refuses it by name.
		{in: "~someone/x", want: "~someone/x"},
		{in: "relative/path", want: "relative/path"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := ExpandHome(tc.in); got != tc.want {
				t.Fatalf("ExpandHome(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
