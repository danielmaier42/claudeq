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
| **D1** | Limit detection | **Purely reactive.** Start the task; if it hits the rate limit, parse the reset timestamp from the CLI output and remember it **globally**. Block all starts until that reset. No pre-flight estimation / no undocumented endpoint. FA-05 is realized via this reactive gate. |
| **D2** | GUI form factor | **Native window app**, implemented with **Wails** (Go core + WebView UI) — chosen over Fyne because the dashboard/log/history UI is far easier in HTML/CSS. |
| **D3** | Process architecture | **Headless background daemon (launchd)** does all the work; a **thin native app** is only the UI. They talk over a **local 127.0.0.1** channel. Daemon runs regardless of whether the app window is open. |
| **D4** | Resume after limit | **Resume the session** via `claude --resume <session-id>`. Marked *verification-required*; **fallback = restart the task** if resume fails, so a task never gets stuck. |
| **D5** | Persistence | **Hybrid.** Tasks & settings in a human-readable **TOML** file (satisfies NFA-07). Run history as a **JSON Lines** index + **one log file per run** on disk. No binary DB. |
| **D6** | Git | **Not managed by claudeq.** FA-34/FA-19 (branch discipline) become a **prompt convention**, not tool-enforced. claudeq only invokes the CLI in the task's working directory. |
| **D7** | Triggers & repetition | **Three trigger modes**: (1) *as-soon-as-possible*, (2) *fixed time (one-shot)*, (3) *recurring (crontab)*. For recurring tasks, if a new occurrence is due while the previous run is still active, it is **skipped**. |
| **D8** | Wake & notifications | Wake via `pmset schedule wake` at **concrete timestamps** (fixed times, cron times, known reset time) **plus an hourly heartbeat wake** as a safety net for "as-soon-as-possible". Notifications delivered by the daemon via a **small user-session helper** for native macOS notifications. |

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

### Limit handling (D1)
```
start task
  └─ run claude CLI
       ├─ finishes → record success/failure, notify on failure
       └─ rate limit hit
             ├─ parse reset timestamp from CLI output
             ├─ set global blocked-until = reset
             ├─ mark run "rate_limited_waiting"
             ├─ plan wake at reset
             └─ at reset: gate reopens → resume via `claude --resume`
                          (fallback: restart task) — D4
```

### Wake (D8)
- Next concrete wake = min(next fixed_at, next cron occurrence, blocked-until) if any.
- Plus recurring hourly heartbeat.
- Registered with `pmset schedule wake`; re-planned after every scheduling pass.

---

## 8. Claude CLI Invocation

- Executed exclusively via **Claude Code CLI**, headless/non-interactive, in `working_dir`
  (FA-27, catalog §2).
- **Model**: per-task `model` overrides `default_model` (FA-28/30).
- **Permissions**: if effective setting is "skip", pass the CLI's skip-permissions flag so the
  unattended run is not blocked by interactive prompts (FA-29/31). Otherwise default behavior.
- **Session capture**: record the session id from the run so `--resume` can continue it after a
  limit wait (D4).
- **Auth detection**: if the CLI reports a login/expired-auth error, mark the run `auth_error`
  and notify (FA-38); do not silently retry.
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

## 10. Open Technical Verification Points

To be validated during a spike before/at the start of implementation:

1. **Headless resume (D4):** Does `claude --resume <session-id>` reliably continue a run that
   was interrupted by a rate limit in non-interactive mode? Confirm session-id capture and
   define the exact restart fallback.
2. **Rate-limit output parsing (D1):** Confirm the exact CLI signal (exit code and/or message
   text) for rate limiting and how the reset time is expressed, so parsing is robust.
3. **Native notifications from a daemon (D8/FA-39):** Confirm the user-session helper approach
   for Notification Center delivery and how the daemon signals it.
4. **`pmset schedule wake` behavior (D8/FA-32):** Confirm reliability, permissions, and how
   multiple scheduled wakes coexist (concrete times + recurring heartbeat).
5. **Auth-error detection (FA-38):** Confirm the CLI signal for expired/invalid login.

---

## 11. Suggested Build Phases (informational)

1. **Spike** — resolve the five verification points (§10).
2. **Core daemon** — store, scheduler, executor, reactive limit gate (headless, CLI-driven).
3. **Wake & resilience** — launchd integration, pmset wake, heartbeat.
4. **Notifications** — macOS helper + Pushover.
5. **App (Wails)** — task management, news dashboard, history, logs, settings.
6. **Hardening** — unattended-run safety, install/uninstall (NFA-05/06).

---

## 12. Next Step

Review this plan. On approval, proceed to the spike (§10) or directly to implementation of the
core daemon (§11), as directed. No code is written until explicitly commissioned.
