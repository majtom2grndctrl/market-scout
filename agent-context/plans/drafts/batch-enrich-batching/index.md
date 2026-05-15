# batch-enrich multi-posting batching

## Goal

Classify multiple postings per `claude` subprocess call so the system-prompt taxonomy (~18K tokens today, growing with the taxonomy) amortizes across N postings instead of being re-paid per posting. Cuts input token cost roughly N× at the wave level. Operator controls N via `--batch-size`; default is conservative (5) to manage Haiku context-rot risk, sweepable for empirical tuning.

## Scope

### In scope

- New `--batch-size` flag (int, default 5, must be >= 1). Validated like the other knobs in `Config.Validate`.
- Agent input shape: one user message lists N postings (each with heading + description). System prompt unchanged — still taxonomy + agent contract.
- Agent output shape: a JSON array of per-posting objects, each object matching the current `AgentResponse` schema (one new top-level wrapper key: `"results"`). Each element carries its own `posting_id`; orchestrator addresses results by id, not by array position.
- Per-posting failure isolation: if any posting in a batch fails JSON parse or validation, the rest of the batch's successes are accepted; only the failed posting(s) re-enter the retry loop, and they re-enter as single-posting calls (batch size 1). The whole-batch retry path is not used.
- Per-posting `Outcome` and `PostingResult` semantics unchanged: each posting in a batched call still terminates as `enriched`, `json_failed`, `validation_failed`, or `db_failed`. Existing report and `failures.jsonl` shapes unchanged.
- `PromptVersion` bump (e.g. `batch-enrich-v3`) because the agent contract changes shape. Per `cmd/batch-enrich/config.go` the version is a pinned constant — bump in source.
- Agent contract update: a new section explaining the batched output schema, with explicit instruction that every input posting must produce exactly one output entry keyed by `posting_id`. `RenderSystemPrompt` carries the updated contract.
- Cache-friendliness preserved: system prompt remains stable across every call in a wave (taxonomy + contract). Only user message varies.

### Out of scope

- Adaptive batch sizing (pack to a token budget). Fixed size only.
- Embedding-based or token-overlap taxonomy filtering. Independent change.
- Per-batch-position quality instrumentation. Tracked as a follow-up.
- Multi-batch-size sweeps in CI. Operator runs `--batch-size=N` manually to compare.
- Retiring single-posting mode. `--batch-size=1` remains a supported runtime configuration and the fallback path for retries.
- Skill (`.claude/skills/batch-enrich/SKILL.md`) update. Skill is post-deprecation cleanup; not a prerequisite.

## Acceptance criteria

- [ ] `--batch-size=N` with N >= 1 sends groups of up to N postings per `claude` subprocess invocation; the final group of a wave may be smaller.
- [ ] `--batch-size=0` or negative is rejected at startup with a structured error.
- [ ] Within a wave, the number of subprocess invocations is `ceil(wave_size / batch_size)` on the happy path (no retries). Verifiable via a fake `agentRunner` that counts calls.
- [ ] Each agent invocation receives a user message listing every posting in the group; the system prompt is byte-identical across every invocation in a wave.
- [ ] An agent response that parses as JSON, contains a result for every posting in the input group, and passes per-posting validation, produces one `enriched` outcome per posting with the correct `posting_id` mapping.
- [ ] An agent response that parses but is missing one or more `posting_id`s produces `enriched` outcomes for the postings present and routes the missing posting(s) into the single-posting retry path.
- [ ] An agent response that parses and addresses all postings, where some posting entries fail validation, produces `enriched` outcomes for the passing postings and routes the failing posting(s) into the single-posting retry path with hints scoped to that posting only.
- [ ] An agent response that fails JSON parse routes every posting in the group into the single-posting retry path.
- [ ] Retries are always single-posting (batch size 1). The retry budget remains `cfg.MaxRetries` per posting and is consumed only by that posting's own retry calls, not by failures of batchmates.
- [ ] Per-posting writeback (current `WriteBack` behavior, one transaction per posting) is unchanged. Enriched postings from a batch each get their own transaction.
- [ ] Run report counts (`enriched`, `json_failed`, `validation_failed`, `db_failed`, `re_enriched`) and `failures.jsonl` lines reflect per-posting outcomes regardless of which call surfaced them. Cancelled postings continue to be excluded via the existing `reasonCancelled` sentinel.
- [ ] `PromptVersion` is bumped in `config.go`; the new value appears in every `classifications.prompt_version` row written by the run.
- [ ] `go build ./cmd/batch-enrich` succeeds; `go test ./cmd/batch-enrich/...` passes with no live DB or `claude` CLI required.

## Tasks

### Task 1: Config and flag

Add `BatchSize int` to `Config` in `cmd/batch-enrich/config.go`. Register `--batch-size` (default 5). Add validation in `Config.Validate` mirroring the `--wave-size` check: must be >= 1. Add to the startup log line in `cmd/batch-enrich/main.go`.

### Task 2: Agent contract — batched output schema

In `cmd/batch-enrich/prompt.go`, extend the `agentContract` constant with a new section describing the batched response shape: a JSON object `{"results": [<per-posting object>, ...]}` where each entry matches today's per-posting schema and carries its own `posting_id`. Existing per-posting fields (classification, canonical_roles, specializations, skills, summary) remain unchanged. Add explicit instruction: emit exactly one entry per input `posting_id`, no extras, no omissions; `posting_id` must echo back unchanged.

Bump `PromptVersion` in `config.go` (e.g. `"batch-enrich-v3"`). Update the source-of-truth comment that today references `.claude/skills/batch-enrich/SKILL.md` — note that the skill is deprecated for batched runs.

### Task 3: User message renderer for batches

Add `RenderBatchedUserMessage(postings []SelectedPosting) string` to `prompt.go`. Renders one heading + description block per posting in input order. Keep the existing `RenderUserMessage(posting SelectedPosting)` for the single-posting retry path — the retry path continues to use it unchanged.

### Task 4: Batched response type and parser

Add a new wrapper type in `cmd/batch-enrich/validate.go` (or a sibling file if validate grows uncomfortable): `BatchedAgentResponse struct { Results []AgentResponse json:"results" }`. The retry hint vocabulary in `prompt.go` (FormatHint cases) is unchanged — hints are still per-posting.

Add a parser that consumes the agent's text response (already post-`stripCodeFence` in `dispatch.go`) and returns `[]AgentResponse` keyed by `posting_id`. Surface two distinct error conditions:

1. JSON parse failure — the whole batch falls through to single-posting retry.
2. Parse succeeded but posting_id coverage is incomplete — return the successfully-addressed `AgentResponse`s and the list of missing posting_ids.

### Task 5: Batched dispatch in `RunWave`

Restructure `cmd/batch-enrich/dispatch.go` so a wave is split into batches of `cfg.BatchSize` postings. Each batch is one `agentRunner.Run` call (still bounded by `MaxParallelAgents` for cases where wave_size > batch_size and multiple batches per wave are dispatched concurrently). Per-batch goroutine:

1. Render user message via `RenderBatchedUserMessage`.
2. Call `runner.Run(ctx, systemPrompt, userPrompt)`.
3. On runner failure or JSON parse failure: every posting in the batch is queued for single-posting retry via `classifyOne`.
4. On parse success: for each posting in the batch, look up its result by `posting_id`:
   - Present and validates: produce `enriched` `PostingResult` (with `Attempts = 1` for that posting; batched and single-posting attempts increment the same counter).
   - Present and fails validation: queue for single-posting retry; carry the validation hints into the retry seed so the first single-posting call already includes the corrected guidance.
   - Missing from the response: queue for single-posting retry with the generic "return JSON only" hint.
5. Single-posting retries run via the existing `classifyOne` flow with `cfg.BatchSize` effectively forced to 1 for that call path.

Cancellation semantics extend the existing `reasonCancelled` pattern: a cancelled batched call stamps `reasonCancelled` on every posting in the batch.

The `agentRunner` interface signature (`Run(ctx, systemPrompt, userPrompt) (agentText, rawStdout, err error)`) is unchanged — batching is a caller-side concern.

### Task 6: Tests

Unit tests, no live DB or `claude` CLI:

- `dispatch_test.go`: extend the fake runner to count calls and serve canned responses keyed by input. Cases:
  1. Batch of N postings, all succeed → 1 call, N `enriched` results.
  2. Batch of N postings, 1 fails validation → 1 batched call + 1 single-posting retry call; N-1 `enriched` from the batch, 1 `enriched` (or `validation_failed` if retry budget exhausts) from the retry path.
  3. Batched response missing 1 posting_id → N-1 `enriched` from the batch, 1 single-posting retry for the missing posting.
  4. JSON parse failure on the batched call → every posting in the batch enters single-posting retry.
  5. Wave of 2N postings with batch size N produces exactly 2 batched calls on the happy path.
  6. `--batch-size=1` produces N single-posting calls for a wave of N postings (regression check).
- `prompt_test.go`: `TestRenderBatchedUserMessage_*` asserts the message contains each posting's heading and description, in input order, and does not contain taxonomy/contract content.
- `validate_test.go`: `BatchedAgentResponse` parses round-trips; missing posting_ids surface separately from JSON parse failure.

Integration tests are not required for batching — the existing writeback integration tests cover the per-posting transaction path that batching reuses unchanged.

### Task 7 (follow-up, deferred): per-batch-position quality instrumentation

After a few real runs at varying `--batch-size`, instrument the report to track per-position quality signals: validation failure rate by position within batch, mint rate by position, mean summary length by position. Goal is detecting position bias and cross-contamination empirically.

Out of scope for the initial landing; tracked here so the follow-up doesn't get lost.

## Sequencing

**Phase 1 (sequential):** Task 1 — flag and config gate every other change.
**Phase 2 (concurrent):** Task 2 (contract + prompt-version bump), Task 3 (batched user-message renderer), Task 4 (batched response type and parser). All independent of one another.
**Phase 3 (sequential):** Task 5 (dispatch) consumes Tasks 2, 3, 4.
**Phase 4 (concurrent):** Task 6 (tests) lands alongside Task 5.

## Rough sketch

Where the change lives:

- `cmd/batch-enrich/config.go` — `BatchSize` field, flag, validation.
- `cmd/batch-enrich/prompt.go` — `agentContract` extended with batched output schema; new `RenderBatchedUserMessage`; `PromptVersion` bump.
- `cmd/batch-enrich/validate.go` (or sibling) — `BatchedAgentResponse` wrapper type and a small parser that returns `(map[int64]AgentResponse, []int64 missing, error)`.
- `cmd/batch-enrich/dispatch.go` — `RunWave` slices the wave into `BatchSize` groups; new helper (call it `classifyBatch`) handles the per-batch fan-out, and `classifyOne` continues to serve the single-posting retry path verbatim.
- `cmd/batch-enrich/main.go` — startup log adds `batch_size`.

What does not change:

- `agentRunner.Run` signature (system + user prompt; system stays stable across the wave).
- `Validate` signature and behavior — still validates one `AgentResponse` at a time. The new wrapper just unpacks the array.
- `WriteBack` — still per-posting transactions, called once per wave with the merged results from batched and retried calls.
- `Outcome` enum, `PostingResult` shape, `RunReport`, `FailureLine`, `failures.jsonl` schema.

Open implementation question (decide during build, not in this spec): whether `classifyBatch`'s per-posting retry queue is drained inline (one goroutine per batch, retries serialize after the batched call returns) or merged into a wave-wide retry queue that runs after every batch in the wave completes. The first is simpler and matches today's per-posting goroutine model; the second amortizes the prompt cache across retries but complicates concurrency. Start with inline.

## Boundary inventory

| Name | Go struct field | JSON key (agent) | SQL column |
|---|---|---|---|
| Batched wrapper | `BatchedAgentResponse.Results` | `"results"` | — |
| Posting id (echoed by agent in each result) | `AgentResponse.PostingID` | `"posting_id"` | `job_postings.id` |
| Batch size knob | `Config.BatchSize` | — (flag `--batch-size`) | — |
| Prompt version (bumped) | `Config.PromptVersion` | — (report `"prompt_version"`) | `classifications.prompt_version` |

All other boundaries are unchanged from `agent-context/plans/in-progress/batch-enrich-go-cmd/index.md`.

## Open questions

- **Default batch size.** Spec lands on 5 to manage Haiku context-rot risk. Settle empirically by sweeping 1, 3, 5, 10 on a small corpus and comparing per-posting validation rate, mint rate, and summary length distribution. Default may move after the first sweep.
- **Retry seeding with validation hints.** When a posting fails validation in a batched call, the spec says to "carry the validation hints into the retry seed so the first single-posting call already includes the corrected guidance." This means the first single-posting retry is itself a retry (hints attached), not a fresh attempt. Confirm this counts toward `MaxRetries` correctly — Attempts increments by 1 for the batched call + N for retries, capped at `MaxRetries + 1` total.
- **Wave size vs batch size relationship.** Today `WaveSize` defaults to 10 and `MaxParallelAgents` defaults to 10 — every wave is one fan-out round. With `BatchSize=5`, a wave of 10 dispatches 2 batched calls in parallel. With `BatchSize=10`, a wave of 10 dispatches 1 batched call. Confirm this composition is intentional and add a startup log line surfacing the effective concurrency.
