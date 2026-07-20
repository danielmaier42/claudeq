package fileaccess

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	readableEmpty := t.TempDir()

	readableWithEntry := t.TempDir()
	if err := os.WriteFile(filepath.Join(readableWithEntry, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist")

	tests := []struct {
		name        string
		paths       []string
		wantOK      bool
		wantBlocked string
		wantReason  Reason
	}{
		{name: "empty dir is readable", paths: []string{readableEmpty}, wantOK: true},
		{name: "dir with entry is readable", paths: []string{readableWithEntry}, wantOK: true},
		{name: "missing dir is skipped", paths: []string{missing}, wantOK: true},
		{name: "no paths", paths: nil, wantOK: true},
		{name: "empty and duplicate paths ignored", paths: []string{"", readableEmpty, readableEmpty}, wantOK: true},
		{
			name:        "permission-denied dir is blocked",
			paths:       []string{denyDir(t)},
			wantBlocked: "", // filled in below
			wantReason:  ReasonPermission,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Probe(tc.paths, time.Second)
			if tc.wantReason == ReasonPermission {
				// The denied dir is the sole path; assert on reason + that a path
				// was flagged rather than pinning the temp path string.
				if got.OK || got.Reason != ReasonPermission || got.BlockedPath == "" {
					t.Fatalf("want a permission block, got %+v", got)
				}
				return
			}
			if got.OK != tc.wantOK {
				t.Fatalf("OK = %v, want %v (result %+v)", got.OK, tc.wantOK, got)
			}
		})
	}
}

// TestProbeFirstBlockedWins verifies a readable dir before a blocked one still
// reports the blocked one, and the result names it.
func TestProbeFirstBlockedWins(t *testing.T) {
	good := t.TempDir()
	bad := denyDir(t)
	got := Probe([]string{good, bad}, time.Second)
	if got.OK || got.BlockedPath != bad || got.Reason != ReasonPermission {
		t.Fatalf("want block on %q, got %+v", bad, got)
	}
}

// denyDir returns a directory that cannot be read. Skips the test when running
// as root (which bypasses filesystem permissions and defeats the check).
func denyDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("cannot simulate permission denial as root")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	// Restore permissions on cleanup so t.TempDir removal succeeds.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	return dir
}
