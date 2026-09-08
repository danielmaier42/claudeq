package update

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultAppPath is where the installer package puts the app bundle. A daemon
// running from anywhere else (a second copy the user kept in ~/Applications, a
// stale LaunchAgent still pointing at an old bundle) is exactly the situation
// [InstalledApp] exists to detect.
const DefaultAppPath = "/Applications/ClaudeQ.app"

// Installed describes an installed ClaudeQ app bundle on disk.
type Installed struct {
	// Version is the bundle's CFBundleShortVersionString, normalized ("0.8.1").
	Version string
	// Path is the bundle itself, e.g. "/Applications/ClaudeQ.app".
	Path string
}

// InstalledApp reports the newest ClaudeQ app bundle on disk, comparing the
// bundle the running executable lives in with the canonical install location.
// It returns nil when neither carries a real release version.
//
// The running daemon's own version can lag behind that bundle: the installer
// replaces the files in /Applications, but if handing the LaunchAgent over to
// the new binary fails, launchd keeps the previous build alive. The dashboard
// would then keep offering an update that is, in fact, already installed.
func InstalledApp(exePath string) *Installed {
	return installedApp(exePath, DefaultAppPath)
}

// installedApp is [InstalledApp] with the canonical location injected, so tests
// can point it at a temporary bundle instead of /Applications.
func installedApp(exePath, defaultApp string) *Installed {
	var best *Installed
	for _, app := range candidateApps(exePath, defaultApp) {
		v := BundleVersion(app)
		if !IsReleaseVersion(v) {
			continue
		}
		if best == nil || IsNewer(v, best.Version) {
			best = &Installed{Version: Normalize(v), Path: app}
		}
	}
	return best
}

// candidateApps lists the app bundles worth inspecting: the one containing
// exePath (…/ClaudeQ.app/Contents/MacOS/claudeqd) and the canonical location.
func candidateApps(exePath, defaultApp string) []string {
	apps := []string{}
	if app := enclosingBundle(exePath); app != "" {
		apps = append(apps, app)
	}
	if len(apps) == 0 || apps[0] != defaultApp {
		apps = append(apps, defaultApp)
	}
	return apps
}

// enclosingBundle returns the .app bundle a binary lives in, "" if it does not
// sit inside one.
func enclosingBundle(exePath string) string {
	if exePath == "" {
		return ""
	}
	dir := filepath.Dir(exePath) // …/Contents/MacOS
	if filepath.Base(dir) != "MacOS" {
		return ""
	}
	contents := filepath.Dir(dir)
	if filepath.Base(contents) != "Contents" {
		return ""
	}
	app := filepath.Dir(contents)
	if !strings.HasSuffix(app, ".app") {
		return ""
	}
	return app
}

// BundleVersion reads CFBundleShortVersionString from an app bundle's
// Info.plist, "" if the bundle or the key is missing. The plist is the one
// scripts/build-app.sh writes — a small, flat XML file — so it is scanned
// directly rather than shelling out to a plist parser.
func BundleVersion(appPath string) string {
	if appPath == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(appPath, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	return plistString(string(b), "CFBundleShortVersionString")
}

// plistString returns the <string> value following <key>name</key> in an XML
// plist, "" if the key has no string value.
func plistString(plist, name string) string {
	i := strings.Index(plist, "<key>"+name+"</key>")
	if i < 0 {
		return ""
	}
	rest := plist[i+len("<key>"+name+"</key>"):]
	start := strings.Index(rest, "<string>")
	if start < 0 {
		return ""
	}
	// A following <key> before any <string> means this key holds another type.
	if k := strings.Index(rest, "<key>"); k >= 0 && k < start {
		return ""
	}
	rest = rest[start+len("<string>"):]
	end := strings.Index(rest, "</string>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}
