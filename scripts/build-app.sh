#!/usr/bin/env bash
# Build claudeq.app — a native macOS application bundle wrapping the daemon and
# the WKWebView window. The bundle is what makes macOS treat claudeq as its own
# foreground app (its name in the menu bar, its own Dock icon, no terminal
# parent) instead of a bare executable owned by Terminal.
#
# Usage: scripts/build-app.sh [output-dir]   (default: build/)
# Requires: go, rsvg-convert (librsvg), iconutil, sips — all macOS/brew tools.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/build}"
APP="$OUT/claudeq.app"
CONTENTS="$APP/Contents"
MACOS="$CONTENTS/MacOS"
RES="$CONTENTS/Resources"
LOGO="$ROOT/internal/api/web/logo.svg"

# Prefer a release tag (v1.2.3); fall back to 0.1.0 so the plist always has a
# valid dotted version even on an untagged working tree.
VERSION="$(git -C "$ROOT" describe --tags --abbrev=0 2>/dev/null || true)"
VERSION="${VERSION#v}"
case "$VERSION" in
  ''|*[!0-9.]*) VERSION="0.1.0" ;;
esac

echo "==> Building binaries"
mkdir -p "$MACOS" "$RES"
go build -o "$MACOS/claudeqapp" "$ROOT/cmd/claudeqapp"
go build -o "$MACOS/claudeqd" "$ROOT/cmd/claudeqd"

echo "==> Rendering icon from $LOGO"
if command -v rsvg-convert >/dev/null 2>&1; then
  ICONSET="$(mktemp -d)/claudeq.iconset"
  mkdir -p "$ICONSET"
  # macOS iconset: base size + @2x for 16/32/128/256/512.
  for spec in "16:16" "16:32@2x" "32:32" "32:64@2x" \
              "128:128" "128:256@2x" "256:256" "256:512@2x" \
              "512:512" "512:1024@2x"; do
    base="${spec%%:*}"; rest="${spec#*:}"; px="${rest%@*}"
    suffix="${rest#*@}"; [ "$suffix" = "$rest" ] && suffix="" || suffix="@2x"
    rsvg-convert -w "$px" -h "$px" "$LOGO" -o "$ICONSET/icon_${base}x${base}${suffix}.png"
  done
  iconutil -c icns "$ICONSET" -o "$RES/claudeq.icns"
  rm -rf "$(dirname "$ICONSET")"
else
  echo "   rsvg-convert not found — bundling without an icon"
fi

echo "==> Writing Info.plist"
cat > "$CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>ClaudeQ</string>
	<key>CFBundleDisplayName</key><string>ClaudeQ</string>
	<key>CFBundleIdentifier</key><string>ag.dc.claudeq</string>
	<key>CFBundleExecutable</key><string>claudeqapp</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>${VERSION}</string>
	<key>CFBundleVersion</key><string>${VERSION}</string>
	<key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
	<key>CFBundleIconFile</key><string>claudeq</string>
	<key>LSMinimumSystemVersion</key><string>12.0</string>
	<key>NSHighResolutionCapable</key><true/>
	<key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
	<key>LSUIElement</key><false/>
</dict>
</plist>
PLIST

# Ad-hoc code-sign the whole bundle under the bundle identifier. The Go linker
# only ad-hoc-signs the individual executables with a generic "a.out" identity;
# UNUserNotificationCenter needs a stable bundle-level identity to register the
# app and deliver notifications with its icon. A real Developer ID signature, if
# configured, is applied later by build-pkg.sh and overrides this.
echo "==> Ad-hoc signing bundle"
codesign --force --deep --sign - --identifier ag.dc.claudeq "$APP" || \
  echo "   codesign failed; notifications may fall back to a generic icon"

# Refresh Launch Services / Finder so the new icon and metadata show immediately.
touch "$APP"
echo "==> Built $APP (version $VERSION)"
