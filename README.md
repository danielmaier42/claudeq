<div align="center">
  <img src="internal/api/web/logo.svg" width="96" height="96" alt="ClaudeQ">
  <h1>ClaudeQ</h1>
  <p>Queue Claude Code tasks during the day, run them at night.</p>
</div>

ClaudeQ is a small, local-only macOS app. During the day you add tasks — a prompt
and a working folder — to a queue. A background daemon runs them with the
[Claude Code](https://claude.com/claude-code) CLI overnight, when your usage
allowance has reset, and you review the results in the morning.

Everything stays on your Mac: tasks, settings, and run history live under
`~/Library/Application Support/claudeq`, and the daemon listens only on loopback.
Nothing but the `claude` CLI (and, on demand, GitHub's public release API for
update checks) ever leaves the machine.

## Contents

- [How it works](#how-it-works)
- [Features](#features)
- [The app](#the-app)
- [Settings](#settings)
- [Install](#install)
- [Using it](#using-it)
- [Command-line interface](#command-line-interface)
- [Letting a task queue follow-up work](#letting-a-task-queue-follow-up-work)
- [How scheduling and the limit gate behave](#how-scheduling-and-the-limit-gate-behave)
- [Data on disk](#data-on-disk)
- [Uninstall](#uninstall)
- [Build from source](#build-from-source)
- [Architecture](#architecture)
- [Requirements](#requirements)
- [License](#license)

## How it works

ClaudeQ is two pieces that share a file-based store:

- **A background daemon (`claudeqd`)** installed as a per-user launchd
  LaunchAgent. It starts at login, restarts if it exits, and does all the work:
  scheduling, running tasks through the Claude Code CLI, tracking the rate
  limit, planning wake-ups, pruning history, sending notifications, and serving
  the dashboard on `127.0.0.1`.
- **A native app window (`claudeqapp`)** that wraps that dashboard in a macOS
  WKWebView window. The app is just the UI — closing it never stops scheduling,
  and the daemon keeps running with the window shut.

The nightly cycle looks like this:

1. You queue tasks during the day. Each task is a **prompt** plus the **folder**
   it runs in (its repo/context), with an optional trigger time.
2. The daemon watches the queue and starts due tasks with the Claude Code CLI in
   headless mode, one at a time by default.
3. If a run hits the **rate limit**, ClaudeQ pauses the whole queue and, once the
   limit clears, resumes the *same* Claude session — no work is lost.
4. To run a timed task past a scheduled sleep, it wakes the Mac with `pmset`, and
   holds the Mac awake (`caffeinate`) while a run is in flight so a task never
   freezes mid-run.
5. Results — success, failure, rate-limit wait, or auth problem — land in
   **Activity** with the full log, optionally with a notification.

## Features

**Scheduling**

- **Three trigger modes** per task:
  - *As soon as possible* — runs once on the next opportunity (typically the
    nightly window, as soon as the limit allows and a slot is free).
  - *Earliest start* — a fixed date/time; runs once at or after it. If the limit
    is blocked at that time, it starts when the gate reopens.
  - *Cron* — recurring, on a standard 5-field cron schedule (e.g. `0 3 * * *`).
    If a new occurrence is due while the previous run is still going, it is
    skipped.
- **Manual priority** — tasks run in list order, top = highest. Reorder them with
  the up/down controls (or `claudeq move`).
- **Concurrency control** — one task at a time by default; mark a task *parallel*
  to let it run alongside other parallel tasks (no fixed upper bound). Priority
  and timing still apply.

**Per-task and global controls**

- **Per-task overrides** — model, permission handling (default vs.
  "skip permission prompts"), and whether to notify on the result — layered over
  your global defaults.
- **Custom system prompt** — standing guidance (conventions, tone, tools to
  prefer) appended to every run after ClaudeQ's built-in instructions.
- **Self-queueing** — a running task can schedule follow-up tasks itself, so a
  prompt can say things like *"if you find something to optimize, queue it as a
  separate task instead of doing it now."* See
  [below](#letting-a-task-queue-follow-up-work).

**Reliability**

- **Rate-limit aware** — a reactive global gate: a run that hits the limit is
  paused, the wait is derived from the CLI's retry signal, and the session is
  resumed automatically once the limit resets (falling back to a fresh restart if
  resume fails), so a task never gets stuck.
- **Auth-error aware** — a login/authentication failure is detected, surfaced as
  its own outcome, and notified — never silently retried.
- **Unattended-safe** — kills hung runs (no output for a configurable timeout,
  killing the whole process group), recovers orphaned runs after a crash or power
  loss, holds the Mac awake through a run and is sleep-aware so a run frozen
  across a long sleep isn't falsely killed, and prunes old history to bound disk
  use.
- **Wake from sleep** — schedules `pmset` wakes at each task's time (plus an
  hourly safety-net heartbeat) so a timed task wakes the Mac, runs, and lets it
  sleep again. A broken wake setup is surfaced in the dashboard rather than
  failing silently.
- **File-access prompts up front** — the daemon reads each task's folder at
  login and when you add or edit a task, so macOS raises its "allow access to
  your Documents?" prompt while you're present — not at 3 a.m. mid-run.

**Visibility**

- **Notifications** — native macOS notifications, plus optional
  [Pushover](https://pushover.net) push to your phone. Failures and auth problems
  always notify; successes notify only if the task opts in.
- **Usage insight** — tokens, runs, and API-equivalent cost per day (what the
  same work would have cost through the API), over the last 14 days.
- **Full history** — every run is kept with its complete log, viewable as a chat
  transcript or raw output, and can be replayed.

**Platform & distribution**

- **Native macOS** — its own app window, Dock icon, menu bar, About panel, and
  live system accent color; light/dark aware.
- **Automatic updates** — checks GitHub for a newer release hourly and flags it
  in Settings; one click downloads the installer and opens it. Dismiss a version
  to only hear about the next one. The banner aggregates the notes of every
  version you skipped.
- **Local & private** — data is human-readable TOML/JSON under your Library
  folder; the API is loopback-only.

## The app

The dashboard (and the native window that wraps it) has four views:

- **Queue** — the pending tasks in priority order. Add, edit, delete, enable/pause,
  reorder, or **run now** (a manual test run, independent of the trigger). A
  running one-shot task moves to Activity; a recurring task stays here with a
  *running* badge and shows its next occurrence on hover.
- **Activity** — every run, newest first, with an unread badge for new results.
  Open a run to see the live/finished log as a chat view or raw output, along
  with the prompt; mark one or all read; filter by a from–to date range; page
  through history; and replay a task.
- **Usage** — a per-day bar chart of runs, tokens, and cost for the last 14 days,
  plus totals and a 7-day summary.
- **Settings** — global defaults and integrations (below). A red badge here means
  an update is available.

The dashboard is also reachable in a normal browser at
`http://127.0.0.1:8765` while the daemon is running.

## Settings

| Group | Setting | What it does |
|-------|---------|--------------|
| **General** | Default model | Model used for runs unless a task overrides it (empty = Claude's own default). |
| | Skip permission prompts by default | Global "may do anything" default for runs. |
| | Check for due tasks every | How often the daemon wakes to look for work (15 min – 6 h; also the wake safety-net interval). |
| **Claude Code CLI** | Claude binary | Absolute path to the `claude` executable. The daemon can't see your shell `PATH`, so this is auto-detected and pre-filled; override if needed. |
| **System prompt** | Custom system prompt | Extra instructions appended to every run after the built-in prompt. |
| **Reliability** | Stop a run with no output for | Idle-timeout watchdog: kills a hung run (default 30 min; a working run keeps streaming and is unaffected; Off disables it). |
| | Keep run history | How many runs (and their logs) to retain before pruning (default 500; Unlimited keeps everything). |
| **Notifications · Pushover** | Send to Pushover | Toggle plus API token and user key for phone push. |
| **About** | Version / Software updates | Current version and a manual "Check for updates" button. |

## Install

1. Download the latest `claudeq-<version>.pkg` from the
   [Releases](https://github.com/danielmaier42/claudeq/releases) page.
2. Open it and follow the installer.

The package installs **ClaudeQ** to `/Applications` and sets up a per-user
LaunchAgent so the daemon starts at login. Open **ClaudeQ** from Applications to
start adding tasks.

> The package is not notarized, so on first launch macOS may warn that it is from
> an unidentified developer. Right-click **ClaudeQ → Open**, then confirm — or
> allow it under **System Settings → Privacy & Security**.

You'll also see two normal macOS prompts by design: **Allow notifications?** on
first launch, and **allow access to your Documents?** the first time a task's
folder is in a protected location (Documents, Desktop, Downloads). Allow both so
unattended runs aren't blocked.

To run tasks past a scheduled sleep, ClaudeQ schedules wakes with `pmset`, which
needs one sudoers entry (the daemon prints the exact line on install, and the
dashboard shows it if a wake ever fails):

```sh
echo "$USER ALL=(root) NOPASSWD: /usr/bin/pmset" | sudo tee /etc/sudoers.d/claudeq
```

## Using it

1. **New task** — give it a prompt, pick the working folder, and choose a trigger
   (as-soon-as-possible, earliest start, or cron). Optionally override the model
   or permissions, or enable *parallel* / *notify on result*.
2. Leave it queued. The daemon runs it at the scheduled time (or overnight when
   the allowance resets).
3. Check **Activity** for the outcome, open a run to read the full log, or replay
   it. A finished one-shot task leaves the queue but stays in history; recurring
   tasks remain queued for their next occurrence.
4. **Usage** shows your consumption over the last 14 days.

## Command-line interface

Everything the app does is also available on the command line — useful for
scripting or for driving the queue without the window. Both binaries ship inside
the app bundle at `/Applications/ClaudeQ.app/Contents/MacOS/` (`claudeqd` and
`claudeq`); add that directory to your `PATH` or call them by full path.

### `claudeqd` — the daemon

```
claudeqd run [--interval 5s] [--no-wake] [--addr 127.0.0.1:8765]
claudeqd install      # install & start the LaunchAgent (autostart at login)
claudeqd uninstall    # stop & remove the LaunchAgent
claudeqd --version
```

The installer runs `install` for you; you rarely need these directly.

### `claudeq` — the control CLI

```
claudeq list                                   # show the queue
claudeq add    --id ID --prompt P --dir DIR [--name N]
               [--trigger asap|fixed|cron] [--at RFC3339] [--cron EXPR]
               [--model M] [--parallel] [--skip-permissions]
claudeq queue  --prompt P [--at RFC3339 | --in DUR | --cron EXPR] [--dir DIR] [--name N]
claudeq rm ID
claudeq enable ID | claudeq disable ID
claudeq move   ID INDEX                        # 0 = highest priority
claudeq run-now ID                             # run once, now, for testing
claudeq status [--all]                         # recent runs; unread marked *
claudeq read RUNID | claudeq read-all
claudeq settings [--default-model M] [--skip-permissions=BOOL]
                 [--pushover-token T] [--pushover-user U]
claudeq --version
```

## Letting a task queue follow-up work

A running task can enqueue *new* ClaudeQ tasks instead of doing everything inline.
Every run is told, via its system prompt, that the capability exists and how to
use it — so a prompt like *"scan for TODOs and, for anything non-trivial, queue a
separate task to fix it"* just works. From inside a run, Claude calls:

```sh
claudeq queue --prompt "…"
```

with an optional time (`--at <RFC3339>`, `--in <duration>` like `90m`, or
`--cron "<expr>"`; default is as-soon-as-possible), an optional `--dir`, and an
optional `--name`. The new task **inherits** the calling task's model,
permissions, parallelism, and notification settings automatically — only the
prompt, timing, and directory are set per call. This works because the daemon
injects the CLI's path and the parent task into each run's environment.

## How scheduling and the limit gate behave

- **Eligibility.** On each tick the daemon starts every task that is due and
  permitted by priority, concurrency, and the limit gate.
- **The limit gate is global.** When any run reports a rate limit, all new starts
  pause until the reset. The wait comes from the CLI's `retry_delay_ms` signal
  (falling back to 15 minutes when none is exposed). At reset the gate reopens and
  the blocked task **resumes its session** rather than starting over.
- **Auth problems don't retry.** A login/authentication error is recorded as
  `auth_error` and notified so you can re-login; it is not retried automatically.
- **Wake-ups.** After each pass the daemon registers a `pmset` wake at the nearest
  relevant time (next fixed start, next cron occurrence, or limit reset) plus a
  recurring heartbeat wake, so the Mac can sleep between runs and wake when
  there's work.
- **Run outcomes** are one of: `running`, `success`, `failed`,
  `rate_limited_waiting`, `auth_error`.

## Data on disk

Everything lives under `~/Library/Application Support/claudeq` (override with the
`CLAUDEQ_HOME` environment variable):

| Path | Contents |
|------|----------|
| `config.toml` | Global settings + the ordered task list (human-readable, versionable). |
| `history.jsonl` | Append-only index of every run. |
| `runs/<run-id>.log` | Full log for each run. |
| `state.json` | Machine bookkeeping: read/unread flags, cron anchors, pending-resume sessions, dismissed update version. |
| `claudeqd.out.log` / `claudeqd.err.log` | Daemon stdout/stderr. |

The LaunchAgent itself is at
`~/Library/LaunchAgents/de.maierdaniel.claudeq.plist`.

## Uninstall

```sh
/Applications/ClaudeQ.app/Contents/MacOS/claudeqd uninstall   # remove the LaunchAgent
sudo rm -f /etc/sudoers.d/claudeq                             # remove the pmset wake permission
rm -rf /Applications/ClaudeQ.app
```

Or run [`scripts/uninstall.sh`](scripts/uninstall.sh) (does all of the above).
Your tasks and history in `~/Library/Application Support/claudeq` are left in
place; delete that folder to remove them too.

## Build from source

Requires Go 1.26+ and `librsvg` (`brew install librsvg`) for icon rendering.

```sh
scripts/build-app.sh    # build/ClaudeQ.app          (double-click or `open` it)
scripts/build-pkg.sh    # dist/claudeq-<version>.pkg  (installer)
```

Run the same quality gates CI enforces with `make check` (format, vet, lint, and
race tests). Releases are built automatically: pushing a `v*` tag runs
[`.github/workflows/release.yml`](.github/workflows/release.yml), which builds the
universal `.pkg` on macOS and attaches it to the GitHub Release.

## Architecture

A headless Go daemon owns all state and logic; a thin WKWebView app is the only
UI, talking to the daemon over loopback. The daemon spawns the `claude` CLI once
per task in the task's directory using `--output-format stream-json`, which lets
it watch for rate-limit and auth events as they happen and capture the session id,
token usage, and cost from the final result. ClaudeQ performs **no Git
operations** — any branch/commit behavior is driven entirely by your prompts and
the repo's own configuration. The full design, decisions, and verification notes
are in [PLAN.md](PLAN.md).

## Requirements

- macOS 12 or newer
- The [Claude Code](https://claude.com/claude-code) CLI, installed and
  authenticated

## License

[MIT](LICENSE) © 2026 Daniel Maier
