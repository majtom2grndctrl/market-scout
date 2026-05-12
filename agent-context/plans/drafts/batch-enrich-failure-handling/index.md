# Batch-Enrich Failure Handling

## Goal

Close three gaps surfaced by the 2026-05-12-1203 run: a missing `sales` role dimension caused a valid classification to be rejected and likely contributed to two JSON-malformation failures on sales postings; failed agent attempts leave no machine-readable trace; recurring failures on the same posting are invisible until a human re-reads multiple report markdown files.

## Scope

### In scope

- New migration: insert `sales` into `role_dimensions` (idempotent).
- Batch-enrich skill: up to 3 JSON-repair retry attempts when a Haiku agent returns unparseable JSON.
- Batch-enrich skill: append one JSONL line per failed attempt to a single rolling file `agent-output/batch-enrich/failures.jsonl`.
- Batch-enrich skill: run report includes a "Recurring failures" section listing postings that failed in ≥2 distinct runs across all history.

### Out of scope

- Retrying validation failures (unknown dimension, missing seniority, bad slug). Re-selection on the next non-`--force` run is the existing handling and stays.
- DB-side tracking of attempts. Failures live in files; the DB stays canonical-knowledge-only. This is a deliberate choice — see Rough sketch.
- Pruning or rotation policy for `failures.jsonl`. Deferred until file size becomes a problem.
- Backfilling failure data for the three already-failed postings (14, 23, 25). They will be re-selected naturally.
- Removing or renaming any existing `role_dimensions` row.
- Persistence of `skills[].requirement` and `summary` — unchanged, still report-only.

## Acceptance criteria

- [ ] Running `migrate up` against a fresh DB produces a `role_dimensions` row with slug `sales`; running it against an existing DB that already contains `sales` is a no-op.
- [ ] After the migration, the batch-enrich orchestrator's in-memory dimension set (loaded in skill step 4) contains `sales`, and a Haiku agent emitting a canonical role with `"dimensions": ["sales"]` clears Phase A validation.
- [ ] When a Haiku agent returns unparseable JSON, the orchestrator issues up to 3 repair-prompt retries against the same agent before marking the posting failed. The first parseable response wins; the posting proceeds to Phase A validation normally.
- [ ] Every failed agent attempt — the initial call and every repair retry — appends exactly one JSON object as a single line to `agent-output/batch-enrich/failures.jsonl`. Each line contains: `run_timestamp`, `posting_id`, `attempt` (1-based across the run for that posting), `outcome` (one of `json_failed`, `validation_failed`), `reason` (short string), `raw_response` (string, full text), `prompt_version`, `model`, `attempted_at` (ISO-8601). The file is created on first failure if it does not exist; it is never truncated or rewritten.
- [ ] The run report includes a "Recurring failures" section. The section is computed by scanning `failures.jsonl` and counting distinct `(posting_id, run_timestamp)` pairs per `posting_id`. Postings with ≥2 distinct failed runs (across the current run and all history) appear in the section with their `posting_id`, total failed-run count, and the set of distinct `reason` strings observed across all attempts. Postings absent from `failures.jsonl` and postings with only 1 failed run are omitted.
- [ ] When `failures.jsonl` does not exist, the "Recurring failures" section is present in the report but reads "(none)". The skill does not error.
- [ ] A successful posting — including one that succeeded only after retries — produces zero lines in `failures.jsonl`. Successes are not logged there.

## Tasks

### Task 1: Migration — add `sales` dimension

New migration pair under `internal/db/migrations/`. Up migration inserts the row with `ON CONFLICT (slug) DO NOTHING`. Down migration deletes the row by slug; rely on the existing `RESTRICT` FK on `canonical_role_dimensions` to surface any historical references rather than silently breaking them.

### Task 2: Batch-enrich skill — retry, log, surface

Single cohesive edit to `.claude/skills/batch-enrich/SKILL.md`. Three behavior changes that all touch the orchestrator's failure path:

1. **Repair retries.** Step 6 ("Dispatch waves") and step 7 ("Agent contract") gain a retry loop: when an agent's response fails JSON parse, re-prompt the same agent with the original task plus the prior response and a "your previous output was invalid JSON, return only the corrected JSON" directive. Cap at 3 retries (so up to 4 total attempts per posting). First parseable response wins and proceeds to Phase A validation. Exhausted retries → posting marked `json_failed`.

2. **JSONL failure logging.** New subsection (logical home: between step 8 writeback and step 9 report). Specify the file path, the line schema, append-only semantics, and what triggers a write (every failed attempt: JSON parse failures across all retry attempts, and Phase A validation failures — one line each). Validation failures are one line per posting because Phase A runs once after the agent ultimately succeeds at returning parseable JSON.

3. **Recurring failures in report.** Step 9 ("Report") gains a new section above "Per-Posting Summaries". Section logic: read `failures.jsonl` (tolerate missing file), group by `posting_id`, count distinct `run_timestamp` values per posting, render postings with count ≥2.

## Sequencing

**Phase 1 (concurrent):** Task 1, Task 2 — independent files (`internal/db/migrations/` vs `.claude/skills/batch-enrich/SKILL.md`), no shared contracts beyond the agreed dimension slug `sales`.

## Rough sketch

**Why files not DB for failures.** The DB is the canonical knowledge store (`project.md` §The database as AI agent knowledge store). Every row is intended as trustworthy signal for downstream agents and queries. Failure metadata is process telemetry — high-volume, noisy, useful for debugging the enrichment pipeline itself but not for any classification consumer. Files give natural pruning (`rm` directories), append-only semantics matching the data shape, and zero schema commitment. The only query we actually want — "postings that failed twice in a row" — is a 10-line scan of JSONL, not a SQL query worth a migration.

**JSONL over per-attempt files.** One rolling `failures.jsonl` keeps trend analysis trivial (`jq -r .reason failures.jsonl | sort | uniq -c`) and survives tool-call inspection from future sessions. Per-attempt files were the earlier sketch and got walked back — N files don't scan, JSONL does.

**Repair retry prompt.** The skill should not over-specify wording. A short directive plus the prior raw output is enough; the implementer picks the exact phrasing. The key constraint is that retries reuse the same agent context (same posting, same taxonomy, same focus) so the corrected response is comparable.

**"Distinct failed runs" not "distinct failed attempts."** Three retries within one run that all fail is one failed run for that posting, not three. The metric is whether the posting can be enriched at all in a sitting. JSONL has one line per attempt; the report counts `DISTINCT (posting_id, run_timestamp)`.

**Append-only failures.jsonl.** No truncation, no rotation, no per-run file. One file for the project. If it grows to megabytes in a year, splitting by month is a five-minute follow-up.

## Open questions

None blocking. Two deferred items captured as out-of-scope: failures.jsonl pruning policy, and whether validation-failure retries are worth adding later (today's re-selection-on-next-run path is fine).
