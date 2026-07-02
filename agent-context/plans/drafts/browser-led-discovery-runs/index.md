# Browser-Led Discovery Runs

## Goal

Define the browser-led discovery run: the durable record of one scouting session in which an agent discovers companies from a web source, investigates them in a browser, and onboards the supported ones. This is the foundation the later multi-company-page and recent-news milestones build on. It adds two things that don't exist today: a **dedup preflight** that filters candidate names against the tracked set before browser work, and an **append-only run ledger** that records source inputs, candidate records, provenance, statuses, and a run summary.

`add_company` remains the sole gate that writes a company into the fetcher set. A discovery run does not onboard companies; it records what a session found and what happened to each candidate.

## Scope

### In scope

- A `dedup_candidates` read-only MCP preflight: batch-classify candidate names (and optional `(ats, board_token)`) as new / duplicate / stale, using the `watchlist.md` §Dedup rules.
- Two append-only tables for run persistence: one run row per session, one candidate row per discovered company.
- A `save_discovery_run` action MCP tool that persists a finished run (source input + all candidates) in one call through an approved `mcp` function, and returns the run summary.
- A `discovery_run_summary` read-only MCP tool that returns a prior run's source, timing, and candidate counts by status.
- Candidate status taxonomy aligned with `watchlist.md`, plus discovery-specific `onboarded` and `pending`.
- Provenance: every candidate carries the run it was found in and the source URL it was seen at.
- `watchlist.md` gains a "Browser-led discovery run" section documenting the loop.
- Unit tests for dedup classification, run-save validation and response mapping, and summary rollup; integration tests for the action-role write boundary on the new function.

### Out of scope

- Driving the browser. The agent supplies discovered names and observed URLs; no crawling, web search, or headless automation lives in Go or MCP. (`discovery-source-recipes` milestone.)
- Consuming runs in the web app / multi-company pages. (Later milestone.)
- Reconciling onboarded companies back into the seed file. `add_company`'s existing seed follow-up stays the only signal; source-file writeback is `stage-company-seed-patch`.
- Merging stale DB rows. `stale-needs-merge` is recorded and flagged, never auto-resolved.
- Changing `add_company` or `detect_ats`. Companies still onboard live through `add_company`; URL evidence still parses through `detect_ats`.
- Converging the research-list sidecar (`cmd/onboard`) into discovery runs. Runs are the new general path for interactive browser sourcing; the frozen-list sidecar stays. Both share the `watchlist.md` dedup rules and status taxonomy as one source of truth.
- Mutating a saved run. A run is an immutable ledger of one session; re-investigating a company is a new run with a new candidate row.
- New ATS adapter support.

## Acceptance criteria

- [ ] `dedup_candidates` accepts a batch of candidates (each `name` required; `ats`, `board_token`, `recency_days` optional) and returns one verdict per candidate without opening a browser, making HTTP requests, or writing to the DB.
- [ ] A candidate with a supplied `(ats, board_token)` that matches a tracked company with a snapshot inside the recency window returns `duplicate`; a match with no recent snapshot returns `stale`.
- [ ] A candidate matched only by normalized name (punctuation and whitespace stripped, lowercased, equality — substring is not a match) returns `stale` with the matched company surfaced for inspection, never a silent `duplicate`.
- [ ] A candidate with no `(ats, board_token)` match and no normalized-name match returns `new`.
- [ ] `recency_days` defaults to 30 when omitted; the same normalization drives both `dedup_candidates` and any name comparison, from one shared definition.
- [ ] `save_discovery_run` persists one run row plus one candidate row per supplied candidate through an approved `mcp` function, and returns the run id and summary. Repeated calls append new runs; they never update or delete prior runs or candidates.
- [ ] A saved candidate carries its run id, source URL provenance, status, verbatim source metadata, and — when `status = onboarded` — the `company_id` `add_company` produced.
- [ ] `save_discovery_run` rejects an unknown `source_kind`, an unknown candidate `status`, an empty candidate `name`, and a `company_id` that does not exist, returning an `ok=false` envelope with `errors[]` in the shared `path`/`code`/`message` shape — not an MCP transport error.
- [ ] `save_discovery_run` runs on the action pool and reaches the DB only through the approved `mcp` function; the action role cannot insert into the discovery tables directly. Integration tests assert the direct-write denial and that repeated calls append rows.
- [ ] `discovery_run_summary` returns a run's `source_kind`, source reference, start/finish timing, total candidate count, and counts broken down by status, using the read-only pool.
- [ ] The new `mcp` function follows the established pattern: `SECURITY DEFINER SET search_path = pg_catalog`, fully-qualified app objects, `REVOKE ALL ... FROM PUBLIC` in the migration, and an explicit `EXECUTE` grant block added to `action_role.sql`.
- [ ] `watchlist.md` documents the browser-led discovery loop: dedup preflight → browser investigation → `detect_ats` → `add_company` → `save_discovery_run`, and states the run is an append-only session ledger.
- [ ] From `apps/tools`: `go test ./cmd/mcp -count=1` and `go test ./...` pass; `go build ./...`, `go vet ./...`, and `staticcheck ./...` report no new issues.
- [ ] From `apps/tools`: `go test -tags=integration ./cmd/mcp -count=1` proves the new action-role boundary when `DATABASE_URL`, `DATABASE_URL_RO`, and `DATABASE_URL_ACTIONS` are set.

## Tasks

### Task 1: Discovery schema migration

Add migration `000013` creating two tables. `discovery_runs`: identity, `source_kind` (checked enum: `company-list-page`, `news-article`, `manual`), an optional source URL, an optional source note/title, `started_at`, and a nullable `finished_at`. `discovery_candidates`: identity, a `run_id` FK to `discovery_runs` (`ON DELETE RESTRICT`, consistent with the repo's other FKs), `name`, an optional `source_url` (provenance), a `status` checked enum (the taxonomy in the Boundary Inventory), nullable `ats` and `board_token`, a nullable `company_id` FK to `companies` (set when `onboarded`), a `metadata jsonb` column holding verbatim source fields, an optional `notes`, and `created_at`. Both tables are append-only — no update path. Add the down migration. Grants to the action role come in Task 3, not here.

### Task 2: Dedup preflight tool

Add a `dedup_candidates` read-only MCP tool bound to `pools.readOnly`. It takes a batch of candidates and an optional `recency_days` (default 30) and returns a verdict per candidate. For a candidate with `(ats, board_token)`, reuse the existing `(ats, board_token)` + recency logic (`FindCompanyDedupStatus` in `apps/tools/internal/db/queries/onboard.sql`): match with a recent snapshot → `duplicate`; match without → `stale`. For a candidate without a token, run a normalized-name match against `companies`; a name match → `stale` with the matched company surfaced; no match → `new`. Normalization is the `watchlist.md` §Dedup rule, defined once and shared (SQL expression or Go helper — implementor's call, but a single source of truth). Add whatever sqlc query the name match needs; do not fork the existing `(ats, board_token)` query. No DB writes, no probes.

### Task 3: Run persistence and save action

Add migration `000014` creating an approved `mcp` function that inserts one `discovery_runs` row and its `discovery_candidates` rows in a single statement, returning the run id and a status-count summary as `jsonb` (a scalar return, so a sqlc-generated call fits — cf. `save_enrichment`). Follow the approved-function pattern in full (see AC). Add the `EXECUTE` grant block to `apps/tools/internal/db/setup/action_role.sql`. Add the sqlc input/query for the call, regenerate sqlc, commit input and generated output together.

Add a `save_discovery_run` action MCP tool bound to `pools.action`. It validates the payload (source-kind enum, per-candidate status enum and non-empty name, `company_id` existence for onboarded candidates — the existence check may use the read-only pool, cf. `save_enrichment`), then calls the approved function. Validation, DB, and marshalling failures return an `ok=false` envelope with `errors[]` in the shared `path`/`code`/`message` shape; reserve MCP transport errors for undecodable calls. Response echoes the run id and the summary.

### Task 4: Run summary tool

Add a `discovery_run_summary` read-only MCP tool bound to `pools.readOnly`, taking a run id and returning the run's source input, timing, total candidate count, and per-status counts. Add the supporting read-only sqlc query. The save action and this tool should return the same summary shape so a caller can rely on one structure.

### Task 5: Document the loop

Add a "Browser-led discovery run" section to `agent-context/lib/watchlist.md`. Cover: the agent discovers candidates from a source; calls `dedup_candidates` to drop already-tracked ones before browser work; investigates survivors, using `detect_ats` then `add_company` to onboard supported boards; and calls `save_discovery_run` once at the end to record the session. State that the run is an append-only ledger, that `add_company` stays the onboarding gate, and that seed-file drift is still surfaced by `add_company`'s follow-up. Keep the existing sidecar and live-onboard sections intact; add a one-line pointer distinguishing the three paths (batch sidecar, one-off live onboard, browser-led run).

### Task 6: Tests

Unit tests: dedup verdict classification across the token-match, name-match, and no-match paths and the recency boundary; `save_discovery_run` payload validation (bad source_kind, bad status, empty name, nonexistent company_id) and response mapping against a fake saver; summary rollup counts. Integration tests (skip when any DSN is absent): the action role saves a run through the approved function; the action role cannot insert into `discovery_runs` / `discovery_candidates` directly; repeated saves append runs without mutating prior ones; the read-only role cannot write. Keep the per-tool testable-deps seam (`xHandlerWithDeps`) the other MCP tools use.

## Sequencing

**Phase 1 (sequential):** Task 1 — the schema every persistence task consumes. Task 2 reads only existing tables and could start here too, but gating it on Phase 1 keeps the tree building at each phase boundary.
**Phase 2 (concurrent):** Task 2, Task 3, Task 4 — dedup preflight, run save, and summary are independent once the schema exists; Task 3 and Task 4 agree on the summary shape (Boundary Inventory) rather than on each other's code.
**Phase 3 (sequential):** Task 5, Task 6 — docs and the full test/integration sweep after the surfaces exist.

## Rough sketch

Mirror the existing MCP tool conventions exactly (see `research.md`): `mcp.NewTool` + `s.AddTool`, per-tool request DTO decoded with `BindArguments`, `ok=false` JSON envelopes for all tool-level failures, and inline `pool*`-backed seams wired by `xHandler(pool)` into `xHandlerWithDeps(...)` for tests. Read-only tools (`dedup_candidates`, `discovery_run_summary`) take `pools.readOnly`; `save_discovery_run` takes `pools.action` (plus `pools.readOnly` for the `company_id` existence check).

Reuse `main.actionError` for envelopes. The approved function follows `mcp.save_enrichment`: `LANGUAGE plpgsql`, `SECURITY DEFINER SET search_path = pg_catalog`, fully-qualified objects, scalar `jsonb` return, trailing `REVOKE ALL ... FROM PUBLIC`; the `EXECUTE` grant goes in `action_role.sql`. A run is written whole and never updated — the append-only ledger is the safety rail, as with `save_enrichment`.

`dedup_candidates` batches cheap read queries; a bounded loop or a single `unnest`-based query both work — no fan-out concurrency needed (see developer-guide §5.6: concurrency is for outbound HTTP waits, not DB reads).

## Boundary inventory

Candidate status enum (SQL `CHECK`, JSON `status`): `pending`, `onboarded`, `duplicate`, `stale-needs-merge`, `unsupported-ats`, `no-careers`, `invalid-token`, `dead`. The middle six align with `watchlist.md`; `pending` (discovered, unresolved when the session ended) and `onboarded` (passed `add_company`, `company_id` set) are discovery-specific.

Source kind enum (SQL `CHECK`, JSON `source_kind`): `company-list-page`, `news-article`, `manual`.

Dedup verdict (JSON `verdict`, tool-only, not persisted): `new`, `duplicate`, `stale`.

| Name | Go field | JSON key | SQL column |
|---|---|---|---|
| Run id | `RunID` | `"run_id"` | `discovery_runs.id` |
| Source kind | `SourceKind` | `"source_kind"` | `discovery_runs.source_kind` |
| Source URL | `SourceURL` | `"source_url"` | `discovery_runs.source_url` |
| Source note | `SourceNote` | `"source_note"` | `discovery_runs.source_note` |
| Started at | `StartedAt` | `"started_at"` | `discovery_runs.started_at` |
| Finished at | `FinishedAt` | `"finished_at"` | `discovery_runs.finished_at` |
| Candidate name | `Name` | `"name"` | `discovery_candidates.name` |
| Candidate source URL | `SourceURL` | `"source_url"` | `discovery_candidates.source_url` |
| Status | `Status` | `"status"` | `discovery_candidates.status` |
| ATS | `ATS` | `"ats"` | `discovery_candidates.ats` |
| Board token | `BoardToken` | `"board_token"` | `discovery_candidates.board_token` |
| Company id | `CompanyID` | `"company_id"` | `discovery_candidates.company_id` |
| Metadata | `Metadata` | `"metadata"` | `discovery_candidates.metadata` |
| Notes | `Notes` | `"notes"` | `discovery_candidates.notes` |
| Recency days | `RecencyDays` | `"recency_days"` | (param) `FindCompanyDedupStatus.recency_days` |
| Verdict | `Verdict` | `"verdict"` | — (computed) |
| Summary counts | `Counts` | `"counts"` | — (aggregate) |
| Error | (`actionError`) | `"path"`/`"code"`/`"message"` | — |

`dedup_candidates` uses generic `source_url`/`name` on input candidates that need no persistence; `save_discovery_run` uses the same keys, mapped to columns. The reuse is deliberate — a caller passes near-identical candidate objects to both tools.

## Open questions

1. **Foundation weight.** This drafts the full DB-backed foundation (persistence + preflight). If a thinner first cut is wanted, Task 2 (`dedup_candidates`) is independently valuable and could ship alone, deferring the run ledger (Tasks 1, 3, 4) until browser sourcing is exercised and its shape is proven. Flagged because the user was away when the storage/scope forks were decided; defaults were DB-backed persistence + build-the-preflight.
2. **Candidate uniqueness within a run.** Should `(run_id, normalized name)` be unique, or may a run legitimately list the same name twice (e.g. seen in two articles)? Lean: no uniqueness constraint — a run is a raw ledger and duplicate sightings are themselves signal; dedup is the preflight's job, not the table's.
3. **`pending` in a saved run.** Allowed, so a session that ran out of budget can still be recorded honestly. Alternative: forbid non-terminal statuses at save time and force the agent to resolve every candidate first. Lean: allow `pending` — the ledger should reflect reality, including incomplete sessions.
4. **Summary as query vs. column.** The run summary is computed by aggregating candidates on read (no denormalized counts column), keeping the tables append-only and drift-free. Confirm this over caching counts on `discovery_runs`.
