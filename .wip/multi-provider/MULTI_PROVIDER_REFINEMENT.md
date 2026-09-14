# Refinement: Multi-provider support and SwarmQ rename

Status: agreed product direction, ready for staged implementation

Scope: provider abstraction, Claude Code compatibility, Codex beta support,
durable multi-provider workflows

Release name: SwarmQ

## Purpose

ClaudeQ currently schedules and runs Claude Code tasks. It shall support several
local agent harnesses without taking over their internal agent orchestration.

The application coordinates jobs across providers. Each provider harness owns
the execution of one job, including any subagents it starts. A running job can
queue another independent job and select a different provider and model for it.
It can also fan work out to several provider jobs and schedule a separate join
job that runs after those jobs finish. The parent does not stay alive while it
waits.

The first implementation supports Claude Code and Codex. Codex is marked as a
beta feature in Settings, but its adapter, API, CLI support, scheduler integration
and persistence are always part of the application. The beta setting controls
only whether Codex setup and selection are offered in the UI.

The application will be renamed from ClaudeQ to SwarmQ in a separate change
after multi-provider support is complete and before its public release.

## Terminology

| Term | Meaning |
|---|---|
| Adapter kind | An implementation for one CLI harness, such as `claude-code` or `codex` |
| Provider | A configured adapter instance selected by its stable ID, such as `claude`, `claude-secondary` or `codex` |
| Model | An opaque model name passed to a provider, such as `opus` or `gpt-5.6-sol` |
| Job | One queued task and its resulting top-level provider process |
| Harness | The CLI process that runs a job and owns its tools, context and subagents |
| Subagent | An agent started and coordinated inside a provider harness |
| Workflow | A durable group of top-level jobs created to produce one combined outcome |
| Dependency | A specific queued job whose terminal result must exist before another job becomes eligible |
| Join job | A job that depends on several jobs and receives their results for consolidation |

Provider ID and adapter kind are separate. This permits two Claude subscriptions
to use the same `claude-code` adapter with separate configuration directories,
sessions and rate-limit state.

## Goals

- Preserve all existing ClaudeQ behavior for migrated Claude Code tasks.
- Let tasks select a provider and model independently.
- Run Claude Code and Codex jobs through one scheduler and history.
- Keep rate limits, authentication state and sessions separate per provider.
- Let provider harnesses use their native subagent support.
- Let jobs queue follow-up work on the same or another provider.
- Let a job fan work out across configured providers and join their results
  without keeping the parent harness alive.
- Make later adapters, such as OpenCode, possible without changing the scheduler.
- Show provider installation and authentication problems before an unattended
  job is allowed to start.

## Non-goals

- SwarmQ does not implement its own subagent protocol.
- SwarmQ does not relay live messages between provider harnesses.
- The first version does not show individual provider-owned subagent threads.
- The first version does not provide a general DAG editor. It supports the
  fan-out and join shape needed by unattended multi-provider work.
- The first version does not expose UI controls for multiple accounts of the
  same adapter kind.
- The first version does not add OpenCode, a standalone xAI harness or arbitrary
  user-defined executable adapters.
- The provider feature does not rename application binaries, paths, bundle IDs
  or environment variables. The rename is a later change.
- SwarmQ does not store provider passwords, OAuth tokens or API keys.

## Product behavior

### Provider and model selection

Every task has an optional provider ID and model. Empty fields inherit defaults.
The provider identifies a configured instance, not merely a vendor.

Examples:

```text
provider = claude
model = opus

provider = codex
model = gpt-5.6-sol

provider = claude-secondary
model = sonnet
```

Model IDs remain free strings. An adapter may return suggestions for the UI,
but SwarmQ does not reject an otherwise valid task because a model is absent
from a cached or incomplete catalog.

### Resolution and inheritance

The effective provider and model are resolved together:

| Task or queue override | Effective provider | Effective model |
|---|---|---|
| Neither specified | Parent provider for self-queue, otherwise global default provider | Parent model for self-queue, otherwise that provider's default model |
| Only model specified | Parent provider for self-queue, otherwise global default provider | Explicit model |
| Only provider specified | Explicit provider | Explicit provider's default model |
| Provider and model specified | Explicit provider | Explicit model |

Changing the provider without naming a model must never carry a model from the
old provider into the new one.

There is no automatic fallback to another provider, account or model. A job
must run with the configuration the operator selected or report why it cannot.

### Subagents

Each top-level process counts as one active SwarmQ job, regardless of how many
subagents its harness starts. The harness decides how to spawn, steer, wait for
and consolidate its subagents.

Subagents stay inside their parent's provider. A provider change creates a new
SwarmQ job:

```text
Claude implementation job
  -> Claude-owned implementation and test subagents
  -> queue a Codex review job
  -> Codex-owned bug, race and maintainability subagents
  -> queue a Claude fix job when needed
```

Canceling the top-level job terminates and verifies its complete descendant
process tree, including processes and subagents owned by that harness.

## Settings UX

### Providers section

Settings gains a Providers section. It always shows the primary Claude Code
provider, even when the CLI is missing:

```text
Claude Code
Status          Not installed
Binary          Auto-detect
Configuration   CLI default
Default model   Claude default

[Check again]
```

When ready, the same card shows the resolved binary and authentication status.
A fresh installation must not imply that Claude Code is available merely
because `claude` is the default provider.

### Codex beta opt-in

Codex setup is initially hidden behind a UI-only beta option:

```text
Beta features

[ ] Set up Codex provider
    Run tasks through the locally installed Codex CLI.
```

After opt-in, Settings shows the Codex provider card and task forms offer
`Codex (Beta)`. Codex runs and activity rows carry a beta badge.

The option must not:

- register or unregister the Codex adapter;
- enable or disable API or CLI support;
- block a Codex job created through the CLI;
- pause an existing Codex task;
- change scheduler behavior.

If a Codex task exists while the beta UI is hidden, Queue and Activity still
render it correctly. Hiding setup controls must never hide actual work.

### Task form

The current model field becomes two fields:

```text
Provider    Default: Claude Code
Model       Provider default
```

Changing Provider refreshes the model suggestions and resets an inherited model
to the new provider's default. An explicitly entered model is retained only
after the user confirms that it should be used with the new provider.

Only ready providers are offered for a new task. An existing task whose provider
is no longer ready remains visible with the reason it is blocked.

### Internal functions

Settings selects provider and model independently for:

- normal task defaults;
- prompt review;
- the feedback assistant.

If the prompt-review provider is unavailable, prompt review reports itself as
unavailable and does not prevent an otherwise valid task from being saved. The
feedback flow retains its manual fallback when its provider cannot run.

## CLI

The existing `--model` option remains. `--provider` is added to `add`, `edit`
and `queue`:

```sh
swarmq queue --provider claude --model opus --prompt "Implement the change"
swarmq queue --provider codex --model gpt-5.6-sol --prompt "Review the branch"
swarmq queue --provider codex --model gpt-6-astra --reasoning-effort xhigh --prompt "Investigate the failure"
```

The feature is implemented before the rename, so these commands initially use
the `claudeq` binary with the same arguments. The later rename changes the
executable name without changing command semantics.

Provider inspection and configuration use a provider command group:

```sh
swarmq provider list [--json]
swarmq provider show ID [--json]
swarmq provider check ID [--json]
swarmq provider add ID --kind KIND [--name NAME] [--path PATH]
swarmq provider edit ID [--name NAME] [--path PATH]
                         [--config-dir PATH] [--default-model MODEL]
swarmq provider enable ID
swarmq provider disable ID
swarmq provider rm ID
```

`provider list --json` includes each configured instance's enabled state,
readiness, readiness reason and default model. A running job uses that output
when the operator asks it to involve several or all providers.

Global assignments are configurable from the CLI:

```sh
swarmq settings --default-provider claude
swarmq settings --prompt-review-provider codex --prompt-review-model gpt-5.6-sol
swarmq settings --feedback-provider claude --feedback-model haiku
```

`provider rm` refuses to remove an instance used by tasks or global settings
and lists every reference that must first be changed.

### CLI failure behavior

An unknown provider is rejected:

```text
swarmq: unknown provider "codex-work"
```

A configured but unavailable provider is also rejected for new jobs:

```text
swarmq: provider "codex" is not ready: Codex CLI was not found
```

```text
swarmq: provider "claude" is not ready: Claude Code is not authenticated
```

The error names the relevant Settings section or login command. The CLI does
not silently substitute the default provider.

## Configuration and persistence

### Provider configuration

A provider is a configured instance of an adapter kind:

```toml
[[providers]]
id = "claude"
kind = "claude-code"
name = "Claude Code"
binary_path = "/Users/me/.local/bin/claude"
config_dir = ""
default_model = "sonnet"
enabled = true

[[providers]]
id = "codex"
kind = "codex"
name = "Codex"
binary_path = "/opt/homebrew/bin/codex"
config_dir = ""
default_model = "gpt-5.6-sol"
enabled = true
```

`config_dir` is part of the model from the start. An empty value uses the CLI's
normal configuration. A later provider instance can select another account by
using a separate directory. The Claude Code adapter maps it to the CLI's Claude
configuration directory mechanism. The Codex adapter maps it to `CODEX_HOME`.
The first UI does not expose duplicate-provider setup, but the store and adapter
contract must not assume one instance per kind.

Adapter-specific configuration may be stored as validated, non-secret options.
Arbitrary environment variables and arbitrary command fragments are not
accepted. They would make validation unreliable and could leak credentials into
the human-readable configuration file.

### Global settings

```toml
[settings]
default_provider = "claude"
prompt_review_provider = "codex"
prompt_review_model = "gpt-5.6-sol"
feedback_provider = "claude"
feedback_model = "haiku"
show_codex_beta = true
```

`show_codex_beta` is presentation state. The engine and API must never inspect
it when deciding whether a run may start.

### Task fields

Tasks gain:

```toml
provider = "codex"
model = "gpt-5.6-sol"
reasoning_effort = "high"
```

An empty provider inherits the global default. An empty model inherits the
effective provider's default. Reasoning effort remains optional and is passed
only when the adapter and selected model support it.

### Run snapshot

History records the effective execution identity for every run:

```text
provider_id
provider_kind
provider_name
model
reasoning_effort
access_mode
session_id
```

These values are snapshots. Editing a provider later must not rewrite the
identity shown for old runs.

Runs also persist their normalized `final_output`. Jobs created from another
run record `parent_run_id` and `workflow_id`. A dependent one-shot task stores
its immutable `depends_on` job IDs and whether dependency results must be
injected. Removing a completed one-shot task from the active queue must not
remove the run result needed by a waiting join.

### Pending sessions and rate limits

A pending resume stores at least:

```text
task_id
run_id
provider_id
provider_kind
session_id
resume_at
```

Rate-limit gates are keyed by provider ID. Two provider instances of the same
adapter kind do not share a gate.

## Provider adapter contract

The engine depends on a generic adapter registry. Provider-specific argument
construction and output parsing stay inside adapters.

Conceptually, an adapter provides:

```text
Kind
DetectBinary
ValidateConfig
CheckHealth
ListModels
Start
Resume
InteractiveResumeCommand
ParseEvent
ClassifyFailure
```

The registry is keyed by adapter kind. A configured provider selects one adapter
and supplies instance settings. No scheduler, API or store code may branch on a
literal provider name when capability information can answer the question.

Adapters report capabilities such as:

```text
session resume
interactive resume
structured output
model discovery
reasoning effort
usage metrics
cost metrics
rate-limit resume
subagents
access modes
```

The executor normalizes provider events into application events:

```text
session started
output received
completed
rate limited
authentication failed
model rejected
failed
```

The Claude Code adapter preserves the current CLI arguments, stream parser,
session behavior, retry handling and metrics. The Codex adapter uses the Codex
non-interactive JSONL mode and translates its events into the same internal
result model.

Adding an OpenCode adapter later must require a new adapter package and registry
entry, not changes to scheduling, history or task persistence.

Grok is normally a model selected through a harness such as OpenCode. It becomes
a SwarmQ adapter kind only if SwarmQ directly runs a separate Grok or xAI CLI.

## Provider health

### Validation levels

Provider checks have three distinct levels:

| Level | Examples | Result |
|---|---|---|
| Configuration validity | Duplicate ID, unknown kind, invalid path or option | Reject the settings change |
| Readiness | Binary missing, not executable, not authenticated, inaccessible config directory | Keep the provider visible but not runnable |
| Runtime | Model rejected, rate limited, session invalidated, CLI process failure | Record a run result |

Readiness checks must not consume model tokens. They use filesystem checks,
`--version` and the harness's authentication-status command. At the time of this
refinement, the local CLIs expose `claude auth status --json` and
`codex login status`.

Model catalogs may be incomplete or unavailable. A separate optional model test
may perform a small real run after clearly stating that it can consume usage.

### Health states

```text
ready
not_installed
not_authenticated
invalid_configuration
disabled
check_failed
```

`rate_limited` is temporary execution state, not provider health.

The daemon caches health briefly but rechecks the selected provider immediately
before launching a job. A configuration change invalidates the cache.

### Unready providers

New tasks and queued follow-ups targeting an unready provider are rejected with
a concrete reason. This prevents a known-bad unattended run from being filed.

A task that already exists when its provider becomes unready remains queued:

- the daemon does not launch it;
- one-shot completion and cron start state are not advanced;
- Queue shows `Blocked: provider not ready` and the reason;
- the operator receives one notification when the provider enters the blocked
  state, not one per scheduler tick;
- the task becomes eligible automatically when the provider is ready again.

If authentication fails only after the process starts, the run ends with the
normal `auth_error` status and retains its log.

## Scheduling, concurrency and cancellation

The scheduler continues to order tasks by priority, trigger and existing
parallelism rules. It resolves and checks the provider before recording a start.

A task with unresolved dependencies is not due. It records no start and does
not consume a concurrency slot. Once every dependency has a terminal result,
normal priority and parallelism rules apply. Dependency readiness is derived
from persisted run state, so it survives daemon restarts without a separate
in-memory waiter.

Provider health and rate limits affect only the selected provider instance. A
rate-limited `claude` provider does not block `claude-secondary` or `codex`.

One top-level harness is one active job for scheduler concurrency. Internal
subagent concurrency is controlled by the harness. SwarmQ neither reserves one
slot per subagent nor tries to infer their number.

Cancel terminates the provider's complete descendant process tree. Resume calls
the same provider instance and adapter that created the session. If
provider-native resume fails, the existing restart fallback remains available
but is recorded clearly. SwarmQ never attempts to resume a Claude session
through Codex or another account.

## Access modes

SwarmQ expresses intent with provider-neutral access modes:

```text
provider-default
read-only
workspace-write
full-access
```

Each adapter validates and maps only the modes it can enforce. The UI must not
claim that a mode is available when the provider cannot provide the stated
restriction. Existing Claude `default` and `skip` behavior migrates without an
authority change.

Subagents inherit the security boundary of their parent harness. A review job
should normally use read-only access. Parallel write-heavy subagents remain the
provider's responsibility, but the built-in review guidance should not request
write access.

## Self-queue and provider handoff

The built-in run instructions document provider selection:

```sh
"${SWARMQ_BIN:-swarmq}" queue --provider codex --model gpt-5.6-sol --prompt "Review the current branch"
```

Before the rename, the existing `CLAUDEQ_BIN` variable and `claudeq` command are
used. The rename migrates the environment variable while retaining a temporary
compatibility path where required.

Self-queue copies the parent task, then applies explicit timing, directory,
provider, model, access and notification overrides. The inheritance table above
is authoritative.

Cross-provider jobs are independent sessions. The queued prompt must contain or
point to the information the next provider needs. SwarmQ does not translate one
provider's conversation history into another provider's session.

## Multi-provider workflows

### Fan-out and join

A running job may create several independent provider jobs and one dependent
join job. This is a durable fan-out and join, not a live conversation between
harnesses:

```text
Morning Digest root run
  -> Claude job using Claude's default model
  -> Codex job using Codex's default model
  -> another configured provider job using its default model
  -> join job, eligible after every child has a terminal result
       -> consolidate results
       -> publish the single digest artifact
```

The root queues the join job immediately after the children and then exits. It
must not keep its harness process alive while waiting. The daemon persists the
dependencies, so a restart, Mac sleep, long child run or provider-local rate
limit does not lose the workflow.

A dependency is bound to the specific one-shot job ID returned by `queue`, not
to a task name, provider or future recurring execution. The first version only
allows a new one-shot job to depend on existing one-shot jobs. Dependencies are
immutable after queueing. Creation order therefore prevents dependency cycles.

The join becomes eligible when every dependency has a terminal result. Success,
failure, authentication error and cancellation are terminal. A rate-limited run
that is scheduled to resume is not terminal. The join still runs when a child
failed, so an unattended digest can identify missing input instead of never
appearing.

### Result hand-off

Every adapter returns a normalized final response in addition to the complete
run log. SwarmQ stores that final response on the run record as `final_output`.
Provider diagnostics, tool events and raw logs stay in the run log and are not
treated as the answer to consolidate.

A dependent job created with `--include-results` receives a generated context
block for every dependency containing:

```text
job ID and name
provider ID and model snapshot
terminal status and error detail
final response
whether the response was truncated
```

Dependency output is untrusted input. SwarmQ delimits it from instructions and
the built-in prompt tells the join harness to treat it as data, never as new
instructions. The injected total is bounded. Truncation is explicit and the
complete run log remains available by path.

### CLI primitives

`queue --json` returns the new job ID and workflow identity. Dependencies are
repeatable:

```sh
swarmq queue --provider codex --prompt "Run skill XYZ and return the requested evidence" --json

swarmq queue --provider claude --model opus \
  --depends-on q-20260914T050000-a1b2c3 \
  --depends-on q-20260914T050000-d4e5f6 \
  --include-results \
  --prompt "Consolidate the attached provider results and publish the digest"
```

Omitting `--model` selects the target provider's default model. A job queued
from another job records its parent run and inherits that run's workflow ID. If
the parent has no workflow ID yet, its run ID becomes the workflow ID. This
groups ordinary self-queued chains as well as multi-provider fan-out without a
separate workflow creation command.

`queue --wait` may be added as a convenience for short interactive scripts. It
is not used by the built-in multi-provider instructions and is not the basis of
unattended workflows. A waiting client process is not durable across restarts
and can deadlock behind an exclusive parent job.

### Meaning of "all providers"

SwarmQ does not parse task prose itself. The built-in system prompt teaches the
running harness how to translate natural-language multi-provider intent into
the CLI primitives above.

When the operator says "all providers", "every provider" or an equivalent
phrase, it means every configured provider instance that is enabled and ready
when the root run takes its snapshot. It refers to provider instances, not
adapter kinds. A future `claude-secondary` instance is therefore a separate
participant. A configured Codex provider participates even while its Beta setup
controls are hidden, because that preference affects presentation only.

For each selected provider, the root queues one child with that provider and no
model override unless the operator named one. The provider's default model is
therefore used. Disabled providers do not participate. Enabled but unready
providers are not queued and must be named as skipped input in the join prompt.
The ready-provider snapshot does not change midway through a workflow.

The instruction applies by intent and context, not by keyword matching:

- "Run skill A and skill B, then consolidate" normally stays inside one job.
- "Ask Claude and Codex" creates two provider jobs.
- "Ask all providers and consolidate" creates one job per ready provider and a
  dependent join job.
- Multiple provider jobs may finish independently when no combined result was
  requested.

The join exists because several top-level jobs produced input that needs to be
combined. The word "consolidate" on its own never creates a join job.

Skill execution remains prompt-level behavior owned by each harness. SwarmQ
passes the requested skill name and output contract to the child job but does
not implement or emulate another harness's skill system. A missing skill is a
normal child failure that the join reports.

Only the join job publishes or announces the combined deliverable unless the
operator explicitly asks for separate child artifacts. This prevents one
Morning Digest workflow from producing several competing notifications and
tables.

## Prompt review and feedback

Prompt review becomes provider-neutral. Deterministic path inspection remains in
SwarmQ. The selected adapter receives only the inspected facts and prompt text,
runs without tools or project instructions where supported, and returns the
existing structured result.

Feedback also selects a provider and model. Its isolation, structured response,
turn limit and manual GitHub fallback remain unchanged. Adapter capabilities
decide whether a provider can support the required structured, resumable flow.

Neither feature silently falls back to another provider. Prompt review becomes
temporarily unavailable. Feedback offers its existing manual path.

## Export and import

A task bundle must not depend on a machine-local provider ID or configuration
directory. It includes an optional provider hint:

```json
{
  "kind": "codex",
  "model": "gpt-5.6-sol",
  "provider_name": "Codex"
}
```

Import attempts to match the hint to local providers. With no unambiguous ready
match, the task sheet requires the operator to select a provider. Import never
creates provider instances, copies configuration paths or imports credentials.

The CLI import command accepts `--provider` and `--model` overrides. Without a
match or override it reports the unresolved provider rather than queuing a task
that cannot run.

## Migration from current ClaudeQ configuration

Configuration migration creates a provider with ID `claude` and kind
`claude-code`:

- `claude_path` becomes its binary path;
- `default_model` becomes its default model;
- existing tasks are assigned to `claude`;
- each task retains its model override;
- existing task permissions retain their effective authority;
- prompt review is assigned to `claude` and retains its model;
- feedback is assigned to `claude` with its current model behavior;
- an empty CLI path continues to use auto-detection.

Migration must be idempotent. A migrated configuration must produce the same
Claude Code invocation and scheduling behavior as the old configuration.

On a fresh installation, the `claude` provider exists even when no Claude Code
binary is found. Its Settings card reports `Not installed`, and new tasks cannot
target it until it is ready.

## Security and credentials

- Provider CLIs own authentication and credential storage.
- SwarmQ stores configuration-directory paths, not tokens or passwords.
- Provider configuration and API responses never expose credential contents.
- Health checks redact command output before logging it.
- Logs record provider ID, adapter kind and status, but not environment values.
- Arbitrary environment-variable maps and arbitrary shell fragments are not
  accepted as provider configuration.
- Binary paths are invoked directly without a shell.
- Access settings never widen silently during migration or provider changes.

## Activity, usage and notifications

Queue and Activity show provider, model and beta state. A run continues to show
the snapshot it used after its provider configuration changes.

Activity groups jobs with the same workflow ID and shows the parent, children
and join without pretending they are provider-owned subagents. A waiting join
shows how many dependencies remain. Queue keeps each active job independently
actionable for cancellation and inspection.

Usage stores metrics reported by the provider:

- input and output tokens;
- duration and turn count;
- provider-reported cost when available;
- provider ID and model.

SwarmQ does not calculate missing cost from a built-in price table. A CLI may be
using subscription allowance rather than direct API billing, and prices change
independently of the application.

Provider health notifications are transition-based. Repeated scheduler ticks
must not produce repeated alerts for the same unresolved condition.

## Implementation sequence

### PR 1: Provider core and Claude compatibility

- Add provider instances, adapter registry, capabilities and normalized results.
- Move current Claude execution behind the Claude Code adapter.
- Add configuration migration and golden argument tests.
- Keep visible behavior unchanged.

### PR 2: Provider configuration and health

- Add provider persistence, validation and readiness states.
- Add provider API and CLI commands.
- Add Settings provider cards and blocked-task presentation.
- Seed the Claude provider on migration and fresh installation.

### PR 3: Codex adapter and beta UI

- Implement Codex execution, JSONL parsing, sessions, cancellation and metrics.
- Add Codex model suggestions and reasoning effort.
- Add the Settings beta opt-in as presentation state only.
- Verify that CLI-created Codex tasks run while beta controls are hidden.

### PR 4: Provider-aware scheduling and secondary flows

- Split rate-limit gates and pending resumes by provider instance.
- Update self-queue, prompt review, feedback and interactive continuation.
- Store provider snapshots in history and usage.
- Update task bundles and import resolution.

### PR 5: Durable multi-provider workflows

- Persist parent and workflow identity, immutable dependencies and normalized
  final output.
- Add dependency-aware scheduling, result injection and queue JSON output.
- Extend the built-in system prompt with multi-provider intent and safe join
  instructions.
- Group workflows in Activity and show waiting joins in Queue.
- Verify the Morning Digest fan-out and join across fake provider adapters.

### PR 6: SwarmQ rename

- Rename the visible app, binaries, bundle ID and LaunchAgent.
- Migrate the application data directory and environment-variable names.
- Remove the old installed LaunchAgent safely.
- Preserve existing tasks, history, artifacts and settings.
- Rebuild and verify the installer as SwarmQ.

## Provider spike

The [Codex provider spike](CODEX_PROVIDER_SPIKE.md) against `codex-cli 0.154.0`
is complete. It captured real Codex CLI behavior in an isolated configuration
directory:

1. JSONL events for success, authentication failure, rejected model and rate
   limit.
2. Session ID discovery, non-interactive resume and interactive continuation.
3. Process-tree and subagent behavior when the parent run is canceled.
4. Reliable delivery of the built-in self-queue, artifact and notification
   instructions.

The spike records sanitized fixtures for adapter integration tests. CI must use
those fixtures and fake binaries, never real provider calls.

One Beta exit check remains: capture a naturally occurring ChatGPT allowance
exhaustion to learn whether its production JSONL includes a reset time. The
adapter must support a provider-local fallback delay because a controlled 429
did not preserve `Retry-After` in JSONL.

The spike also found that Codex tool processes create their own process groups.
The Codex adapter must terminate and verify the full descendant tree instead of
only killing the top-level process group.

Multiple-account UI is deferred. The provider model and adapters still accept a
separate configuration directory from the first implementation so adding a
second Claude or Codex subscription does not require a store or engine redesign.

## Acceptance criteria

### Compatibility

- An existing ClaudeQ configuration migrates automatically and idempotently.
- Migrated Claude tasks produce the same effective CLI arguments, permissions,
  schedule and notification behavior as before.
- Existing history, logs, artifacts and scheduling state remain readable.

### Provider routing

- `--provider` and `--model` work on add, edit and self-queue.
- Provider-only, model-only and combined overrides follow the documented table.
- Unknown or unready providers are rejected for new jobs with a useful error.
- No failure path silently changes provider, account or model.
- A third fake adapter can be registered without changing engine or scheduler
  code.

### Beta behavior

- Codex execution code is always built and registered.
- Codex API and CLI behavior does not depend on the UI beta preference.
- A CLI-created Codex task runs and remains visible while Codex setup is hidden.
- The task form offers Codex only after the operator reveals the beta feature.
- Codex is marked Beta wherever the UI offers or identifies it.

### Health and scheduling

- Claude and Codex readiness checks detect missing binaries and missing login
  without consuming model usage.
- Fresh installations show the real Claude provider status.
- Existing tasks remain queued when their provider becomes unready.
- Blocked tasks do not advance one-shot or cron scheduling state.
- Restoring provider readiness makes blocked tasks eligible automatically.
- A provider health transition produces at most one notification until its
  state changes again.
- A rate limit blocks only the affected provider instance.

### Sessions and execution

- Claude and Codex success, failure, auth, model and rate-limit output is parsed
  into normalized run statuses.
- Resume always uses the original provider instance and adapter.
- Cancel terminates and verifies every descendant process group owned by the
  top-level harness.
- A job with provider-owned subagents counts as one SwarmQ active job.
- Run history preserves provider and model snapshots after settings change.

### Secondary flows

- Self-queue inherits provider and model unless explicitly overridden.
- Cross-provider self-queue uses the target provider's default model when only
  the provider changes.
- Prompt review and feedback honor their configured provider and model.
- Import never copies provider configuration directories or credentials.
- Usage displays available provider metrics without inventing missing costs.

### Multi-provider workflows

- A root run can queue one child per ready provider and one join that depends on
  their returned job IDs, then exit without waiting.
- `queue --json` returns stable job and workflow identities.
- Omitting a child model uses that provider instance's default model.
- Dependency state and workflow grouping survive daemon restart.
- A join does not start while any dependency is running or waiting to resume
  after a rate limit.
- A join starts after every dependency is terminal, including failed,
  authentication-error and canceled children, and receives each status.
- `--include-results` injects normalized final responses rather than raw logs.
- Injected results are bounded, explicitly delimited as untrusted data and
  identify truncation while preserving the full log path.
- Missing or recurring dependency targets, and any target without a stable
  terminal result, are rejected when the join is queued.
- The built-in prompt defines "all providers" as all enabled and ready provider
  instances at the time of fan-out, including configured providers hidden by a
  presentation-only Beta preference.
- The built-in prompt uses each selected provider's default model unless the
  operator explicitly names an override.
- The built-in prompt does not infer a join from the word "consolidate" alone.
  It creates one only when results from several top-level jobs must be combined.
- Enabled but unready providers are listed as skipped input to the join instead
  of leaving the workflow blocked indefinitely.
- The Morning Digest acceptance fixture fans out to three fake providers,
  includes one failed child, runs one join, and publishes exactly one artifact.

### Quality gates

- Fake Claude and Codex binaries cover arguments, output parsing, health, auth,
  rate limits, sessions, resume, cancellation and malformed output.
- Tests use injected clocks and no wall-clock sleeps.
- `go build ./...`, formatting, `go vet ./...`, `golangci-lint run` and
  `go test ./... -race` pass.
- The relevant README sections and the phase table in `PLAN.md` match the
  shipped behavior in every implementation PR.
- Each runtime-facing PR includes an explicit end-to-end exercise.
- The release PR produces an installable SwarmQ development package from its
  final commit.
