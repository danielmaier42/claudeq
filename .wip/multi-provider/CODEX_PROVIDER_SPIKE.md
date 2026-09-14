# Codex provider spike

Date: 2026-09-11

Tested CLI: `codex-cli 0.154.0`

Tested model: `gpt-5.6-sol`

Status: complete, with one rate-limit follow-up that requires a naturally
exhausted ChatGPT allowance

## Outcome

The Codex CLI can run as a SwarmQ provider. It has non-interactive JSONL output,
persistent sessions, non-interactive resume, interactive continuation, isolated
account state through `CODEX_HOME`, model discovery and provider-owned
subagents.

The adapter needs two Codex-specific safeguards:

- check `codex login status` before every launch, because an unauthenticated
  `codex exec` retries both transports before failing;
- kill the complete descendant process tree, because Codex starts tool
  processes in process groups that differ from the top-level CLI process group.

The JSONL rate-limit event identifies HTTP 429 but did not preserve the
`Retry-After` value in the controlled test. SwarmQ therefore needs a
provider-specific retry policy until a real ChatGPT allowance exhaustion shows
whether the production response includes a reset time.

## Test setup

All runs used a temporary `CODEX_HOME` with mode `0700`. The authenticated runs
copied only the existing `auth.json` into that directory with mode `0600`. No
credential contents were printed or committed. The temporary directory had no
Codex configuration, plugins or custom agents.

The successful calls used the read-only sandbox. Prompts either prohibited tool
use or requested one harmless command. The rate-limit case used a local HTTP
server that returned status 429 and `Retry-After: 17`; it made no OpenAI request.

Official references:

- [Codex developer commands](https://learn.chatgpt.com/docs/developer-commands?surface=cli)
- [Codex configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)
- [Codex subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents)

## Configuration directory and health

`CODEX_HOME` isolates authentication, configuration and stored sessions. An
empty directory produced:

```text
$ CODEX_HOME=<EMPTY> codex login status
Not logged in
exit 1
```

After placing a valid `auth.json` in the same directory:

```text
$ CODEX_HOME=<AUTHENTICATED> codex login status
Logged in using ChatGPT
exit 0
```

The status text is written to stderr in both cases. The adapter should classify
readiness primarily by exit status and retain a redacted form of stderr for the
operator.

A readiness check must perform these operations in order:

1. Resolve the configured binary and verify that it is executable.
2. Run `codex --version` with a short timeout.
3. Run `CODEX_HOME=<provider config directory> codex login status` with a short
   timeout.

It must not use `codex exec` for health. Without authentication, version 0.154.0
made five WebSocket attempts, fell back to HTTPS and made another five attempts
before exiting.

## Invocation contract

The adapter should send the prompt through stdin and pass `-` as the prompt
argument:

```text
codex exec \
  --json \
  --sandbox <read-only|workspace-write|danger-full-access> \
  --cd <working-directory> \
  --model <model> \
  -
```

When the prompt was passed as a positional argument while stdin was not a TTY,
Codex wrote `Reading additional input from stdin...` to stderr. Sending the
prompt through stdin avoided that noise and removed ambiguity about whether
stdin should be appended.

The isolated spike also used `--ignore-user-config` and `--ignore-rules`. The
production adapter must not add those flags. A configured provider owns its
`CODEX_HOME`, and SwarmQ should respect the configuration and policies stored
there.

SwarmQ can inject its run contract with the documented
`developer_instructions` config override:

```text
-c developer_instructions="<SwarmQ run contract>"
```

This was strong enough to make the agent execute the exact command exposed by
`SWARMQ_BIN`. The override replaces a `developer_instructions` value from the
provider's own config instead of appending to it. The adapter implementation
must resolve that conflict explicitly. It must either combine the provider
instructions with the SwarmQ contract or document that SwarmQ owns this field.
Silently discarding provider instructions is not acceptable.

## JSONL success and usage

A minimal successful run emitted four stdout records and exited with 0:

```jsonl
{"type":"thread.started","thread_id":"<THREAD_ID>"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"<ITEM_ID>","type":"agent_message","text":"SPIKE_OK"}}
{"type":"turn.completed","usage":{"input_tokens":15967,"cached_input_tokens":11136,"cache_write_input_tokens":0,"output_tokens":7,"reasoning_output_tokens":0}}
```

The adapter should capture the session ID from `thread.started`, append agent
message text to the run log, and use `turn.completed` as the successful terminal
event. Codex reports input, cached input, cache-write input, output and reasoning
tokens. It did not report monetary cost.

The large input count on a trivial prompt is expected. Codex includes its base
instructions and available tool definitions. SwarmQ should store the provider's
reported values and must not infer cost from them.

## Resume and interactive continuation

Non-interactive resume used:

```text
codex exec resume --json --model <model> <THREAD_ID> -
```

The resumed run returned the same thread ID, emitted the same four-event shape
and included the earlier conversation in its input usage. It exited with 0.

Interactive continuation used:

```text
codex resume --include-non-interactive \
  --sandbox <mode> \
  --cd <working-directory> \
  <THREAD_ID>
```

The TUI opened the session and displayed both earlier non-interactive turns.
`--include-non-interactive` makes the command's intent explicit and also allows
these sessions to appear when the CLI needs to resolve them through its picker.

Resume must always use the original provider instance and its `CODEX_HOME`.
SwarmQ should store the ID as opaque text even though the current CLI emits a
UUID-compatible value.

## Authentication failure

Running against an empty `CODEX_HOME` emitted `thread.started`, `turn.started`,
several `error` events and a terminal `turn.failed`, then exited with 1. The
error text contained HTTP status 401 and `Unauthorized`.

The adapter should classify any of these signals as `auth_error`:

- the preflight `codex login status` exits non-zero;
- a JSONL error contains HTTP 401 or `Unauthorized`;
- the terminal `turn.failed` repeats an authentication error.

The preflight is the normal path. Runtime classification still matters when a
credential expires between the check and the request.

## Rejected model

An invalid model emitted a non-terminal metadata warning, then HTTP 400 in both
an `error` event and `turn.failed`. The process exited with 1:

```jsonl
{"type":"item.completed","item":{"id":"item_0","type":"error","message":"Model metadata for `swarmq-invalid-model-spike` not found. Defaulting to fallback metadata; this can degrade performance and cause issues."}}
{"type":"turn.started"}
{"type":"error","message":"{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The 'swarmq-invalid-model-spike' model is not supported when using Codex with a ChatGPT account.\"}}"}
{"type":"turn.failed","error":{"message":"{\"type\":\"error\",\"status\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The 'swarmq-invalid-model-spike' model is not supported when using Codex with a ChatGPT account.\"}}"}}
```

The first warning is not terminal. The adapter should classify the terminal 400
with the unsupported-model message as `model_error`.

## Rate limit

A local Responses-compatible endpoint returned HTTP 429, a JSON error with code
`rate_limit_exceeded`, and `Retry-After: 17`. With request and stream retries set
to zero, Codex emitted:

```jsonl
{"type":"thread.started","thread_id":"<THREAD_ID>"}
{"type":"turn.started"}
{"type":"error","message":"exceeded retry limit, last status: 429 Too Many Requests"}
{"type":"turn.failed","error":{"message":"exceeded retry limit, last status: 429 Too Many Requests"}}
```

The process exited with 1. The JSONL output did not preserve the response body
or `Retry-After` header. The adapter can classify 429 as `rate_limited`, but this
test does not provide an exact `resume_at` value.

Until a real ChatGPT allowance limit supplies better evidence, Codex should use
a bounded provider-specific retry delay and resume the same session. This delay
must not block other provider instances. A naturally occurring allowance limit
should be captured, sanitized and added as another fixture before Codex leaves
Beta.

## Subagents

Codex spawned a child agent in non-interactive mode. The persisted rollout
contained the `spawn_agent` call, a separate child rollout, `CHILD_OK` from the
child and delivery of that result to the parent. The parent then returned
`PARENT_OK`.

The top-level `--json` stream did not include the spawn event or child message.
It exposed only a `collab_tool_call` wait item and the parent's final message.
This confirms the product boundary in the refinement:

- one top-level Codex process is one SwarmQ job;
- Codex owns its subagent threads and consolidation;
- SwarmQ must tolerate `collab_tool_call` records but must not reconstruct
  subagents from the JSONL stream;
- provider-level token usage is the only reliable usage total for the run.

## Cancellation and process tree

The first cancellation test started Codex in a new POSIX process group and
instructed it to run `/bin/sleep 300`. Once the command was running, the process
tree was:

```text
codex                     PGID = codex PID
codex-code-mode-host      PGID = its own PID
/bin/sleep                PGID = its own PID
```

Killing the top-level Codex process group removed Codex and its code-mode host,
but `/bin/sleep` survived, was reparented to PID 1 and continued running. The
test removed that orphan explicitly.

The current executor's single process-group kill is therefore insufficient for
Codex. Before terminating the parent, SwarmQ must enumerate descendants,
capture their process-group IDs, terminate those groups leaf-first, terminate
the Codex group and verify that none remain. This logic needs a real Codex
integration test on macOS in addition to the existing fake-binary tests.

A second test had the parent spawn a subagent and made that subagent run the
same sleep command. Killing the parent process group again left the
subagent-owned `/bin/sleep` alive in its own process group. The test then killed
that group explicitly. The descendant-group strategy therefore covers direct
and subagent-owned tool processes without depending on private Codex session
files.

## Model discovery

`codex debug models --bundled` returned JSON with a `models` array and exited
with 0 without a model call. Version 0.154.0 returned 11 bundled entries,
including `gpt-5.6-sol` and `gpt-6-astra`. Each entry includes a slug, display
name, supported reasoning levels and visibility metadata.

This is useful for UI suggestions but remains a debug command. SwarmQ should
cache successful results and retain a small adapter-owned fallback list. Model
IDs remain free strings, and a discovery failure must not invalidate an
otherwise configured provider.

## Adapter decisions from the spike

| Concern | Decision |
|---|---|
| Account isolation | Set `CODEX_HOME` per provider instance. |
| Readiness | Use binary checks, `--version`, then `login status`; never make a model call. |
| Prompt transport | Pipe the complete prompt to stdin and pass `-`. |
| Structured output | Use `codex exec --json` and parse one JSON value per line. |
| Success | Require `turn.completed` and process exit 0. |
| Session ID | Capture `thread.started.thread_id` as opaque text. |
| Resume | Use `codex exec resume` with the original provider and session ID. |
| Interactive continuation | Use `codex resume --include-non-interactive`. |
| Authentication | Preflight with `login status`; also classify runtime 401. |
| Invalid model | Classify terminal 400 unsupported-model failures separately. |
| Rate limit | Classify 429; use provider-local backoff when no reset time is present. |
| Subagents | Treat them as internal Codex activity and tolerate collaboration events. |
| Cancellation | Kill and verify all descendant process groups, not only the parent group. |
| Built-in instructions | Use developer instructions only after resolving how provider instructions are preserved. |
| Model suggestions | Read the bundled catalog opportunistically; never use it as strict validation. |

## Captured fixtures

Sanitized parser fixtures are under
`.wip/multi-provider/testdata/codex-cli-0.154.0/`. They contain
no credentials, local paths, request IDs or real session IDs. The 429 fixture
comes from the controlled local endpoint; the others come from real CLI runs.

The adapter tests may copy these files into their package when implementation
starts. Tests should also cover blank lines, malformed JSON, unknown event and
item types, stderr noise, a missing terminal event and a non-zero exit after an
apparently successful item.

## Remaining Beta exit check

Capture one naturally occurring ChatGPT allowance exhaustion. Record whether
the JSONL stream contains a reset timestamp, duration or retry header and
whether `codex exec resume` succeeds after that window. This is evidence
collection, not an architectural blocker. The provider-local fallback delay is
required even if the real response currently contains a reset time.
