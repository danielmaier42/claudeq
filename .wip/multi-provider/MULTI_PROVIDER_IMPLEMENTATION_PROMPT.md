# Implementation prompt: multi-provider SwarmQ

Implement the next unfinished delivery stage of the multi-provider SwarmQ
refinement in this repository.

Read these files completely before changing anything:

1. `AGENTS.md`
2. `.wip/multi-provider/MULTI_PROVIDER_REFINEMENT.md`
3. `.wip/multi-provider/CODEX_PROVIDER_SPIKE.md`
4. `PLAN.md`
5. the relevant sections of `README.md`

Treat `.wip/multi-provider/MULTI_PROVIDER_REFINEMENT.md` as the product and
architecture authority. Treat the captured CLI behavior and fixtures in
`.wip/multi-provider/CODEX_PROVIDER_SPIKE.md` as evidence, not as permission to
call real providers in automated tests.

## Delivery scope

Inspect `origin/main` and the merged pull requests. Select the first unfinished
stage under the refinement's section "Implementation sequence". Implement that
stage completely and only that stage. If none of the stages has landed yet,
start with "PR 1: Provider core and Claude compatibility".

This prompt is reusable after each preceding PR has been merged. Do not start a
later stage on an unmerged foundation, do not combine all stages into one large
PR, and do not perform the SwarmQ rename before stages 1 through 5 are on main.

For stage 5, the required behavior includes the full durable fan-out and join
design from "Multi-provider workflows": persisted dependencies, workflow
identity, normalized final output, bounded and untrusted result injection,
natural-language guidance for "all providers", Activity grouping, and the
three-provider Morning Digest acceptance case. Do not implement this as a
parent process blocked on `queue --wait`.

## Working method

- Start from the latest `origin/main` on a new `feature/<short-topic>` branch.
- Preserve unrelated or pre-existing working-tree changes.
- Make the smallest design that satisfies the selected stage and keeps later
  stages possible. Do not add speculative adapters or a general DAG editor.
- Keep provider-specific arguments, parsing, health and session behavior inside
  adapters. Scheduler and store code must use provider IDs and capabilities,
  never literal provider names.
- Preserve existing ClaudeQ behavior unless the selected stage explicitly
  changes it. Migration must be automatic, idempotent and must not widen access.
- Never use a real Claude or Codex invocation in automated tests. Use fake
  binaries and the sanitized spike fixtures.
- Add table-driven tests for every new resolution, validation, scheduling and
  failure branch. Inject clocks and process dependencies where determinism
  requires it.
- Update `README.md` and the phase table in `PLAN.md` in the same change whenever
  user-visible behavior or phase status changes.
- Do not leave TODOs, dead compatibility branches, swallowed errors or known
  races.

## Verification and delivery

Before reporting completion:

1. Run `gofmt` and `goimports` on changed Go files.
2. Run `go build ./...`.
3. Run `go vet ./...`.
4. Run `golangci-lint run`.
5. Run `go test ./... -race`.
6. Run the relevant acceptance tests and an explicit end-to-end exercise with
   fake provider binaries.
7. Run `/code-review` on the complete diff and resolve every confirmed finding.
8. Run `/verify` for the runtime behavior and record what was observed.
9. Rebuild the installable development package with `scripts/build-pkg.sh`.

Commit the finished stage, push the feature branch, and open a pull request
against `main`. Never merge it and never enable auto-merge. Leave CI green.

Report only the delivered behavior, verification results, PR URL, package path
and the exact commit used for the package. If a real blocker remains, do not
claim the stage is complete.
