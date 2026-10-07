// Package uninstall finds and starts ClaudeQ's uninstaller, "Uninstall
// ClaudeQ.pkg", which ships inside the app bundle (scripts/build-uninstall-pkg.sh).
// The package does the removal itself, as root, behind the macOS Installer's
// own password prompt (NFA-06).
package uninstall

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/danielmaier42/claudeq/internal/system"
	"github.com/danielmaier42/claudeq/internal/update"
)

// PkgName is the uninstaller's file name in the bundle's Contents/Resources.
const PkgName = "Uninstall ClaudeQ.pkg"

// DataChoice is the package's optional choice that deletes tasks, history and
// settings too ("Customize" in the Installer).
const DataChoice = "data"

// Locate returns the uninstaller of the app bundle exe belongs to, or else of
// /Applications/ClaudeQ.app.
func Locate(exe string) (string, error) {
	return locate(exe, update.DefaultAppPath)
}

func locate(exe, defaultApp string) (string, error) {
	var apps []string
	if app := update.EnclosingBundle(exe); app != "" {
		apps = append(apps, app)
	}
	apps = append(apps, defaultApp)
	for _, app := range apps {
		pkg := filepath.Join(app, "Contents", "Resources", PkgName)
		if fi, err := os.Stat(pkg); err == nil && !fi.IsDir() {
			return pkg, nil
		}
	}
	return "", fmt.Errorf("%s not found in %s", PkgName, filepath.Join(defaultApp, "Contents", "Resources"))
}

// Stage copies pkg out of the bundle and returns the copy: the uninstaller
// deletes the bundle it ships in, so the Installer must not read it from
// there. The copy always goes to the same place in the user's temporary
// folder, so asking twice opens the one Installer window again instead of a
// second one, and leaves no pile of copies behind.
func Stage(pkg string) (string, error) {
	dir := filepath.Join(os.TempDir(), "claudeq-uninstall")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create staging folder: %w", err)
	}
	dst := filepath.Join(dir, PkgName)
	if err := copyFile(pkg, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// Open stages the uninstaller of exe's bundle and opens it in the macOS
// Installer, which takes it from there: welcome, Customize, password, removal.
func Open(ctx context.Context, r system.Runner, exe string) error {
	pkg, err := Locate(exe)
	if err != nil {
		return err
	}
	staged, err := Stage(pkg)
	if err != nil {
		return err
	}
	if out, err := r.Run(ctx, "open", staged); err != nil {
		return fmt.Errorf("open %s: %w (%s)", staged, err, out)
	}
	return nil
}

// ChoiceChanges is the `installer -applyChoiceChangesXML` file that selects
// the data choice, so a terminal uninstall can delete the data too.
func ChoiceChanges() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<array>
	<dict>
		<key>attributeSetting</key>
		<integer>1</integer>
		<key>choiceAttribute</key>
		<string>selected</string>
		<key>choiceIdentifier</key>
		<string>` + DataChoice + `</string>
	</dict>
</array>
</plist>
`)
}

// copyFile writes src to dst through a temporary file and a rename, so an
// Installer that still has an earlier copy open keeps reading intact bytes.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // the uninstaller inside our own bundle
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(filepath.Dir(dst), ".staging-*")
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() { _ = os.Remove(out.Name()) }()
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if err := os.Rename(out.Name(), dst); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
