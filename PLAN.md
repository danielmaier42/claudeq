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

**launchd:** a `LaunchAgent` in the user session (`~/Library/LaunchAgents/ag.dc.claudeq.plist`)
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
| `skip_permissions_default` | "may do anything" default (FA-29) |
| `heartbeat_interval` | wake safety-net interval, default 1h (D8) |
| `pushover.token`, `pushover.user_key` | Pushover credentials (FA-41) |

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
- **Permissions** (FA-29/31): "skip" → `--dangerously-skip-permissions` (≡
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
Built with `pkgbuild`/`productbuild`. The **root postinstall** script performs the auto-setup:
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
| 2 | **Core daemon** — store, scheduler, executor, reactive limit gate (headless, CLI-driven) | ✅ done | Packages: task, store (TOML/JSONL/state), clock, limit, schedule, executor, engine; plus `claudeqd run` loop and the `claudeq` control CLI (add/list/rm/enable/move/run-now/status/read/settings). Unit + integration tests (fake `claude`, injected clock), all gates green, verified end-to-end. Residual V2 (real-429 field shape + exit code) is handled defensively but still doc-derived — to confirm against an actual 429. See D10–D11. |
| 3 | **Wake & resilience** — launchd integration, pmset wake (sudoers), heartbeat | ✅ done | Packages: system (command runner), wake (next-wake computation + pmset scheduler with reschedule tolerance), launchd (plist + install/uninstall). `claudeqd install`/`uninstall` manage the LaunchAgent; `run` plans wakes each tick (best-effort, needs sudoers). All gates green; acceptance covers install/wake/uninstall with fakes. **Manual check remaining:** real wake-from-sleep on the user's Mac + the one-time sudoers entry (system-modifying, run by the user). |
| 4 | **Notifications** — macOS `osascript` + Pushover | ✅ done | notify package (mac via osascript, Pushover via HTTP, fan-out, all tested); engine notifies on failure/auth (not on success/rate-limit); daemon builds the notifier from settings. Acceptance covers a failure→notification path. |
| 5 | **App (GUI)** — task management, news dashboard, history, logs, settings | ✅ done | macOS-styled web dashboard (embedded, served by `claudeqd run` on 127.0.0.1). Task CRUD/reorder/edit/run-now, queue hides running tasks, Activity with live log (chat view + prompt), replay, unread/history, Usage stats (tokens/runs/cost per day), settings + Pushover, native folder dialog (osascript), notifications, GitHub/About. Follows macOS light/dark + accent (CSS). Verified end-to-end in-browser + API/engine tests. |
| 5b | **Native window** — wrap the dashboard in a WKWebView app window | ✅ done | `cmd/claudeqapp` (darwin): thin native window via `webview_go` (same WKWebView engine; lighter than full Wails, same D2 intent). Loads the daemon dashboard, starts `claudeqd run` if the port is down, and injects the real macOS accent via a native bridge. Shipped as a real `.app` **bundle** (`scripts/build-app.sh` → `build/claudeq.app`: `Info.plist` with `CFBundleName=claudeq`, `.icns` from `logo.svg`, `claudeqd` bundled alongside) so macOS treats it as its own foreground app (menu-bar name, Dock icon, no terminal parent) — `webview_go` self-activates only when bundled. Native menu bar built via cgo (`menu_cocoa.m`, kept in a `.m` file so the Obj-C class compiles once): App (About = standard panel with icon+name+version, Settings ⌘,, Hide, Quit ⌘Q), File (New Task ⌘N, Close ⌘W), Edit (Cut/Copy/Paste/Select-All so shortcuts work in inputs), Window — custom items drive the dashboard via `openAdd()` / `select('settings')`. Live accent: read the `AppleAccentColor` index → hex and re-apply at 0/0.2/0.5/1/1.8/2.8s (defeats the 1-3s cfprefsd flush lag), triggered by **both** the distributed `AppleInterfaceThemeChangedNotification` (dark/light) **and** the local `NSSystemColorsDidChangeNotification` (accent — a plain accent change fires no distributed notification, the key finding). Dashboard assets served `no-cache` so a rebuilt daemon's logo/CSS never shows stale in WKWebView. **User-verified**: native window, menu, correct logo, and live accent switching all working. |
| 6 | **Packaging & release** — unsigned `.pkg` + postinstall, GitHub Actions release pipeline (notarization-ready stubs), install/uninstall (NFA-05/06) | ✅ done | `scripts/build-pkg.sh` → `dist/claudeq-<version>.pkg` (`pkgbuild`, `ditto`-staged, installs `claudeq.app` to `/Applications`). `scripts/pkg/postinstall` sets up the per-user LaunchAgent by running the bundled daemon's own `install` inside the console user's GUI domain (`launchctl asuser <uid> sudo -u <user>`), so autostart is configured without the app on first launch. Unsigned by default with **notarization-ready** hooks gated on `CLAUDEQ_SIGN_APP_ID`/`CLAUDEQ_SIGN_PKG_ID`/`CLAUDEQ_NOTARY_PROFILE`. `scripts/uninstall.sh` removes the agent + app (NFA-06). `.github/workflows/release.yml` builds the `.pkg` on a `v*` tag and attaches it to the GitHub Release. `README.md` documents install/uninstall/build. Acceptance grew 8 static packaging checks (41/41). Pkg **structure-verified** (payload → `/Applications/claudeq.app`, identifier `ag.dc.claudeq`, postinstall present); a real GUI install is user-verified. |
| 7 | **Hardening** — unattended-run safety | ✅ done | Four unattended-safety measures. **Panic recovery** per run (`runGuarded`) turns a panic into a failed result instead of crashing the daemon. **Startup reconcile** (`ReconcileRunningRuns`) marks runs left `running` after a crash/power-loss as interrupted, so they don't show as forever-running. **Idle watchdog** (`IdleTimeout`, default 30 min, 0/neg = off) kills a run that produces no output for too long — a hung/deadlocked process — while a working run keeps streaming and is unaffected; the kill targets the whole **process group** (`Setpgid` + group SIGKILL) so grandchildren die too. **History retention** (`MaxRunHistory`, default 500) prunes old runs + their log files (`PruneHistory`) at startup and after each run, bounding disk. Idle timeout and retention are configurable in Settings → Reliability. Tests cover the watchdog kill, reconcile, prune, and the setting defaults. |

---

## 13. Status

All planned phases (0–7) are complete: the daemon, scheduling/concurrency,
wake & resilience, notifications, the dashboard, the native app, the installer &
release pipeline, and unattended-run hardening. The remaining work is a first
tagged release (build the `.pkg` via the release workflow) and any follow-ups
that surface from real overnight use.
