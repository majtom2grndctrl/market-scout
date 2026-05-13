# batch-enrich Go cmd

## Goal

Port the `batch-enrich` Claude skill's mechanical coordination into a `cmd/batch-enrich` Go binary. Enrichment runs without a human Claude session; the binary is cron-schedulable. The skill shrinks to a thin invocation wrapper. Classification still happens inside Haiku via `claude -p` subprocess. Writeback moves off raw psql heredocs onto the existing sqlc query layer.

## Scope

### In scope

- New binary `cmd/batch-enrich` running the full coordination loop: select → strip → load taxonomy → dispatch agents in parallel waves → validate → retry → write back serially through sqlc → emit a structured run report on stdout.
- Subprocess invocation of the official `claude` CLI (`claude -p <prompt> --output-format json --model <MODEL>`) for per-posting classification. No `anthropic-sdk-go` or direct REST.
- Validation rules matching the current skill (A: slug format — slugs must match `^[a-z0-9]+(-[a-z0-9]+)*$`, max 64 chars; B: names and notes must not contain null bytes (0x00) — Postgres rejects them at the protocol level; SQL-injection is handled by sqlc parameters) plus a cross-table slug check: a slug emitted in one taxonomy slot must not collide with an existing slug in any other taxonomy table. The cross-table index covers all four taxonomy tables — `canonical_roles`, `specializations`, `skills`, and `role_dimensions` — so a newly-emitted canonical_role / specialization / skill slug can never reuse an existing dimension slug. Closed-set validation against `role_dimensions` runs independently on the agent's emitted dimension arrays.
- Targeted retry: on validation failure, re-invoke the agent with the original prompt plus a single structured hint bundling all simultaneous failures (e.g. `slug X already exists as a skill — pick a different name`). Up to 3 retries per posting (4 total agent calls).
- New SQL queries driving the binary: list-unclassified-postings (with optional focus filter and force toggle), list-canonical-roles, list-specializations, list-skills, list-role-dimensions. Identified gaps land in `internal/db/queries/`; sqlc regenerated.
- Boilerplate stripping via `exec.Command` on the prebuilt binary at `./bin/strip-boilerplate`, using the same `≥3 selected postings per company` threshold and JSON contract.
- Run config: `PROMPT_VERSION`, `MODEL`, wave size, max retries, max parallelism, run-report format (`json|markdown`), force toggle, focus string, count — as a small embedded constants block (`PromptVersion`, `MODEL`) plus CLI flags for everything else (single source of truth per knob).
- Structured run report to stdout (JSON by default, markdown on `--report-format=markdown`): run params, counts (selected / dispatched / enriched / json_failed / validation_failed / skipped_no_description / re_enriched), new taxonomy added (slug+name+source), per-posting summaries, failures list with reason and last raw response.
- `failures.jsonl`: binary appends exactly one line per failed posting (`json_failed`, `validation_failed`) to `agent-output/batch-enrich/failures.jsonl` (db_failed postings are excluded — DB failures surface in the run report only). One line per posting is the contract — not one line per attempt. Schema: `run_timestamp`, `posting_id`, `attempt` (total calls made), `outcome`, `reason`, `raw_response` (last attempt's exact output), `prompt_version`, `model`, `attempted_at` (see Task 9 for full field specs). Required for observability under cron — without it the failure history vanishes between runs.
- Logs to stderr via `slog` with the `[batch-enrich]` subsystem tag.
- Non-zero exit on infrastructure failure (DB unreachable, claude CLI missing, non-zero exit from `./bin/strip-boilerplate`, invalid `PROMPT_VERSION`/`MODEL`). Exit 0 on a clean run, even if individual postings failed validation — those surface in the report.

### Out of scope

- Cron infrastructure / scheduling wrapper. The binary is cron-invokable; setup is a separate concern.
- Modifying `.claude/skills/batch-enrich/SKILL.md`. The skill stays until the binary is proven; deprecation is a follow-on.
- Next.js / app-layer changes.
- pgvector embedding of the summary field. Summary remains report-only.
- `--force` reclassification beyond the skill's existing flag. Treat force as a flag that drops the `NOT EXISTS` filter on selection; no additional reclassification logic.
- Concurrent orchestration (two binaries running at once). Single-orchestrator assumption retained; writeback stays serial.
- Embedding storage column for `skills[].requirement` — agent contract preserves the field; writeback drops it as today.
- Recurring-failure detection from `failures.jsonl` history (SKILL.md §9). The binary reports only current-run failures; multi-run failure aggregation is deferred. Cron operators can script across the append-only jsonl history separately if needed.

## Acceptance criteria

- [ ] `go build ./cmd/batch-enrich` produces a binary; `go test ./cmd/batch-enrich/...` passes with no live DB or `claude` CLI required (subprocess + DB faked at the seams).
- [ ] Running with `--count=N` selects up to N postings whose latest snapshot has a non-null `description_text` and that lack any existing `classifications` row, ordered by `job_postings.first_seen_at` ASC.
- [ ] `--focus="<text>"` adds an ILIKE prefilter on the latest snapshot's `title` or `description_text` and is included in each agent's prompt.
- [ ] `--force` drops only the unclassified filter; the description-non-null filter and ordering are preserved; a re-classify inserts a new `classifications` row rather than mutating any existing one.
- [ ] For companies with ≥3 selected postings, the binary calls `./bin/strip-boilerplate` via stdin JSON and substitutes the returned `cleaned_text` into the per-posting prompt. Companies below threshold pass through unchanged. A non-zero exit from `./bin/strip-boilerplate` aborts the run with a non-zero exit and a structured error.
- [ ] Agents dispatch in parallel waves of configurable size (default 10); each wave completes before the next starts; the final wave may be smaller.
- [ ] Each agent invocation runs `claude -p --output-format json --model <MODEL>` as a subprocess with the prompt written to stdin, and parses the resulting JSON envelope (shape: `{"result": "<string>", "is_error": <bool>}`) to extract the agent's textual response.
- [ ] An agent response that fails JSON parsing, fails any validation rule (A/B, seniority closed set, non-empty dimensions, known dimension slug, cross-table slug uniqueness) triggers a retry with a single structured hint bundling all simultaneous failures appended to the original prompt. Retries cap at 3 (4 total calls).
- [ ] Cross-table slug check covers all four taxonomy tables: `canonical_roles`, `specializations`, `skills`, `role_dimensions`. A slug emitted in one slot that already exists in any sibling slot (including a dimension slug) causes a rejection and retry with a targeted hint. The closed-set dimension check on the agent's emitted dimension arrays runs independently. Verified by a test using a fixture-driven faked subprocess and a seeded taxonomy. The cross-table collision test is a unit test using an in-memory `Taxonomy` fixture — no DB required.
- [ ] All taxonomy upserts and join inserts run via existing sqlc functions (`GetOrCreateCanonicalRole`, `GetOrCreateSpecialization`, `GetOrCreateSkill`, `InsertCanonicalRoleDimension`, `InsertClassification`, `InsertJobPostingRole`, `InsertJobPostingSpecialization`, `InsertJobPostingSkill`). No raw INSERT/UPDATE SQL strings in the binary.
- [ ] Writeback per posting runs in a single DB transaction. On any DB error the transaction rolls back, the posting is logged as `db_failed`, and the batch continues.
- [ ] Writeback across postings is serial; only one posting's transaction is open at a time.
- [ ] After each wave's writebacks, the in-memory canonical-roles list reloads from DB so subsequent waves' prompts include newly-introduced roles (once per wave, after all serial writebacks for that wave complete; slugs added within a wave are not visible to other agents in the same wave's parallel dispatch). Specializations and skills follow the same pattern. Reload is unconditional — simpler than tracking dirty state; cost is four small SELECTs per wave. The cross-table slug index (Task 7) is rebuilt from the reloaded taxonomy maps immediately after the reload, before the next wave dispatches. Concurrent orchestrators are out of scope; between-run taxonomy additions picked up at reload are not a correctness concern.
- [ ] On a clean run the process exits 0. On infrastructure failure (DB unreachable, missing `claude` binary, `./bin/strip-boilerplate` non-zero exit, invalid `PROMPT_VERSION`/`MODEL` strings) the process exits non-zero and writes a structured error to stderr.
- [ ] The run report (JSON by default) is written to stdout exactly once at the end of the run: run params, counts, new-taxonomy list, per-posting summaries, failures list. Logs go to stderr. Stdout is parseable as a single JSON document.
- [ ] After each run, the binary appends one JSON line per posting failure (`json_failed`, `validation_failed`) to `agent-output/batch-enrich/failures.jsonl` using the schema defined in Task 9. The file is created if absent; prior lines are never modified.

## Tasks

### Task 1: SQL queries and sqlc

Add hand-written SQL to `internal/db/queries/` for operations the binary needs that don't already exist. At minimum: `ListUnclassifiedPostings` (parameters for focus ILIKE and limit, returning `posting_id, company_id, title, description_text` from the latest snapshot per posting), `ListUnclassifiedPostingsForced` (drops the NOT EXISTS clause), `ListCanonicalRoles`, `ListSpecializations`, `ListSkills`, `ListRoleDimensions`. Regenerate sqlc; commit the diff. Confirm against `internal/db/classifications.sql.go` that no naming collisions exist with current queries.

The two list-unclassified queries reuse the `LATERAL` shape from the skill (one row per posting from the latest snapshot). Both queries must include `ORDER BY job_postings.first_seen_at ASC`. Both queries must return the same column list and row struct so the call site treats them uniformly. Accept `''` as "no filter" on the ILIKE parameter rather than adding extra query variants, to keep the call site flat. The force toggle is a separate concern — it requires two named queries because the NOT EXISTS clause changes the query shape, not just a parameter value. Use `(@focus = '' OR (s.title ILIKE '%' || @focus || '%' OR s.description_text ILIKE '%' || @focus || '%'))` as the WHERE guard.

Use named sqlc params: `@focus` (text) and `@row_limit` (int, avoiding the reserved word `limit`). Both queries use the same params and return the same column list. Each query gets its own `<QueryName>Params` struct generated by sqlc (e.g. `ListUnclassifiedPostingsParams`, `ListUnclassifiedPostingsForcedParams`). The call site builds whichever the force branch selects. SELECT clause: `jp.id AS posting_id, jp.company_id, s.title, s.description_text`. These aliases pin the sqlc-generated row struct field names.

Both queries live in a new file `internal/db/queries/enrich.sql` (independent of `classifications.sql` — different selection criteria).

### Task 2: Config and flags

Define a small `config` struct inside `cmd/batch-enrich` holding `PromptVersion`, `Model`, `WaveSize`, `MaxRetries`, `MaxParallelAgents`, `ReportFormat`. `PromptVersion` and `Model` are pinned constants in source — written verbatim into every `classifications` row, not overridable by flag. Initial values match the skill's `classification-pins` block: `PromptVersion = "batch-enrich-v2"`, `Model = "claude-haiku-4-5-20251001"`. The binary is a port, not a new prompt version — reusing `batch-enrich-v2` keeps semantically-identical outputs under one version label and avoids a meaningless partition boundary in the `classifications` table. Validate both against `^[A-Za-z0-9._-]+$` at startup; abort on mismatch.

CLI flags: `--count` (int, default 10), `--focus` (string; empty string or flag omitted = no filter; `%` and `_` are interpreted as SQL ILIKE wildcards — document both behaviors in binary's flag help text), `--force` (bool), `--report-format` (`json|markdown`, default `json`), `--wave-size` (int, default 10), `--max-retries` (int, default 3), `--max-parallel` (int, default 10). `MaxParallelAgents` in the Config struct is set from the `--max-parallel` flag.

### Task 3: Selection and taxonomy

Implement the selection step: load `DATABASE_URL`, open the pool (pgx stdlib), invoke the appropriate sqlc list query based on `force`, log how many postings were skipped because their latest snapshot had a null description. When `force=true`, after selection, query for which selected posting ids already have at least one `classifications` row. Store this set as `alreadyClassified []int64`. Used in Task 9 to compute `re_enriched`. Load full taxonomy (canonical_roles, specializations, skills, role_dimensions) into in-memory maps keyed by slug. Taxonomy reload after each wave reuses these loaders.

### Task 4: Boilerplate stripping

Group selected postings by `company_id`. For each company with ≥3 entries, write to stdin the JSON object `{"company_id": <int64>, "selected_ids": [<int64>, ...]}` (matching the `input` struct in `cmd/strip-boilerplate/main.go`), exec `./bin/strip-boilerplate`, capture stdout, unmarshal the response as `{"postings": [{"posting_id": <int64>, "cleaned_text": "<string>"}, ...]}` (matching the `output` / `outputPosting` structs in the same file), substitute `cleaned_text` into a per-posting working copy. Note: `strip-boilerplate` opens its own DB connection from `DATABASE_URL` — `cmd/batch-enrich` passes only the company id and selected posting ids on stdin; the subprocess self-fetches the description corpus. Companies below threshold pass through unchanged. Non-zero exit from the child aborts the run. If the child returns `cleaned_text: ""` for a posting (posting not in corpus, or entire description was boilerplate), fall back to the original `description_text` for that posting and log a warning at `[batch-enrich]`. Fallback applies regardless of the reason for the empty string.

### Task 5: Prompt template

Build the per-posting prompt as a Go string template. Inputs: posting id, title, description (cleaned if available), focus guidance (omitted if empty), the four taxonomy lists rendered as bullet lists, the agent contract block (output schema + classification discipline + slug discipline + summary contract) copied verbatim from `.claude/skills/batch-enrich/SKILL.md` §7 "Agent contract" (the **Output schema**, **Classification discipline**, **Slug discipline**, and **Summary contract** subsections). Render `role_dimensions` under the exact heading `## Role dimensions (closed set)` — heading text is part of the agent contract.

Retain a separate template for the retry prompt: original prompt + an appended `## Retry guidance` block carrying the structured hint from the validator. Deliver prompts to the subprocess via stdin, not as a positional argument — prompts routinely exceed safe argv limits.

Hint format per failure reason — these templates are the contract between Task 7's validator and the retry prompt, so they live with the prompt template:

- Invalid slug format: `` `<slug>` is not a valid slug — use lowercase letters, digits, and hyphens only, max 64 chars. ``
- Null byte in name/notes: `` `<field>` contains a null byte (\x00) which is not allowed. ``
- Seniority invalid: `` `<value>` is not a valid seniority — use one of: intern, junior, mid, senior, staff, principal, lead, director, unknown. ``
- Seniority missing: `seniority is required — emit one of: intern, junior, mid, senior, staff, principal, lead, director, unknown.`
- Empty dimensions on a role: `` canonical_role `<slug>` has no dimensions — every role must include at least one dimension slug. ``
- Unknown dimension slug: `` `<slug>` is not a known dimension — use one of: <list of dimension slugs>. ``
- Cross-table slug collision: `` `<slug>` is already a <table> — choose a different slug. ``

Multiple simultaneous failures concatenate as separate lines under the `## Retry guidance` heading.

### Task 6: Agent dispatch loop

For each wave (slice of `WaveSize` postings), spawn goroutines bounded by a semaphore at `MaxParallelAgents`. Each goroutine: render prompt → pipe prompt to stdin of `exec.Command("claude", "-p", "--output-format", "json", "--model", model)` → capture stdout → unmarshal the outer envelope (shape: `{"result": "<string>", "is_error": <bool>}`) → if `is_error` is true or exit code is non-zero, treat as a parse failure, consume the retry budget identically to a JSON parse failure, and land as `json_failed` if all retries exhaust; the runner still returns `rawStdout` containing the full subprocess stdout, which the dispatcher records as `raw_response` for failures.jsonl → otherwise extract `envelope["result"]` as the agent's text response → unmarshal that string as `AgentResponse`. Envelope unmarshaling happens inside the runner (see Rough sketch); the dispatcher receives the inner agent text string and handles `AgentResponse` JSON-parse, validation, and retry. Pass `ctx` through; cancel on shutdown signal so in-flight subprocesses exit. `main` installs `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` and passes the derived context to the dispatch loop. On cancellation: in-flight subprocesses exit when their context is cancelled; no run report is emitted; process exits non-zero. The failures.jsonl append is also skipped on cancellation — partial failure records are not written. The next scheduled run re-selects unclassified postings. The DB is the audit trail for a cancelled run: classification rows already committed before cancellation remain, and the next non-force run correctly skips those postings because they are classified. No separate audit log of "what ran in the partial run" is kept — DB state is the ground truth. Stderr (slog) is the only per-posting trace of in-progress work at cancellation time. Cron operators who need a persistent audit of cancelled runs are responsible for capturing stderr.

Retry loop sits inside each goroutine: on parse or validation failure, build the retry prompt, re-invoke. Cap at `MaxRetries`. Final outcome per posting: `enriched`, `json_failed` (parse never succeeded), `validation_failed` (parse succeeded but never passed validation), `db_failed` (passed validation, transaction failed). The validator and the dispatcher share a single typed `Outcome` enum.

### Task 7: Validation

A `validate` file in `cmd/batch-enrich` exposes a single function: given a parsed classification, the in-memory taxonomy, and the in-memory cross-table slug index, return either a `ValidatedClassification` (ready for writeback) or a `ValidationFailure` with a `Reason` and `Hint` string. Reasons: invalid slug format, null byte in name/notes, seniority missing or not one of (intern|junior|mid|senior|staff|principal|lead|director|unknown), empty dimensions array on any canonical role, unknown dimension slug, cross-table slug collision. Rule B (null byte) covers `name` and `notes` fields only. Slugs are covered by Rule A's regex. Rule B runs on the raw value before Task 8's null-coalescing. If `notes` is `"\x00"`, it fails Rule B even though Task 8 would ultimately coalesce a whitespace-only string to NULL. The validator rejects it; the posting is retried. `summary` is not persisted and needs no null-byte check. All simultaneous failures are bundled into a single hint string appended to the retry prompt — write it as plain English the agent can act on. The non-empty dimensions check is per-role: each canonical role in the agent response must include at least one dimension slug. An agent response with zero canonical_roles is valid — write zero join rows for that posting. Same for empty specializations and skills arrays.

Build the cross-table slug index at startup as `map[string]string` (slug → owning table name) covering all four taxonomy tables: `canonical_roles`, `specializations`, `skills`, `role_dimensions`. Including dimensions in the index prevents an agent from inventing a new canonical_role, specialization, or skill whose slug collides with an existing dimension slug. The closed-set validation on the agent's emitted dimension arrays remains a separate check. Rebuild the cross-table index from the reloaded taxonomy maps after each wave so newly-added slugs participate in subsequent validations. Within a wave, two agents may independently invent the same slug. Both will pass the cross-table check (the index is wave-start state). Both writebacks succeed — `GetOrCreate*` is idempotent and returns the existing row for the slug. Both postings are classified, both referencing the same taxonomy row. This is acceptable.

### Task 8: Writeback

For each `ValidatedClassification`, open a DB transaction, run `db.New(tx).GetOrCreateCanonicalRole / Specialization / Skill` per slug, capture returned ids, call `InsertCanonicalRoleDimension` for each (role, dimension) pair. Dimension ids come from `Taxonomy.RoleDimensions[slug]`; a missing slug is impossible post-validation (closed-set check already passed). These inserts are taxonomy-scoped (per canonical role, not per classification) — the table associates a role with its dimensions once, reused across all classifications that reference that role. Dimensions accumulate via union across all classifications referencing a canonical role — `InsertCanonicalRoleDimension`'s ON CONFLICT DO NOTHING produces this naturally. For existing slugs, `GetOrCreate*` preserves the stored `name`; the agent-emitted `name` is used only on first insert. See the ON CONFLICT comment in `internal/db/queries/classifications.sql`. Call `InsertClassification` with `(posting_id, model, prompt_version, seniority, notes)`. The writeback function receives `cfg Config` (or the two strings directly) alongside the `ValidatedClassification` — `PromptVersion` and `Model` are pinned run-wide constants and are not copied onto per-posting types. Notes as `sql.NullString`; empty string, missing field, or whitespace-only after `strings.TrimSpace` → `sql.NullString{Valid: false}`; non-empty after trimming → `sql.NullString{Valid: true, String: <raw value, not trimmed>}`. The agent's `summary` field is not persisted; carry it on `PostingResult.Summary` for the run report only. Capture the returned classification id, call the three join inserts, commit. On any error: rollback, mark the posting `db_failed`, continue. Writeback is serial across postings — postings within a wave queue up after the wave's agent calls all complete.

### Task 9: Run report and exit codes

Aggregate per-posting outcomes into a `RunReport` struct. Marshal to stdout as JSON or render as markdown per `--report-format`. Include: run params block, counts block, new taxonomy block (slugs+names introduced this run, partitioned by table), per-posting list (`posting_id`, title, outcome, summary, last failure reason if any), failures block (non-`enriched` outcomes plus the last raw response).

failures.jsonl is appended only on a normal run end. Infrastructure failures (DB unreachable, missing claude CLI, strip-boilerplate non-zero exit) abort the run before the report phase; in-memory failure records for earlier waves are not written. This matches cancellation behavior — the DB state and run report are the only evidence.

After emitting the report, append one JSON line per `json_failed` or `validation_failed` posting to `agent-output/batch-enrich/failures.jsonl` (one line per posting, with `attempt` = total calls made for that posting; `db_failed` postings are excluded — those surface in the run report only). This is an intentional simplification from SKILL.md §8d, which logs one line per attempt for `json_failed` postings (up to 4 lines per posting across retries). The total attempt count and final raw response are sufficient for the binary's cron observability goal; per-attempt detail mattered in the orchestrated-skill model where each attempt was a distinct Agent tool call, but in the Go binary all attempts for a posting share one goroutine and one context. Create the parent directory `agent-output/batch-enrich/` with `os.MkdirAll` if absent; create the file if absent; never truncate prior lines. Each line is a JSON object with fields (per SKILL.md §8d): `run_timestamp` (string, `YYYY-MM-DD-HHMM`, captured once at run start), `posting_id` (int), `attempt` (int, 1-based; same value as `Attempts` on `PostingResult`; different JSON key for SKILL.md §8d schema compatibility), `outcome` (string: `json_failed` or `validation_failed`), `reason` (string), `raw_response` (string, exact agent output — not re-serialized), `prompt_version` (string), `model` (string), `attempted_at` (string, ISO-8601). `skipped_no_description` is a pre-dispatch count (postings excluded by the `description_text IS NOT NULL` filter, not an `Outcome` value). `re_enriched` is the count of postings in `alreadyClassified` (from Task 3) whose outcome is `enriched`. Zero when `force=false`. This is exact — not approximate. Use JSON key `re_enriched`.

Exit codes: 0 on a completed run (regardless of per-posting failures). Non-zero on: DB open / ping failure, missing `claude` binary in PATH, non-zero exit from `./bin/strip-boilerplate`, invalid `PROMPT_VERSION`/`MODEL`, context cancellation (SIGTERM/SIGINT mid-run — no report emitted, exit non-zero).

Note: SKILL.md writes the markdown report to `agent-output/batch-enrich/<timestamp>.md`. This binary writes exclusively to stdout. Cron wrappers are responsible for redirecting stdout to a per-run log file if on-disk report archives are needed.

### Task 10: Tests

Cover at the seams. Fake the `claude` subprocess via an interface around `exec.Command` (or a function variable assignable in tests). Fake DB via the existing test pattern (Postgres integration test under `//go:build integration`). The DB unique constraint is the backstop for within-wave slug collisions; the unit tests cover the cross-wave cross-table check.

Partition tests by setup cost:

Unit tests (no build tag, no live DB or `claude` CLI required):
- Validator: every failure reason including cross-table collision, against fixture JSON and an in-memory `Taxonomy` fixture.
- Retry hint assembly: each failure type produces the corresponding hint template from Task 5; multiple simultaneous failures concatenate correctly.
- Report JSON shape: a small fixture `[]PostingResult` slice round-trips through `RunReport` marshal/unmarshal.
- Prompt rendering: a fixture posting + taxonomy produces a string containing the expected sections (taxonomy headings, agent contract, focus block when present).

Integration tests (`//go:build integration`, requires live DB):
- Selection: `force=false` skips classified postings; `force=true` includes them; focus filter applies; ordering by `first_seen_at` ASC.
- Writeback: taxonomy upserts, classification insert, all three join inserts, single-open-transaction ordering across postings.

## Sequencing

**Phase 1 (sequential):** Task 1 — SQL queries and sqlc regen block every consumer.
**Phase 2 (concurrent):** Task 2 (config), Task 3 (selection / taxonomy), Task 5 (prompt template). All consume only sqlc output from Phase 1; independent of each other.
**Phase 2b (sequential):** Task 7 (validator) — consumes `Taxonomy` type from Task 3.
**Phase 3 (sequential):** Task 4 (boilerplate strip) consumes Task 3's selected-postings slice shape.
**Phase 4 (sequential):** Task 6 (dispatch loop) consumes Tasks 2, 3, 4, 5, 7 — the orchestrator wires them together.
**Phase 5 (sequential):** Task 8 (writeback) consumes Task 7's `ValidatedClassification` type and Task 6's per-posting outcome stream.
**Phase 6 (sequential):** Task 9 (report) consumes the full outcome set from Task 8.
**Phase 7 (concurrent):** Task 10 (tests) can land alongside each task as it's built; the final integration test sits at the end.

## Rough sketch

Package layout: a single `cmd/batch-enrich` package, one `main.go` plus co-located files split by responsibility (`config.go`, `select.go`, `boilerplate.go`, `prompt.go`, `dispatch.go`, `validate.go`, `writeback.go`, `report.go`). No new sub-packages unless `validate` grows enough to earn its own — per `developer-guide.md` §4 (split by responsibility, not size).

Key types (in the binary's package):

- `Config` — pinned constants + flag-derived knobs (Task 2).
- `SelectedPosting` — `PostingID int64`, `CompanyID int64`, `Title string`, `DescriptionText string` (after optional boilerplate strip). Returned by Task 3.
- `Taxonomy` — four `map[string]TaxonomyEntry` where `TaxonomyEntry` holds `{ID int64, Name string}`, keyed by slug. Plus a `crossTable map[string]string` (slug → table name). Reloaded from DB between waves; no concurrent access during a wave — waves are strictly sequential. The `Name` field drives the new-taxonomy list in the run report.
- `AgentResponse` — the parsed contracted JSON (mirrors the schema in the current skill).
- `ValidatedClassification` — `AgentResponse` plus resolved `[]int64` for role / specialization / skill / dimension ids. Output of Task 7. Does not carry `PromptVersion` or `Model` — those live on `Config` and are passed into writeback alongside the classification.
- `Outcome` — string enum: `enriched`, `json_failed`, `validation_failed`, `db_failed`.
- `PostingResult` — `PostingID`, `Outcome`, `Title`, `Summary`, `LastReason`, `LastRawResponse`, `Attempts`.
- `RunReport` — top-level shape marshaled to stdout.

External shell: `exec.Command("claude", "-p", "--output-format", "json", "--model", model)` with the rendered prompt written to `cmd.Stdin`. Prompt delivered via stdin — positional args would exceed argv limits for large prompts. Envelope shape: `{"result": "<string>", "is_error": <bool>}`. Unmarshal leniently — a struct with only `Result string` and `IsError bool` fields; extra envelope fields are ignored. If `result` is absent or the envelope is not valid JSON, treat as a parse failure. Non-zero exit or `is_error: true` → parse failure, enter retry loop. Runner extracts `envelope["result"]` and returns it as the agent text string; dispatcher unmarshals that string as `AgentResponse`.

DB plumbing: `cmd/batch-enrich` opens `sql.DB` exactly once (same pattern as `cmd/fetcher` and `cmd/strip-boilerplate`). All sqlc calls go through `db.New(pool)` or `db.New(tx)` inside a writeback transaction.

Subprocess plumbing: `exec.Command` is wrapped behind a small `agentRunner` interface (one method: `Run(ctx context.Context, prompt string) (agentText string, rawStdout string, err error)`) so tests can substitute a fake without touching `os/exec`. The runner handles subprocess invocation, envelope unmarshal, and `is_error` / exit-code checking. On success: `agentText = envelope["result"]`, `rawStdout = full subprocess stdout as string`, `err = nil`. On `is_error: true` or non-zero exit: `agentText = ""`, `rawStdout = full subprocess stdout as string` (the JSON envelope or partial output), `err = non-nil sentinel`. `rawStdout` is always populated so the dispatcher can use it as `raw_response` in failures.jsonl on the failure path. The dispatcher handles JSON-parse of `agentText` into `AgentResponse`, validation, and retry. The default implementation is constructed with the model string (`newClaudeRunner(model string)`) and passes `--model <model>` to the subprocess; the interface method takes only `(ctx context.Context, prompt string)`. The test implementation returns canned strings.

`cmd/strip-boilerplate` invocation: uses the prebuilt binary at `./bin/strip-boilerplate`. No fallback — cron deployments require the binary to exist.

## Boundary inventory

| Name | Go struct field | JSON key (agent / report) | SQL column |
|---|---|---|---|
| Posting id | `PostingID` | `"posting_id"` | `job_postings.id` |
| Company id | `CompanyID` | `"company_id"` | `companies.id` |
| Description text | `DescriptionText` | `"description_text"` (boilerplate output) | `posting_snapshots.description_text` |
| Seniority | `Seniority` | `"seniority"` | `classifications.seniority` |
| Notes | `Notes` (`sql.NullString`) | `"notes"` | `classifications.notes` |
| Prompt version | `PromptVersion` | `"prompt_version"` | `classifications.prompt_version` |
| Model | `Model` | `"model"` | `classifications.model` |
| Canonical role slug | `Slug` | `"slug"` | `canonical_roles.slug` |
| Specialization slug | `Slug` | `"slug"` | `specializations.slug` |
| Skill slug | `Slug` | `"slug"` | `skills.slug` |
| Dimension slug | `Slug` | `"slug"` | `role_dimensions.slug` |
| Classification id | `ClassificationID` | `"classification_id"` (report only) | `classifications.id` |
| Summary | `Summary` | `"summary"` (report only, not persisted) | — |
| Dimension slugs (per role) | `Dimensions` (in AgentResponse role object) | `"dimensions"` | `role_dimensions.slug` |
| Attempts (total; for report and failures.jsonl) | `Attempts` (on `PostingResult`); `Attempt` (on `FailureLine`) | `"attempts"` (report), `"attempt"` (failures.jsonl, SKILL.md §8d schema compat) | — |
| Last raw response | `LastRawResponse` (sourced from runner's `rawStdout` — full subprocess stdout envelope, not parsed `agentText`) | `"last_raw_response"` (failures.jsonl only) | — |

## Settled decisions

- **Skill deprecation timing.** `.claude/skills/batch-enrich/SKILL.md` is deprecated after the binary passes satisfactory tests. Deletion is a follow-on at that point.
- **Pre-run build requirement.** Run `go build -o bin/strip-boilerplate ./cmd/strip-boilerplate` before invoking `cmd/batch-enrich`. Cron deployments must include this step. No fallback — if `./bin/strip-boilerplate` is absent, the run aborts.
- **`db_failed` postings excluded from failures.jsonl.** DB failures are infrastructure failures, not agent-output failures. The agent produced a valid, validated classification; the transaction failed. The remedy is to retry the whole run (the posting is unclassified in the DB and will be reselected). The run report is the surface for DB failures — no further on-disk evidence is written for individual db_failed postings.
