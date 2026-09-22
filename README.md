<div align="center">
  <img src="internal/api/web/logo.svg" width="96" height="96" alt="ClaudeQ">
  <h1>ClaudeQ</h1>
  <p>A queue for unattended coding-agent runs on your Mac.</p>
</div>

<!-- TODO: screenshot of the Queue view goes here (docs/queue.png, light and dark). -->

ClaudeQ takes the work you would otherwise babysit in a terminal and runs it
while you are away, most usefully overnight, once your usage allowance has
reset. A task is a prompt plus the folder it runs in, with a time it may start.
A background daemon runs it headless through
[Claude Code](https://claude.com/claude-code),
[Codex](https://learn.chatgpt.com/docs/developer-commands?surface=cli) or
[opencode](https://opencode.ai), and in the morning the result is waiting: the
full transcript in **Activity**, any file the run produced in **Artifacts**,
and a notification on your phone if you asked for one.

It began as a way to spend the nightly Claude Code allowance, and the rate
limit is still where it does most of its thinking: a run that hits the limit is
paused, and the same session continues when the window reopens, or another
provider takes the task over. Around that, the queue has grown into a small
workflow engine:

- **Several agents, one queue.** Any number of providers and accounts, each with
  its own allowance. A task names the one it runs on, or is asked of several at
  once, with a further task that joins the answers.
- **Tasks that make tasks.** A run can queue follow-up work, publish a report
  or an export, and send you a message, all through the bundled `claudeq` CLI.
  A prompt can say *"if you find something worth fixing, queue it instead of
  doing it now."*
- **Built for 3 a.m.** Every run is told it is unattended. Prompts are checked
  against this Mac before they are queued. Hung runs are killed, orphaned runs
  recovered, and the Mac is woken for timed work and held awake while it runs.
- **Nothing leaves the machine.** Tasks, settings and history are plain files
  under `~/Library/Application Support/claudeq`, and the daemon listens on
  loopback only. The agent CLIs talk to their own services; ClaudeQ itself
  contacts nothing but GitHub's release API for update checks, and
  [feedback](#sending-feedback) is filed by you, in your browser.

## What you can do with it

- **Queue the big refactor at lunch**, let it run after midnight on the fresh
  allowance, and review the branch over coffee.
- **Run a watcher every fifteen minutes** that checks a feed, a mailbox or a
  build. Write it as a *script job* and it costs no usage at all, keeps running
  while the allowance is gone, and queues a real agent job the moment it finds
  something. Quiet history keeps its successes out of the record; a real change
  reaches your phone as a push notification.
- **Ask Claude and Codex the same question**, with a third task that waits for
  both and merges the answers into one report.
- **Publish a nightly PDF or HTML report** to Artifacts and open it on your
  phone straight from the notification.
- **Turn a backlog into tasks**: one run reads the list and queues one task per
  item, so each gets its own session, log and outcome.
- **Hand a task to a colleague** as a `.claudeq` file. They import it, adjust
  the folder, and get the same job on their Mac.

## Contents

**Getting started**

- [How it works](#how-it-works)
- [Install](#install)
- [Using it](#using-it)
- [The app](#the-app)
- [Settings](#settings)

**Doing more with it**

- [Providers](#providers)
- [Command-line interface](#command-line-interface)
- [From another tool or agent](#from-another-tool-or-agent)
- [What every run is told](#what-every-run-is-told)
- [Letting a task queue follow-up work](#letting-a-task-queue-follow-up-work)
- [Asking several providers at once](#asking-several-providers-at-once)
- [Letting a task publish artifacts](#letting-a-task-publish-artifacts)
- [Letting a task send a notification](#letting-a-task-send-a-notification)
- [Script jobs](#script-jobs)
- [Quiet history for frequent jobs](#quiet-history-for-frequent-jobs)
- [Sharing tasks as files](#sharing-tasks-as-files)
- [The prompt review](#the-prompt-review)
- [Sending feedback](#sending-feedback)

**Reference**

- [Features](#features)
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
  scheduling, running tasks through the provider's CLI, tracking the rate
  limit, planning wake-ups, pruning history, sending notifications, and serving
  the dashboard on `127.0.0.1`. Exactly one daemon owns a data directory: a
  second one started against the same store says so and exits, so a queue is
  never scheduled twice.
- **A native app window (`claudeqapp`)** that wraps that dashboard in a macOS
  WKWebView window. The app is just the UI — closing it never stops scheduling,
  and the daemon keeps running with the window shut.

The nightly cycle looks like this:

1. You queue tasks during the day. Each task is a **prompt** plus the **folder**
   it runs in (its repo/context), with an optional trigger time. A task can also
   be a **script job**, where that text is a program instead of a prompt — see
   [Script jobs](#script-jobs).
2. The daemon watches the queue and starts due tasks headless, through the CLI
   of the provider each task names (Claude Code by default), one at a time
   unless a task is marked parallel. A script job is started the same way, but
   as a program: no provider, no model, no allowance spent.
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
daemon that answers on `127.0.0.1:10765` reports the version it just installed,
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

1. **New task** — give it a prompt, choose the **job type** (*Agent*, the
   default, or *Script* — see [Script jobs](#script-jobs)), pick the working folder (prefilled from
   **Settings → General → Prefill new tasks with**, if you set one), and choose a trigger
   (as-soon-as-possible, earliest start, or cron; a cron schedule is validated as
   you type, with a preview of its next three runs). Optionally pick the provider
   it runs on (only ones that can actually run are offered), override the model
   or reasoning effort or permissions, or enable *parallel* / *notify on result*. The folder dialog
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

## Features

The complete list, one line each. Every item links to the section that
explains it.

**Scheduling**

- **Three triggers per task**: *as soon as possible*, *earliest start* at a
  fixed date and time, or a 5-field *cron* schedule that is validated as you
  type, with a preview of its next three runs. See
  [How scheduling and the limit gate behave](#how-scheduling-and-the-limit-gate-behave).
- **Priority is list order**, top first. Reorder in the app or with
  `claudeq move`.
- **One at a time by default.** A task marked *parallel* runs alongside other
  parallel tasks.
- **Pause everything** with one switch in Settings: a run in flight finishes,
  nothing new starts, and the Mac is not woken.

**Providers**

- **Three harnesses, one queue**: Claude Code, and in beta Codex and opencode.
  Same queue, same history, same limit handling. See [Providers](#providers).
- **Any number of accounts.** Two providers of the same kind with separate
  configuration directories are two accounts with two allowances.
- **Checked before the night.** A provider whose CLI is missing or logged out
  blocks its tasks with the reason instead of failing them; they start by
  themselves once it works again. See [Is it ready?](#is-it-ready).
- **Rate limit per provider.** A run that hits the limit is paused and its
  session resumed when the limit clears; only that provider's tasks wait, and
  the pause is visible as *rescheduled* with the time it continues. See
  [When the limit is reached](#when-the-limit-is-reached).
- **A fallback for the limit.** A provider can name another one to take its
  tasks while its allowance is used up, so the night carries on.
- **Auth errors are their own outcome**, notified, never silently retried.

**Tasks that do more**

- **Two kinds of job.** An *agent job* sends its prompt to a model; a *script
  job* runs it as a program — deterministic, free, and never held up by a rate
  limit. See [Script jobs](#script-jobs).
- **Per-task overrides** for provider, model, reasoning effort, permission
  handling, notification and quiet history, layered over global defaults.
- **Built-in run framing.** Every run is told it is headless and unattended,
  so it finishes or hands on instead of waiting for someone. See
  [What every run is told](#what-every-run-is-told).
- **Custom system prompt** appended to every run after the built-in one.
- **Prompt review.** Before a task is queued, Claude reads the prompt against
  this Mac and names what would go wrong at 3 a.m.; one click applies the
  rewrite. See [The prompt review](#the-prompt-review).
- **Self-queueing.** A run schedules follow-up tasks with `claudeq queue`. See
  [Letting a task queue follow-up work](#letting-a-task-queue-follow-up-work).
- **Fan-out and join.** One job per provider, and a job that waits for all of
  them and combines the results. See
  [Asking several providers at once](#asking-several-providers-at-once).
- **Artifacts.** A run publishes a file with `claudeq publish`; it lands in the
  Artifacts view with an in-app viewer for HTML, PDF, images and text. See
  [Letting a task publish artifacts](#letting-a-task-publish-artifacts).
- **Own notifications.** A run sends a message with `claudeq notify`,
  optionally with a link that opens on click. See
  [Letting a task send a notification](#letting-a-task-send-a-notification).
- **Quiet history** for frequent watchers: successful runs leave no record,
  failures do. See [Quiet history for frequent jobs](#quiet-history-for-frequent-jobs).
- **Share a task** as a `.claudeq` file; the importer adjusts folder and prompt
  before it is queued. See [Sharing tasks as files](#sharing-tasks-as-files).

**Unattended by design**

- **Hung runs are killed** after a configurable idle timeout, whole process
  group included. Orphaned runs are recovered after a crash or power loss.
- **Wake and stay awake.** `pmset` wakes the Mac at each task's time plus an
  hourly heartbeat, `caffeinate` holds it through a run, and a run frozen
  across a long sleep is not mistaken for a hung one. A broken wake setup is
  shown in the dashboard.
- **File-access prompts up front.** Task folders are read at login and on
  edit, so macOS asks for Documents access while you are present.
- **Bounded history.** Old runs are pruned to a configurable count.

**Seeing what happened**

- **Notifications** natively on macOS and, optionally, to
  [Pushover](https://pushover.net), an [ntfy](https://ntfy.sh) topic, or any
  incoming webhook (Slack, Discord, Home Assistant, n8n) with a JSON body you
  template. Failures always notify, successes only if the task opts in.
- **Full history** of every run with its complete log as a chat transcript or
  raw output, and replay.
- **Continue in Chat…** opens Terminal in the task's folder and resumes the
  run's session interactively, with the harness that owns it.
- **Usage**: tokens, runs and API-equivalent cost per day over the last 14
  days.

**Platform**

- **Scriptable.** The bundled `claudeq` CLI does everything the window does,
  with `--json` output for tools and agents. See
  [Command-line interface](#command-line-interface).
- **Native macOS**: its own window, Dock icon, menu bar, live accent colour,
  light and dark.
- **Automatic updates** from GitHub releases, one click to install. A version
  whose daemon never took over is flagged with a **Finish update** button.
- **Feedback that writes itself.** Describe a bug or a wish, Claude drafts the
  GitHub issue, and you file it yourself. See [Sending feedback](#sending-feedback).
- **Local and private.** Human-readable TOML/JSON under your Library folder;
  the API is loopback-only.

## The app

The dashboard (and the native window that wraps it) has five views. Four sit at
the top of the sidebar; **Settings** and **Feedback** ([below](#sending-feedback))
sit at the bottom, out of the way of the work:

- **Queue** — the pending tasks in priority order. Add, edit, delete, enable/pause,
  reorder, or **run now** (a manual test run, independent of the trigger). Each
  row carries its task's name, what it runs on (the provider and the model that
  provider will use for it), its trigger and when it is next due. The
  **All / Active** switch in the toolbar hides the paused tasks; moving a task
  up then puts it above the next visible one, skipping the hidden tasks in
  between. Tasks can be filed into **groups**: grab a row by its handle and drag
  it onto a group's header to move it there, between two rows to reorder it, or
  onto **Drop here to start a new group** to make a group on the spot — it is
  named as it is created. A group header folds its section shut and says how
  many tasks are inside; whether it is open or folded is remembered across
  restarts. Dragging a header moves the whole section, so the groups can be put
  in the order the work happens in, and the pencil on a header renames the group
  — typing the name of a group that already exists merges the two, after asking. Dragging the last task out of a group removes the group, since a
  group is nothing but the tasks that name it. While the global pause switch is on, a yellow banner sits above the
  list (with a **Resume runs** button) and **Run now** is disabled on every
  row. A running one-shot task moves to
  Activity; a recurring task stays here with a *running* badge; hovering its
  cron expression shows the next occurrence and when it last ran. Underneath
  each task sits a badge for every option it has switched on: *script* (it runs
  as a program, without a model), *parallel*, *granted* (orange, the task skips
  permission prompts), *notifies* (blue), and *silent* (quiet history). A script
  job says **Script** where the others name their provider and model. A task the rate limit interrupted carries a
  *rescheduled* badge (orange) whose tooltip names when its interrupted session
  continues, and a task whose provider cannot run it carries a red *blocked*
  badge naming what is wrong. An **export** button on each row saves the task as a `.claudeq`
  file via the native save panel, and **Import…** in the toolbar opens such a
  file in the task sheet for review. Whenever a prompt is written or changed,
  Claude checks it against this Mac and shows what it found in a purple banner
  under the prompt box, with **Apply** to take the rewrite; just opening a sheet
  again shows the earlier finding and costs nothing. A [script job](#script-jobs)
  is never reviewed — its text is a program, and the sheet hides the provider,
  model and permission settings it cannot have.
  See [The prompt review](#the-prompt-review).
- **Activity** — every run, newest first, with an unread badge for new results.
  Open a run to see the live/finished log as a chat view or raw output, along
  with the prompt; a running task can be stopped from there with **Cancel task**
  (its process is terminated and the run is recorded as `canceled`); a run the
  rate limit paused shows as **rescheduled** with the time it continues, and
  **Cancel resume** — on the row and in its log view — drops that plan: the
  interrupted session is discarded, the run is recorded as `canceled`, and a
  one-shot task leaves the queue instead of starting again (a recurring task
  keeps its schedule and starts fresh at its next occurrence); a finished
  run offers **Continue in Chat…**, which opens Terminal in the task's folder
  and resumes the run's session interactively — with the harness that owns it
  (`claude --resume`, `codex resume` for a Codex run, `opencode --session`
  for an opencode run) and the same permission
  mode the run had (a skip-permissions task resumes with
  `--dangerously-skip-permissions`);
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
  still be resumed — **Continue in Chat…**, the same interactive resume as in
  a run's log. Either way, opening an artifact marks it read automatically; you can
  also mark one or all read by hand, or delete one (which removes the stored copy).
  The view is built like Activity: a from–to date filter in the toolbar, 25
  artifacts per page, and a footer with the count, the page and the pager.
  Clicking the notification of a newly published artifact lands here with that
  artifact already open — on the page that holds it, with a date filter that
  would hide it dropped.
- **Usage** — a per-day bar chart of runs, tokens, and cost for the last 14 days,
  plus totals and a 7-day summary.
- **Settings** — global defaults and integrations (below). The custom system
  prompt gets the same review banner as a task prompt. A red badge here means
  an update is available.

The dashboard is also reachable in a normal browser at
`http://127.0.0.1:10765` while the daemon is running.

## Settings

Settings is split into four tabs — **General**, **Providers**,
**Notifications** and **System**. An available update is announced by a banner
above the tabs and by a red dot on **General**, which is where the About section
and the update button live.

| Tab | Group | Setting | What it does |
|-----|-------|---------|--------------|
| **General** | Defaults for every run | Custom system prompt | Extra instructions appended to every run after the built-in prompt. |
| | New tasks | Prefill new tasks with | The folder a new task's working directory starts at. Still editable per task; empty leaves the field blank. |
| | Prompt review | Check prompts with Claude | Whether Claude reviews a prompt against this Mac before the task is queued (on by default). |
| | | Review provider | Which provider answers the review. Only providers that can hold an *aside* are offered. *Default* follows the global default provider. |
| | | Review model | Model used for that review; *The provider's default model* falls back to the reviewing provider's own default. |
| | Feedback | Feedback provider | Which provider drafts the GitHub issue on the [Feedback](#sending-feedback) page. Same rule as above. |
| | | Feedback model | Model it drafts with; left at *ClaudeQ's choice* it uses a small, fast model rather than whatever you picked for real tasks. |
| | Execution | Pause all runs | Global stop switch: nothing starts while it is on, not even *Run now*; a run already in flight keeps going. Applies immediately, without pressing Save. |
| | About | Version / Software updates | Current version and a manual "Check for updates" button. |
| **Providers** | One block per provider | Its settings | Name, status, binary path, configuration directory, default model, the provider that takes over when the limit is reached (and the model it uses), and an on/off switch — written by the **Save** button at the top of Settings, like every other field here. **Check again**, **Make default** and **Remove** are actions and take effect at once. The block is headed by the provider's name and type. See [Providers](#providers). |
| | | **Add provider** | Below the blocks, and only with beta features on: opens a sheet asking for an id, a type and optionally a configuration directory. |
| **Notifications** | macOS | Alerts that wait for you | Opens System Settings → Notifications, where ClaudeQ's alert style lives: *Banners* disappear on their own, *Alerts* stay until you click them. |
| | Pushover | Send to Pushover | Toggle plus API token and user key for phone push. |
| | ntfy | Send to ntfy | Toggle, server (empty = ntfy.sh), topic, and an optional access token for a protected topic. |
| | Webhook | Post to a webhook | Toggle, endpoint URL, and a JSON body template using `{{title}}`, `{{message}}`, `{{url}}` (empty = ClaudeQ's own body). |
| **System** | Runs | Stop a run with no output for | Idle-timeout watchdog: kills a hung run (default 30 min; a working run keeps streaming and is unaffected; Off disables it). |
| | | Keep run history | How many runs (and their logs) to retain before pruning (default 500; Unlimited keeps everything). |
| | Scheduler | Check for due tasks every | How often the daemon wakes to look for work (15 min – 6 h; also the wake safety-net interval). |
| | Beta features | Beta features | Reveals the parts of ClaudeQ that are not finished yet — currently **Add provider** and the **Codex** and **opencode** providers. Presentation only: anything already set up keeps working and the CLI accepts it either way. |

## Providers

A **provider** is one configured agent harness — the CLI a task is actually run
by. Every installation has one, `claude`, which is the
[Claude Code](https://claude.com/claude-code) CLI; an existing configuration is
migrated into it automatically, keeping the binary path and default model it
already used.

Three harnesses are supported:

| Type | CLI | Notes |
|------|-----|-------|
| Claude (`claude-code`) | [Claude Code](https://claude.com/claude-code) | Reports token counts and cost. Its only authority settings are "ask" and "skip every prompt". |
| Codex (`codex`) | [Codex](https://learn.chatgpt.com/docs/developer-commands?surface=cli) | **Beta.** Takes a reasoning effort and a real sandbox mode, so read-only and workspace-write actually mean something. Tasks may use a working folder that is not itself a Git repository. Reports tokens but no cost — ClaudeQ never invents one. |
| opencode (`opencode`) | [opencode](https://opencode.ai) | **Beta.** Runs whatever model opencode is configured for, including local ones (it was brought up against LM Studio). Takes a reasoning effort as opencode's *variant*. Permission handling is all or nothing: the CLI's own prompts, or none. Reports tokens and cost as opencode states them. It cannot answer ClaudeQ's own questions yet, so the prompt review and the feedback assistant are not offered on it, and its errors do not tell a rate limit from any other failure, so a limited run fails instead of pausing. |

You can configure as many instances as you like, including two of the same kind:
give each its own configuration directory and they are two accounts, with their
own sessions and their own rate limit. A limit on one does not hold up the
other.

A provider has:

| Field | What it is |
|-------|------------|
| Id | The stable name a task selects it by (`claude`). Fixed once created. |
| Type | Which harness it runs — Claude, Codex or opencode. Fixed once created. The app says *type* and names the harness; the adapter kind it maps to (`claude-code`) is what the file stores. |
| Name | The label shown in the app and in run messages. |
| Binary | Absolute path to the CLI. The background daemon can't see your shell `PATH`, so a full path is safest; empty auto-detects and the card offers what it found. |
| Configuration directory | Where that CLI keeps its account and sessions. Empty uses the CLI's own (`~/.claude`, `~/.codex`, `~/.config/opencode`), which the field shows as its placeholder. Two providers with separate directories are two separate accounts. |

Both paths must be absolute; a leading `~` is expanded and stored resolved, so
the file says what is actually used.
| Default model | Used for tasks on this provider that name no model of their own. |
| When the limit is reached | Another provider that takes this one's tasks while its allowance is used up. Empty means they wait for the window to reopen, which is what ClaudeQ did before. |
| Model there | The model those substituted runs are given. Empty (*Decided by ClaudeQ*) keeps the task's own model between two accounts of the same CLI and takes the substitute's default model otherwise. Only shown once a fallback is chosen, and cleared with it. |
| Enabled | Off keeps the provider and its tasks, but runs nothing on it. |

One provider is the **default**: tasks that name none run on it. It cannot be
switched off or removed while it holds that role — make another one the default
first — because most tasks name no provider and would all stop at once.

**Settings → Providers manages all of this.** Each provider gets its own block,
headed by its name and type, where you edit the name, binary, configuration
directory, default model and the on/off switch; one **Save** at the top of
Settings writes them all, together with everything else on the page. *Check
again*, *Make default* and *Remove* are actions and take effect at once. Below them sits **Add provider**, which opens a sheet asking for the two things
that cannot be changed afterwards — the id and the type — plus an optional
configuration directory, which is what makes the new one a second account.

`claudeq provider …` does exactly the same things, and both refuse the same
changes for the same reasons — removing an instance a task still names, for
instance, which names the tasks that have to change first.

### Beta features

Two things are behind one switch, **Settings → System → Beta features**: adding
providers at all, and the two beta harnesses, Codex and opencode. With it off
there is no **Add provider** button and neither is offered; with it on all of
them appear, the button marked as beta, and anything on a beta provider is
labelled *beta* wherever it shows up. The switch itself does not enumerate what it contains — that is what
this section is for.

That switch decides what the app *offers* and nothing else. The adapter is
always part of the build, the API and `claudeq` always accept Codex and
opencode, and the scheduler never looks at the switch — so a Codex task created from the command
line runs, and stays visible in Queue and Activity, whatever the app is showing.
Hiding setup controls never hides actual work.

Codex is beta for one concrete reason: what a real exhausted ChatGPT allowance
looks like in its output has not been observed yet. A rate-limited Codex run is
paused and resumed on a fixed delay rather than at the time the provider names,
because in the controlled test it named none.

opencode is beta because its error reporting is the weakest of the three:
running it locally, every failure came back in the same generic shape, so
ClaudeQ cannot tell an exhausted allowance or a logged-out account from any
other error. A failed opencode run is therefore recorded as failed, not paused
and resumed, and the readiness check can only confirm that the binary runs.

Editing a task changes its provider only when you say so. Every other edit — the
prompt, the folder, the schedule — goes through whatever state the current
provider is in, since that edit may well be how you are fixing it.

### Is it ready?

Before a task starts, ClaudeQ checks that its provider can actually run it. The
check costs no model usage — it looks at the binary, asks for its version, and
asks the CLI whether anyone is logged in — and it reports one of:

| Status | Meaning |
|--------|---------|
| Ready | Jobs can start. |
| Not installed | The CLI was not found, or the configured path cannot be run. |
| Not logged in | The CLI is there, but no account is signed in (`claude auth login`). |
| Invalid configuration | The provider's own settings cannot be used — an unreachable configuration directory, say. |
| Switched off | You disabled the provider. |
| Could not be checked | The check itself did not reach a verdict, so ClaudeQ does not assume either way. |

A fresh installation therefore shows the real state of Claude Code rather than
assuming it is there because it is the default provider.

What follows from an unready provider:

- **New tasks are refused** — in the app and from the CLI — with the reason. A
  job that is known in advance to fail is not worth filing. *Could not be
  checked* is the exception: "I could not ask" is not "it does not work", so the
  task is filed and simply waits, rather than a running job losing the follow-up
  it just queued because one probe timed out.
- **Tasks that already exist stay queued.** They are not started, they take no
  concurrency slot, and nothing about their schedule advances: a one-shot task
  is not marked done, a cron task keeps its next occurrence. The Queue row shows
  a red *blocked* badge with the reason.
- **When the provider works again, they start by themselves** on the next check.
  Nothing has to be re-queued.
- **You are told once.** A provider that breaks raises one notification, and one
  more when it recovers — not one per scheduler tick, and not again after a
  daemon restart.

A rate limit is not a provider problem: that pauses the run and resumes it, as
it always did — and it pauses only the provider that hit it. The other providers
keep working, because the allowance belongs to one account.

### When the limit is reached

A provider can name a **fallback**: the provider its tasks run on while its own
allowance is used up. Without one — the default — those tasks simply wait, as
before.

- It applies to the rate limit and to nothing else. A provider that is missing,
  logged out or switched off is still never answered by running the work
  somewhere else; those tasks stay queued and say why.
- The chain is followed as far as it reaches. A provider in it that cannot take
  the work right now — rate-limited itself, switched off, not installed, not
  logged in — is stepped over and its own fallback is asked. When nothing in the
  chain can work, the tasks wait: the queue is honest about being stuck.
- **The interrupted session does not travel.** A session belongs to the account
  that issued it, so the substitute starts the task again from the beginning and
  says so in the run's log, which also names the provider it came from. Once
  that run finishes the paused session is dropped rather than resumed later:
  the work has been done, and doing it twice is worse than losing a
  conversation. The paused run stays in Activity as the record of what
  happened.
- **The model can be chosen with the fallback** ("Model there"). Left open, the
  task's own model travels only between two accounts of the same harness, and a
  fallback of a different type runs its own default model instead of a name it
  would not understand.
- A fallback must name another configured provider, and the chain may not close
  into a circle; removing a provider that is somebody's fallback is refused by
  name, exactly like removing one a task still uses.
- The waiting banner names both: *Claude → Second account*, and says the queue
  keeps running rather than that nothing starts. With several providers blocked
  it says which part of the work carries on.

Set it on the provider's block in **Settings → Providers** ("When the limit is
reached", plus "Model there"), or with
`claudeq provider edit ID --fallback OTHER --fallback-model MODEL`; `none`
clears either flag.

### Credentials

ClaudeQ stores no passwords, tokens or API keys. It stores the *path* of a
CLI's configuration directory; the CLI owns what is inside it. Readiness checks
read only whether a login exists, never whose it is, and neither the daemon log
nor the API ever carries account details.

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
claudeqd run [--interval 5s] [--no-wake] [--addr 127.0.0.1:10765]
claudeqd install      # install & start the LaunchAgent (autostart at login)
claudeqd uninstall    # stop & remove the LaunchAgent
claudeqd --version
```

The installer runs `install` for you; you rarely need these directly.

### `claudeq` — the control CLI

```
claudeq list [--json]                          # show the queue
claudeq show   ID [--json]                     # one task in full, prompt included
claudeq add    --id ID --prompt P --dir DIR [--name N] [--group G]
               [--kind agent|script]           # script = a program, no model
               [--trigger asap|fixed|cron] [--at RFC3339] [--cron EXPR]
               [--provider ID] [--model M] [--reasoning-effort E] [--parallel]
               [--skip-permissions] [--notify] [--quiet-history]
claudeq edit   ID                              # open the whole task in $EDITOR
claudeq edit   ID [--name N] [--prompt P | --prompt-file PATH] [--dir DIR]
               [--group G]                     # "" takes it out of its group
               [--kind agent|script]           # switching to script drops the model settings
               [--trigger asap|fixed|cron] [--at RFC3339] [--cron EXPR]
               [--provider ID] [--model M] [--reasoning-effort E]
               [--parallel=BOOL] [--enabled=BOOL]
               [--skip-permissions=BOOL] [--notify=BOOL] [--quiet-history=BOOL]
claudeq queue  --prompt P [--at RFC3339 | --in DUR | --cron EXPR] [--dir DIR] [--name N]
               [--group G] [--kind agent|script]   # a queued job is an agent job unless told
               [--provider ID] [--model M] [--reasoning-effort E]
               [--parallel=BOOL] [--skip-permissions=BOOL]
               [--notify=BOOL] [--quiet-history=BOOL]
               [--depends-on JOBID]...            # wait for these jobs to finish
               [--include-results]                # and put their answers in the prompt
                                                  # (a script reads them on stdin)
               [--json]                           # print the new job, id included
claudeq publish --file PATH [--title T] [--description D]   # publish a file as an artifact
claudeq notify --title T --message M [--url U]  # send a notification, no artifact
claudeq export ID [--out PATH] [--force]       # write the task to a .claudeq file
claudeq import PATH [--id ID] [--provider ID] [--model NAME]   # add the task from a .claudeq file
claudeq rm ID
claudeq enable ID | claudeq disable ID
claudeq move   ID INDEX                        # 0 = highest priority
claudeq run-now ID                             # run once, now, for testing
claudeq status [--all]                         # recent runs; unread marked *
claudeq read RUNID | claudeq read-all
claudeq provider list [--json]                 # the harnesses tasks run on
claudeq provider show ID [--json]
claudeq provider check ID [--json]             # probe it now
claudeq provider add  ID --kind claude-code|codex [--name N] [--path PATH]
                      [--config-dir PATH] [--default-model MODEL]
                      [--fallback ID] [--fallback-model MODEL]
claudeq provider edit ID [--name N] [--path PATH]
                      [--config-dir PATH] [--default-model MODEL]
                      [--fallback ID|none]      # who takes over at the limit
                      [--fallback-model M|none] # what they run it with
claudeq provider enable ID | claudeq provider disable ID
claudeq provider default ID                    # run tasks that name none on it
claudeq provider rm ID
claudeq settings [--json] [--default-provider ID]
                 [--heartbeat-minutes N]
                 [--idle-timeout-minutes N] [--max-run-history N]
                 [--system-prompt S | --system-prompt-file PATH]
                 [--paused=BOOL]                # pause/resume every run
                 [--pushover=BOOL] [--pushover-token T] [--pushover-user U]
                 [--ntfy=BOOL] [--ntfy-server S] [--ntfy-topic T] [--ntfy-token T]
                 [--webhook=BOOL] [--webhook-url U] [--webhook-template J]
claudeq --version
```

### Looking at the queue

`claudeq list` prints one line per task in priority order, index 0 first:

```
#  ID              NAME            KIND    TRIGGER  WHEN         PARALLEL  ENABLED
0  nightly-sweep   Nightly sweep   agent   cron     0 3 * * *    false     true
1  needs-review    Needs review    script  cron     */5 * * * *  true      true
```

Prompts are often pages long, so that table leaves them out, and a name longer
than 40 characters is cut with an ellipsis so the columns stay readable.
`claudeq show ID` prints every setting of one task, its full name and then its
complete prompt — or, for a script job, its script; the settings only an agent
job has are left out. Add `--json` to either command for the same data as JSON,
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
claudeq edit nightly-sweep --provider claude             # run it on another provider
claudeq edit review-branch --provider codex --reasoning-effort xhigh
claudeq edit prod-watch --quiet-history=true              # drop its successful runs
claudeq edit nightly-sweep --group "Nightly"              # file it under a queue group
claudeq edit nightly-sweep --group ""                     # and take it back out
claudeq edit nightly-sweep --enabled=false                # pause it
claudeq edit prod-watch --kind script --prompt-file ./watch.sh   # make it a script job
```

- `--prompt-file` reads the prompt from a file, or from stdin when you pass `-`.
  That is the practical way to set a long prompt without fighting shell quoting.
- `--at` implies `--trigger fixed` and `--cron` implies `--trigger cron`, so a
  reschedule is one flag. Changing the trigger clears the timing fields that no
  longer apply.
- `--model ""` drops a per-task model override back to the provider's default
  model, and `--provider ""` back to the default provider.
- `--reasoning-effort` asks the model to think harder or less hard, for the
  harnesses that take such a setting (Codex does, Claude Code does not). It is
  ignored by the ones that do not, so it survives a move between providers.
- Changing `--provider` without naming a `--model` clears the model too, so the
  new provider's own default applies — one harness's model is never carried into
  another. Pass both to keep an explicit model across the change.
- A task is refused if the provider it names is unknown or cannot run right now;
  the error says which and why (see [Providers](#providers)). A
  [script job](#script-jobs) is never refused for that reason: it runs no
  harness, so no provider can be in its way.
- `--kind script` turns a task into a script job and clears the provider, model,
  reasoning effort and permission bypass it no longer uses. `--kind agent` turns
  it back.
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
claudeq settings --default-provider claude              # provider for tasks that name none
claudeq settings --system-prompt-file ./house-style.md   # custom system prompt
claudeq settings --default-working-dir ~/Code            # a new task's folder starts here
claudeq settings --idle-timeout-minutes 45 --max-run-history 1000
claudeq settings --paused=true                          # stop every run
claudeq settings --paused=false                         # let the queue run again
claudeq settings --prompt-review=false                  # turn the prompt review off
claudeq settings --prompt-review-provider claude-cheap   # who answers the review ("" = default provider)
claudeq settings --prompt-review-model haiku            # ("" = the provider's default model)
claudeq settings --feedback-provider claude-cheap       # who drafts the GitHub issue
claudeq settings --feedback-model haiku                 # ("" = claudeq's own small, fast choice)
claudeq settings --pushover=true --pushover-token T --pushover-user U
claudeq settings --ntfy=true --ntfy-topic claudeq-daniel   # server defaults to ntfy.sh
claudeq settings --webhook=true --webhook-url https://hooks.slack.com/services/... \
                  --webhook-template '{"text":"{{title}}: {{message}}"}'
```

For the numeric settings `0` means "use the default". `--idle-timeout-minutes`
also takes a negative value for "never kill a run", and `--max-run-history` for
"keep every run". You can set channel credentials here, but the CLI never
prints them back; the output only says whether they are configured. A channel
that is switched on but cannot deliver (no topic, no URL, an unknown
`{{placeholder}}`) is refused at save time, from the CLI same as from the app.
Use `claudeq notify --title X --message Y` to check a channel actually
delivers — there is no separate Test button in the app.

The binary path and the default model belong to a provider now, not to the
global settings:

```sh
claudeq provider list                                   # ids, kinds, default models, status
claudeq provider check claude                           # probe it and print why it is not ready
claudeq provider edit claude --path /Users/me/.local/bin/claude
claudeq provider edit claude --default-model opus
claudeq provider disable claude                         # keep it, run nothing on it

# a second Claude account, a Codex provider, and opencode with a local model
claudeq provider add claude-work --kind claude-code --config-dir ~/.claude-work
claudeq provider add codex --kind codex --default-model gpt-5.6-sol
claudeq provider add local --kind opencode
```

`provider list --json` gives each instance's enabled state, readiness, the
reason it is not ready, and its default model — which is how a running job finds
out where it can send follow-up work. `provider rm` refuses while a task or the
default-provider setting still names the instance, and lists what has to change
first.

## From another tool or agent

This README is the whole interface. An external app or agent needs nothing but
this document to work with ClaudeQ.

- **Invoke it by full path.** `/Applications/ClaudeQ.app/Contents/MacOS/claudeq`
  is not on `PATH`.
- **Read with `--json`.** `claudeq list --json`, `claudeq show ID --json`,
  `claudeq settings --json` and `claudeq provider list --json` emit structured
  output. The other commands print for humans.
- **A task is refused when its provider cannot run it.** `--provider ID` selects
  one; leaving it out uses the default. An unknown or unready provider exits
  non-zero with the reason, so ask `claudeq provider list --json` first rather
  than assuming one is there. See [Providers](#providers).
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

You don't need to repeat any of this in a task's prompt. None of it applies to
a [script job](#script-jobs): there is no model to tell anything to.

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
optional `--name`. The new task **inherits** the calling task's provider, model,
permissions, parallelism, and notification settings automatically. This works
because the daemon injects the CLI's path and the parent task into each run's
environment.

When the follow-up needs different settings from the task that queues it, pass
the same flags `claudeq add` takes; anything you leave out keeps inheriting:

```sh
claudeq queue --prompt "Review the change thoroughly …" --model claude-opus-5 --notify
```

- `--model M` runs the new task on another model (an empty value means the
  effective provider's default model). This is what lets a cheap watcher — Haiku
  every five minutes, quiet history — hand expensive work to a visible Opus run.
- `--provider ID` runs the new task on another configured harness — which is how
  a Claude run hands a review to Codex, or the other way round. Without a
  `--model` alongside it, that provider's own default model applies — a model
  from the calling task is never carried across.
- `--reasoning-effort E` sets how hard the new task's model should think, where
  the harness takes it. `claudeq provider list --json`
  says which providers exist and which of them can run right now; queueing for
  one that cannot fails with the reason instead of filing work that would not
  start (see [Providers](#providers)).
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

## Asking several providers at once

Some work wants more than one harness — *"ask Claude and Codex and give me one
summary"*, *"get every provider's take on the release notes"*. That is a
**fan-out** with a **join**, and it is built from the same `queue` command:

```sh
# one job per provider; --json so the ids can be read back out
claudeq queue --json --provider claude --prompt "Summarise what changed on main today"
claudeq queue --json --provider codex  --prompt "Summarise what changed on main today"

# one job that waits for both and combines them
claudeq queue --provider claude \
  --depends-on q-20260914T050000-a1b2c3 \
  --depends-on q-20260914T050000-d4e5f6 \
  --include-results \
  --prompt "Consolidate the attached results into one digest and publish it"
```

Every run is told how to do this in its system prompt, so a task that says "ask
all providers and consolidate" arranges it itself. ClaudeQ does not read the
prompt and guess: the harness understands the request, and this just gives it
the vocabulary.

**It is durable, not a live conversation.** The root run queues the children and
the join and then stops — it does not wait, and could not: a run ends when the
harness stops writing. The daemon owns the rest, so a restart, a closed lid, a
long child or a rate limit on one provider costs time and nothing else.

**When the join runs.** Once every job it waits for has a terminal result:
success, failure, auth error or cancellation. A job pausing on a rate limit has
*not* finished — its session is scheduled to continue — so the join keeps
waiting. A job that failed **does** release it: an unattended digest that never
appears is worse than one that says which input is missing. So does a job that
was deleted before it ran, because nothing about it is ever going to change.

**What the join gets.** `--include-results` puts each dependency's final answer
in front of the join's prompt, with the job name, the provider and model it ran
on, its status and any error. It is fenced and labelled as data, and the system
prompt tells the harness to read it as data and never as instructions. The
injected text is bounded; a shortened answer says so and names the run log that
has all of it.

**Rules worth knowing:**

- A job may only wait for jobs that **already exist**. Queue the children first.
  That is also why a cycle cannot happen — dependencies only ever point
  backwards.
- Dependencies are fixed when the job is queued and never change afterwards.
- A recurring (cron) job cannot depend on one-shot jobs: its second occurrence
  would find them long finished and run immediately, which is no dependency at
  all. Nor can you wait *for* a recurring job — it never has a last result.
- Leave `--model` off a cross-provider job unless you mean it. A model name
  belongs to the harness it was chosen for, so each provider uses its own
  default.
- Let only the join publish the artifact or send the notification, unless you
  want each provider's result separately. Otherwise one request produces several
  competing reports.

In the app, a job that is waiting says so in the Queue (*waiting for 2 jobs*,
with the names on hover), and Activity groups the runs of one workflow into a
single **Workflow** block instead of scattering them among unrelated runs. A job
queued by a run joins that run's workflow, which is also what groups an ordinary
self-queued chain.

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
open it. Each publish also raises a notification (macOS, plus every remote channel that
is configured); clicking the macOS one brings up ClaudeQ with that artifact open — in
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
Notification Center, Pushover, ntfy and the webhook, each independently — with
the same look as ClaudeQ's own alerts, and attributed to the task that sent it
(its name is appended to the message). Nothing is stored: no artifact, no
history entry. The run's own outcome is still announced according to the
task's settings, so a watcher that finds nothing sends nothing and stays
silent.

- `--title` and `--message` are required. A title longer than 250 characters
  or a message longer than 1024 (Pushover's limits) is cut, not rejected.
- `--url` is optional and must be an absolute `http` or `https` URL. Clicking
  the macOS notification opens it; Pushover and ntfy show it as the message's
  link, and the webhook fills it into `{{url}}` if the template uses it.
- The CLI hands the notification to the daemon, which sends it on its next tick
  (a few seconds). Exit code 0 means it was queued, not that a channel accepted
  it — a channel that fails is logged by the daemon (`claudeqd.err.log`).
- A notification that waited more than 24 hours — the daemon was not running —
  is dropped with a log line rather than delivered as if it were current.
- It also works outside a run, from a shell: then it is sent without attribution.

## Script jobs

A job is one of two kinds, and the task form's **Job type** switch (or `--kind`
on `claudeq add` / `edit` / `queue`) decides which:

- an **agent job** — the default — sends its prompt to a model through a
  provider's CLI, and
- a **script job** runs that same text as a **program**. No model is involved,
  no provider is chosen, nothing is spent, and nothing about it is guessed at.

Everything else is shared: the same queue, the same triggers and priority, the
same groups, the same run log in Activity, the same notifications, the same
quiet history, the same dependencies.

**What it runs.** The script is written to a temporary file and executed. Begin
it with a shebang to choose the interpreter — `#!/bin/zsh`,
`#!/usr/bin/env python3`, `#!/usr/bin/env node` — or leave it out and it runs
under `/bin/sh`. It starts in the task's **working directory**, and it inherits
the daemon's environment plus the variables every run gets: `CLAUDEQ_BIN`,
`CLAUDEQ_HOME`, `CLAUDEQ_RUN_ID`, `CLAUDEQ_TASK_ID`, `CLAUDEQ_WORKFLOW_ID` and
`CLAUDEQ_PARENT_TASK`.

**What counts as the outcome.** Exit code `0` is a success, anything else a
failure — that is the whole classification; nothing reads the output looking for
trouble. Standard output *and* standard error go to the run log, and standard
output alone is kept as the job's **answer**: the text a notification quotes and
a dependent job consolidates. More output than a run record may carry keeps the
**end** — a script says what matters last — with a line in front saying so. A hung script is killed by the same idle timeout
as an agent run, and **Cancel task** stops it and its whole process tree.

**What it cannot have.** A provider, a model, a reasoning effort, or the
permission-prompt bypass. Those all steer a model, and a job that runs none is
refused rather than quietly ignoring them. Switching an existing task to
*Script* drops them for you.

**Why it exists: watchers.** The classic frequent job — poll a query, diff it
against a seen-list, and act on what is new — spends a model's allowance on work
that is pure plumbing. As a script job it spends nothing, and it keeps running
when every provider is out of allowance, because the rate-limit gate belongs to
an account and a script job has none. What it finds, it hands to a real agent
job:

```sh
#!/bin/zsh
set -euo pipefail
new=$(./bin/poll-needs-review --since-file .agents/seen.json)
[[ -z "$new" ]] && exit 0            # nothing changed: say nothing, spend nothing

while IFS= read -r id; do
  "${CLAUDEQ_BIN:-claudeq}" queue --model opus \
    --name "Review $id" --prompt "Run the review for work item $id."
done <<< "$new"

"${CLAUDEQ_BIN:-claudeq}" notify --title "Needs review" --message "$(wc -l <<< "$new") new item(s)"
```

A job queued from a script is an **agent job** — that is the point of the split,
so the kind is never inherited. A script that genuinely wants to queue another
script says `--kind script`.

**Dependencies.** A script job can wait for other jobs with `--depends-on` like
any other. With `--include-results` their answers arrive on the script's
**standard input** rather than in front of its text, because a program cannot
have a digest pasted on top of it. Without dependencies, standard input is
closed immediately — an unattended run never waits for input nobody will type.

## Quiet history for frequent jobs

A task that runs every few minutes would, by default, produce hundreds of
successful runs a day: each one unread in Activity, each one counting against
the `Max run history` limit until it pushes a run you actually care about out of
the record. Mark such a task **Quiet history** (the switch in the task form, or
`--quiet-history` on `claudeq add` / `claudeq edit` / `claudeq queue`) and:

- A run that **succeeds leaves no trace**: it is never written to history, and
  its log is deleted when it finishes.
- A run that **fails, hits an auth problem, is cancelled, or is paused by the
  rate limit is recorded** with its log, unread, exactly like an ordinary run,
  and notifies as usual. A pause is kept even though the daemon resumes it by
  itself: it closes that provider's gate and holds up every other task on it,
  so it is the answer to "why is nothing running?" — and its Activity entry is
  where **Cancel resume** lives.
- While it is running, the Queue shows the task's *running* badge as usual, but
  there is no Activity entry to open (and so no live log or **Cancel task**).

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

**The provider travels as a hint, never as an account.** A file records which
*kind* of harness the task was written for (`claude-code`, `codex`), the model,
and what the exporter called their provider — never a provider id, a
configuration directory or anything to do with a login. On import ClaudeQ looks
for your own provider of that kind: exactly one match is taken, with the model.
Anything else — no match, or several accounts of that kind — leaves the choice
to you, and the sheet says which harness the file wants. Two accounts are not
interchangeable (separate allowances, separate logins, often separate
employers), so ClaudeQ does not pick one for you. Importing never creates a
provider, copies a path, or touches credentials.

**Import from the CLI.** `claudeq import PATH` (optionally `--id ID` to choose
the id) adds the task straight to the end of the queue with its settings
**exactly as exported** — working directory, schedule, model, permissions and
enabled state included. When the file's provider hint matches no single provider
here, the import is refused and names what it wants; `--provider ID` places it
(and `--model NAME` picks the model, since the exporter's belongs to their
harness). A working directory that does not exist on this machine
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

## The prompt review

A prompt written for one Mac rarely fits the next one unchanged, and a task
queued for 3 a.m. has nobody around to notice. So whenever a draft prompt is
written or changed — in a new task, an edit, a replay or an import — ClaudeQ has
Claude read it against *this* machine and, if something is off, shows it in a
purple banner under the prompt box:

> ✦ **ClaudeQ suggests:** docs/spec.md does not exist here, so the run has
> nothing to read. Also out/weekly.md would be written into an out/ directory
> that does not exist yet, so the rewrite creates it first.
>
> [ Apply ] [ Dismiss ]

**Apply** replaces the prompt with the rewrite, in the box, where you can still
change it before saving; the review then runs again on the result. **Dismiss**
just hides the banner. Nothing is ever saved, queued or changed on your behalf,
and a task can always be added exactly as written — the banner is advice, not a
gate.

What it looks for:

- **A path that isn't there.** ClaudeQ resolves every path the prompt mentions
  (absolute, `~/…`, or relative to the task's working directory) and checks it.
  An input file or folder that does not exist is reported; the right path is
  yours to supply, so no rewrite is offered.
- **A file to be written into a folder that doesn't exist.** The missing file is
  normal; the missing parent directory is the problem. The rewrite tells the run
  to create it first.
- **Guidelines it would depend on.** If the prompt says to follow the rules in
  some file and that file is small, the rewrite pastes its content straight into
  the prompt, so the task no longer depends on the file still being readable
  when it runs.
- **A prompt that waits for you.** A question, a confirmation, a choice — an
  unattended run has nobody to answer it, so the rewrite decides up front.

The same banner sits under the **custom system prompt** in Settings, with the
rules adjusted to what that text is: it applies to every task, in every
directory, so a relative path there means something different every time.

Practicalities:

- It runs on a provider, so it costs a little usage each time. Which provider
  and which model are yours to pick under Settings → General → Prompt review —
  reviewing is frequent, cheap work with no claim on the allowance you reserved
  for real tasks, so a second account or a fast model earns its keep. Or switch
  the whole thing off with the toggle above. Only providers that can answer a
  question of ClaudeQ's own are offered (see below); if none can, the banner
  simply stays away.
- The review itself is the narrowest invocation ClaudeQ makes: no tools at all
  (ClaudeQ hands it the path checks and the small files it read), no CLAUDE.md,
  skills, plugins, hooks or MCP servers, no saved session, and a working
  directory outside any repository. It reads; it never writes. ClaudeQ calls
  this an *aside* — a question it asks a harness on its own behalf rather than
  to do your work — and a provider has to be able to hold one to be offered for
  it. Claude Code can; Codex cannot yet.
- It runs on a change, not on a look. Editing the prompt, choosing a working
  directory or importing a task starts a review from scratch and cancels the one
  still in flight — including its Claude process — so only the newest answer is
  ever shown. Opening a sheet changes nothing, so it asks nothing: an answer is
  remembered for a day, and reopening the same task, or the app, shows that
  finding again for free, headed *ClaudeQ suggested earlier* and with a **Check
  again** button for when the machine has moved on — the missing file exists by
  now — but the prompt has not. Undoing an edit is free too: the finding for the
  text you are back at is still remembered. **Dismiss** forgets a finding rather
  than hiding it, so it does not come back by itself; editing the prompt back and
  forth after that asks once more.
- Switching the review off hides the banner everywhere, remembered findings
  included. A finding is also only ever reused for the reviewer that produced
  it, so changing the review provider or model — or the default provider the
  review follows — never replays an answer the new one did not give.
- A task sheet without a working directory yet — an imported task, whose folder
  came from another Mac — waits for you to choose one before reviewing, since
  every relative path would otherwise be unresolvable.
- File content it was given is treated as data. Instructions found inside a
  file cannot steer the review, and any rewrite is shown to you in the prompt
  box before it can run.

## Sending feedback

**Feedback** at the bottom of the sidebar is a page of its own, and turns a bug
report or a wish into a GitHub issue without you having to write one.

1. Say what is wrong, or what ClaudeQ should be able to do — in whatever
   language you think in.
2. Claude reads it and either asks one short clarifying question (at most twice,
   and only when the report cannot be acted on as it stands) or goes straight to
   a draft. This runs on the provider you chose under Settings → General →
   Feedback, on a small model by default, and costs a fraction of a cent per
   message; it does not go through the queue, so it works while tasks are
   running.
3. You get the finished issue — an English title and body, labelled either
   `bug` or `enhancement` — in editable fields.
   Your ClaudeQ version and macOS version are named there and ride along as a
   last line of the issue.
4. **Open on GitHub** opens GitHub's prefilled *new issue* page in your browser.
   The issue exists only once you press **Create** there — and everything,
   including that version line, can still be changed or deleted on that page.

ClaudeQ never talks to GitHub for this and stores no token: your browser is
already signed in, and the whole issue travels in the page's URL. If the
assistant cannot be reached — no provider set up to draft one, or your usage
limit is exhausted — the page keeps what you typed and lets you write the issue
by hand.

The chat runs as an *aside* (see [The prompt review](#the-prompt-review)): tools,
MCP servers, skills and `CLAUDE.md` files all switched off, in an empty
throwaway directory, so it can neither touch your machine nor pull project
context into a public issue. Only a provider that can hold such a conversation
is offered for it. It is told not to put
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
- **The limit gate belongs to the provider.** When a run reports a rate limit,
  new starts on *that provider* pause until the reset; the other providers carry
  on. The wait comes from the reset time the CLI reports, then from its
  `retry_delay_ms` signal, and falls back to 15 minutes when neither is exposed.
  At reset the gate reopens and the blocked task **resumes its session** rather
  than starting over.
- **A fallback provider skips the wait.** If the blocked provider names one (see
  [When the limit is reached](#when-the-limit-is-reached)), its tasks run there
  meanwhile — from the start, since a session belongs to the account that issued
  it. The interrupted session is then dropped instead of being resumed after the
  reset, so the task does not run twice.
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
| `config.toml` | Global settings, the configured [providers](#providers), and the ordered task list — the order is the priority, and tasks of one group sit together in it (human-readable, versionable). No credentials: a provider entry holds its CLI's path and configuration directory, never what is inside them. |
| `history.jsonl` | Append-only index of every run (except a quiet-history task's successful ones, which are never written). |
| `runs/<run-id>.log` | Full log for each run. |
| `artifacts.json` | Index of published artifacts (title, source task/run, file name, size, type). |
| `artifacts/<id>/<file>` | The published files themselves (snapshots copied at publish time). |
| `notifications.json` | Outbox of notifications sent with `claudeq notify`, waiting for the daemon to deliver them (normally empty). |
| `state.json` | Machine bookkeeping: read/unread flags (runs and artifacts), which artifacts have been notified about, cron anchors, pending-resume sessions, the provider health you were last told about, which queue groups are folded shut, dismissed update version. |
| `claudeqd.out.log` / `claudeqd.err.log` | Daemon stdout/stderr. |
| `.lock` / `.daemon.lock` | Lock files, both empty of interest. `.lock` serializes config/state writes between the daemon and a `claudeq` command; `.daemon.lock` holds the running daemon's pid and is what makes a second daemon on the same store refuse to start. Both are released when the holding process exits, so neither needs clearing by hand. |

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
UI, talking to the daemon over loopback. Runs go through a **provider adapter**:
the daemon knows a task runs on a configured provider instance with a model and
an access mode, and the adapter for that provider owns the rest. The Claude Code
adapter spawns the `claude` CLI once per task in the task's directory using
`--output-format stream-json`, which lets it watch for rate-limit and auth events
as they happen and capture the session id, token usage, and cost from the final
result. The daemon also spawns a second, far smaller kind of `claude` call for
the [prompt review](#the-prompt-review): one turn, no tools, no session. ClaudeQ
performs **no Git operations** — any branch/commit behavior is driven entirely by
your prompts and the repo's own configuration. The dashboard itself is plain ES
modules with no build step and no framework: design tokens and shared primitives
under `styles/`, and one directory per component holding its markup, its style
and its code together. The full design, decisions, and
verification notes are in [PLAN.md](PLAN.md).

## Requirements

- macOS 12 or newer
- The [Claude Code](https://claude.com/claude-code) CLI, installed and
  authenticated
- Optionally the [Codex](https://learn.chatgpt.com/docs/developer-commands?surface=cli)
  CLI, installed and logged in, for tasks you want to run on it (beta)

## License

[MIT](LICENSE) © 2026 Daniel Maier
