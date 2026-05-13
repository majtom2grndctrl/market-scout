# batch-enrich Go cmd

## Goal

Port the `batch-enrich` Claude skill's mechanical coordination into a `cmd/batch-enrich` Go binary. Enrichment runs without a human Claude session; the binary is cron-schedulable. The skill shrinks to a thin invocation wrapper. Classification still happens inside Haiku via `claude -p` subprocess. Writeback moves off raw psql heredocs onto the existing sqlc query layer.

## Scope

### In scope

- New binary `cmd/batch-enrich` running the full coordination loop: select → strip → load taxonomy → dispatch agents in parallel waves → validate → retry → write back serially through sqlc → emit a structured run report on stdout.
- Subprocess invocation of the official `claude` CLI (`claude -p <prompt> --output-format json --model haiku-4-5`) for per-posting classification. No `anthropic-sdk-go` or direct REST.
- Validation rules matching the current skill (A: slug format, B: null byte / control character rejection — SQL-injection is now handled by sqlc parameters, C: null byte rejection on names and notes) plus a cross-table slug check: a slug emitted in one taxonomy slot must not collide with an existing slug in a sibling slot (skill vs. specialization vs. canonical_role). `role_dimensions` is excluded from the cross-table check — closed-set validation already rejects unknown dimension slugs before writeback.
- Targeted retry: on validation failure, re-invoke the agent with the original prompt plus a single structured hint bundling all simultaneous failures (e.g. `slug X already exists as a skill — reuse it or pick a new name`). Up to 3 retries per posting (4 total agent calls).
- New SQL queries driving the binary: list-unclassified-postings (with optional focus filter and force toggle), list-canonical-roles, list-specializations, list-skills, list-role-dimensions, list-classified-posting-ids. Identified gaps land in `internal/db/queries/`; sqlc regenerated.
- Boilerplate stripping via `exec.Command` on the prebuilt binary at `./bin/strip-boilerplate`, using the same `≥3 selected postings per company` threshold and JSON contract.
- Run config: `PROMPT_VERSION`, `MODEL`, wave size, max retries, max parallelism, run-report format (`json|markdown`), force toggle, focus string, count — as flags or a small embedded constants block (single source of truth).
- Structured run report to stdout (JSON by default, markdown on `--report-format=markdown`): run params, counts (selected / dispatched / enriched / json-failed / validation-failed / skipped-no-description / re-enriched), new taxonomy added (slug+name+source), per-posting summaries, failures list with reason and last raw response.
- `failures.jsonl`: binary appends one line per posting failure (`json_failed`, `validation_failed`) to `agent-output/batch-enrich/failures.jsonl`. Same schema as the current skill. Required for observability under cron — without it the failure history vanishes between runs.
- Logs to stderr via `slog` with the `[batch-enrich]` subsystem tag.
- Non-zero exit on infrastructure failure (DB unreachable, claude CLI missing, abort condition). Exit 0 on a clean run, even if individual postings failed validation — those surface in the report.

### Out of scope

- Cron infrastructure / scheduling wrapper. The binary is cron-invokable; setup is a separate concern.
- Modifying `.claude/skills/batch-enrich/SKILL.md`. The skill stays until the binary is proven; deprecation is a follow-on.
- Next.js / app-layer changes.
- pgvector embedding of the summary field. Summary remains report-only.
- `--force` reclassification beyond the skill's existing flag. Treat force as a flag that drops the `NOT EXISTS` filter on selection; no additional reclassification logic.
- Concurrent orchestration (two binaries running at once). Single-orchestrator assumption retained; writeback stays serial.
- Embedding storage column for `skills[].requirement` — agent contract preserves the field; writeback drops it as today.

## Acceptance criteria

- [ ] `go build ./cmd/batch-enrich` produces a binary; `go test ./cmd/batch-enrich/...` passes with no live DB or `claude` CLI required (subprocess + DB faked at the seams).
- [ ] Running with `--count=N` selects up to N postings whose latest snapshot has a non-null `description_text` and that lack any existing `classifications` row, ordered by `job_postings.first_seen_at` ASC.
- [ ] `--focus="<text>"` adds an ILIKE prefilter on the latest snapshot's `title` or `description_text` and is included in each agent's prompt.
- [ ] `--force` drops only the unclassified filter; the description-non-null filter and ordering are preserved; a re-classify inserts a new `classifications` row rather than mutating any existing one.
- [ ] For companies with ≥3 selected postings, the binary calls `./bin/strip-boilerplate` via stdin JSON and substitutes the returned `cleaned_text` into the per-posting prompt. Companies below threshold pass through unchanged. A non-zero exit from `./bin/strip-boilerplate` aborts the run with a non-zero exit and a structured error.
- [ ] Agents dispatch in parallel waves of configurable size (default 10); each wave completes before the next starts; the final wave may be smaller.
- [ ] Each agent invocation runs `claude -p <prompt> --output-format json --model <MODEL>` as a subprocess and parses the resulting JSON envelope to extract the agent's textual response.
- [ ] An agent response that fails JSON parsing, fails any validation rule (A/B/C, seniority closed set, non-empty dimensions, known dimension slug, cross-table slug uniqueness) triggers a retry with a single structured hint bundling all simultaneous failures appended to the original prompt. Retries cap at 3 (4 total calls).
- [ ] Cross-table slug check covers `canonical_roles`, `specializations`, and `skills` only — `role_dimensions` is excluded. A slug emitted in one of those slots that already exists in a sibling slot causes a rejection and retry with a targeted hint. Verified by a test using a fixture-driven faked subprocess and a seeded taxonomy.
- [ ] All taxonomy upserts and join inserts run via existing sqlc functions (`GetOrCreateCanonicalRole`, `GetOrCreateSpecialization`, `GetOrCreateSkill`, `InsertCanonicalRoleDimension`, `InsertClassification`, `InsertJobPostingRole`, `InsertJobPostingSpecialization`, `InsertJobPostingSkill`). No raw INSERT/UPDATE SQL strings in the binary.
- [ ] Writeback per posting runs in a single DB transaction. On any DB error the transaction rolls back, the posting is logged as `db_failed`, and the batch continues.
- [ ] Writeback across postings is serial; only one posting's transaction is open at a time.
- [ ] After each wave's writebacks, the in-memory canonical-roles list reloads from DB so subsequent waves' prompts include newly-introduced roles. Specializations and skills follow the same pattern.
- [ ] On a clean run the process exits 0. On infrastructure failure (DB unreachable, missing `claude` binary, `./bin/strip-boilerplate` non-zero exit, invalid `PROMPT_VERSION`/`MODEL` strings) the process exits non-zero and writes a structured error to stderr.
- [ ] The run report (JSON by default) is written to stdout exactly once at the end of the run: run params, counts, new-taxonomy list, per-posting summaries, failures list. Logs go to stderr. Stdout is parseable as a single JSON document.
- [ ] After each run, the binary appends one JSON line per posting failure (`json_failed`, `validation_failed`) to `agent-output/batch-enrich/failures.jsonl`, using the same schema as the current skill. The file is created if absent; prior lines are never modified.

## Tasks

### Task 1: SQL queries and sqlc

Add hand-written SQL to `internal/db/queries/` for operations the binary needs that don't already exist. At minimum: `ListUnclassifiedPostings` (parameters for focus ILIKE and limit, returning `posting_id, company_id, title, description_text` from the latest snapshot per posting), `ListUnclassifiedPostingsForced` (drops the NOT EXISTS clause), `ListCanonicalRoles`, `ListSpecializations`, `ListSkills`, `ListRoleDimensions`. Regenerate sqlc; commit the diff. Confirm against `internal/db/classifications.sql.go` that no naming collisions exist with current queries.

The two list-unclassified queries reuse the `LATERAL` shape from the skill (one row per posting from the latest snapshot). Accept `''` as "no filter" on the ILIKE parameter rather than adding extra query variants, to keep the call site flat.

### Task 2: Config and flags

Define a small `config` struct inside `cmd/batch-enrich` holding `PromptVersion`, `Model`, `WaveSize`, `MaxRetries`, `MaxParallelAgents`, `ReportFormat`. `PromptVersion` and `Model` are pinned constants in source — written verbatim into every `classifications` row, not overridable by flag. Validate both against `^[A-Za-z0-9._-]+$` at startup; abort on mismatch.

CLI flags: `--count` (int, default 10), `--focus` (string), `--force` (bool), `--report-format` (`json|markdown`, default `json`), `--wave-size` (int, default 10), `--max-retries` (int, default 3).

### Task 3: Selection and taxonomy

Implement the selection step: load `DATABASE_URL`, open the pool (pgx stdlib), invoke the appropriate sqlc list query based on `force`, log how many postings were skipped because their latest snapshot had a null description. Load full taxonomy (canonical_roles, specializations, skills, role_dimensions) into in-memory maps keyed by slug. Taxonomy reload after each wave reuses these loaders.

### Task 4: Boilerplate stripping

Group selected postings by `company_id`. For each company with ≥3 entries, build the `{company_id, selected_ids}` JSON, exec `./bin/strip-boilerplate`, pipe stdin, capture stdout, parse the response, substitute `cleaned_text` into a per-posting working copy. Companies below threshold pass through unchanged. Non-zero exit from the child aborts the run.

### Task 5: Prompt template

Build the per-posting prompt as a Go string template. Inputs: posting id, title, description (cleaned if available), focus guidance (omitted if empty), the four taxonomy lists rendered as bullet lists, the agent contract block (schema + classification discipline + slug discipline + summary contract) copied verbatim from the current skill. Render `role_dimensions` under the exact heading `## Role dimensions (closed set)` — heading text is part of the agent contract.

Retain a separate template for the retry prompt: original prompt + an appended `## Retry guidance` block carrying the structured hint from the validator.

### Task 6: Agent dispatch loop

For each wave (slice of `WaveSize` postings), spawn goroutines bounded by a semaphore at `MaxParallelAgents`. Each goroutine: render prompt → `exec.Command("claude", "-p", prompt, "--output-format", "json", "--model", model)` → capture stdout → unmarshal the outer envelope → extract the textual response field → unmarshal that as the contracted classification JSON. Pass `ctx` through; cancel on shutdown signal so in-flight subprocesses exit.

Retry loop sits inside each goroutine: on parse or validation failure, build the retry prompt, re-invoke. Cap at `MaxRetries`. Final outcome per posting: `enriched`, `json_failed` (parse never succeeded), `validation_failed` (parse succeeded but never passed validation), `db_failed` (passed validation, transaction failed). The validator and the dispatcher share a single typed `Outcome` enum.

### Task 7: Validation

A `validate` file in `cmd/batch-enrich` exposes a single function: given a parsed classification, the in-memory taxonomy, and the in-memory cross-table slug index, return either a `ValidatedClassification` (ready for writeback) or a `ValidationFailure` with a `Reason` and `Hint` string. Reasons: invalid slug format, null byte in name/notes, seniority missing/invalid, empty dimensions array, unknown dimension slug, cross-table slug collision. All simultaneous failures are bundled into a single hint string appended to the retry prompt — write it as plain English the agent can act on.

Build the cross-table slug index at startup as `map[string]string` (slug → owning table name) covering `canonical_roles`, `specializations`, and `skills`. `role_dimensions` is excluded — closed-set validation catches unknown dimension slugs before writeback, so no cross-table check is needed. Update the index incrementally after each wave's writebacks so newly-added slugs participate in subsequent validations.

### Task 8: Writeback

For each `ValidatedClassification`, open a DB transaction, run `db.New(tx).GetOrCreateCanonicalRole / Specialization / Skill` per slug, capture returned ids, call `InsertCanonicalRoleDimension` for each (role, dimension) pair, call `InsertClassification` with `(posting_id, model, prompt_version, seniority, notes)` (notes as `sql.NullString`), capture the returned classification id, call the three join inserts, commit. On any error: rollback, mark the posting `db_failed`, continue. Writeback is serial across postings — postings within a wave queue up after the wave's agent calls all complete.

### Task 9: Run report and exit codes

Aggregate per-posting outcomes into a `RunReport` struct. Marshal to stdout as JSON or render as markdown per `--report-format`. Include: run params block, counts block, new taxonomy block (slugs+names introduced this run, partitioned by table), per-posting list (`posting_id`, title, outcome, summary, last failure reason if any), failures block (non-`enriched` outcomes plus the last raw response).

After emitting the report, append one JSON line per `json_failed` or `validation_failed` posting to `agent-output/batch-enrich/failures.jsonl`. Create the file if absent; never truncate prior lines.

Exit codes: 0 on a completed run (regardless of per-posting failures). Non-zero on: DB open / ping failure, missing `claude` binary in PATH, `./bin/strip-boilerplate` non-zero exit, invalid `PROMPT_VERSION`/`MODEL`, context cancellation that prevented the report from being emitted.

### Task 10: Tests

Cover at the seams. Fake the `claude` subprocess via an interface around `exec.Command` (or a function variable assignable in tests). Fake DB via the existing test pattern (Postgres integration test under `//go:build integration`) for at least: selection (force on/off, focus filter), full writeback for one posting (taxonomy upserts, classification insert, join inserts), serial-writeback ordering. Unit-test the validator against fixture JSON for each failure reason, including the cross-table collision case. Test the report's JSON shape against a small fixture run.

## Sequencing

**Phase 1 (sequential):** Task 1 — SQL queries and sqlc regen block every consumer.
**Phase 2 (concurrent):** Task 2 (config), Task 3 (selection / taxonomy), Task 5 (prompt template), Task 7 (validator). All consume only sqlc output from Phase 1; independent of each other.
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
- `Taxonomy` — four `map[string]int64` (slug → id) plus a `crossTable map[string]string` (slug → table name). Mutated under a single mutex as waves complete.
- `AgentResponse` — the parsed contracted JSON (mirrors the schema in the current skill).
- `ValidatedClassification` — `AgentResponse` plus resolved `[]int64` for role / specialization / skill / dimension ids. Output of Task 7.
- `Outcome` — string enum: `enriched`, `json_failed`, `validation_failed`, `db_failed`.
- `PostingResult` — `PostingID`, `Outcome`, `Title`, `Summary`, `LastReason`, `LastRawResponse`, `Attempts`.
- `RunReport` — top-level shape marshaled to stdout.

External shell: `exec.Command("claude", "-p", prompt, "--output-format", "json", "--model", model)`. The outer JSON returned by `claude -p --output-format json` is an envelope; the agent's text response sits inside. Unmarshal envelope → extract response text → unmarshal response text as `AgentResponse`. The exact field name (`result` or similar) must be confirmed at implementation time — see Open Questions.

DB plumbing: `cmd/batch-enrich` opens `sql.DB` exactly once (same pattern as `cmd/fetcher` and `cmd/strip-boilerplate`). All sqlc calls go through `db.New(pool)` or `db.New(tx)` inside a writeback transaction.

Subprocess plumbing: `exec.Command` is wrapped behind a small `agentRunner` interface (one method: `Run(ctx, prompt) (rawJSON []byte, err error)`) so tests can substitute a fake without touching `os/exec`. Default implementation runs `claude`; the test implementation returns canned fixtures.

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

## Open questions

1. **`claude -p --output-format json` envelope shape.** The exact field name carrying the agent's textual response inside the JSON envelope must be confirmed during implementation. Spec assumes one parse hop (`envelope.result` or similar) then a second JSON parse over the text. If the envelope returns the structured JSON directly (no inner string), the dispatcher simplifies — adjust then.
2. **Skill deprecation timing.** `.claude/skills/batch-enrich/SKILL.md` continues in parallel until the binary is proven against at least one full batch. Deletion or rewrite is a follow-on once the binary is exercised in cron.
