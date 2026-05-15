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
- [ ] An agent call that fails (runner error or JSON parse failure) routes every posting in the group into the single-posting retry path.
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

Bump `PromptVersion` in `config.go` (e.g. `"batch-enrich-v3"`). Update the source-of-truth comments in `prompt.go` (all three SKILL.md references (file header, `RenderSystemPrompt` doc comment, `agentContract` doc comment)) that reference `.claude/skills/batch-enrich/SKILL.md` — note that the skill is deprecated for batched runs. `config.go`'s only edit is bumping the constant value.

Also prepend a one-line deprecation note to `.claude/skills/batch-enrich/SKILL.md` pointing to `cmd/batch-enrich` as the active implementation. The skill file is otherwise unchanged — full retirement is the post-deprecation cleanup already noted in Out of scope.

### Task 3: User message renderer for batches

Add `RenderBatchedUserMessage(postings []SelectedPosting) string` to `prompt.go`. Renders one heading + description block per posting in input order. `RenderUserMessage` is deleted.

`RenderRetryPrompt`'s signature is unchanged. `classifyBatch` passes the output of `RenderBatchedUserMessage` as the `originalUserMessage` argument — even for 1-element retry calls. This means the agent always receives a `{"results":[…]}`-shaped base message, so the response contract is uniform across batched and retry calls.

### Task 4: Batched response type and parser

Add a new wrapper type in `cmd/batch-enrich/validate.go`: `BatchedAgentResponse struct { Results []AgentResponse \`json:"results"\` }`. The retry hint vocabulary in `prompt.go` (FormatHint cases) is unchanged — hints are still per-posting.

Add a parser:

```go
// Proposed design
func ParseBatchedResponse(agentText string, expected []int64) (results map[int64]AgentResponse, missing []int64, err error)
```

`expected` is the set of posting IDs the caller sent; the parser uses it to compute `missing` and drop unexpected IDs (debug-logged). Input is already post-`stripCodeFence` (handled in `dispatch.go`). Surface two distinct error conditions:

1. JSON parse failure — the whole batch falls through to single-posting retry.
2. Parse succeeded but posting_id coverage is incomplete — return the successfully-addressed `AgentResponse`s and the list of missing posting_ids.

### Task 5: Batched dispatch in `RunWave`

Restructure `cmd/batch-enrich/dispatch.go` so a wave is split into batches of `cfg.BatchSize` postings. Each batch is one `agentRunner.Run` call (still bounded by `MaxParallelAgents` for cases where wave_size > batch_size and multiple batches per wave are dispatched concurrently).

`classifyBatch` signature:

```go
// Proposed design
func classifyBatch(
    ctx context.Context,
    postings []SelectedPosting,
    taxonomy Taxonomy,
    cfg Config,
    runner agentRunner,
    systemPrompt string,
    seedHints map[int64][]string,
) []PostingResult
```

`seedHints` is keyed by `PostingID`. Entries are folded into the first retry call for that posting. A nil or empty map means no seeded hints — the typical case for the initial wave call.

Per-batch goroutine:

1. Render user message via `RenderBatchedUserMessage`.
2. Call `runner.Run(ctx, systemPrompt, userPrompt)`.
3. On runner failure or JSON parse failure: every posting in the batch is queued for single-posting retry via `classifyBatch` (1-element each).
4. On parse success: for each posting in the batch, look up its result by `posting_id`. Call `Validate(resp, taxonomy)` per map entry:
   - Present and validates: produce `enriched` `PostingResult`.
   - Present and fails validation: `Validate` returns a `ValidationFailure`; its `.Hints` become the `seedHints` entry for that posting's single-posting retry. Queue for retry.
   - Missing from the response: queue for single-posting retry with the generic "return JSON only" hint.

`classifyBatch` runs an internal parse → validate → retry loop for postings that did not succeed on the first call, mirroring today's `classifyOne` loop. Each retry is a fresh `runner.Run` call with a 1-element user message (rendered via `RenderBatchedUserMessage`). Seeded hints from a batched-call validation failure — and any subsequent retry hints — are folded into the user prompt via `RenderRetryPrompt`. The per-posting budget is `cfg.MaxRetries + 1` total completed calls; the initial batched call counts as the first attempt for every posting it addressed. When the budget exhausts, the posting terminates with `OutcomeJSONFailed` or `OutcomeValidationFailed` using the same terminal logic as `classifyOne`.

Attempts rule: a single completed `runner.Run` call increments `Attempts` by 1 for every posting it addressed, regardless of per-posting outcome. A batched call of N that parse-fails charges 1 attempt to each of the N postings. Cap remains `MaxRetries + 1` total per posting.

| Path | Attempts charged |
|---|---|
| Batched call succeeds for P | 1 |
| Batched call fails validation for P → retry succeeds | 2 |
| Batched call parse-fails (whole batch) → retry succeeds | 2 |
| Any retry exhausts budget | Attempts at budget exhaustion; outcome is `validation_failed` or `json_failed` |

Single-posting retries call `classifyBatch` with a 1-element slice. The system prompt is byte-identical to batched calls; the parser looks up the single `posting_id` in the returned map, exactly like the N-element case.

5. Hoist `RenderSystemPrompt(taxonomy, cfg.Focus)` from per-posting to once per wave in `RunWave`. Pass the rendered string into every `classifyBatch` call so every invocation in a wave receives the byte-identical system prompt the cache benefit assumes. This is a micro-optimization — today's per-posting render already produces byte-identical strings because `writeTaxonomyList` sorts deterministically; the hoist avoids redundant rendering.

Cancellation semantics extend the existing `reasonCancelled` pattern:

- A context-cancelled call (batched or single) does not increment `Attempts` for the postings it was addressing; prior completed attempts for those postings remain charged. The cancelled call itself is not a completed call.
- A posting that already received a completed batched call (success or failure) retains that call's `LastReason` and `Attempts` if cancellation fires before its retry runs. Only postings that no completed call ever addressed are stamped `reasonCancelled`. This preserves the batched-call failure signal in `failures.jsonl` — consistent with the project's append-only / preserve-signal ethos.
- Postings sitting in the retry queue that no completed call ever addressed are stamped `reasonCancelled`, not the failure that would have queued them. Operator-initiated cancellation is the terminal signal for those postings.
- `reasonCancelled` postings are excluded from run report counts, consistent with existing behavior.

`MaxParallelAgents` now bounds concurrent batched `runner.Run` calls, not concurrent postings. Effective posting concurrency is `MaxParallelAgents × BatchSize`. The startup log line surfaces both knobs and the derived effective concurrency so the operator can sanity-check the composition.

The `agentRunner` interface signature (`Run(ctx, systemPrompt, userPrompt) (agentText, rawStdout, err error)`) is unchanged — batching is a caller-side concern.

### Task 6: Tests

Unit tests, no live DB or `claude` CLI:

- `dispatch_test.go`: extend the fake runner to count calls and serve canned responses keyed by input. Cases:
  1. Batch of N postings, all succeed → 1 call, N `enriched` results.
  2. Batch of N postings, 1 fails validation → 1 batched call + 1 single-posting retry call; N-1 `enriched` from the batch, 1 `enriched` (or `validation_failed` if retry budget exhausts) from the retry path. Assert `Attempts == 1` for passing postings, `Attempts == 2` for the retried posting (1 for the batched call + 1 for the single-posting retry).
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
**Phase 2 (concurrent):** Task 2 (contract + prompt-version bump), Task 3 (batched user-message renderer), Task 4 (batched response type and parser). Task 2 and Task 4 share the `"results"` wire-format contract — pin the JSON shape (top-level `"results"` array of per-posting objects, each with `posting_id`) before either lands.
**Phase 3 (sequential):** Task 5 (dispatch) consumes Tasks 2, 3, 4.
**Phase 4 (concurrent):** Task 6 (tests) lands alongside Task 5.

## Rough sketch

Where the change lives:

- `cmd/batch-enrich/config.go` — `BatchSize` field, flag, validation.
- `cmd/batch-enrich/prompt.go` — `agentContract` extended with batched output schema; new `RenderBatchedUserMessage`; `PromptVersion` bump.
- `cmd/batch-enrich/validate.go` — `BatchedAgentResponse` wrapper type and a small parser that returns `(map[int64]AgentResponse, []int64 missing, error)`.
- `cmd/batch-enrich/dispatch.go` — `RunWave` slices the wave into `BatchSize` groups; new helper `classifyBatch` handles all calls (batched and single-posting retries). `classifyOne` is deleted; `classifyBatch` absorbs its retry-loop logic. Any existing tests that call `classifyOne` directly are rewritten against `classifyBatch`.
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
| Prompt version (bumped) | `Config.PromptVersion` | — (report `prompt_version` + `classifications.prompt_version` DB column) | `classifications.prompt_version` |

All other boundaries are unchanged from `agent-context/plans/in-progress/batch-enrich-go-cmd/index.md`.

## Settled decisions

- **Default batch size: 5.** Manages Haiku context-rot risk; sweepable via `--batch-size` once Task 7 instrumentation lands. Default stays 5 until empirical data says otherwise.
- **Prompt cache mechanism: confirmed.** Anthropic's cache is server-side, scoped to API key. Byte-identical system prompts get cache hits across separate subprocess invocations within the TTL window (~5 min standard, ~1 hr for Max subscribers). Verify on the first real run by inspecting `cache_read_input_tokens` in the CLI output — not a prerequisite for shipping, but a required first-run check before treating the cost reduction as established.
