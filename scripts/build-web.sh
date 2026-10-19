#!/usr/bin/env bash
# Build the web UI (web/) into internal/api/web, where the daemon embeds it.
# Run before `go build` whenever the UI should be part of the binary;
# build-app.sh does it for every app build.
#
# Usage: scripts/build-web.sh [--deps-only]   (--deps-only: install, do not build)
# Requires: Node.js 22+ with npm (web/.nvmrc names the version CI uses).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB="$ROOT/web"

# nvm puts node on the PATH only in interactive shells; a script started from
# elsewhere falls back to the newest nvm-installed version, then Homebrew.
if ! command -v npm >/dev/null 2>&1; then
  for dir in $(ls -d "$HOME"/.nvm/versions/node/v*/bin 2>/dev/null | sort -V -r) /opt/homebrew/bin /usr/local/bin; do
    if [ -x "$dir/npm" ]; then export PATH="$dir:$PATH"; break; fi
  done
fi
command -v npm >/dev/null 2>&1 || { echo "build-web: npm not found — install Node.js 22+" >&2; exit 1; }

cd "$WEB"
# Install only when the lockfile changed since the last install.
if [ ! -f node_modules/.package-lock.json ] || [ package-lock.json -nt node_modules/.package-lock.json ]; then
  npm ci --no-audit --no-fund
fi
[ "${1:-}" = "--deps-only" ] && exit 0
npm run build
