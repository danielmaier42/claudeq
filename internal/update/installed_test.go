package update

import (
	"os"
	"path/filepath"
	"testing"
)

// writeBundle creates a minimal .app bundle with the given short version.
func writeBundle(t *testing.T, dir, version string) string {
	t.Helper()
	app := filepath.Join(dir, "ClaudeQ.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>ClaudeQ</string>
	<key>CFBundleShortVersionString</key><string>` + version + `</string>
	<key>LSUIElement</key><false/>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}
	return app
}

func TestBundleVersion(t *testing.T) {
	app := writeBundle(t, t.TempDir(), "0.8.1")
	if got := BundleVersion(app); got != "0.8.1" {
		t.Fatalf("BundleVersion = %q, want 0.8.1", got)
	}
	if got := BundleVersion(filepath.Join(t.TempDir(), "Nope.app")); got != "" {
		t.Fatalf("missing bundle: got %q, want empty", got)
	}
	if got := BundleVersion(""); got != "" {
		t.Fatalf("empty path: got %q, want empty", got)
	}
}

func TestPlistString(t *testing.T) {
	tests := []struct {
		name  string
		plist string
		key   string
		want  string
	}{
		{"present", "<key>A</key><string>1.2</string>", "A", "1.2"},
		{"missing key", "<key>B</key><string>1.2</string>", "A", ""},
		{"non-string value", "<key>A</key><true/><key>B</key><string>x</string>", "A", ""},
		{"unterminated", "<key>A</key><string>1.2", "A", ""},
		{"no value at all", "<key>A</key>", "A", ""},
		{"whitespace trimmed", "<key>A</key>\n\t<string> 1.2 </string>", "A", "1.2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := plistString(tc.plist, tc.key); got != tc.want {
				t.Fatalf("plistString = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnclosingBundle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/Applications/ClaudeQ.app/Contents/MacOS/claudeqd", "/Applications/ClaudeQ.app"},
		{"/usr/local/bin/claudeqd", ""},
		{"/x/Contents/MacOS/claudeqd", ""},
		{"/x/ClaudeQ.app/MacOS/claudeqd", ""},
		{"", ""},
	}
	for _, tc := range tests {
		if got := enclosingBundle(tc.in); got != tc.want {
			t.Fatalf("enclosingBundle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInstalledAppPrefersNewestBundle(t *testing.T) {
	// A daemon running out of an old bundle still reports the newer copy, which
	// is what makes a failed hand-over visible to the dashboard.
	oldDir := t.TempDir()
	oldApp := writeBundle(t, oldDir, "0.7.0")
	newDir := t.TempDir()
	newApp := writeBundle(t, newDir, "0.8.1")

	got := installedApp(filepath.Join(oldApp, "Contents", "MacOS", "claudeqd"), newApp)
	if got == nil || got.Version != "0.8.1" || got.Path != newApp {
		t.Fatalf("installedApp = %+v, want 0.8.1 at %s", got, newApp)
	}

	// The daemon's own bundle wins when it is the newer one.
	got = installedApp(filepath.Join(newApp, "Contents", "MacOS", "claudeqd"), oldApp)
	if got == nil || got.Version != "0.8.1" || got.Path != newApp {
		t.Fatalf("installedApp = %+v, want 0.8.1 at %s", got, newApp)
	}
}

func TestInstalledAppNoRealVersion(t *testing.T) {
	dir := t.TempDir()
	if got := installedApp(filepath.Join(dir, "claudeqd"), filepath.Join(dir, "Missing.app")); got != nil {
		t.Fatalf("installedApp = %+v, want nil", got)
	}
	// A dev build's bundle carries no comparable version either.
	app := writeBundle(t, t.TempDir(), "dev")
	if got := installedApp(filepath.Join(app, "Contents", "MacOS", "claudeqd"), app); got != nil {
		t.Fatalf("installedApp = %+v, want nil for a dev bundle", got)
	}
}
