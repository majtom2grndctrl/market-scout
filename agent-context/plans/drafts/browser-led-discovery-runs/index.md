# Browser-Led Discovery Runs

## Goal

Remove the friction in browser-led company sourcing: before an agent spends browser time investigating discovered companies, it should be able to drop the ones already tracked in one call. This spec builds that **dedup preflight** (`dedup_candidates`) and records — but does not yet build — the run-ledger contract (source inputs, candidate records, provenance, statuses, summaries) that the `discovery-source-recipes` milestone will materialize when it has a concrete consumer.

`add_company` remains the sole gate that writes a company into the fetcher set. `detect_ats` remains the URL-evidence parser. This spec adds one read-only preflight in front of that loop; it persists nothing.

## Scope

### In scope

- A `dedup_candidates` read-only MCP tool: batch-classify candidates (each `name`, optional `ats`/`board_token`) as `new` / `duplicate` / `stale`, using the `watchlist.md` §Dedup rules.
- The supporting sqlc query for normalized-name matching against `companies`. Reuse the existing `(ats, board_token)` + recency query, extending it only additively (one column for the matched name); do not duplicate it into a second query.
- A "Browser-led discovery run" section in `watchlist.md` documenting the loop and where the preflight sits.
- A written **deferred run-ledger contract** (see the section below) so the shape is decided once and inherited by the recipes milestone.
- Unit tests for dedup classification across every verdict path, including the bool→verdict mapping for both recency states.

### Out of scope

- **The run ledger itself.** No `discovery_runs` / `discovery_candidates` tables, no `save_discovery_run` action, no summary tool, no `mcp` function, no action-role grants. Deferred to `discovery-source-recipes`, which will have the first real consumer and can validate the shape. The contract is recorded here so that milestone doesn't re-decide it. Rationale: a persisted session ledger has no current consumer (multi-company pages and recent-news scouting are later milestones), and a single-operator tool gains little from provenance record-keeping the operator already remembers. Building it now would freeze a schema with nothing to validate it against.
- Driving the browser. No crawling, web search, or headless automation in Go or MCP. (`discovery-source-recipes`.)
- Consuming discovery data in the web app / multi-company pages. (Later milestone.)
- Seed-file reconciliation. `add_company`'s existing follow-up stays the only drift signal; source-file writeback is `stage-company-seed-patch`.
- Merging stale DB rows. The preflight flags `stale`; it never auto-resolves.
- Changing `add_company` or `detect_ats`.
- Converging the research-list sidecar (`cmd/onboard`) into this path. The sidecar stays the batch verifier; both paths share the `watchlist.md` dedup rules and status taxonomy as one source of truth.
- New ATS adapter support.

## Acceptance criteria

- [ ] `dedup_candidates` accepts a batch of candidates (each `name` required; `ats`, `board_token` optional) plus a separate top-level, optional `recency_days` (default 30) applied to the whole batch, and returns one verdict per candidate without opening a browser, making HTTP requests, or writing to the DB. It binds the read-only pool.
- [ ] A candidate with a supplied `(ats, board_token)` matching a tracked company with a snapshot inside the recency window returns `duplicate`; a match with no recent snapshot returns `stale`, with the matched company surfaced.
- [ ] A candidate matched only by normalized name (punctuation and whitespace stripped, lowercased, equality — substring is not a match) returns `stale` with the matched company surfaced, never a silent `duplicate`.
- [ ] A candidate with no `(ats, board_token)` match and no normalized-name match returns `new`.
- [ ] `recency_days` defaults to 30 when omitted. Name normalization has one shared definition; the tool does not carry a second copy of the rule.
- [ ] Malformed input (empty `name`) fails the whole batch: the tool returns `ok=false`, an empty/absent `results`, and one `actionError` per offending candidate — `path` in the indexed form `candidates[i].name` (following `save_enrichment`'s `canonical_roles[0].slug` precedent), `code` a stable snake_case code, `message` a human hint. Not an MCP transport error.
- [ ] `watchlist.md` documents the browser-led loop: `dedup_candidates` → browser investigation of survivors → `detect_ats` → `add_company`, and distinguishes the three sourcing paths (batch sidecar, one-off live onboard, browser-led preflight).
- [ ] From `apps/tools`: `go test ./cmd/mcp -count=1` and `go test ./...` pass; `go build ./...`, `go vet ./...`, and `staticcheck ./...` report no new issues.

## Tasks

### Task 1: Dedup preflight tool

Add a `dedup_candidates` read-only MCP tool bound to `pools.readOnly`, following `enrichment_preview`'s tool conventions (`mcp.NewTool` + `s.AddTool`, request DTO via `BindArguments`, `ok=false` envelope for tool-level failures, a `pool*`-backed seam wired by `dedupCandidatesHandler(pool)` into a `dedupCandidatesHandlerWithDeps` body for tests) — `enrichment_preview` is the only existing tool with this ok-envelope-plus-seam shape; `detect_ats` is DB-free and out of scope as a model here. `"dedup_candidates"` is the MCP tool string only — it stays snake_case; the Go identifiers are camelCase. It takes a batch of candidates and a top-level optional `recency_days *int` (default 30, following `previewRequest.Count *int`), returns a verdict per candidate. Empty `name` is semantic validation after a successful `BindArguments`, returned as an `ok=false` envelope — exactly as `enrichment_preview` handles an out-of-range `count`; a genuinely undecodable tool call still returns an MCP transport error.

Verdict precedence: if `(ats, board_token)` is supplied and matches a tracked company, reuse `FindCompanyDedupStatus` (`apps/tools/internal/db/queries/onboard.sql`) — a recent snapshot → `duplicate`; no recent snapshot → `stale`. Otherwise (no token supplied, or a token supplied but no `(ats, board_token)` match), run a normalized-name match against `companies`: a name match → `stale` with the matched company surfaced; no match → `new`. The name-match `:many` query also computes `has_recent_snapshot` via the same recency `EXISTS` subquery `FindCompanyDedupStatus` uses, so `Matched.has_recent_snapshot` is populated on both branches — the name branch still always returns `stale` regardless of recency; the field is informational. Normalization is the `watchlist.md` §Dedup rule, expressed once and shared (SQL expression or a Go helper — implementor's call, but a single source of truth). Add the name-match sqlc query. `FindCompanyDedupStatusRow` currently returns only `company_id` and `has_recent_snapshot` — no name — so the matched company can't be surfaced on the token branch as-is; extend it only additively — add the matched company `name` to its select list — rather than duplicating it into a second query, then regenerate sqlc. No writes, no probes, no fan-out concurrency (these are cheap read queries — developer-guide §5.6).

### Task 2: Document the preflight

Add a "Browser-led discovery run" section to `agent-context/lib/watchlist.md`. Cover the loop: discover candidates from a source; call `dedup_candidates` to drop tracked companies before browser work; investigate survivors with `detect_ats` then `add_company`. State that `add_company` stays the onboarding gate and that seed drift is still surfaced by its follow-up. Add a one-line pointer distinguishing the three sourcing paths. Keep the existing sidecar and live-onboard sections intact. Do not document the deferred run ledger here — it is not built.

### Task 3: Tests

Unit tests for dedup verdict classification: the `(ats, board_token)` duplicate and stale paths, the name-match `stale` path, and the no-match `new` path. The recency boundary itself lives in SQL (the reused query's `EXISTS` subquery) and is exercised there, not re-asserted at the Go layer; the Go-layer coverage is the bool→verdict mapping for both recency states via the fake seam (`has_recent_snapshot=true` → `duplicate`, `false` → `stale`). Cover empty-`name` rejection and response mapping against a fake dedup source, using the `...WithDeps` seam.

## Sequencing

**Phase 1 (sequential):** Task 1 — the tool and its query.
**Phase 2 (concurrent):** Task 2, Task 3 — docs and tests are independent once the tool's behavior is settled.

## Rough sketch

Mirror `enrichment_preview` — the model for both the `ok=false` envelope and the `pool*`-backed seam (`detect_ats` is a general read-only-tool reference only: it is DB-free, has no `pool*` seam, and uses a `status`/`errors` envelope with no `ok` field): `mcp.NewTool` registration in `newMCPServer`, a request DTO decoded with `BindArguments`, a `pool*`-backed seam (`poolDedupSource{pool}`) satisfying a small interface so tests inject a fake, and `ok=false` JSON envelopes reusing `main.actionError` for the empty-`name` case. Read path only — bind `pools.readOnly`.

The `(ats, board_token)` branch is a direct reuse of `FindCompanyDedupStatus`. The name branch needs one new `:many` query matching normalized company name; a single `unnest`-based query or a bounded per-name loop both work. Keep normalization in one place — if SQL does it (`lower(regexp_replace(name, '[^[:alnum:]]', '', 'g'))`), the Go side passes raw names and never re-implements the rule.

## Boundary inventory

Dedup verdict (JSON `verdict`, tool-only, not persisted): `new`, `duplicate`, `stale`.

Top-level wire shape mirrors `previewEnvelope`: `Ok` (`"ok"`, bool), `Results` (`"results"`, an array of per-candidate result objects — each carrying the `Name`/`Verdict`/`Matched` fields below), and `Errors` (`"errors"`, `[]actionError`). The table below describes the fields of one `results` element unless noted otherwise.

| Name | Go field | JSON key | Source |
|---|---|---|---|
| Candidate name | `Name` | `"name"` | input |
| ATS | `ATS` | `"ats"` | input, optional |
| Board token | `BoardToken` | `"board_token"` | input, optional |
| Recency days | `RecencyDays *int` | `"recency_days"` | top-level batch input (not per-candidate), optional — a pointer so an omitted value (default 30) is distinguishable from an explicit `0`, following `previewRequest.Count *int`; converts to `int32` at the `FindCompanyDedupStatusParams.RecencyDays` boundary (the generated param is `int32`) |
| Verdict | `Verdict` | `"verdict"` | output, computed |
| Matched company | `Matched` | `"matched"` | output; `id`/`name`/`ats`/`board_token`/`has_recent_snapshot` when matched, else null — `name` from the added `FindCompanyDedupStatus` column on the token branch (from the name-match query directly on the name branch); `ats`/`board_token` may echo the input |
| Error | (`actionError`) | `"path"`/`"code"`/`"message"` | output on failure; one per offending candidate, `path` indexed as `candidates[i].name` (fails the whole batch — `results` empty/absent) |

## Deferred: run-ledger contract (defined, not built)

Recorded so `discovery-source-recipes` materializes one agreed shape instead of re-deciding. Not implemented by this spec. When built, it follows the append-only + approved-`mcp`-function pattern (`save_enrichment`): a run written whole and never updated; a re-investigation is a new run, not a mutation.

- **Source input** (per run): `source_kind` ∈ `company-list-page`, `news-article`, `manual`; an optional source URL; an optional note.
- **Candidate record** (per candidate): `name`; optional `source_url` provenance (the page/article it was seen at); `status`; optional `ats` / `board_token`; optional `company_id` (set when `onboarded`); verbatim source `metadata`; optional `notes`.
- **Status taxonomy**: the `watchlist.md` set — `duplicate`, `stale-needs-merge`, `unsupported-ats`, `no-careers`, `invalid-token`, `dead` — plus discovery-specific `onboarded` (passed `add_company`) and `pending` (discovered, unresolved when the session ended; allowed so incomplete sessions record honestly).
- **Provenance**: every candidate carries its run and its source URL.
- **Run summary**: source input, start/finish timing, total candidate count, and counts by status — computed by aggregating candidates on read, not denormalized onto the run row.

## Open questions

1. **Normalization location** — SQL expression vs. a Go helper. Lean SQL, so the input array passes raw and the rule has exactly one implementation (the branch that already normalizes in the DB). Confirm at implementation.
2. **Batch size bound** — should `dedup_candidates` cap the candidate count (as `enrichment_preview` caps `count` at 100)? Lean: yes, a generous cap (e.g. 200) to bound a single read; decide the number when the first recipe shows realistic batch sizes.
