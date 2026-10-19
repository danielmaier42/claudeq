package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyHomeMovesTheDataAndLinksTheOldPath(t *testing.T) {
	base := t.TempDir()
	legacy, home := filepath.Join(base, "claudeq"), filepath.Join(base, "dispatchq")
	if err := os.MkdirAll(filepath.Join(legacy, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "runs", "r1.log"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyHome(legacy, home); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(home, "runs", "r1.log")); err != nil || string(data) != "log" {
		t.Fatalf("data not moved: %q %v", data, err)
	}
	// A log path recorded under the old directory still resolves.
	if data, err := os.ReadFile(filepath.Join(legacy, "runs", "r1.log")); err != nil || string(data) != "log" {
		t.Fatalf("old path does not resolve: %q %v", data, err)
	}
	if info, err := os.Lstat(legacy); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("old path is not a link: %v %v", info, err)
	}

	// Idempotent: a second start finds the move done.
	if err := migrateLegacyHome(legacy, home); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestMigrateLegacyHomeLeavesAnExistingHomeAlone(t *testing.T) {
	base := t.TempDir()
	legacy, home := filepath.Join(base, "claudeq"), filepath.Join(base, "dispatchq")
	for _, d := range []string{legacy, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateLegacyHome(legacy, home); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if info, err := os.Lstat(legacy); err != nil || !info.IsDir() {
		t.Fatalf("legacy dir must stay untouched: %v %v", info, err)
	}
}

func TestMigrateLegacyHomeWithoutClaudeQ(t *testing.T) {
	base := t.TempDir()
	if err := migrateLegacyHome(filepath.Join(base, "claudeq"), filepath.Join(base, "dispatchq")); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(base, "dispatchq")); !os.IsNotExist(err) {
		t.Fatalf("nothing should be created: %v", err)
	}
}

func TestHomeOverrideFallsBackToClaudeQsVariable(t *testing.T) {
	t.Setenv(EnvHome, "")
	t.Setenv(LegacyEnvHome, "/old")
	if got := HomeOverride(); got != "/old" {
		t.Fatalf("HomeOverride = %q, want /old", got)
	}
	t.Setenv(EnvHome, "/new")
	if got := HomeOverride(); got != "/new" {
		t.Fatalf("HomeOverride = %q, want /new", got)
	}
}

func TestPrimeWhatsNewOnlyOnAFreshInstall(t *testing.T) {
	fresh, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.PrimeWhatsNew("2.0"); err != nil {
		t.Fatal(err)
	}
	if st, _ := fresh.LoadState(); st.WhatsNewSeen != "2.0" || !st.RenameCheckDone {
		t.Errorf("fresh install: seen = %q, check done = %v, want 2.0 and done", st.WhatsNewSeen, st.RenameCheckDone)
	}

	upgraded, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := upgraded.SaveConfig(Config{}); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.PrimeWhatsNew("2.0"); err != nil {
		t.Fatal(err)
	}
	if st, _ := upgraded.LoadState(); st.WhatsNewSeen != "" || st.RenameCheckDone {
		t.Errorf("upgrade: seen = %q, check done = %v, want neither so the dialog and banner show", st.WhatsNewSeen, st.RenameCheckDone)
	}
}
