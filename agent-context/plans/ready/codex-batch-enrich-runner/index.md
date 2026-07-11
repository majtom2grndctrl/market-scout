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
- Each production runner owns its optional per-call child deadline. After a
  subprocess returns, parent cancellation wins over the child deadline;
  otherwise the child deadline maps to the shared agent-timeout sentinel.
- Live model calls remain operator decisions. Task agents run unit tests only.

## Acceptance criteria

- [ ] Running without `--runner` selects `codex-exec`; passing
  `--runner=claude` selects the existing Claude implementation.
- [ ] An unsupported runner value fails flag validation before posting selection
  or agent dispatch.
- [ ] Startup checks only the selected runner executable (`codex` or `claude`)
  and returns an actionable error when it is unavailable. The existing
  `strip-boilerplate` preflight remains unchanged.
- [ ] Each Codex call uses the pinned model, read-only sandbox, ephemeral mode,
  ignored user config and exec rules, an isolated temporary working directory,
  disabled agent tools, and a committed JSON Schema contract.
- [ ] Codex calls send posting titles, descriptions, taxonomy, focus text, and
  retry hints through stdin. None appear in Codex process arguments. Claude's
  existing `--system-prompt` behavior remains unchanged.
- [ ] Codex stdout contains only the final schema-conformant JSON document.
  JSONL event mode is not used. Stderr is diagnostic only.
- [ ] Codex output must match the existing batched response shape and still pass
  expected-ID routing plus semantic validation before writeback.
- [ ] A batched response containing any repeated `posting_id` is rejected before
  expected/unexpected routing. It enters the existing retry path instead of
  silently overwriting one result.
- [ ] Parent cancellation terminates the subprocess, preserves cancellation, adds
  zero to `PostingResult.Attempts`, and keeps exit code 130 behavior. It wins
  when parent cancellation and the per-call deadline are both observed.
- [ ] A configured runner-owned per-call timeout terminates the subprocess, adds
  exactly one to `PostingResult.Attempts`, and follows the existing retry
  budget. This behavior is identical for Claude and Codex runners.
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

Update `config.go`, `config_test.go`, and `integration_test.go`.

- Add runner constants and `Config.Runner`.
- Replace the single model constant with runner-specific pinned model constants.
- Parse and validate `--runner`; resolve `Config.Model` from the selected runner.
- Keep model pins non-overridable. Provenance must not depend on local Codex
  config.
- Preserve all existing flag defaults except the selected runner and model.
- Set runner-specific `Config` values in integration tests that construct a
  config for `Model` provenance.

Do not:

- Add a user-settable `--model` flag.
- Change `PromptVersion`.

### Task 2: Codex subprocess boundary

Add focused Codex runner source and tests under `cmd/batch-enrich`.

- Implement the existing `agentRunner` interface.
- Check in `cmd/batch-enrich/batched_response.schema.json` and embed it with
  the binary. Materialize it in a private temporary directory for
  `codex exec --output-schema`; clean the directory when the runner closes.
- Omit `$schema`; stay within the JSON Schema subset accepted by
  `codex exec --output-schema`.
- Require every Go JSON field except `classification.notes`,
  `canonical_roles[].notes`, and `skills[].requirement`. Those fields may be
  omitted or `null`; Go normalizes either form to an empty string.
- Enum only the stable seniority values. Keep dynamic taxonomy membership,
  including role dimensions, in Go validation.
- Set `additionalProperties: false` on every schema object, including the
  wrapper and array-item objects.
- Invoke exactly: `codex exec --model <model> --sandbox read-only --ephemeral
  --ignore-user-config --ignore-rules --skip-git-repo-check --cd <temp-cwd>
  --output-schema <schema-file> --color never --disable shell_tool -c
  web_search="disabled" --disable apps --disable multi_agent -`.
- This configuration was verified with `codex exec --strict-config -c
  'web_search="disabled"' --help` against codex-cli 0.137.0.
- `--ignore-user-config` prevents user-configured MCP loading. The isolated
  temporary cwd prevents project config and project instruction discovery.
  Global Codex instructions may still load; do not treat these flags as a full
  instruction-isolation boundary.
- Write the complete classification contract and input to stdin. Delimit posting
  content as untrusted data that cannot override the contract.
- Capture final stdout as `agentText` and `rawStdout`. Include stderr in returned
  errors, never in successful payloads.
- Fail construction before dispatch when temporary schema setup fails.
- Use a fake command boundary or Go helper process in tests. Never invoke the
  installed CLI.
- Add schema-sync tests with representative existing responses. Cover omitted,
  `null`, and string forms of `classification.notes`,
  `canonical_roles[].notes`, and `skills[].requirement`.
- Prove every required Go JSON field has a schema property and is required
  there. Prove every schema object shape rejects additional properties.

Do not:

- Use `codex exec --json`.
- Run Codex from the repository working directory.
- Give the subprocess workspace-write or network-dependent responsibilities.

### Task 3: Failure-boundary hardening

Update `dispatch.go`, `dispatch_test.go`, `validate.go`, and
`validate_test.go`. `dispatch.go` remains the direct `claudeRunner`
implementation owner.

- Add an agent-timeout sentinel shared by both production runners.
- Each runner creates its child deadline only when `AgentTimeout` is non-zero.
  After its subprocess returns, inspect the parent context first. Return parent
  cancellation when present; otherwise translate an expired child deadline to
  the shared agent-timeout sentinel.
- Preserve parent `context.Canceled` as a terminal cancellation: add zero to
  `PostingResult.Attempts`, exit, and do not retry.
- Treat a runner-owned per-call timeout as one completed failed attempt in
  `PostingResult.Attempts`; follow the existing single-posting retry budget.
- Add dispatch-level tests proving `errAgentTimeout` consumes an attempt and
  retries for both an initial batched call and a single-posting retry. Prove
  parent cancellation remains a zero-attempt terminal result.
- Make `ParseBatchedResponse` reject any repeated `posting_id` before
  expected/unexpected routing.
- Add direct Claude runner and Go helper-process tests. Cover both runners'
  successful output, non-zero exit with stderr, parent cancellation, and
  runner timeout.
- Cover Claude malformed JSON envelopes and `is_error=true`, duplicate IDs,
  and successful structured output.

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
- Reconcile provider-neutral comments in `main.go`, `dispatch.go`, `types.go`,
  `validate.go`, and `prompt.go`. Keep Claude-specific comments only around
  `claudeRunner`.

### Task 5: Thin Codex skill

Replace `.agents/skills/batch-enrich/SKILL.md` with an operator workflow around
the Go command.

- Remove every exact `--force` token from supplied arguments, wherever it
  appears; remember whether any appeared. Add one `--force` to the Go command
  only when the user supplied it.
- Treat the first remaining token as a required positive base-10 `<count>`.
  Use 10 when no count is supplied. Invalid, zero, or negative counts stop
  without dispatch.
- Join remaining focus tokens with one space. Pass the result as one `--focus`
  value.
- Run from `apps/tools/` with `--runner=codex-exec` and
  `--agent-timeout=5m`.
- Treat invocation of the skill with arguments as authorization for the stated
  batch only.
- Surface the Go command's final counts and its stdout report.
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
  verify report provenance, one classification row, and structured output.
- Compare the tracked-worktree diff before and after the nested `codex exec`
  call. Exclude only the known parent-owned, gitignored report output. DB
  writes are parent-owned and checked separately.
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
