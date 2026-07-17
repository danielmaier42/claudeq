# AGENTS.md — Working agreement & quality bar for claudeq

This file binds any AI agent working in this repository. The maintainer does **not** review
code and is not interested in reading it. Full responsibility for code and software quality
rests with the agent. The maintainer wants to see **results** and discuss **process/details** —
never code walkthroughs.

See `PLAN.md` for the architecture and decisions (D1–D9, verification V1–V5).

## 1. Non-negotiable quality bar

Nothing is presented as "done" until **all** of the following hold:

- **Builds clean:** `go build ./...` with zero errors.
- **Formatted:** `gofmt`/`goimports` applied; no diffs.
- **Vetted & linted:** `go vet ./...` and `golangci-lint run` pass with zero findings.
- **Tested:** `go test ./... -race` passes. New/changed behavior has tests (see §2).
- **Self-reviewed:** the agent runs `/code-review` on its own diff and resolves every
  confirmed finding before showing results — the maintainer does not do this.
- **Behaviour-verified:** for anything with a runtime surface, run `/verify` (or an explicit
  end-to-end exercise) and observe it actually working — not just green tests.

If any gate fails, it is fixed before the work is reported — never reported with caveats like
"tests are red but…".

## 2. Testing standards

- **Unit tests** for all core logic: scheduler, trigger evaluation (asap/fixed/cron),
  priority ordering, parallelism rules, the reactive limit gate, store (TOML/JSONL), and
  rate-limit / auth output parsing. Prefer Go **table-driven tests**.
- **Integration tests** for the Claude CLI invocation layer using a **fake `claude`
  binary/stub** (never the real API in CI): assert flag construction, session-id reuse,
  resume-after-limit flow, and the failure/auth/rate-limit branches.
- **Determinism:** inject the clock and any randomness so scheduling/wake logic is testable;
  no reliance on wall-clock sleeps in tests.
- Coverage is a means, not a target — cover behaviour and edge cases, not lines for a number.

## 3. Engineering practices

- **Lightweight & few dependencies** (NFA-01): justify every third-party module; prefer the
  standard library. Keep the daemon's footprint minimal.
- **Clear errors:** wrap with context (`fmt.Errorf("...: %w", err)`); never swallow errors;
  no silent failure paths in the unattended runner.
- **Small, focused commits** with descriptive messages; the tree stays releasable on `main`.
- **No dead code, no TODO-as-shipping-plan, no commented-out blocks.**
- **Concurrency safety:** the daemon runs tasks in parallel — guard shared state, run tests
  with `-race`, avoid data races by design.
- **Secrets:** Pushover credentials and any tokens live in config/keychain, never in code,
  logs, or the repo. Local-only; nothing leaves the machine (NFA-04).

## 4. CI

- A GitHub Actions **CI workflow** runs on every push/PR: build + `gofmt` check + `go vet` +
  `golangci-lint` + `go test ./... -race`. Red CI blocks a release.
- The **release workflow** (tag `v*`) is separate and builds the universal binary + unsigned
  `.pkg` (see `PLAN.md` §11); it depends on CI being green.

## 5. How results are reported to the maintainer

- Report **outcomes**: what works, CI status, a demo/screenshot or a short recorded run,
  and a plain-language changelog. Not code.
- Surface **process/design choices and trade-offs** for discussion — the maintainer wants
  those. Keep them non-code-level (behaviour, UX, reliability, install experience).
- Be honest about limitations and anything skipped or deferred.
