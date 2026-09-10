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
update checks) ever leaves the machine. The one thing you can send out is
[feedback](#sending-feedback), and only by pressing *Create* on GitHub yourself.

## Contents

- [How it works](#how-it-works)
- [Features](#features)
- [The app](#the-app)
- [Settings](#settings)
- [Install](#install)
- [Using it](#using-it)
- [Command-line interface](#command-line-interface)
- [From another tool or agent](#from-another-tool-or-agent)
- [What every run is told](#what-every-run-is-told)
- [Letting a task queue follow-up work](#letting-a-task-queue-follow-up-work)
- [Letting a task publish artifacts](#letting-a-task-publish-artifacts)
- [Letting a task send a notification](#letting-a-task-send-a-notification)
- [Quiet history for frequent jobs](#quiet-history-for-frequent-jobs)
- [Sharing tasks as files](#sharing-tasks-as-files)
- [Sending feedback](#sending-feedback)
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
   limit clears, resumes the *same* Claude session — no work is lost. The pause is
   visible while it lasts: a banner names the time the queue continues, the run is
   marked **rescheduled** in Activity with its resume time, and you can drop that
   resume there so the task does not start again.
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

- **Pause everything** — one global switch in Settings stops the queue: no task
  starts while it is on, not even a manual *Run now*, and the Mac is no longer
  woken for scheduled work. A run already in flight keeps going, and a task that
  came due meanwhile starts as soon as you switch it back off. The Queue view
  carries a yellow banner while it is on, so an idle queue is never a mystery.
- **Per-task overrides** — model, permission handling (default vs.
  "skip permission prompts"), whether to notify on the result, and *quiet
  history* for frequent jobs — layered over your global defaults.
- **Built-in run framing** — every run is told up front that it is headless and
  unattended, so it finishes its work or hands it on instead of ending on
  *"waiting for the build"*. See [below](#what-every-run-is-told).
- **Custom system prompt** — standing guidance (conventions, tone, tools to
  prefer) appended to every run after ClaudeQ's built-in instructions.
- **Self-queueing** — a running task can schedule follow-up tasks itself, so a
  prompt can say things like *"if you find something to optimize, queue it as a
  separate task instead of doing it now."* See
  [below](#letting-a-task-queue-follow-up-work).
- **Quiet history** — a watcher that runs every few minutes can be told to keep
  its successful runs out of the record, so it neither floods Activity with
  unread entries nor pushes real work out of the bounded run history. Failures
  are kept. See [below](#quiet-history-for-frequent-jobs).

**Reliability**

- **Rate-limit aware** — a reactive global gate: a run that hits the limit is
  paused, the wait is derived from the CLI's retry signal, and the session is
  resumed automatically once the limit resets (falling back to a fresh restart if
  resume fails), so a task never gets stuck. A waiting run says so — *rescheduled*
  with the time it continues — and its resume can be cancelled if you no longer
  want the work.
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
  always notify; successes notify only if the task opts in. Every published
  artifact is announced too, and **clicking that notification opens the artifact**
  right in the window. A task can also **send its own notification** with
  `claudeq notify` — the way a watcher job reports a change without leaving a
  file behind — optionally with a link that opens on click. See
  [below](#letting-a-task-send-a-notification).
- **Usage insight** — tokens, runs, and API-equivalent cost per day (what the
  same work would have cost through the API), over the last 14 days.
- **Full history** — every run is kept with its complete log, viewable as a chat
  transcript or raw output, and can be replayed.
- **Continue with Claude** — pick up a finished run's conversation interactively:
  one click opens Terminal in the task's folder and resumes the very same Claude
  session (`claude --resume`), with the full context of everything the run did.
- **Artifacts** — a task can publish a finished file (report, export, HTML page,
  PDF, …) with `claudeq publish`; it's copied into ClaudeQ and listed in a
  central **Artifacts** view with an unread flag, independent of run history.
  HTML and PDF get an in-app viewer; anything opens externally — and opening one
  marks it read. Each publish also raises a notification that opens the artifact
  when clicked. See [below](#letting-a-task-publish-artifacts).
- **Share a task** — export any task as a `.claudeq` file (a zip holding its
  settings as JSON and its prompt as Markdown) and hand it to a colleague, who
  imports it in the app or with `claudeq import`. In the app the file opens in
  the task sheet first, so the prompt and the working directory — the exporter's
  path, not yours — are adjusted before the task is queued. See
  [below](#sharing-tasks-as-files).

**Platform & distribution**

- **Scriptable** — a bundled `claudeq` CLI does everything the window does. It
  lists and inspects tasks, edits a prompt or any other setting, changes the
  global settings, triggers a test run, and reads run history, with `--json`
  output for other tools and agents. See [below](#command-line-interface).
- **Native macOS** — its own app window, Dock icon, menu bar, About panel, and
  live system accent color; light/dark aware.
- **Automatic updates** — checks GitHub for a newer release hourly and flags it
  in Settings; one click downloads the installer and opens it, and the new
  version is running again as soon as the installer finishes. Dismiss a version
  to only hear about the next one. The banner aggregates the notes of every
  version you skipped. If a version was installed but the background service
  never switched over to it, Settings says so and a **Finish update** button
  hands it over.
- **Feedback that writes itself** — the **Feedback** entry at the bottom of the
  sidebar opens a short chat. Describe a bug or a wish, Claude asks at most one
  clarifying question and drafts a GitHub issue, and you edit it before anything
  is filed. The last step just opens GitHub's prefilled *new issue* page in your
  browser — ClaudeQ holds no GitHub credentials and files nothing itself. See
  [below](#sending-feedback).
- **Local & private** — data is human-readable TOML/JSON under your Library
  folder; the API is loopback-only.

## The app

The dashboard (and the native window that wraps it) has five views, plus a
**Feedback** entry at the bottom of the sidebar ([below](#sending-feedback)):

- **Queue** — the pending tasks in priority order. Add, edit, delete, enable/pause,
  reorder, or **run now** (a manual test run, independent of the trigger). The
  **All / Active** switch in the toolbar hides the paused tasks; the list then
  says how many are hidden, and moving a task up puts it above the next visible
  one, skipping the hidden tasks in between. While the global pause switch is
  on, a yellow banner sits above the list (with a **Resume runs** button) and
  **Run now** is disabled on every row. A running one-shot task moves to
  Activity; a recurring task stays here with a *running* badge; hovering its
  cron expression shows the next occurrence and when it last ran. Underneath
  each task sits a badge for every option it has switched on: *parallel*,
  *granted* (orange, the task skips permission prompts), *notifies* (blue), and
  *silent* (quiet history). A task the rate limit interrupted carries a
  *rescheduled* badge (orange) whose tooltip names when its interrupted session
  continues. An **export** button on each row saves the task as a `.claudeq`
  file via the native save panel, and **Import…** in the toolbar opens such a
  file in the task sheet for review.
- **Activity** — every run, newest first, with an unread badge for new results.
  Open a run to see the live/finished log as a chat view or raw output, along
  with the prompt; a running task can be stopped from there with **Cancel task**
  (its process is terminated and the run is recorded as `canceled`); a run the
  rate limit paused shows as **rescheduled** with the time it continues, and
  **Cancel resume** — on the row and in its log view — drops that plan: the
  interrupted session is discarded, the run is recorded as `canceled`, and a
  one-shot task leaves the queue instead of starting again (a recurring task
  keeps its schedule and starts fresh at its next occurrence); a finished
  run offers **Continue with Claude**, which opens Terminal in the task's folder
  and resumes the run's Claude session interactively (`claude --resume`) so you
  can keep chatting with full context — with the same permission mode the run
  had (a skip-permissions task resumes with `--dangerously-skip-permissions`);
  the button needs the session to still exist — Claude Code prunes old sessions
  after ~30 days; mark one or
  all read; filter by a from–to date range; page through history; and replay a
  task. Runs of a *quiet history* task appear here only if they did not
  succeed.
- **Artifacts** — files your tasks published, newest first, with an unread badge.
  Each shows its title, source task, file type, and size. **View** opens the
  artifact in an in-app viewer: HTML, PDF, images, and text are previewed
  inline, anything else (archives, binaries) shows a short note instead of a
  preview. The viewer offers **Open externally**, which hands the file to your
  browser to open or save, and — when the run that published the artifact can
  still be resumed — **Continue with Claude**, the same interactive resume as in
  a run's log. Either way, opening an artifact marks it read automatically; you can
  also mark one or all read by hand, or delete one (which removes the stored copy).
  Clicking the notification of a newly published artifact lands here with that
  artifact already open.
- **Usage** — a per-day bar chart of runs, tokens, and cost for the last 14 days,
  plus totals and a 7-day summary.
- **Settings** — global defaults and integrations (below). A red badge here means
  an update is available.

The dashboard is also reachable in a normal browser at
`http://127.0.0.1:8765` while the daemon is running.

## Settings

| Group | Setting | What it does |
|-------|---------|--------------|
| **Execution** | Pause all runs | Global stop switch: nothing starts while it is on, not even *Run now*; a run already in flight keeps going. Applies immediately, without pressing Save. |
| **General** | Default model | Model used for runs unless a task overrides it (empty = Claude's own default). |
| | Check for due tasks every | How often the daemon wakes to look for work (15 min – 6 h; also the wake safety-net interval). |
| **Claude Code CLI** | Claude binary | Absolute path to the `claude` executable. The daemon can't see your shell `PATH`, so this is auto-detected and pre-filled; override if needed. |
| **System prompt** | Custom system prompt | Extra instructions appended to every run after the built-in prompt. |
| **Reliability** | Stop a run with no output for | Idle-timeout watchdog: kills a hung run (default 30 min; a working run keeps streaming and is unaffected; Off disables it). |
| | Keep run history | How many runs (and their logs) to retain before pruning (default 500; Unlimited keeps everything). |
| **Notifications · macOS** | Alerts that wait for you | Opens System Settings → Notifications, where ClaudeQ's alert style lives: *Banners* disappear on their own, *Alerts* stay until you click them. |
| **Notifications · Pushover** | Send to Pushover | Toggle plus API token and user key for phone push. |
| **About** | Version / Software updates | Current version and a manual "Check for updates" button. |

## Install

1. Download the latest `claudeq-<version>.pkg` from the
   [Releases](https://github.com/danielmaier42/claudeq/releases) page.
2. Open it and follow the installer.

The package installs **ClaudeQ** to `/Applications`, sets up a per-user
LaunchAgent so the daemon starts at login, and opens **ClaudeQ** when it is
done, so you can start adding tasks right away. Installing over an existing
version works the same way: the installer closes the open ClaudeQ window first
(the daemon and any running task are not interrupted) and reopens the new
version at the end, so an update takes effect without a manual restart.

The installer verifies the hand-over instead of assuming it: it waits until the
daemon that answers on `127.0.0.1:8765` reports the version it just installed,
retries once (dropping a stale LaunchAgent that still points at an old copy of
the app), and reports the install as *failed* if the new daemon never takes
over — rather than finishing green while the machine keeps running the old
version. It also lists any other `ClaudeQ.app` copies it finds, since a second
copy is the usual reason an update looks like it did nothing.

> The package is not notarized, so on first launch macOS may warn that it is from
> an unidentified developer. Right-click **ClaudeQ → Open**, then confirm — or
> allow it under **System Settings → Privacy & Security**.

You'll also see two normal macOS prompts by design: **Allow notifications?** on
first launch, and **allow access to your Documents?** the first time a task's
folder is in a protected location (Documents, Desktop, Downloads). Allow both so
unattended runs aren't blocked.

macOS, not ClaudeQ, decides how long a notification stays on screen. ClaudeQ asks
for the **Alerts** style, which waits until you click it — but if macOS already
knows the app (or overrides it), set it under **System Settings → Notifications →
ClaudeQ → Alerts**. Settings has a button that opens that pane directly.

To run tasks past a scheduled sleep, ClaudeQ schedules wakes with `pmset`, which
needs one sudoers entry (the daemon prints the exact line on install, and the
dashboard shows it if a wake ever fails):

```sh
echo "$USER ALL=(root) NOPASSWD: /usr/bin/pmset" | sudo tee /etc/sudoers.d/claudeq
```

## Using it

1. **New task** — give it a prompt, pick the working folder, and choose a trigger
   (as-soon-as-possible, earliest start, or cron). Optionally override the model
   or permissions, or enable *parallel* / *notify on result*. The folder dialog
   starts at the folder currently set for the task; if that folder no longer
   exists it opens at the nearest existing parent, and at your home folder when
   nothing is set — so a task whose folder was deleted or renamed can always be
   pointed somewhere new.
2. Leave it queued. The daemon runs it at the scheduled time (or overnight when
   the allowance resets).
3. Check **Activity** for the outcome, open a run to read the full log, or replay
   it. A finished one-shot task leaves the queue but stays in history; recurring
   tasks remain queued for their next occurrence.
4. **Usage** shows your consumption over the last 14 days.

## Command-line interface

Everything the app does is also available on the command line. You can list the
queue, read and change a task's prompt and settings, edit the global settings,
trigger a test run, and read run history. This section is written to stand on its
own, so another tool or agent needs nothing else to drive ClaudeQ. See
[From another tool or agent](#from-another-tool-or-agent).

### Where the binaries are

Both binaries ship **inside the app bundle**, and neither is on your `PATH`:

```
/Applications/ClaudeQ.app/Contents/MacOS/claudeq     # the control CLI
/Applications/ClaudeQ.app/Contents/MacOS/claudeqd    # the daemon
```

Call them by full path:

```sh
/Applications/ClaudeQ.app/Contents/MacOS/claudeq list
```

…or put the directory on your `PATH` once:

```sh
export PATH="/Applications/ClaudeQ.app/Contents/MacOS:$PATH"
```

The rest of this section writes them as plain `claudeq` and `claudeqd`. Both work
on the store under `~/Library/Application Support/claudeq` directly (override it
with `CLAUDEQ_HOME`), so they work whether or not the app window is open. The
running daemon re-reads the store on its next tick, so a change never needs a
restart.

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
claudeq list [--json]                          # show the queue
claudeq show   ID [--json]                     # one task in full, prompt included
claudeq add    --id ID --prompt P --dir DIR [--name N]
               [--trigger asap|fixed|cron] [--at RFC3339] [--cron EXPR]
               [--model M] [--parallel] [--skip-permissions] [--notify]
               [--quiet-history]
claudeq edit   ID                              # open the whole task in $EDITOR
claudeq edit   ID [--name N] [--prompt P | --prompt-file PATH] [--dir DIR]
               [--trigger asap|fixed|cron] [--at RFC3339] [--cron EXPR]
               [--model M] [--parallel=BOOL] [--enabled=BOOL]
               [--skip-permissions=BOOL] [--notify=BOOL] [--quiet-history=BOOL]
claudeq queue  --prompt P [--at RFC3339 | --in DUR | --cron EXPR] [--dir DIR] [--name N]
               [--model M] [--parallel=BOOL] [--skip-permissions=BOOL]
               [--notify=BOOL] [--quiet-history=BOOL]
claudeq publish --file PATH [--title T] [--description D]   # publish a file as an artifact
claudeq notify --title T --message M [--url U]  # send a notification, no artifact
claudeq export ID [--out PATH] [--force]       # write the task to a .claudeq file
claudeq import PATH [--id ID]                  # add the task from a .claudeq file (as exported)
claudeq rm ID
claudeq enable ID | claudeq disable ID
claudeq move   ID INDEX                        # 0 = highest priority
claudeq run-now ID                             # run once, now, for testing
claudeq status [--all]                         # recent runs; unread marked *
claudeq read RUNID | claudeq read-all
claudeq settings [--json] [--default-model M] [--claude-path PATH]
                 [--heartbeat-minutes N]
                 [--idle-timeout-minutes N] [--max-run-history N]
                 [--system-prompt S | --system-prompt-file PATH]
                 [--paused=BOOL]                # pause/resume every run
                 [--pushover=BOOL] [--pushover-token T] [--pushover-user U]
claudeq --version
```

### Looking at the queue

`claudeq list` prints one line per task in priority order, index 0 first:

```
#  ID              NAME            TRIGGER  WHEN         PARALLEL  ENABLED
0  nightly-sweep   Nightly sweep   cron     0 3 * * *    false     true
```

Prompts are often pages long, so that table leaves them out, and a name longer
than 40 characters is cut with an ellipsis so the columns stay readable.
`claudeq show ID` prints every setting of one task, its full name and then its
complete prompt. Add `--json` to either command for the same data as JSON,
prompts and full names included.

### Editing a task

`claudeq edit` changes a task that already exists. The id is fixed and cannot be
changed. There are two ways to use it.

**With flags.** Only the settings you pass are touched, everything else keeps its
value. This is the form to use from a script or an agent.

```sh
claudeq edit nightly-sweep --prompt-file ./new-brief.md   # replace just the prompt
claudeq edit nightly-sweep --cron "30 2 * * 1-5"          # reschedule
claudeq edit nightly-sweep --model opus --notify=true     # per-task overrides
claudeq edit prod-watch --quiet-history=true              # drop its successful runs
claudeq edit nightly-sweep --enabled=false                # pause it
```

- `--prompt-file` reads the prompt from a file, or from stdin when you pass `-`.
  That is the practical way to set a long prompt without fighting shell quoting.
- `--at` implies `--trigger fixed` and `--cron` implies `--trigger cron`, so a
  reschedule is one flag. Changing the trigger clears the timing fields that no
  longer apply.
- `--model ""` drops a per-task model override back to the global default.
- Every edit is validated before it is written. An invalid cron, an unparseable
  time, or an empty prompt fails with a message and leaves the task as it was.

**Interactively.** `claudeq edit ID` with no flags opens the whole task as a
commented TOML document in `$VISUAL` or `$EDITOR`, falling back to `vi`. Every
setting is in there, and the prompt is an editable multi-line block. Save and
close to apply; leave the file unchanged to cancel. If what you wrote does not
parse or does not validate, nothing is written and the CLI tells you where it
kept your draft. This form needs a terminal. Without one it stops and points you
at the flags.

### Global settings

`claudeq settings` with no flags prints every global setting. With flags it
changes the ones you name and prints the result. They are the same values as the
app's [Settings](#settings) view.

```sh
claudeq settings                                        # show everything
claudeq settings --default-model opus                   # global default model
claudeq settings --claude-path /Users/me/.local/bin/claude
claudeq settings --system-prompt-file ./house-style.md   # custom system prompt
claudeq settings --idle-timeout-minutes 45 --max-run-history 1000
claudeq settings --paused=true                          # stop every run
claudeq settings --paused=false                         # let the queue run again
claudeq settings --pushover=true --pushover-token T --pushover-user U
```

For the numeric settings `0` means "use the default". `--idle-timeout-minutes`
also takes a negative value for "never kill a run", and `--max-run-history` for
"keep every run". You can set the Pushover credentials here, but the CLI never
prints them back; the output only says whether they are configured.

## From another tool or agent

This README is the whole interface. An external app or agent needs nothing but
this document to work with ClaudeQ.

- **Invoke it by full path.** `/Applications/ClaudeQ.app/Contents/MacOS/claudeq`
  is not on `PATH`.
- **Read with `--json`.** `claudeq list --json`, `claudeq show ID --json` and
  `claudeq settings --json` emit structured output. The other commands print for
  humans.
- **Write with `edit`, `add`, `import`, `rm`, `enable`, `disable` and `move`**
  instead of editing `config.toml` by hand. Those commands validate the change
  and write it atomically alongside the app's own writes.
- **Move a task between machines with `export` and `import`.** The `.claudeq`
  file carries the whole task; see [Sharing tasks as files](#sharing-tasks-as-files).
- **Exit code 0 means success.** Any failure exits non-zero and writes the reason
  to stderr, prefixed `claudeq:`.
- **Test a change with `claudeq run-now ID`** rather than waiting for the
  schedule. It exits non-zero while the global pause switch is on
  (`claudeq settings --paused=false` lifts it).
- Every run is framed as a [headless, unattended run](#what-every-run-is-told)
  with no next turn — it finishes or hands work on, it does not wait.
- A task that is itself a ClaudeQ run has three extra abilities, described below:
  [queueing follow-up work](#letting-a-task-queue-follow-up-work),
  [publishing artifacts](#letting-a-task-publish-artifacts) and
  [sending a notification](#letting-a-task-send-a-notification).
- [Data on disk](#data-on-disk) lists the files these commands read and write.

## What every run is told

Every run starts with a built-in system prompt from ClaudeQ, ahead of your own
[custom system prompt](#settings). Most of it documents the three abilities
below — queueing, publishing, notifying — but it opens with the one thing a run
cannot work out for itself: **it is headless**. Nobody is at the keyboard, there
is no next turn, and the process is torn down the moment Claude stops writing.
Without that framing a model behaves as if a conversation continues: it
schedules a wakeup for later, leaves a watcher running in the background, or
ends with a question — all of which die with the run. So the prompt tells it to

- decide rather than ask,
- never end a run by announcing that it is waiting for something,
- either finish work in flight inline and blocking, or
  [queue a follow-up task](#letting-a-task-queue-follow-up-work) (`--in 30m`)
  and stop, and
- name anything unfinished concretely — pull request, build, run ids and links —
  in the last message, because that message is what reaches your phone.

You don't need to repeat any of this in a task's prompt.

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
permissions, parallelism, and notification settings automatically. This works
because the daemon injects the CLI's path and the parent task into each run's
environment.

When the follow-up needs different settings from the task that queues it, pass
the same flags `claudeq add` takes; anything you leave out keeps inheriting:

```sh
claudeq queue --prompt "Review the change thoroughly …" --model claude-opus-5 --notify
```

- `--model M` runs the new task on another model (an empty value means the
  global default). This is what lets a cheap watcher — Haiku every five minutes,
  quiet history — hand expensive work to a visible Opus run.
- `--parallel=BOOL` and `--notify=BOOL` switch the respective setting on or
  off regardless of what the caller has.
- `--skip-permissions=BOOL` grants or withdraws the task's own permission
  bypass. Note that a run can grant a follow-up more than it has itself; the
  system prompt tells Claude to do so only when the queued work cannot be done
  without it.
- `--quiet-history=BOOL` opts the new task into (or, explicitly, out of) quiet
  history. Without it a queued task is never quiet, even when the caller is
  (see [below](#quiet-history-for-frequent-jobs)).

A long `--name` is accepted as given; `claudeq list` and the Queue view cut it
to fit their column (hover the app's row for the whole name), `claudeq show` and
`--json` print it in full.

## Letting a task publish artifacts

When a task produces a file worth keeping — a report, an export, a generated
HTML page or PDF — it can publish it as an **artifact**. Every run is told, via
its system prompt, that the capability exists, so a prompt like *"write the
summary to report.html and publish it"* just works. From inside a run, Claude
calls:

```sh
claudeq publish --file report.html --title "Nightly summary" --description "…"
```

The file is **copied into ClaudeQ** (a permanent snapshot — later changes to the
original don't affect it) and appears in the **Artifacts** view, attributed to
the task and run that produced it, with an unread flag that clears as soon as you
open it. Each publish also raises a notification (macOS, plus Pushover when it is
configured); clicking the macOS one brings up ClaudeQ with that artifact open — in
the in-app viewer for HTML, PDF, images and text, in your browser for anything
else. Artifacts that were already there when this version first ran are not
announced retroactively. HTML and PDF open in an in-app viewer; any type can be opened in your
browser. Artifacts are kept until you delete them, independent of run-history
pruning. `--title` defaults to the file name; `--file` may be relative to the
task's working directory.

## Letting a task send a notification

Some jobs have nothing to hand over — a watcher that checks a deployment, a
feed, or a metric every quarter hour only needs to say *something changed*.
Rather than publishing a throwaway artifact to trigger a push, a run can send a
notification directly. Every run is told, via its system prompt, that the
capability exists, so a prompt like *"compare with the last snapshot and notify
me only if it differs"* just works. From inside a run, Claude calls:

```sh
claudeq notify --title "Prod drifted" --message "3 commits behind main" --url "https://…"
```

The notification goes out over the **channels you already configured** — macOS
Notification Center and Pushover when it is set up — with the same look as
ClaudeQ's own alerts, and attributed to the task that sent it (its name is
appended to the message). Nothing is stored: no artifact, no history entry. The
run's own outcome is still announced according to the task's settings, so a
watcher that finds nothing sends nothing and stays silent.

- `--title` and `--message` are required. A title longer than 250 characters
  or a message longer than 1024 (Pushover's limits) is cut, not rejected.
- `--url` is optional and must be an absolute `http` or `https` URL. Clicking
  the macOS notification opens it; Pushover shows it as the message's link.
- The CLI hands the notification to the daemon, which sends it on its next tick
  (a few seconds). Exit code 0 means it was queued, not that a channel accepted
  it — a channel that fails is logged by the daemon (`claudeqd.err.log`).
- A notification that waited more than 24 hours — the daemon was not running —
  is dropped with a log line rather than delivered as if it were current.
- It also works outside a run, from a shell: then it is sent without attribution.

## Quiet history for frequent jobs

A task that runs every few minutes would, by default, produce hundreds of
successful runs a day: each one unread in Activity, each one counting against
the `Max run history` limit until it pushes a run you actually care about out of
the record. Mark such a task **Quiet history** (the switch in the task form, or
`--quiet-history` on `claudeq add` / `claudeq edit` / `claudeq queue`) and:

- A run that **succeeds leaves no trace**: it is never written to history, and
  its log is deleted when it finishes. The same goes for a run paused by the
  rate limit, which the daemon resumes by itself.
- A run that **fails, hits an auth problem, or is cancelled is recorded** with
  its log, unread, exactly like an ordinary run, and notifies as usual.
- While it is running, the Queue shows the task's *running* badge as usual, but
  there is no Activity entry to open (and so no live log or **Cancel task**).
  Same for a pause on the rate limit: the Queue shows the task's *rescheduled*
  badge, but with no Activity entry there is no **Cancel resume** — pause or
  delete the task in the Queue to stop it from continuing.

Everything else — notifications the run sends, artifacts it publishes, tasks it
queues — is unaffected. A task queued from inside a quiet-history run is **not**
quiet itself unless the call says `--quiet-history`: follow-up work is real
work, and you will want to see its run.
The trade-off: successful quiet runs leave no log to look at afterwards and are
absent from the Usage statistics.
## Sharing tasks as files

A task can be handed to a colleague as a single `.claudeq` file. It is a plain
zip archive with two entries:

| Entry | Contents |
|-------|----------|
| `task.json` | Every setting of the task (id, name, working directory, trigger and schedule, parallel, enabled, model, permissions, notify) inside a small envelope: `format` (`claudeq-task`), `format_version` (`1`), `exported_at`, and `task`. |
| `prompt.md` | The prompt, byte for byte, as Markdown. |

Unzip it to read or edit either part by hand; zip the two files back up and the
result imports again (a folder around them, as Finder's *Compress* adds, is
fine).

**Export.** In the app, the export button on a task row opens the native save
panel, pre-filled with `<id>.claudeq`. From the CLI:

```
claudeq export nightly-sweep                       # ./nightly-sweep.claudeq
claudeq export nightly-sweep --out ~/Desktop       # a directory: default name inside it
claudeq export nightly-sweep --out share/brief     # a file: .claudeq is appended
claudeq export nightly-sweep --out x.claudeq --force   # overwrite an existing file
```

Without `--force` the CLI refuses to overwrite; the app's save panel asks
before replacing a file.

**Import in the app.** **Import…** on the Queue toolbar picks a file and opens
its task in the normal task sheet, titled *Import task*. Nothing is queued yet:
prompt, working directory, schedule and every switch come from the file and can
be changed, and **Add task** creates the task, exactly as if you had typed it in.
Cancel and nothing happened.

The working directory is the one place where a shared file cannot be trusted, so
it is checked against this machine: if that folder does not exist here, the field
is left empty and the sheet says which path was dropped — you pick a real one
before the task can be added. A folder that exists but that ClaudeQ may not read
yet counts as existing and is kept.

**Import from the CLI.** `claudeq import PATH` (optionally `--id ID` to choose
the id) adds the task straight to the end of the queue with its settings
**exactly as exported** — working directory, schedule, model, permissions and
enabled state included. A working directory that does not exist on this machine
is kept, with a warning naming it; fix it with `claudeq edit ID --dir PATH`.
Because settings arrive as-is, a task exported as *enabled* with an *as soon as
possible* trigger is eligible to run right after a CLI import; pause or edit it
first if that is not what you want.

What the file cannot decide is filled in, either way:

- Ids may contain only letters, digits, `.`, `-` and `_` (they appear in the
  app's own URLs); anything else is rejected. A CLI import keeps the file's id,
  or a numeric suffix (`nightly-sweep-2`, `-3`, …) when that id is already in
  use — the existing task is never touched; a file with no id gets one derived
  from its name. A task added from the sheet gets a fresh id like any other new
  task.
- Missing `permissions` mean `default`; a missing name falls back to the id.
- On a CLI import, any scheduling history left behind by an earlier task with
  the same id (deleted before this version cleaned up after itself) is dropped,
  so the import starts fresh.

A file with an invalid task (no prompt, unknown trigger, bad cron) is rejected
in both paths and nothing is added.

## Sending feedback

**Feedback** at the bottom of the sidebar turns a bug report or a wish into a
GitHub issue without you having to write one.

1. Say what is wrong, or what ClaudeQ should be able to do — in whatever
   language you think in.
2. Claude reads it and either asks one short clarifying question (at most twice,
   and only when the report cannot be acted on as it stands) or goes straight to
   a draft. This runs through your local `claude` CLI on Haiku and costs a
   fraction of a cent per message; it does not go through the queue, so it works
   while tasks are running.
3. You get the finished issue — an English title and body, labelled either
   `bug` or `enhancement` — in editable fields.
   Your ClaudeQ version and macOS version are named there and ride along as a
   last line of the issue.
4. **Open on GitHub** opens GitHub's prefilled *new issue* page in your browser.
   The issue exists only once you press **Create** there — and everything,
   including that version line, can still be changed or deleted on that page.

ClaudeQ never talks to GitHub for this and stores no token: your browser is
already signed in, and the whole issue travels in the page's URL. If the
assistant cannot be reached — no `claude` binary, or your usage limit is
exhausted — the sheet keeps what you typed and lets you write the issue by hand.

The chat runs the CLI with tools, MCP servers, skills and `CLAUDE.md` files all
switched off, in an empty throwaway directory, so it can neither touch your
machine nor pull project context into a public issue. It is told not to put
personal data (paths, names, addresses, prompt contents) in the issue — and
because you see the text before anything is filed, you have the last word on
that.

## How scheduling and the limit gate behave

- **Eligibility.** On each tick the daemon starts every task that is due and
  permitted by priority, concurrency, and the limit gate.
- **The pause switch wins over everything.** While *Pause all runs* is on, a tick
  starts nothing and records nothing, `claudeq run-now` and the app's **Run now**
  are refused, and no task wake is registered (only the heartbeat stays, so the
  daemon notices when you switch it off). Nothing is lost: a task that came due
  while paused is still due afterwards.
- **The limit gate is global.** When any run reports a rate limit, all new starts
  pause until the reset. The wait comes from the CLI's `retry_delay_ms` signal
  (falling back to 15 minutes when none is exposed). At reset the gate reopens and
  the blocked task **resumes its session** rather than starting over.
- **A blocked queue says so.** While the gate is closed a banner names the time
  it reopens, the paused run is marked *rescheduled* in Activity with that time,
  and the task carries a *rescheduled* badge in the Queue — a waiting queue is
  never mistaken for a stuck one. **Cancel resume** on the run drops the plan:
  the pending session is forgotten, the run is recorded as `canceled`, and a
  one-shot task leaves the queue. The gate itself stays closed until the reset,
  since the limit is not yours to lift.
- **Auth problems don't retry.** A login/authentication error is recorded as
  `auth_error` and notified so you can re-login; it is not retried automatically.
- **Wake-ups.** After each pass the daemon registers a `pmset` wake at the nearest
  relevant time (next fixed start, next cron occurrence, or limit reset) plus a
  recurring heartbeat wake, so the Mac can sleep between runs and wake when
  there's work.
- **Run outcomes** are one of: `running`, `success`, `failed`,
  `rate_limited_waiting` (shown as *rescheduled* while its session is still
  queued to continue), `auth_error`, `canceled` (stopped by the user — either the
  running process or a scheduled resume).

## Data on disk

Everything lives under `~/Library/Application Support/claudeq` (override with the
`CLAUDEQ_HOME` environment variable):

| Path | Contents |
|------|----------|
| `config.toml` | Global settings + the ordered task list (human-readable, versionable). |
| `history.jsonl` | Append-only index of every run (except a quiet-history task's successful ones, which are never written). |
| `runs/<run-id>.log` | Full log for each run. |
| `artifacts.json` | Index of published artifacts (title, source task/run, file name, size, type). |
| `artifacts/<id>/<file>` | The published files themselves (snapshots copied at publish time). |
| `notifications.json` | Outbox of notifications sent with `claudeq notify`, waiting for the daemon to deliver them (normally empty). |
| `state.json` | Machine bookkeeping: read/unread flags (runs and artifacts), which artifacts have been notified about, cron anchors, pending-resume sessions, dismissed update version. |
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
