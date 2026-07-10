# Codex Batch-Enrich Runner

## Goal

Run the existing Go batch-enrichment pipeline through subscription-authenticated
`codex exec`. Keep Go responsible for selection, batching, validation, writeback,
provenance, and reporting. Retain `claude -p` as an explicit fallback.

## Scope

### In scope

- Add `codex-exec` and `claude` runner selection to `cmd/batch-enrich`.
- Make `codex-exec` the default runner.
- Pin a model per runner and persist the selected model through the existing
  classification provenance path.
- Constrain Codex to structured classification output in an isolated,
  least-privilege execution environment.
- Distinguish parent cancellation from a per-call agent timeout.
- Reject duplicate posting IDs in batched model output.
- Replace the Codex `batch-enrich` skill's duplicated orchestration with a thin
  operator wrapper around the Go command.
- Add unit tests for runner selection, subprocess behavior, schema compatibility,
  cancellation, and timeout handling.

### Out of scope

- OpenAI API or SDK integration.
- Removing the Claude runner or changing `.claude/skills/batch-enrich/SKILL.md`.
- Database migrations or a new provider column on `classifications`.
- Changes to selection, taxonomy mutation, boilerplate stripping, batching,
  writeback, or report file locations.
- Persisting summaries or `skills[].requirement`.
- Automated live-model tests in `go test`.
- General runner/plugin abstraction outside `cmd/batch-enrich`.

## Decisions

- Runner flag: `--runner=codex-exec|claude`. Default: `codex-exec`.
- Codex model pin: `gpt-5.4-mini`. The current official Codex guidance recommends
  it for lighter work and subagents. The first live smoke run must confirm this
  account can use the slug before a production batch.
- Claude model pin remains `claude-haiku-4-5-20251001`.
- `PromptVersion` remains `batch-enrich-v4`. Classification semantics and the
  `{"results": [...]}` contract do not change. Runner and model provenance
  distinguish the execution path.
- `classifications.model` stores the exact runner-specific model slug. Runner
  name is added to the run report and startup log, not the database.
- `agentRunner.Run` remains the shared dispatch seam. Codex output still passes
  through `ParseBatchedResponse` and `Validate` before writeback.
- Live model calls remain operator decisions. Task agents run unit tests only.

## Acceptance criteria

- [ ] Running without `--runner` selects `codex-exec`; passing
  `--runner=claude` selects the existing Claude implementation.
- [ ] An unsupported runner value fails flag validation before posting selection
  or agent dispatch.
- [ ] Startup checks only the executable required by the selected runner and
  returns an actionable error when it is unavailable.
- [ ] Each Codex call uses the pinned model, read-only sandbox, ephemeral mode,
  ignored user config and exec rules, an isolated temporary working directory,
  disabled agent tools, and a committed JSON Schema contract.
- [ ] Posting titles, descriptions, taxonomy, focus text, and retry hints are
  sent through stdin. None appear in process arguments.
- [ ] Codex stdout contains only the final schema-conformant JSON document.
  JSONL event mode is not used. Stderr is diagnostic only.
- [ ] Codex output must match the existing batched response shape and still pass
  expected-ID routing plus semantic validation before writeback.
- [ ] A batched response containing the same expected `posting_id` more than
  once is rejected and enters the existing retry path instead of silently
  overwriting one result.
- [ ] Parent cancellation terminates the subprocess, preserves cancellation,
  charges no attempt, and keeps exit code 130 behavior.
- [ ] A configured per-call timeout terminates the subprocess, charges one
  attempt, and enters the existing retry budget. This behavior is identical for
  Claude and Codex runners.
- [ ] Reports and startup logs identify runner, model, and prompt version.
  Classification rows and failure lines retain the exact selected model and
  prompt version.
- [ ] The Codex `batch-enrich` skill invokes the Go command and contains no SQL,
  taxonomy, validation, retry, or writeback implementation.
- [ ] Default Go tests never invoke `codex`, `claude`, a live model, or Postgres.
- [ ] `go test ./cmd/batch-enrich`, `go test ./...`, and `go vet ./...` pass from
  `apps/tools/`.
- [ ] Before production use, an operator-approved one-posting smoke run succeeds
  with correct provenance, or the handoff records that approval was withheld and
  leaves the production gate closed.

## Tasks

### Task 1: Runner configuration

Update `config.go` and `config_test.go`.

- Add runner constants and `Config.Runner`.
- Replace the single model constant with runner-specific pinned model constants.
- Parse and validate `--runner`; resolve `Config.Model` from the selected runner.
- Keep model pins non-overridable. Provenance must not depend on local Codex
  config.
- Preserve all existing flag defaults except the selected runner and model.

Do not:

- Add a user-settable `--model` flag.
- Change `PromptVersion`.

### Task 2: Codex subprocess boundary

Add focused Codex runner source and tests under `cmd/batch-enrich`.

- Implement the existing `agentRunner` interface.
- Package the batched response JSON Schema with the binary. Materialize it in a
  private temporary directory for `codex exec --output-schema` and clean the
  directory when the runner closes.
- Run Codex with `--model`, `--sandbox read-only`, `--ephemeral`,
  `--ignore-user-config`, `--ignore-rules`, `--skip-git-repo-check`, isolated
  `--cd`, `--output-schema`, `--color never`, and stdin mode.
- Disable shell, web-search, app, and multi-agent features for the subprocess.
  Classification needs model output only.
- Write the complete classification contract and input to stdin. Delimit posting
  content as untrusted data that cannot override the contract.
- Capture final stdout as `agentText` and `rawStdout`. Include stderr in returned
  errors, never in successful payloads.
- Fail construction before dispatch when temporary schema setup fails.
- Use a fake command boundary or Go helper process in tests. Never invoke the
  installed CLI.

Do not:

- Use `codex exec --json`.
- Run Codex from the repository working directory.
- Give the subprocess workspace-write or network-dependent responsibilities.

### Task 3: Failure-boundary hardening

Update `dispatch.go`, `dispatch_test.go`, `validate.go`, and
`validate_test.go`.

- Add an agent-timeout sentinel shared by both production runners.
- Preserve parent `context.Canceled` as a no-attempt terminal cancellation.
- Treat a runner-owned per-call timeout as a completed failed attempt eligible
  for the existing single-posting retry loop.
- Make `ParseBatchedResponse` reject duplicate expected posting IDs.
- Cover timeout, parent cancellation, duplicate IDs, non-zero subprocess exit,
  malformed stdout, and successful structured output.

Do not:

- Change retry budgets or cancellation report suppression.
- Move semantic validation into the JSON Schema.

### Task 4: Command integration and reporting

Update `main.go`, `report.go`, and their focused tests after Tasks 1-3 land.

- Add a runner factory that performs selected-executable preflight and returns
  any cleanup function required by the Codex runner.
- Replace the hard-coded Claude lookup and construction in `run`.
- Add runner to startup logs and report run parameters.
- Keep model and prompt version plumbing into `WriteBack`, `BuildReport`, and
  `AppendFailures` unchanged.
- Preserve signal handling, wave ordering, taxonomy reloads, and exit codes.

### Task 5: Thin Codex skill

Replace `.agents/skills/batch-enrich/SKILL.md` with an operator workflow around
the Go command.

- Parse `<count>`, optional focus text, and `--force` into documented CLI flags.
- Run from `apps/tools/` with `--runner=codex-exec` and a finite
  `--agent-timeout` suitable for interactive operation.
- Treat invocation of the skill with arguments as authorization for the stated
  batch only. Never add `--force` unless the user supplied it.
- Surface the Go command's final counts and report path.
- Point contract ownership to `cmd/batch-enrich`; remove classification pins and
  duplicated SQL/writeback instructions.

Do not:

- Edit the Claude skill. Its current uncommitted revival is a separate fallback.
- Reimplement any pipeline stage in skill prose.

### Task 6: Verification and first-run gate

The coordinator owns verification after implementation agents finish.

- Run formatting, package tests, full Go tests, vet, and the repository review
  panel.
- Confirm `codex exec --help` still supports every pinned safety/output flag.
- Ask for explicit operator approval before any live model call.
- With approval, run one non-force posting through `--runner=codex-exec` and
  verify report provenance, one classification row, structured output, and no
  repository writes from the nested Codex process.
- If the pinned model is unavailable to ChatGPT-managed CLI auth, stop before
  production use. Update the model pin and this plan through review; do not
  silently fall back to a local default.

## Sequencing

**Phase 1 (concurrent):** Task 1, Task 2, Task 3 — disjoint primary files; all
share the pinned runner names, model slugs, and unchanged `agentRunner` contract.

**Phase 2 (concurrent):** Task 4 consumes Tasks 1-3; Task 5 consumes the final
CLI flag contract and writes only the Codex skill.

**Phase 3 (sequential, coordinator):** Task 6 verifies the integrated result and
owns the operator-approved live smoke gate.

## Rough sketch

Proposed runner lifecycle:

```go
// Proposed design; exact names may change during implementation.
runner, cleanup, err := newAgentRunner(cfg)
if err != nil { /* startup error */ }
defer cleanup()
```

The Codex runner receives `systemPrompt` and `userPrompt` through the existing
interface, joins them into one delimited stdin document, and returns final stdout.
It does not own parsing, validation, retries, or persistence.

## Boundary inventory

| Name | Go field/type | CLI or JSON boundary | SQL/report boundary |
|---|---|---|---|
| Runner | `Config.Runner` | `--runner=codex-exec\|claude` | Report `runner`; no SQL column |
| Model | `Config.Model` | Codex/Claude model flag | `classifications.model`, report/failure `model` |
| Prompt version | `Config.PromptVersion` | Prompt contract | `classifications.prompt_version`, report/failure `prompt_version` |
| Batch wrapper | `BatchedAgentResponse.Results` | JSON `"results"` | Unchanged per-posting writeback |
| Posting ID | `AgentResponse.PostingID` | JSON `"posting_id"` | `classifications.job_posting_id` |
| Classification | `AgentResponse.Classification` | JSON `"classification"` | Existing seniority/notes writeback |
| Taxonomy arrays | `AgentResponse` taxonomy slices | Existing JSON keys | Existing taxonomy and join writes |

## Open questions

None blocking. Account entitlement for `gpt-5.4-mini` is a first-run gate, not a
silent runtime fallback.

## Promotion note

At promotion, reconcile the existing uncommitted edits in `project.md` and
`developer-guide.md`: document the Go classifier's pluggable CLI runner, Codex
as default, Claude as fallback, and live model calls as coordinator/operator
work. Do not alter those files during drafting.
