#!/usr/bin/env bash
# Build claudeq-<version>.pkg — a macOS installer that drops claudeq.app into
# /Applications and (via postinstall) sets up the per-user LaunchAgent so the
# daemon starts at login.
#
# Unsigned by default. To sign + notarize (notarization-ready hooks below), set:
#   CLAUDEQ_SIGN_APP_ID   Developer ID Application: … (signs the .app)
#   CLAUDEQ_SIGN_PKG_ID   Developer ID Installer: …   (signs the .pkg)
#   CLAUDEQ_NOTARY_PROFILE  notarytool keychain profile name (staples the .pkg)
#
# Usage: scripts/build-pkg.sh [output-dir]   (default: dist/)
# Requires: go, rsvg-convert, iconutil, pkgbuild (all macOS tools).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/dist}"
IDENTIFIER="ag.dc.claudeq"

VERSION="$(git -C "$ROOT" describe --tags --abbrev=0 2>/dev/null || true)"
VERSION="${VERSION#v}"
case "$VERSION" in
  ''|*[!0-9.]*) VERSION="0.1.0" ;;
esac

echo "==> Building app bundle"
"$ROOT/scripts/build-app.sh" "$ROOT/build"
APP="$ROOT/build/claudeq.app"

# Optionally sign the app (notarization requires a hardened, signed app).
if [ -n "${CLAUDEQ_SIGN_APP_ID:-}" ]; then
  echo "==> Signing app with $CLAUDEQ_SIGN_APP_ID"
  codesign --deep --force --options runtime --timestamp \
    --sign "$CLAUDEQ_SIGN_APP_ID" "$APP"
fi

echo "==> Staging payload"
STAGE="$(mktemp -d)"
mkdir -p "$STAGE/Applications"
# ditto is the macOS-correct way to copy an app bundle. (pkgbuild encodes each
# executable's com.apple.provenance xattr as an AppleDouble entry in the payload
# — that is standard and the installer decodes it back to an xattr; no visible
# ._ files end up in /Applications.)
ditto "$APP" "$STAGE/Applications/claudeq.app"

echo "==> Building installer package"
mkdir -p "$OUT"
PKG="$OUT/claudeq-$VERSION.pkg"
PKGARGS=(
  --root "$STAGE"
  --identifier "$IDENTIFIER"
  --version "$VERSION"
  --install-location "/"
  --scripts "$ROOT/scripts/pkg"
)
# Sign the installer if a Developer ID Installer identity is provided.
if [ -n "${CLAUDEQ_SIGN_PKG_ID:-}" ]; then
  PKGARGS+=(--sign "$CLAUDEQ_SIGN_PKG_ID")
fi
pkgbuild "${PKGARGS[@]}" "$PKG"

# Notarize + staple if a notarytool profile is configured (no-op otherwise).
if [ -n "${CLAUDEQ_NOTARY_PROFILE:-}" ]; then
  echo "==> Notarizing (profile $CLAUDEQ_NOTARY_PROFILE)"
  xcrun notarytool submit "$PKG" --keychain-profile "$CLAUDEQ_NOTARY_PROFILE" --wait
  xcrun stapler staple "$PKG"
fi

rm -rf "$STAGE"
echo "==> Built $PKG (version $VERSION)"
