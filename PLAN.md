# claudeq — Solution Concept (PLAN)

Status: draft for review · No implementation until explicitly commissioned.
Based on: `anforderungskatalog-taskqueue.md` (v0.2) + clarification dialogue (decisions D1–D8 below).

---

## 1. Summary

**claudeq** is a lightweight, local-only macOS tool that lets the user queue Claude Code
tasks during the day and executes them automatically at night, so the nightly usage
allowance is spent productively while saving time and tokens during the day.

Each task runs Claude Code via its CLI in its own working directory (its own repo/context).
Scheduling is limit-aware, priority-ordered, supports one-at-a-time or parallel execution,
survives reboots, and can wake the machine from sleep. Results, status, logs, unread news
and full history are shown in a native app. Failures and auth problems raise notifications
(native macOS + Pushover).

---

## 2. Goals & Non-Goals

### Goals
- Queue code tasks by day, run them by night, unattended.
- Exploit the nightly usage window; wait cleanly when the shared limit is exhausted.
- Reliable execution even when the machine is idle or asleep.
- Simple, private, local operation; human-readable, backup-able configuration.

### Non-Goals (v1, from catalog §6)
- No local AI models (Claude Code only).
- Code tasks only.
- No mobile/remote access (FA-21 deferred).
- No multi-user/team features.
- **claudeq does not manage Git** (see D6): no auto-branch, no auto-commit. Branch/commit
  behavior is driven by the repos' system prompts and the user prompts.

---

## 3. Clarification Decisions (D1–D8)

These refine/override the catalog where noted.

| # | Topic | Decision |
|---|-------|----------|
| **D1** | Limit detection | **Purely reactive.** Start the task; if it hits the rate limit, remember it **globally** and block all starts until it clears. No pre-flight estimation / no undocumented endpoint. FA-05 is realized via this reactive gate. **Revised by V2 (§10):** an absolute reset timestamp is *not* reliably exposed by the CLI — the block duration is derived from the per-attempt `retry_delay_ms` in `stream-json` output (and/or delegated to the CLI's own retry watchdog). |
| **D2** | GUI form factor | **Native window app**, implemented with **Wails** (Go core + WebView UI) — chosen over Fyne because the dashboard/log/history UI is far easier in HTML/CSS. |
| **D3** | Process architecture | **Headless background daemon (launchd)** does all the work; a **thin native app** is only the UI. They talk over a **local 127.0.0.1** channel. Daemon runs regardless of whether the app window is open. |
| **D4** | Resume after limit | **Resume the session** via `claude --resume <session-id>`. Marked *verification-required*; **fallback = restart the task** if resume fails, so a task never gets stuck. |
| **D5** | Persistence | **Hybrid.** Tasks & settings in a human-readable **TOML** file (satisfies NFA-07). Run history as a **JSON Lines** index + **one log file per run** on disk. No binary DB. |
| **D6** | Git | **Not managed by claudeq.** FA-34/FA-19 (branch discipline) become a **prompt convention**, not tool-enforced. claudeq only invokes the CLI in the task's working directory. |
| **D7** | Triggers & repetition | **Three trigger modes**: (1) *as-soon-as-possible*, (2) *fixed time (one-shot)*, (3) *recurring (crontab)*. For recurring tasks, if a new occurrence is due while the previous run is still active, it is **skipped**. |
| **D8** | Wake & notifications | Wake via `pmset schedule wake` at **concrete timestamps** (fixed times, cron times, known reset time) **plus an hourly heartbeat wake** as a safety net for "as-soon-as-possible". Notifications delivered by the daemon via a **small user-session helper** for native macOS notifications. |
| **D9** | Signing & distribution | **No Apple Developer ID for now** → ship an **unsigned `.pkg`** installer built by CI. `pmset` privileges via a **`sudoers.d` entry** written by the root postinstall (SMAppService needs signing, so deferred). Colleagues do a one-time Gatekeeper bypass. Pipeline is built **notarization-ready** so adding a Developer ID later is additive (secrets + 2 steps), no rework. |
| **D10** | Data location | Data lives in **`~/Library/Application Support/claudeq/`** (`config.toml`, `history.jsonl`, `runs/<id>.log`, `state.json`), overridable via **`CLAUDEQ_HOME`** (used by tests). macOS-native, human-readable (NFA-04/07). |
| **D11** | Control surface (v1) | `claudeqd` (loop) and the `claudeq` CLI share the **file-based store**; the daemon re-reads the store each tick — **no IPC yet**. The local **HTTP API for the GUI is deferred to Phase 5**. Keeps Phase 2 verifiable end-to-end without prematurely fixing the API. |
| **D12** | Global pause | One `paused` setting stops **every** start, manual runs included — a pause a `run-now` could step around would not be a pause. It gates the run loop ahead of trigger evaluation, refuses `RunTaskNow`, and drops task wakes; in-flight runs are left to finish (pausing is not cancelling) and no scheduling state is written, so nothing is lost. It is written through its **own** endpoint (`POST /api/pause`), not the settings payload, so the UI can flip it instantly without carrying every other setting. |

---

## 4. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│  User session                                                      │
│                                                                    │
│   ┌────────────────────┐          ┌──────────────────────────┐    │
│   │  claudeq App (Wails)│  HTTP/WS │  Notification Helper      │    │
│   │  - Task management  │  (local) │  (user-session agent)     │    │
│   │  - Dashboard/News   │          │  - posts native macOS     │    │
│   │  - History + Logs   │          │    notifications          │    │
│   │  - Settings         │          └──────────────▲───────────┘    │
│   └─────────▲──────────┘                          │                │
│             │ 127.0.0.1 (REST/WS)                  │ IPC            │
└─────────────┼─────────────────────────────────────┼────────────────┘
              │                                      │
┌─────────────┼──────────────────────────────────────┼───────────────┐
│  launchd    ▼                                      │                │
│   ┌──────────────────────────────────────────────────────────┐     │
│   │  claudeq Daemon (headless, Go)                            │     │
│   │  ┌───────────┐ ┌───────────┐ ┌────────────────────────┐  │     │
│   │  │ Scheduler │ │ Executor  │ │ Limit Gate (reactive)  │  │     │
│   │  │ (triggers,│ │ (spawns   │ │ (reset memory / block  │  │     │
│   │  │  priority,│ │  claude   │ │  window)               │  │     │
│   │  │  parallel)│ │  CLI)     │ └────────────────────────┘  │     │
│   │  └───────────┘ └───────────┘ ┌────────────────────────┐  │     │
│   │  ┌───────────┐ ┌───────────┐ │ Wake Planner (pmset)   │  │     │
│   │  │ Store     │ │ Notifier  │ └────────────────────────┘  │     │
│   │  │ (TOML +   │ │ (macOS +  │                              │     │
│   │  │  JSONL +  │ │  Pushover)│                              │     │
│   │  │  logs)    │ └───────────┘                              │     │
│   │  └───────────┘                                            │     │
│   └──────────────────────────────────────────────────────────┘     │
│                        │ spawns                                     │
│                        ▼                                            │
│              `claude` CLI (per task, in task's working dir)         │
└─────────────────────────────────────────────────────────────────────┘
```

### Why this split
- The daemon must run at night with no window open and survive reboot/logout → it lives in
  `launchd` and is the single source of truth (FA-09, FA-32, NFA-03).
- The app is disposable UI: closing it never stops scheduling (D3).
- A separate user-session helper is needed because a pure `launchd` daemon generally cannot
  post to Notification Center; the helper runs in the user's GUI session (D8, FA-39).

---

## 5. Components

### 5.1 Daemon (Go, headless)
Long-running process managed by `launchd`. Owns scheduling, execution, limit state,
wake planning, persistence, and notification dispatch. Exposes a local API for the app.

**launchd:** a `LaunchAgent` in the user session (`~/Library/LaunchAgents/de.maierdaniel.claudeq.plist`)
with `RunAtLoad=true` and `KeepAlive=true` so it starts at login and restarts on crash
(NFA-03). A user LaunchAgent (not a system LaunchDaemon) is preferred because it runs inside
the user's context — closer to the notification helper and the user's `claude` login.

### 5.2 Scheduler
- Maintains the task queue ordered by **manual priority (top = highest)** (FA-11).
- Evaluates trigger conditions (D7): as-soon-as-possible, fixed-time (earliest start),
  recurring/cron.
- Enforces concurrency: **one task at a time by default**; `parallel = yes` tasks may run
  together with other `parallel = yes` tasks, **no fixed upper bound** (FA-12/13/33).
- Priority and time planning still apply under parallel execution (FA-14).
- For recurring tasks, skips a new occurrence if the previous run is still active (D7).

### 5.3 Executor
- Spawns the `claude` CLI in the task's working directory with the resolved model and
  permission flags (§8).
- Captures the session id, streams stdout/stderr to the run's log file, records exit status.
- Detects rate-limit outcomes and hands the reset timestamp to the Limit Gate.
- No runtime cap — a task runs until Claude finishes (FA-10).

### 5.4 Limit Gate (reactive, D1)
- Single global gate for the shared allowance (FA-37).
- Normal state: open — tasks may start.
- When a run reports a rate limit: parse reset time → set a **blocked-until** timestamp →
  no new starts until then. On/after reset, gate reopens and blocked tasks resume/retry.
- No automatic retry beyond waiting for the limit reset (FA-35).

### 5.5 Wake Planner (D8)
- After each scheduling pass, computes the next relevant timestamp (nearest of: fixed times,
  cron occurrences, blocked-until reset) and registers it via `pmset schedule wake`.
- Also registers a **recurring hourly heartbeat wake** as a safety net (covers
  as-soon-as-possible tasks and anything without an exact time).
- On wake: re-evaluate queue → run what's due and permitted → re-plan next wake.

### 5.6 Store (D5)
- **`tasks.toml`** — task definitions + global settings (human-readable, versionable → NFA-07).
- **`history.jsonl`** — append-only index of every run (FA-36), independent of read status.
- **`runs/<run-id>.log`** — full log per run (FA-15, FA-25).
- **`state.json`** (internal) — unread flags (FA-26), last-seen marker, remembered reset time.
  (Kept separate from the human-edited `tasks.toml` so machine state never clobbers user edits.)

### 5.7 Notifier (D8)
- **Native macOS**: daemon → user-session helper → Notification Center (FA-39).
- **Pushover**: daemon posts directly to Pushover API with stored token/user-key (FA-40/41).
- **ntfy**: daemon posts JSON to a topic on `ntfy.sh` or a self-hosted server (phase 16).
- **Webhook**: daemon posts a templated JSON body to any URL — Slack, Discord, Home Assistant,
  n8n, or anything else with an incoming webhook (phase 16).
- Every channel is read from settings on each send, so an edit takes effect on the next
  notification, not after a daemon restart.
- Triggers: run failure (FA-35), login/auth problems (FA-38), optional completion/morning
  summary (FA-20, "Kann").

### 5.8 App (Wails, D2/D3)
Thin client over the daemon's local API:
- Task CRUD + manual reordering (FA-02, FA-11), "run now" (FA-16), activate/pause (FA-17).
- **News dashboard**: unread runs since last view, mark-one-read, mark-all-read, jump into a
  run's details/log (FA-22–FA-25).
- **History** view of all past runs (FA-36); no calendar view.
- Settings (global + per-task, §8).

---

## 6. Data Model

### Task (in `tasks.toml`)
| Field | Notes |
|-------|-------|
| `id` | stable identifier |
| `name` | display name |
| `prompt` | the instruction sent to Claude (FA-01) |
| `working_dir` | task context / repo path (FA-01, FA-03) |
| `trigger` | `asap` \| `fixed` \| `cron` (D7, FA-04/18) |
| `fixed_at` | timestamp, for `fixed` (earliest start, FA-08) |
| `cron` | crontab expression, for `cron` (FA-18) |
| `priority` | derived from list order (FA-11) |
| `parallel` | bool, default `false` (FA-12/13) |
| `enabled` | active/paused (FA-17) |
| `model` | `default` \| specific id (FA-30) |
| `permissions` | `default` \| `skip` (FA-31) |

### Global settings (in `tasks.toml`)
| Field | Notes |
|-------|-------|
| `default_model` | model for runs unless overridden (FA-28) |
| `heartbeat_interval` | wake safety-net interval, default 1h (D8) |
| `paused` | global stop switch: no run starts at all while true (D12) |
| `pushover.token`, `pushover.user_key` | Pushover credentials (FA-41) |
| `ntfy.enabled`, `ntfy.server`, `ntfy.topic`, `ntfy.token` | ntfy channel (phase 16) |
| `webhook.enabled`, `webhook.url`, `webhook.template` | generic webhook channel (phase 16) |

### Run (in `history.jsonl` + log file)
| Field | Notes |
|-------|-------|
| `run_id`, `task_id` | linkage |
| `started_at`, `finished_at` | timing (FA-15) |
| `status` | `success` \| `failed` \| `rate_limited_waiting` \| `auth_error` |
| `session_id` | for resume (D4) |
| `log_path` | `runs/<run-id>.log` (FA-15/25) |
| `unread` | tracked in `state.json`, persisted (FA-26) |

---

## 7. Execution & Scheduling Logic

### Global pause (D12)
A single `paused` setting gates the whole run loop, ahead of trigger evaluation: a tick with
it on starts nothing and writes no scheduling state, `RunTaskNow` (CLI `run-now`, the app's
**Run now**, the API) is refused with `store.ErrPaused`, and wake planning drops the task
candidates so the Mac is not woken for work that cannot run (the heartbeat wake stays, so the
daemon notices the switch going off). Runs already in flight are untouched — pausing is not
cancelling. Because no state is recorded, a task that came due while paused is simply due
again afterwards.

### Trigger evaluation (D7)
- **asap**: eligible immediately; runs as soon as the limit gate is open and a concurrency
  slot is free.
- **fixed**: `fixed_at` is the *earliest* start. If the limit is blocked at that time, it
  starts when the gate reopens (FA-08 example: planned 20:00, limit free 22:00 → start 22:00).
- **cron**: each occurrence enqueues a run behaving like `fixed` at the occurrence time;
  overlapping occurrence skipped if previous run still active.

### Priority & concurrency
1. Sort eligible tasks by list priority (top first, FA-11).
2. If a non-parallel task is chosen, it runs alone (default one-at-a-time, FA-12).
3. `parallel = yes` tasks may run concurrently with other `parallel = yes` tasks, unbounded,
   still honoring priority order and the limit gate (FA-13/14/33).

### Limit handling (D1, revised per V2 in §10)
Runs use `--output-format stream-json` so claudeq can observe `api_retry` events
(`error_status: 429`, error category `rate_limit`) as they occur. An absolute reset time is
not exposed, so the block duration comes from the event's `retry_delay_ms`.

```
start task (stream-json)
  └─ run claude CLI
       ├─ finishes → record success/failure, notify on failure
       ├─ auth_error (error "authentication_failed") → mark auth_error, notify, no retry
       └─ rate limit observed (api_retry, 429, "rate_limit")
             ├─ read retry_delay_ms → derive blocked-until
             ├─ set global blocked-until, mark run "rate_limited_waiting"
             ├─ end the process, plan wake at blocked-until (§ Wake)
             └─ at wake: gate reopens → resume via `claude --resume <session-id>`
                          (fallback: restart task) — D4
```
Alternative considered: let the CLI's own retry watchdog wait in-process
(`CLAUDE_CODE_MAX_RETRIES`, `CLAUDE_CODE_RETRY_WATCHDOG=1`). Simpler, but the process stays
alive and the machine cannot sleep during the wait → rejected as default; the detach-and-wake
flow above is preferred for the nightly power profile.

### Wake (D8)
- Next concrete wake = min(next fixed_at, next cron occurrence, blocked-until) if any.
- Plus a re-armed hourly heartbeat (individual `pmset schedule wake` events, **not**
  `pmset repeat`, to avoid the single-repeat-slot clash — see V4 in §10).
- Registered with `pmset schedule wake` (**requires root** → passwordless `sudoers.d` entry for
  `/usr/bin/pmset`, installed by the postinstall, D9); re-planned after every scheduling pass.

---

## 8. Claude CLI Invocation

- Executed exclusively via **Claude Code CLI**, headless/non-interactive, in `working_dir`
  (FA-27, catalog §2). Verified against CLI **v2.1.212**.
- **Invocation shape** (verified):
  `claude -p "<prompt>" --output-format stream-json --model <model> [permission flags] --session-id <uuid>`
- **Session id**: claudeq **assigns** the UUID via `--session-id <uuid>` up front (no need to
  parse it out), and reuses it for `--resume <uuid>` after a limit wait (D4).
- **Model** (FA-28/30): `--model <name>`; per-task `model` overrides `default_model`.
- **Permissions** (FA-31): the task's own "skip" → `--dangerously-skip-permissions` (≡
  `--permission-mode bypassPermissions`). A safer non-default option exists for later:
  `--permission-mode dontAsk` + `--allowedTools "…"`.
- **Auth detection** (FA-38): auth failures exit non-zero with category
  `authentication_failed` (message e.g. `Login expired · Please run /login`); mark the run
  `auth_error` and notify; do not silently retry.
- **Output**: `stream-json` gives per-event visibility (needed for rate-limit detection, §7);
  the final `result` event carries `session_id`, `is_error`, `api_error_status`, `usage`,
  `total_cost_usd` (envelope verified empirically).
- claudeq performs **no Git operations** (D6); any branch/commit behavior must be instructed
  through the repo's system prompt and the task's user prompt.

---

## 9. Requirements Traceability (selected)

| Requirement | Covered by |
|-------------|-----------|
| FA-01/02/03 | §5.8 App CRUD, §6 Task model |
| FA-04/07/08/18 | §7 Trigger evaluation (D7) |
| FA-05/06/37 | §5.4 Limit Gate (reactive, D1) |
| FA-09/32 | §5.5 Wake Planner (D8), launchd daemon |
| FA-10 | §5.3 no runtime cap |
| FA-11/12/13/14/33 | §7 Priority & concurrency |
| FA-15/25/36 | §5.6 Store (logs, history) |
| FA-16/17 | §5.8 run-now, activate/pause |
| FA-19/34 | Prompt convention (D6) — not tool-enforced |
| FA-22/23/24/26 | §5.8 News dashboard, `state.json` unread |
| FA-27/28/29/30/31 | §8 CLI invocation |
| FA-35/38 | §5.4/§5.7 failure & auth notifications |
| FA-39/40/41 | §5.7 Notifier (helper + Pushover) |
| NFA-01/02 | Go daemon, minimal deps, native macOS |
| NFA-03 | launchd LaunchAgent, RunAtLoad + KeepAlive |
| NFA-04 | local-only 127.0.0.1, no data leaves machine |
| NFA-07 | TOML config, JSONL history (human-readable) |

---

## 10. Verification Findings

Verified 2026-07-17 against Claude Code CLI **v2.1.212** (empirical runs on this machine +
official docs). Verdicts: ✅ confirmed · ⚠️ confirmed with constraint · ◐ partially confirmed.

### V1 — Headless resume (D4) · ✅ confirmed
- `--output-format json`/`stream-json` returns `session_id`; **or** claudeq assigns it via
  `--session-id <uuid>` (chosen — no parsing needed).
- `--resume <id>` and `--continue` both work in `-p` mode; resume restores conversation
  history, model, and permission mode. Resume/continue must run from the **same working
  directory**; `--mcp-config`, `--settings`, `--add-dir` are **not** restored (must be
  re-passed).
- After a mid-task interruption the session resumes from the interruption point. Resume must be
  re-invoked by claudeq after the process exits (it does not auto-continue). Fallback = restart
  the task (D4) stays as the safety net.

### V2 — Rate-limit signaling (D1) · ◐ partial → design adjusted
- Detection **works**: `stream-json` emits `api_retry` events with `error_status: 429` and
  error category `rate_limit`; the json envelope exposes `is_error` + `api_error_status`.
- **An absolute reset timestamp is NOT reliably exposed** (no `Retry-After` surfaced). Only a
  per-attempt `retry_delay_ms` is available. → **D1 adjusted**: derive the block window from
  `retry_delay_ms` rather than an absolute reset time.
- Exact non-zero exit code on exhausted retries is not documented.
- Built-in retry knobs exist (`CLAUDE_CODE_MAX_RETRIES`, `CLAUDE_CODE_RETRY_WATCHDOG=1`) —
  considered and rejected as default (keeps machine awake); see §7.
- **Residual item for the spike:** confirm the exact `api_retry`/envelope field shape against a
  *real* 429 (the above is doc-derived), and pin down the exit code.

### V3 — Native notifications (D8/FA-39) · ✅ confirmed
- `osascript -e 'display notification …'` works (returns 0; test notification fired).
  `terminal-notifier` is **not** installed → rely on built-in `osascript`.
- Works because the daemon is a **LaunchAgent** in the user's Aqua session (D3); a system
  LaunchDaemon could not post. Requires Notification Center permission; notifications appear
  under the invoking app (e.g. "Script Editor") unless claudeq ships a signed app bundle.

### V4 — `pmset schedule wake` (D8/FA-32) · ⚠️ confirmed, needs root
- Scheduling works and multiple one-shot events coexist. **`pmset` must run as root** →
  passwordless sudoers entry for `/usr/bin/pmset` or a small privileged helper (setup step).
- `pmset repeat wake` has only **one** system-wide slot → use re-armed individual
  `pmset schedule wake` events for the heartbeat instead (§7 Wake).
- Wakes from sleep only (not shutdown/hibernate); the machine must actually reach sleep.

### V5 — Auth-error detection (FA-38) · ✅ confirmed
- Auth failures exit non-zero with category `authentication_failed`
  (messages: `Login expired · Please run /login`, `Not logged in`, invalid-key →
  `authentication_failed`). Cleanly distinguishable from `rate_limit` and from task failures
  (which carry a normal `result`). → mark run `auth_error`, notify, no retry.

**Net:** all five resolved. Only two residual spike items remain, both under V2: confirm the
real-429 field shape and the exit code. Everything else is ready to build.

---

## 11. Distribution, Installation & Release Pipeline (D9)

Goal: a colleague on any Mac downloads one installer from GitHub Releases, runs it, and
everything (app + background service + wake privileges) is set up automatically. Target
audience is Mac-using developers who already run Claude Code.

### 11.1 Signing posture (D9)
- **v1: unsigned.** No Apple Developer ID yet. The `.pkg` and app are not notarized.
- **Consequence (Gatekeeper):** first run needs a one-time bypass — either right-click → **Open**,
  "Open Anyway" in *System Settings → Privacy & Security*, or install via Terminal
  (`sudo installer -pkg claudeq-<ver>.pkg -target /`, which is not GUI-Gatekeeper-blocked).
- **Upgrade path:** the pipeline is written notarization-ready. Adding a Developer ID later =
  provide certs + App Store Connect key as secrets and enable the (already-stubbed) sign +
  `notarytool` + `staple` steps. No structural rework; the Gatekeeper friction then disappears.

### 11.2 Installer (`.pkg`) contents & postinstall
Built with `pkgbuild`/`productbuild`. A **preinstall** closes the open ClaudeQ window (only
the dashboard app; the daemon keeps running). The **root postinstall** script performs the
auto-setup and, at the end, reopens the freshly installed app in the user's GUI session:
- Install the Wails **app** to `/Applications`.
- Install the **LaunchAgent** plist (user daemon) and bootstrap it (`launchctl bootstrap`).
- Write a **`/etc/sudoers.d/claudeq`** entry granting the user passwordless `/usr/bin/pmset`
  (NOPASSWD, restricted to `pmset`) — enables wake scheduling without a signed helper (D9/V4).
- Provide a matching **uninstall** script (remove app, LaunchAgent, sudoers entry, data opt-in).

**Standard dialogs the user sees (by design):**
| When | Dialog |
|------|--------|
| Install | macOS Installer + **admin password** (postinstall runs as root) |
| First app launch | **Allow notifications?** (TCC) for native notifications (FA-39) |
| Daemon startup, and right after a task is added/edited with a folder in a new protected location | Native **"allow access to your Documents?"** (TCC Files & Folders) — automatic and tickable; see §13 |

### 11.3 GitHub Actions release pipeline
Triggered on tag `v*`:
1. **Build** on `macos-14` runner: compile the Go daemon + helper as a **universal binary**
   (arm64 + amd64 via `lipo`) → supports Apple Silicon *and* Intel colleagues; build the Wails
   `.app`.
2. **(Stub, disabled without Developer ID)** sign app + `.pkg` with Developer ID.
3. **Package**: `pkgbuild`/`productbuild` → `claudeq-<ver>.pkg` (+ optional `.dmg`).
4. **(Stub)** notarize via `notarytool` + `staple`.
5. **Release**: create the GitHub Release and upload the `.pkg` (and `.dmg`) as assets.

### 11.4 Per-colleague prerequisites (documented, not installed by us)
- Claude Code installed, in `PATH`, and **logged in** (catalog §2/§8 assumption).
- One-time Gatekeeper bypass (11.1) until notarization is enabled.

---

## 12. Build Phases & Status

Status legend: ✅ done · 🔄 in progress · ⏳ not started · ⏸ blocked.
This table is kept current — the phase status is updated as work progresses (see AGENTS.md §6).

| # | Phase | Status | Notes |
|---|-------|:------:|-------|
| 0 | **Bootstrap** — toolchain, repo scaffold, CI | ✅ done | Go module, `cmd/claudeqd` skeleton + tested `internal/version`, golangci-lint v2 config, Makefile, `.gitignore`, GitHub Actions CI (build/fmt/vet/lint/test-race on macOS). Dev tools installed: golangci-lint, goimports. |
| 1 | **Spike** — verify CLI behaviour & platform mechanisms | ✅ done | V1–V5 resolved (§10). Two residual V2 items (real-429 field shape + exit code) can only be confirmed against an actual 429 → carried into Phase 2. |
| 2 | **Core daemon** — store, scheduler, executor, reactive limit gate (headless, CLI-driven) | ✅ done | Packages: task, store (TOML/JSONL/state), clock, limit, schedule, executor, engine; plus `claudeqd run` loop and the `claudeq` control CLI (add/list/rm/enable/move/run-now/status/read/settings). Unit + integration tests (fake `claude`, injected clock), all gates green, verified end-to-end. Residual V2 (real-429 field shape + exit code) is handled defensively but still doc-derived — to confirm against an actual 429. See D10–D11. **Follow-up (visible pause, cancellable resume):** a limit pause was correct but looked like a hang — the run sat at `rate_limited_waiting` with no time and no way out. A paused run now records its planned resume (`resume_at`, taken from the gate so another task's longer block is reflected), the API marks the run whose session is actually still queued (`resume_pending`, false once the task runs again) and the task it belongs to (`waiting_for_limit`), and `/api/health` reports `limited_until`. The dashboard reads that as a banner naming the reopen time, a *rescheduled* label plus resume time in Activity, and a *rescheduled* badge in the Queue. `Engine.CancelRun` covers the second way to call a run off: for a run that is only waiting, it drops the pending session, records the run as `canceled`, and retires a one-shot task out of the queue (recurring tasks keep their schedule) — so a job the user no longer wants does not start again when the gate reopens. |
| 3 | **Wake & resilience** — launchd integration, pmset wake (sudoers), heartbeat | ✅ done | Packages: system (command runner), wake (next-wake computation + pmset scheduler with reschedule tolerance), launchd (plist + install/uninstall). `claudeqd install`/`uninstall` manage the LaunchAgent; `run` plans wakes each tick (best-effort, needs sudoers). All gates green; acceptance covers install/wake/uninstall with fakes. **Manual check remaining:** real wake-from-sleep on the user's Mac + the one-time sudoers entry (system-modifying, run by the user). |
| 4 | **Notifications** — native macOS + Pushover | ✅ done | notify package (Pushover via HTTP, fan-out, all tested); engine notifies on failure/auth (not on success/rate-limit); daemon builds the notifier from settings. Acceptance covers a failure→notification path. **Native delivery:** when running from the app bundle, notifications post through `UNUserNotificationCenter` so they carry the ClaudeQ icon; `osascript` remains the fallback. This requires the bundle to have a stable code identity, so `build-app.sh` ad-hoc-signs it under `de.maierdaniel.claudeq` (a generic linker `a.out` identity wasn't enough to register for notifications). |
| 5 | **App (GUI)** — task management, news dashboard, history, logs, settings | ✅ done | macOS-styled web dashboard (embedded, served by `claudeqd run` on 127.0.0.1). Task CRUD/reorder/edit/run-now, queue hides running tasks, Activity with live log (chat view + prompt), replay, unread/history, Usage stats (tokens/runs/cost per day), settings + Pushover, native folder dialog (osascript), notifications, GitHub/About. Follows macOS light/dark + accent (CSS). Activity has a from–to **date-range filter** and **pagination** (25/page); the run time shows an exact start + duration on hover (custom tooltip, since WKWebView ignores native `title`); running cron tasks stay in the queue with a running badge. Verified end-to-end in-browser + API/engine tests. **Follow-up (cron checked while typing):** a wrong cron expression was only caught when the task was saved, and then only as the parser's raw complaint. `task.CheckCron` now owns the verdict for every entry point (app, CLI, config write) and names the offending field plus what it accepts ("the hour field \"99\" is not valid (hour accepts 0-23)"), rejects `@daily`-style shorthands and a wrong field count explicitly, and `task.CronNext` computes upcoming occurrences. `GET /api/cron/check?expr=` answers `{valid, error, next}` — always 200, since a half-typed expression is not a failed request — and the task sheet calls it debounced while typing: the field turns red with the reason, or shows the next three runs, and saving a rejected schedule is refused on the field instead of at the footer. |
| 5b | **Native window** — wrap the dashboard in a WKWebView app window | ✅ done | `cmd/claudeqapp` (darwin): thin native window via `webview_go` (same WKWebView engine; lighter than full Wails, same D2 intent). Loads the daemon dashboard, starts `claudeqd run` if the port is down, and injects the real macOS accent via a native bridge. Shipped as a real `.app` **bundle** (`scripts/build-app.sh` → `build/ClaudeQ.app`: `Info.plist` with `CFBundleName`/`CFBundleDisplayName`=**ClaudeQ** — the user-facing display name, while paths/binaries/bundle-id stay `claudeq`/`de.maierdaniel.claudeq` — `.icns` from `logo.svg`, `claudeqd` bundled alongside, ad-hoc code-signed) so macOS treats it as its own foreground app (menu-bar name, Dock icon, no terminal parent) — `webview_go` self-activates only when bundled. Native menu bar built via cgo (`menu_cocoa.m`, kept in a `.m` file so the Obj-C class compiles once): App (About = standard panel with icon+name+version, Settings ⌘,, Hide, Quit ⌘Q), File (New Task ⌘N, Close ⌘W), Edit (Cut/Copy/Paste/Select-All so shortcuts work in inputs), Window — custom items drive the dashboard via `openAdd()` / `select('settings')`. Live accent: read the `AppleAccentColor` index → hex and re-apply at 0/0.2/0.5/1/1.8/2.8s (defeats the 1-3s cfprefsd flush lag), triggered by **both** the distributed `AppleInterfaceThemeChangedNotification` (dark/light) **and** the local `NSSystemColorsDidChangeNotification` (accent — a plain accent change fires no distributed notification, the key finding). Dashboard assets served `no-cache` so a rebuilt daemon's logo/CSS never shows stale in WKWebView. **User-verified**: native window, menu, correct logo, and live accent switching all working. |
| 6 | **Packaging & release** — unsigned `.pkg` + postinstall, GitHub Actions release pipeline (notarization-ready stubs), install/uninstall (NFA-05/06) | ✅ done | `scripts/build-pkg.sh` → `dist/claudeq-<version>.pkg` (`pkgbuild`, `ditto`-staged, installs `ClaudeQ.app` to `/Applications`). `scripts/pkg/postinstall` sets up the per-user LaunchAgent by running the bundled daemon's own `install` inside the console user's GUI domain (`launchctl asuser <uid> sudo -u <user>`), so autostart is configured without the app on first launch. Unsigned by default with **notarization-ready** hooks gated on `CLAUDEQ_SIGN_APP_ID`/`CLAUDEQ_SIGN_PKG_ID`/`CLAUDEQ_NOTARY_PROFILE`. `scripts/uninstall.sh` removes the agent + app (NFA-06). `.github/workflows/release.yml` builds the `.pkg` on a `v*` tag and attaches it to the GitHub Release. `README.md` documents install/uninstall/build. Acceptance grew 8 static packaging checks (41/41). Pkg **structure-verified** (payload → `/Applications/ClaudeQ.app`, identifier `de.maierdaniel.claudeq`, postinstall present); a real GUI install is user-verified. |
| 7 | **Hardening** — unattended-run safety | ✅ done | Four unattended-safety measures. **Panic recovery** per run (`runGuarded`) turns a panic into a failed result instead of crashing the daemon. **Startup reconcile** (`ReconcileRunningRuns`) marks runs left `running` after a crash/power-loss as interrupted, so they don't show as forever-running. **Idle watchdog** (`IdleTimeout`, default 30 min, 0/neg = off) kills a run that produces no output for too long — a hung/deadlocked process — while a working run keeps streaming and is unaffected; the kill targets the whole **process group** (`Setpgid` + group SIGKILL) so grandchildren die too. **History retention** (`MaxRunHistory`, default 500) prunes old runs + their log files (`PruneHistory`) at startup and after each run, bounding disk. Idle timeout and retention are configurable in Settings → Reliability. **Sleep handling:** the daemon holds a `caffeinate -i` power assertion (ref-counted) while any run is in flight so the Mac doesn't idle-sleep and freeze a task mid-run; and the idle watchdog is sleep-aware — a tick gap far larger than its interval (system slept) rebases the activity clock instead of counting it, so a run frozen across a long sleep isn't falsely killed on wake. Tests cover the watchdog kill, sleep-rebase, reconcile, prune, the sleep-guard refcount, and the setting defaults. |
| 8 | **Self-queue** — a running task can enqueue follow-up tasks | ✅ done | A run can schedule new claudeq tasks instead of doing everything inline (e.g. a nightly check that finds work and files it as its own optimization task). The executor appends a `--append-system-prompt` block telling Claude the capability exists and how to use it, and injects `CLAUDEQ_HOME`, `CLAUDEQ_BIN` (absolute path to the CLI, shipped in the bundle next to `claudeqd`), and `CLAUDEQ_PARENT_TASK` (the calling task as JSON) into each run. The new `claudeq queue` subcommand builds a task from the parent as a template — inheriting model, permissions, parallel and notify (and anything added later, since it copies the whole task) — and only overrides prompt, timing (`--at`/`--in`/`--cron`, default asap) and `--dir`; since the per-call overrides landed, `--model`, `--parallel=BOOL`, `--skip-permissions=BOOL`, `--notify=BOOL` and `--quiet-history=BOOL` replace the inherited value only when passed (one `taskSettings` flag group shared with `add` and `edit`), so a cheap quiet watcher can queue a visible Opus review. Store `UpdateConfig`/`UpdateState` gained a cross-process `flock` (`internal/store/lock.go`) so the daemon and the queueing child never lose each other's writes. Unit tests (queue building/timing/inheritance/errors, env injection, cross-process no-loss) + end-to-end verified with a fake `claude` that self-queues. All gates green. |
| 9 | **In-app updates** — check GitHub for newer releases, download & install | ✅ done | New `internal/update` package: a `Service` polls GitHub's `releases/latest` **hourly** (and once at startup), caches the result in memory (so the dashboard reads it without per-poll network calls), compares against the running build's version (semver-ish `Compare`), and downloads the release's `.pkg` asset into `~/Downloads` then `open`s it (macOS Installer). API: `GET /api/update` (status), `POST /api/update/check` (force), `POST /api/update/dismiss` (ignore this version until a newer one), `POST /api/update/download`. The check reads the **releases list** (not just `releases/latest`) so the banner shows the **aggregated notes of every version the user skipped** (newest first, one heading per version), not only the newest. The dashboard shows a **yellow banner** at the top of Settings with those notes + **Download & install** / **Dismiss** / **All releases on GitHub** (links to the full release history), a red **"1" badge** next to the Settings nav entry, and a **"Check for updates"** button + version chip in the About group. A dismissed version is stored in `state.json` (`dismissed_update_version`) and superseded by any newer release. Update comparison needs the binary to know its own version, so `build-app.sh` now stamps `internal/version.Version` via `-ldflags` (previously the shipped app reported `dev`); a `dev` build reports `supported:false` and never prompts. `build-app.sh`/`build-pkg.sh` gained a `CLAUDEQ_VERSION` override for demo/pre-release builds. Unit tests: version compare table, GitHub JSON parsing + `.pkg` selection (httptest), service caching/error-retention/download (fake fetcher + stub opener), and the four API endpoints incl. dismiss-persist and dev-build suppression. Verified end-to-end in-browser (banner, badge, dismiss all working against the live v0.1.4 release). |
| 10 | **Artifacts** — jobs publish files to a central, read-tracked view with an in-app viewer | ✅ done | A run can publish a deliverable file (report, export, HTML page, PDF, …) that then appears in a new **Artifacts** sidebar view. Mirrors the self-queue mechanism: the executor appends an `--append-system-prompt` block documenting the capability and injects `CLAUDEQ_RUN_ID`/`CLAUDEQ_TASK_ID` (alongside the existing `CLAUDEQ_HOME`/`CLAUDEQ_BIN`/`CLAUDEQ_PARENT_TASK`) so a published artifact is attributed to its run/task. New `claudeq publish --file PATH [--title T] [--description D]` subcommand **copies the file into the store** (`artifacts/<id>/<name>`, a permanent snapshot that survives the source changing/being deleted) and records metadata in `artifacts.json` (atomic write, same cross-process `flock` as config/state via new `UpdateArtifacts`). Read-status lives in `state.json` (`read_artifacts`, mirroring run read-state). API: `GET /api/artifacts` (newest-first, with `unread`), `POST /api/artifacts/{id}/read`, `POST /api/artifacts/read-all`, `DELETE /api/artifacts/{id}` (removes record, stored file, and read-status), `GET /api/artifacts/{id}/content` (serves the file for the viewer/download; the `id` is always resolved against the stored list, never used to build a path, so there is no traversal). The dashboard adds an **Artifacts** nav entry with an unread badge, cards (title/description/source task/file name/size/time, unread dot, mark-read eye, delete), and an **in-app viewer**: HTML in a `sandbox="allow-scripts"` iframe (opaque origin — cannot reach the loopback API) served with a restrictive CSP (`default-src 'none'`; inline CSS/JS + `data:` images only, no network) and `X-Content-Type-Options: nosniff`; PDF in a native-viewer iframe; images inline; text via a fetched `<pre>`; plus an **Open externally** fallback for any type. Opening an artifact — in the in-app viewer (**View**) or externally (**Open**) — marks it read automatically, so the unread badge only counts artifacts nobody has looked at; the eye button and **Mark all read** remain for marking without opening. Artifacts are kept until manually deleted (independent of run-history pruning). Unit + API tests (store round-trip/paths/read-state, publish copies-not-references + duplicate-id + delete, all endpoints incl. safe headers and not-found); verified end-to-end in-browser (publish → list → HTML/text/image viewers render, HTML inline JS runs while network is blocked, read-state + delete). PDF renders in the app's WKWebView (the Chromium test pane blocks its PDF plugin); the Open-externally fallback covers every engine. |
| 11 | **Task notifications & quiet history** — a run sends its own notification; frequent jobs stay out of history | ✅ done | `claudeq notify --title T --message M [--url U]` lets a run (a watcher job, typically) alert the operator without publishing a throwaway artifact. Mirrors the artifact mechanism: the CLI drops the message into an outbox (`notifications.json`, atomic write under the shared cross-process `flock`; the file exists only while something is pending, so the daemon's per-tick check is an ENOENT), attributed via `CLAUDEQ_TASK_ID`/`CLAUDEQ_PARENT_TASK`/`CLAUDEQ_RUN_ID`; the daemon takes the whole outbox atomically on its next tick and sends off the scheduler goroutine over its existing channels — the task name is appended to the body, title/body are cut to Pushover's 250/1024 limits, entries older than 24 h are dropped with a log line, a corrupt outbox is set aside as `.corrupt` rather than wedging the feature, and every failed delivery (this and the existing outcome/artifact notifications) is now logged to stderr. `--url` (absolute http/https, validated in the CLI, re-checked at delivery and again in the click handler) travels as Pushover's supplementary `url` and as `cq_url` in the macOS notification's userInfo, where the window app opens it with `NSWorkspace` on click instead of raising the window. A third built-in system-prompt block documents the command and tells Claude to use it only when the finding warrants it. Store persistence for artifacts and the outbox share one generic `jsonList[T]` helper. **Quiet history** (`quiet_history` on the task; `--quiet-history` on add/edit/queue, a switch in the task form; deliberately *not* inherited by `claudeq queue` unless passed explicitly): a quiet task's run is never appended to history at launch, and on `success` or `rate_limited_waiting` its log is deleted and nothing is recorded — zero history churn for a 15-minute watcher; `failed`/`auth_error`/`canceled` runs are appended as a single terminal record and count as unread like any other. Trade-offs, documented in the README: no Activity row (so no live log / Cancel) while a quiet run is in flight, and successful quiet runs are absent from Usage. Unit tests: outbox queue/take/empty-file-removed/corrupt-quarantine, engine delivery-once + attribution + stale-drop + limits + link recheck + no-notifier + recovery, quiet success/rate-limit dropped vs failure/auth kept vs not recorded while running, Pushover `url` form field, `IsWebURL`, CLI validation (required flags, URL scheme, percent-encoding), patch/TOML round-trips, queue non-inheritance. |
| 12 | **Task sharing** — export/import a task as a `.claudeq` file (app + CLI) | ✅ done | A task travels as one `.claudeq` file: a zip with `task.json` (an envelope `format`/`format_version`/`exported_at` around every task field except the prompt, so new fields round-trip automatically) and `prompt.md` (the prompt verbatim). New `internal/bundle` package (stdlib `archive/zip`, no new deps) writes/reads it and rejects anything that is not a bundle (bad zip, missing entry, wrong format/version, empty prompt) with `ErrInvalid`. `app.ExportTask`/`app.ImportTask` sit on the store: import takes settings **as-is** (the maintainer's call — adjust afterwards with the normal edit) and only fills what the file cannot decide: a taken id gets `-2`, `-3`, … so an existing task is never clobbered, a missing id comes from the name (`task.Slug`, shared with the API's id generator), missing permissions mean default; an id from a file must be URL-safe (`task.CheckID`, also applied to `claudeq add --id` and a client-supplied id on `POST /api/tasks`), and any scheduling state left under the final id by an earlier task is dropped (`State.ForgetTask`; `app.RemoveTask` now cleans up the same way, so a deleted one-shot task's `completed_once` flag can no longer silently disable a later task with the same id). CLI: `claudeq export ID [--out PATH] [--force]` (refuses to overwrite without `--force`; `--out` may be a directory or a file, extension appended) and `claudeq import PATH [--id ID]`. API: `POST /api/tasks/{id}/export` opens the native **Save as** panel via `osascript` (`choose file name` with the prompt and default name passed as run arguments, so no AppleScript escaping; new optional `SaveFile` dep sharing the folder chooser's runner/cancel detection; 204 on cancel, 404 unknown task, 503 headless) and writes the file — the panel handles overwrite confirmation, and a name typed without the extension gets it appended without silently replacing a file the panel never showed; `POST /api/tasks/import` takes the zip as the request body (the dashboard uses a hidden `<input type=file>`, which WKWebView backs with a native open panel, so reading the file never depends on the daemon's own folder access), size-capped at 4 MiB (413 over the cap, and entries are rejected from their zip header before inflating), 201 with the stored task; entries may sit inside one folder level, so a bundle unpacked and re-zipped with Finder's Compress still imports. Dashboard: an export button on every task row and **Import…** on the Queue toolbar. **Fix (follow-up PR):** Import… did nothing in the native window — webview_go hands WKWebView its file-chooser `WKUIDelegate` autoreleased and never retains it, and WKWebView holds the delegate weakly, so it was gone before the first click. `cmd/claudeqapp/openpanel_cocoa.m` installs a delegate the process keeps alive, answering `<input type=file>` with an `NSOpenPanel` sheet on the window. Unit tests: bundle round-trip/layout/rejections, app import id-suffixing and gap-filling, API endpoints (dialog chosen/cancel/error/unavailable, oversized and invalid uploads, extension handling), osascript expression + AppleScript string escaping, CLI round-trip/overwrite/argument forms. README documents the format, both flows, and the as-is trade-off. **Follow-up (import review sheet):** a shared file's paths belong to the machine it came from, so the app no longer queues an import blindly. `POST /api/tasks/import` became a read step (200 `{task, missing_working_dir}`, nothing stored, no id suffixing — creating the task is the normal `POST /api/tasks` the sheet already does), `app.ReadImport` validates the file exactly as `ImportTask` does and drops a `working_dir` that is not a directory here (`app.DirExists`: a stat that fails with "permission denied" counts as existing, so a folder ClaudeQ may not read yet is kept). The dashboard prefills the task sheet titled *Import task* and names the dropped path under the folder field. `claudeq import` still adds the task as exported and now warns on stderr when its working directory is not on this machine. |
| 13 | **Global pause** — one switch that stops every run | ✅ done | A `paused` global setting (config.toml, Settings → Execution) that gates the whole run loop: `Engine.Tick` returns before evaluating triggers, `RunTaskNow` refuses with the shared `store.ErrPaused` sentinel (so CLI `run-now`, the app button and `POST /api/tasks/{id}/run-now` — 409 — all say the same thing), and `wakeCandidates` drops the task wakes so `pmset` is not armed for work that cannot run. In-flight runs are left alone, and since a paused tick records no scheduling state, a task that came due while paused runs as soon as the switch goes off. The switch has its own endpoint (`POST /api/pause`) instead of riding the settings payload, so the dashboard can flip it immediately — like a task's enable switch — without carrying (and overwriting) every other setting; the Queue view shows a yellow banner with a **Resume runs** button while it is on and disables **Run now** on every row, and `claudeqd` logs the state at startup so a night with nothing started is explainable. CLI: `claudeq settings --paused=BOOL`, and `claudeq settings` prints the state first. Tests: paused tick starts/records nothing, resuming runs the task that came due meanwhile, `run-now` refused, wake planning falls back to the heartbeat, `POST /api/pause` persists without clobbering other settings, run-now 409 while paused, `app.SetPaused` touches only that field, CLI patch round-trip and label. |
| 14 | **Feedback → GitHub issue** — a guided chat in the app turns a report into an issue the user files | ✅ done | A **Feedback** entry at the bottom of the sidebar opens a chat sheet. New `internal/feedback` package: `Service.Turn` runs the Claude Code CLI in print mode (`-p --output-format json --json-schema …`) on **Haiku**, with the session locked down — `--tools ""`, `--strict-mcp-config`, `--disable-slash-commands`, `--safe-mode`, and an empty `os.MkdirTemp` directory as cwd — so a feedback chat can neither touch the machine nor pull `CLAUDE.md`/project context into a public issue. The structured answer is either `ask` (one clarifying question, in the user's language) or `ready` (English title/body plus exactly one label, `bug` or `enhancement`, validated server-side so the model cannot invent one GitHub would drop). The conversation resumes via `--session-id`/`--resume` and is capped at three user turns (`MaxUserTurns`); the final turn carries the "no more questions" instruction **in the user message**, because the CLI records the system prompt on a conversation's first request and replays it verbatim on resume. Sessions live in memory with a 1 h TTL. **Auth was the open question and the answer is: none.** Nothing is posted through the API — `feedback.IssueURL` builds GitHub's prefilled `issues/new?title=&body=&labels=` page and the dashboard opens it in the browser (`cqOpenExternal`), where the user presses *Create* in their own signed-in session; ClaudeQ stores no token and files nothing itself. GitHub answers a request URI beyond ~8 KB with **414** (measured: 6000 chars of body ok, 12000 → 414), so the body is trimmed to a 6000-byte URL budget with a "shortened" marker rather than lost. API: `GET /api/feedback` (availability + repo + the two environment values), `POST /api/feedback/turn`, `POST /api/feedback/url`. The review step shows the draft in editable fields; **ClaudeQ version** and **macOS version** (`sw_vers -productVersion`, cached) are named there and appended by the daemon as a footer line, which the user can still delete on GitHub's own form (they were editable inputs in the sheet at first — dropped as redundant, since the prefilled page is editable anyway). When the assistant cannot be reached (no binary, limit exhausted, CLI failure) the sheet keeps what the user typed and offers **Write it myself**. Unit tests: argv construction (new vs resume, lockdown flags), CLI parsing incl. the `result`-only fallback and error results, label filtering, last-turn coercion, URL building and the 414 trim; API tests for all three endpoints. Verified end-to-end against the real CLI through a scratch-home daemon (question → answer → draft, 6–8 s and ~$0.004 per turn) and in-browser through the whole flow, including GitHub rendering the prefilled form with title, body and the `bug` label. |
| 15 | **Prompt review** — Claude checks a draft prompt against this machine before the task is queued | ✅ done | A prompt written elsewhere (or written in a hurry) is checked against *this* Mac while the task sheet is open — new task, edit, replay and import all go through `openSheet`, and Settings' custom system prompt gets the same treatment. New `internal/review` package. claudeq does the filesystem half itself and deterministically: `ExtractPaths` pulls path-shaped tokens out of the prompt (absolute, `~/…`, `./…`, anything with a slash, plus bare filenames with a known extension; URLs, `$VAR`, `user@host` and `host:path` are rejected, an editor's `:42:7` suffix is cut, markdown/quote/backtick wrapping falls out of the tokenizer), `Inspect` resolves each against the working directory and stats it — exists / is a dir / parent exists, with `ErrPermission` counted as "there, but macOS won't let us look" so a TCC-withheld folder never reads as missing — and reads the content of existing text files small enough to inline (caps: 40 paths, 6 files, 16 KiB each, 48 KiB total; oversized, binary and empty files are reported by size alone). Only the judgement half is a model call, and it is the narrowest invocation claudeq makes: `-p --output-format json --safe-mode --no-session-persistence --tools ""` — no tools at all (the facts are already in the message), no CLAUDE.md/skills/plugins/hooks/MCP, no resumable session, run in the user's home rather than the task's folder (which may not exist yet) — with a `--system-prompt` that replaces Claude Code's own and states the whole contract: report only what would actually break the run (a missing input, a report written into a missing directory, guidelines worth inlining, a prompt that waits for an answer nobody will give, a relative path with no working directory), stay silent otherwise, keep the operator's wording, answer as one JSON object. Untrusted material (the prompt, every quoted file) is fenced with a delimiter that is extended until it cannot appear inside the body, and framed as data. API: `POST /api/review/prompt` (`{kind, prompt, working_dir}` → `{enabled, ok, message, revised_prompt}`); a new review cancels the one still running, which kills its `claude` process, and the superseded request answers 204. The review being off, absent or without a claude binary all answer `enabled:false` rather than an error the operator cannot act on from the sheet. Dashboard: a purple, sparkle-marked banner under the prompt box ("ClaudeQ suggests:" + finding + **Apply** / **Dismiss**), re-run from scratch on every prompt or working-directory change (900 ms debounce, `AbortController` on the request in flight); **Apply** drops the rewrite into the box and reviews the result again. Three things keep it from spending usage for nothing: an answer is cached for 3 min under the exact `(kind, prompt, dir)` asked, so reopening a sheet or stepping back into Settings renders instantly; a task sheet with no working directory yet (an import, whose folder came from another Mac) waits for the folder rather than calling every relative path unresolvable on top of the sheet's own hint; and leaving Settings retires that view's review instead of letting the daemon run it to its timeout. The task sheet grew to 720 px with a 190 px prompt box in the same change. Settings: **Check prompts with Claude** (stored as `prompt_review_disabled`, so an existing `config.toml` gains the feature switched *on*) and **Review model** (empty = the global default model), both also on `claudeq settings --prompt-review[-model]`. Tests: path extraction table + inspection (missing/parent/permission/`~`/caps/binary), CLI arg construction, envelope and fenced-JSON parsing, no-op-revision and empty-message normalisation, timeout, and the endpoint (kind mapping, model precedence, disabled, no reviewer, no binary, failure, supersede→204). Verified end-to-end against the real CLI in a scratch `CLAUDEQ_HOME`: findings, Apply, the re-review coming back clean, Dismiss, the off switch, and both light and dark rendering. |
| 16 | **ntfy & webhook notifications** — two more channels next to Pushover, each with its own switch | ✅ done | Chosen from a larger brainstorm (execution window, retry policy, git visibility, task chains, budget cap, notification routing, a Telegram back-channel were parked). **ntfy**: publishes to a topic on `ntfy.sh` or a self-hosted server (`Server` empty = the public instance), as a **JSON body** to the server root rather than headers on `POST /<topic>`, so a title with umlauts or emoji needs no header encoding; an optional bearer token covers a protected topic. **Webhook**: posts a JSON body to any URL, templated with `{{title}}`/`{{message}}`/`{{url}}` (values JSON-escaped, so a message with quotes or newlines cannot break the body) — the one channel that covers a service claudeq does not know about (Slack, Discord, Home Assistant, n8n); an unknown placeholder is refused at save time rather than posted verbatim, and there is deliberately no auth-header field in v1 (tokens ride in the URL, as all four example services expect). Both channels share the outbound `post` helper with Pushover (`internal/notify/http.go`) and gained `ValidateNtfy`/`ValidateWebhook`, called from the shared `app.ValidateNotifications` (also covering Pushover) so a channel switched on but unable to deliver is refused by the API (400) and the CLI alike, while a switched-off channel is never checked. `cmd/claudeqd`'s notifier now reads settings **on every send** (`liveNotifier`/`notifyChannels`) instead of once at startup — fixing a pre-existing defect where an edited Pushover token needed a daemon restart to take effect. Settings gained an ntfy and a Webhook group (Notifications tab); `claudeq settings` gained `--ntfy*`/`--webhook*` flags and masked/labelled output. Tests: ntfy/webhook unit tests (endpoint normalization, payload/template rendering, validation, delivery incl. non-2xx and auth header), `app.ValidateNotifications` (skip disabled, reject incomplete enabled, accept complete), an API test that `PUT /api/settings` 400s an enabled-but-unconfigured channel without persisting it, a `notifyChannels` table (Mac always present, enabled+unconfigured skipped, configured+disabled skipped, all four channels present when configured+enabled), and a store TOML round-trip for both new sections. **Hardening from review:** `notify.Multi` fanned its channels out sequentially under `engine.send`'s one shared 15s context, so growing the roster from at most 2 channels to up to 4 meant a hung first channel could starve the ones behind it of their share of the budget — it now runs every channel concurrently (`sync.WaitGroup`), so the call takes about as long as the slowest single channel instead of the sum of all of them. `Ntfy.Configured()`/`Webhook.Configured()` now delegate to `ValidateNtfy`/`ValidateWebhook` instead of restating a weaker rule, so a channel that only ever reached the disk via a hand-edited `config.toml` (never through the API/CLI save path that runs the real validation) is skipped instead of failing — and logging an error — on every single send; the webhook's unknown-placeholder regex was also too narrow to catch a mistyped `{{title2}}` or `{{run-id}}` (digits/hyphens fell outside the character class), so a typo like that posted verbatim indefinitely instead of being refused at save time. `ValidateNtfy` now also rejects a server URL that carries a path — pasting ntfy's own subscriber URL (`https://ntfy.sh/<topic>`) into the server field used to validate cleanly and then publish to the wrong path, which ntfy answers 2xx while showing the raw JSON as the notification body instead of a proper title/click. `liveNotifier.Notify` no longer swallows a `LoadConfig` failure; it logs to stderr before falling back to macOS-only, so a corrupt `config.toml` is diagnosable instead of silently dropping every remote channel with no trace. **Known follow-up:** four channels now hold secrets in clear text in `config.toml`; a keychain migration belongs in a later release. Also noted but not fixed here — `app.ValidateNotifications` runs against the *whole* settings document on every write (API and CLI alike), so a channel that was already enabled-but-incomplete before this PR (persistable with the old, unvalidated save path) blocks every future settings save, including `claudeq settings --paused=true`, until that channel is fixed or switched off; a proper fix means validating only the channel(s) a write actually touches. |

---

## 13. Status

All planned phases (0–10) are complete: the daemon, scheduling/concurrency,
wake & resilience, notifications, the dashboard, the native app, the installer &
release pipeline, unattended-run hardening, self-queue, in-app updates, and the
Artifacts view (jobs publish files to a central, read-tracked view with an
in-app viewer).

Post-phase refinements from real use:

- **Claude binary resolution** — the launchd daemon runs with a minimal `PATH`
  that excludes `~/.local/bin`, so a bare `claude` lookup failed. The daemon now
  auto-detects the binary (`executor.DetectBinary`: `CLAUDEQ_CLAUDE_BIN` →
  common install dirs → login-shell `PATH`) and `Settings.ClaudePath` lets the
  user set it explicitly; the Settings UI pre-fills it via `GET /api/claude/which`.
- **Display name** — everything the user sees reads **ClaudeQ**; the package,
  binaries, paths and bundle id stay `claudeq` / `de.maierdaniel.claudeq`.
- **Scheduled-wake reliability** — the daemon already schedules `pmset` wakes at
  each task's time and (phase 7) `caffeinate`s the Mac awake through a run, so a
  timed task wakes the machine, runs, and lets it idle-sleep again. Because the
  wake needs a one-time `pmset` sudoers entry, a failing wake is now surfaced in
  the dashboard (`GET /api/health` → `wake_error`; a banner shows the exact
  sudoers command) instead of only logging to stderr.
- **File-access (TCC) prompt timing** — an unattended overnight run stalled
  because the daemon first touched the task folder (under `~/Documents`) mid-run
  at 3am, so macOS raised its automatic "allow access to your Documents?" consent
  prompt with no one there to answer it. The prompt itself is the normal,
  tickable Files & Folders one — the only problem was *when* it fired. The daemon
  now reads each enabled task's working directory at startup
  (`warmEnabledTasks` → `internal/fileaccess`, a timeout-bounded probe that never
  hangs on a pending prompt), so the prompt appears at install/login while the
  user is present; once allowed, later runs proceed. Folders are first collapsed
  to their macOS privacy *category* (`fileaccess.ConsentTargets`): macOS grants
  Files & Folders access per category — Desktop, Documents, Downloads — and one
  grant covers the whole subtree, so ten tasks under `~/Documents` provoke the
  Documents prompt **once**, not one blocked read each. External/network volumes
  and ordinary (unprotected) folders map to themselves. Warming then probes
  *every* remaining target in one pass (`fileaccess.ProbeAll`, not the
  first-block `Probe`): a folder whose prompt is still pending reports a timeout,
  and stopping there would leave other categories un-provoked until the next warm. A folder added *after*
  startup (e.g. a new task under `~/Downloads`) is warmed the moment its task is
  created or edited — the API fires the same probe via `Deps.WarmFileAccess` in
  `addTask`/`updateTask`, so the prompt appears right there in the app rather
  than waiting for the run. And because the daemon is a persistent `KeepAlive`
  LaunchAgent that does **not** restart when the app window is reopened, opening
  the window also re-triggers a warm: `claudeqapp` posts to `POST /api/fs/warm`
  on launch and the daemon re-probes every enabled task's folder — covering the
  natural "quit and reopen the app" case that the daemon-start warm alone misses.
  The probe lives in the
  daemon on purpose: the daemon (and the `claude` it spawns) is what reads the
  files, and macOS attributes both to the ClaudeQ bundle, so the grant the prompt
  records is exactly the one the nightly run needs — no separate Full Disk Access
  step required. **Caveat:** because the bundle is ad-hoc signed, its code
  identity changes on every rebuild, so the prompt returns after a reinstall (and
  is simply re-allowed) — a stable Developer ID signature would make the grant
  persist across updates.
- **CLI parity for reading and editing tasks** — the control CLI could create,
  remove, reorder and enable/disable tasks, but never *show* or *change* one, so
  adjusting a prompt outside the window meant hand-editing `config.toml` with no
  validation and a real chance of clashing with the app's own writes.
  `claudeq show ID` now prints every setting plus the complete prompt, and
  `claudeq list --json` / `show --json` emit the same data structurally.
  `claudeq edit ID` changes a task two ways. With flags it applies only what was
  passed, so editing a prompt leaves the schedule alone; `--prompt-file` takes a
  long brief from a file or stdin, and `--at`/`--cron` imply their trigger and
  clear the timing fields that no longer apply. With no flags it opens the task
  as a commented TOML document in `$EDITOR`: every setting visible, the prompt an
  editable multi-line block, the id read-only, and the draft preserved on a
  parse/validation failure. The mutation runs inside the store's `flock`ed
  `UpdateConfig` (`app.EditTask`), so a concurrent write from the app is never
  clobbered and an invalid edit leaves the stored task untouched.
  `claudeq settings` grew from four flags to the full set the Settings view
  offers (Claude path, heartbeat, idle timeout, run history, custom system
  prompt, Pushover on/off) and prints all of them. It reports the Pushover
  credentials only as configured/not: writing them is fine, echoing them into
  terminal scrollback is not. The driver was external automation, so the README's
  CLI section is now written to be the single document another tool or agent
  needs to drive ClaudeQ.
- **Artifact notifications with click-to-open** — an artifact was only visible if
  the operator went looking for it. Every publish is now announced: the daemon
  re-reads `artifacts.json` on each tick and notifies for anything not yet
  notified (`notified_artifacts` + `artifact_notify_primed` in `state.json`; the
  first pass primes the existing list so an upgrade doesn't replay the backlog,
  and marks before sending so a broken channel costs one message, not a message
  per tick). The macOS notification carries the artifact id in its `userInfo`, and
  `claudeqapp` registers as the bundle's `UNUserNotificationCenter` delegate
  **before** `webview.New` (which runs the launch cycle) so a click that launched
  the app is still delivered; the id is parked in a one-slot mailbox that the page
  drains via `cqTakePendingArtifact` on load or on a nudge, so a click during
  startup and a click while the window is open both open exactly once
  (`window.cqOpenArtifact` → Artifacts view + in-app viewer, or the browser for
  types without one).
  **The real finding from verifying this on the maintainer's Mac:** ClaudeQ's
  notification authorization was `Denied`, so *every* notification the daemon had
  posted for months was accepted, stored and then "presented as none" — nothing
  ever reached the screen (visible only in the unified log, never from the app's
  side). Cause: `RequestMacAuthorization` was only called from `claudeqd`, a
  background LaunchAgent that cannot present the system prompt (the request
  returns `didGrant: 0 hasError: 1`). It is now requested from `claudeqapp`, the
  foreground app, where macOS can actually show the prompt; and because a denial
  cannot be re-prompted, `GET /api/health` exposes `notify_status`
  (`notify.MacAuthorization`) and the dashboard shows a warning bar with a button
  that opens System Settings → Notifications, rather than letting the operator
  assume silence means "nothing happened". How long an alert stays on screen is
  macOS' setting, not the app's: `Info.plist` asks for the alert style
  (`NSUserNotificationAlertStyle`), which is the default for a fresh install only,
  so Settings and the README point at the System Settings toggle.
- Assorted UI fixes (Activity date filter + pagination, hover tooltip, Usage
  bar-chart layout and empty-bar handling).

**Known limitation:** notifications carry the app icon via `UNUserNotificationCenter`
only when the ad-hoc-signed app is installed and granted permission; on the very
newest macOS, reliable delivery may ultimately require a notarized (Developer ID)
build — deferred by choice.

Remaining work: a first tagged release (`v0.1.0` → the release workflow builds
and publishes the `.pkg`), plus any follow-ups from real overnight use.
