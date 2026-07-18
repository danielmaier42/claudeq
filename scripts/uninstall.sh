#!/usr/bin/env bash
# Uninstall claudeq: stop and remove the LaunchAgent, then delete the app.
# Run as the logged-in user (not via sudo) so the LaunchAgent is removed from
# the right GUI domain. The app is removed from /Applications (may prompt for
# admin rights).
set -euo pipefail

APP="/Applications/claudeq.app"
DAEMON="$APP/Contents/MacOS/claudeqd"

if [ -x "$DAEMON" ]; then
  echo "==> Removing LaunchAgent"
  "$DAEMON" uninstall || true
fi

if [ -d "$APP" ]; then
  echo "==> Removing $APP"
  rm -rf "$APP" 2>/dev/null || sudo rm -rf "$APP"
fi

echo "==> Done. Your tasks, config and run history in"
echo "    ~/Library/Application Support/claudeq are left untouched."
echo "    Remove them with: rm -rf \"\$HOME/Library/Application Support/claudeq\""
