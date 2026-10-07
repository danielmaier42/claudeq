#!/usr/bin/env bash
# Build "Uninstall ClaudeQ.pkg" — the uninstaller, a product archive that
# opens in the macOS Installer. It carries no payload, only scripts
# (scripts/uninstall): the always-selected part removes the app, the
# LaunchAgent, the sudoers entry and the receipt; an optional Customize choice
# deletes the user's data as well.
#
# build-app.sh puts it into the app bundle (Contents/Resources), where the
# Settings button and `claudeq uninstall` find it.
#
# Usage: scripts/build-uninstall-pkg.sh <output.pkg>
# Signed with CLAUDEQ_SIGN_PKG_ID when set, like the installer.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/scripts/uninstall"
OUT="${1:?usage: build-uninstall-pkg.sh <output.pkg>}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Each part gets its own scripts folder; the installer extracts the whole
# folder, so common.sh sits next to the postinstall that sources it.
build_part() { # build_part <dir> <identifier> <file>
  mkdir -p "$WORK/$1"
  cp "$SRC/$1/postinstall" "$SRC/common.sh" "$WORK/$1/"
  chmod +x "$WORK/$1/postinstall"
  # Without extended attributes, pkgbuild stores no AppleDouble ._ files.
  xattr -c "$WORK/$1"/*
  pkgbuild --nopayload --scripts "$WORK/$1" --identifier "$2" --version 1 "$WORK/$3" >/dev/null
}
build_part main de.maierdaniel.claudeq.uninstall uninstall.pkg
build_part data de.maierdaniel.claudeq.uninstall.data uninstall-data.pkg

ARGS=(
  --distribution "$SRC/distribution.xml"
  --resources "$SRC/resources"
  --package-path "$WORK"
)
if [ -n "${CLAUDEQ_SIGN_PKG_ID:-}" ]; then
  ARGS+=(--sign "$CLAUDEQ_SIGN_PKG_ID")
fi
mkdir -p "$(dirname "$OUT")"
productbuild "${ARGS[@]}" "$OUT" >/dev/null
