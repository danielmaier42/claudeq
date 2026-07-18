<div align="center">
  <img src="internal/api/web/logo.svg" width="96" height="96" alt="ClaudeQ">
  <h1>ClaudeQ</h1>
  <p>Queue Claude Code tasks during the day, run them at night.</p>
</div>

ClaudeQ is a small, local-only macOS tool. You add tasks — a prompt and a working
folder — to a queue during the day; a background daemon runs them with the
[Claude Code](https://claude.com/claude-code) CLI at night, when your usage
allowance resets. A native menu-bar app shows the queue, activity, and usage.

Everything stays on your Mac: tasks, config, and run history live under
`~/Library/Application Support/claudeq`. The daemon listens only on loopback.

## Install

1. Download the latest `claudeq-<version>.pkg` from the
   [Releases](https://github.com/danielmaier42/claudeq/releases) page.
2. Open it and follow the installer.

The package installs **ClaudeQ** to `/Applications` and sets up a per-user
LaunchAgent so the daemon starts at login. Open **ClaudeQ** from Applications to
add tasks.

> The package is not notarized, so on first launch macOS may warn that it is from
> an unidentified developer. Right-click **ClaudeQ → Open**, then confirm — or
> allow it under **System Settings → Privacy & Security**.

To run tasks past a scheduled sleep, ClaudeQ can schedule a wake with `pmset`,
which needs one sudoers entry (the daemon prints the exact line on install).

## Uninstall

```sh
/Applications/claudeq.app/Contents/MacOS/claudeqd uninstall   # remove the LaunchAgent
rm -rf /Applications/claudeq.app
```

Or run [`scripts/uninstall.sh`](scripts/uninstall.sh). Your tasks and history in
`~/Library/Application Support/claudeq` are left in place; delete that folder to
remove them too.

## Build from source

Requires Go 1.26+ and `librsvg` (`brew install librsvg`) for icon rendering.

```sh
scripts/build-app.sh    # build/claudeq.app  (double-click or `open` it)
scripts/build-pkg.sh    # dist/claudeq-<version>.pkg  (installer)
```

Releases are built automatically: pushing a `v*` tag runs
[`.github/workflows/release.yml`](.github/workflows/release.yml), which builds the
`.pkg` on macOS and attaches it to the GitHub Release.

## Requirements

- macOS 12+
- The [Claude Code](https://claude.com/claude-code) CLI, authenticated
