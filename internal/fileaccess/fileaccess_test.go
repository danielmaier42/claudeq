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

// TestProbeNonDirIsOK verifies that pointing the probe at a regular file (an
// "other error" — not a permission or missing-path case) is not flagged as a
// block: the probe catches privacy denials, not misconfiguration.
func TestProbeNonDirIsOK(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Probe([]string{file}, time.Second); !got.OK {
		t.Fatalf("a regular file should not be flagged as blocked, got %+v", got)
	}
}

// TestProbeAll verifies ProbeAll does not stop at the first block: every blocked
// directory is returned, in order, while readable and missing paths are skipped.
func TestProbeAll(t *testing.T) {
	good := t.TempDir()
	missing := filepath.Join(t.TempDir(), "nope")
	bad1 := denyDir(t)
	bad2 := denyDir(t)

	got := ProbeAll([]string{good, bad1, missing, bad2, bad1 /* dup skipped */}, time.Second)
	if len(got) != 2 {
		t.Fatalf("want 2 blocked dirs, got %d (%+v)", len(got), got)
	}
	if got[0].BlockedPath != bad1 || got[1].BlockedPath != bad2 {
		t.Fatalf("blocked order = %q,%q, want %q,%q", got[0].BlockedPath, got[1].BlockedPath, bad1, bad2)
	}
	for _, r := range got {
		if r.Reason != ReasonPermission {
			t.Fatalf("reason = %q, want %q (%+v)", r.Reason, ReasonPermission, r)
		}
	}
}

// TestProbeAllAllReadable verifies ProbeAll returns nothing when every directory
// is readable or absent.
func TestProbeAllAllReadable(t *testing.T) {
	if got := ProbeAll([]string{t.TempDir(), "", t.TempDir()}, time.Second); len(got) != 0 {
		t.Fatalf("want no blocks, got %+v", got)
	}
}

func TestConsentTargets(t *testing.T) {
	home := "/Users/me"
	docs := filepath.Join(home, "Documents")
	dl := filepath.Join(home, "Downloads")
	desk := filepath.Join(home, "Desktop")

	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{
			name:  "many Documents subfolders collapse to one root",
			paths: []string{filepath.Join(docs, "a"), filepath.Join(docs, "b", "deep"), docs},
			want:  []string{docs},
		},
		{
			name:  "distinct categories are each kept once",
			paths: []string{filepath.Join(docs, "a"), filepath.Join(dl, "x"), filepath.Join(desk, "y"), filepath.Join(dl, "z")},
			want:  []string{docs, dl, desk},
		},
		{
			name:  "non-protected folders map to themselves",
			paths: []string{"/Users/me/projects/app", "/Volumes/Ext/work", filepath.Join(docs, "a")},
			want:  []string{"/Users/me/projects/app", "/Volumes/Ext/work", docs},
		},
		{
			name:  "sibling with shared prefix is not collapsed",
			paths: []string{home + "/Documents-old/thing", filepath.Join(docs, "a")},
			want:  []string{home + "/Documents-old/thing", docs},
		},
		{
			name:  "order of first appearance is preserved and empties dropped",
			paths: []string{"", filepath.Join(dl, "a"), filepath.Join(docs, "b"), "", filepath.Join(dl, "c")},
			want:  []string{dl, docs},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConsentTargets(tc.paths, home)
			if !equalStrings(got, tc.want) {
				t.Fatalf("ConsentTargets = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestConsentTargetsNoHome verifies that without a home dir, no category grouping
// happens — paths are only de-duplicated as given.
func TestConsentTargetsNoHome(t *testing.T) {
	in := []string{"/Users/me/Documents/a", "/Users/me/Documents/b", "/Users/me/Documents/a"}
	got := ConsentTargets(in, "")
	want := []string{"/Users/me/Documents/a", "/Users/me/Documents/b"}
	if !equalStrings(got, want) {
		t.Fatalf("ConsentTargets(no home) = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
