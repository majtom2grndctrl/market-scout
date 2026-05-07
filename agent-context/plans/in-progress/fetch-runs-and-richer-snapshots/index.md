# Fetch Runs and Richer Snapshots

## Goal

Capture more queryable signal from each fetch without changing the fetch path's external contract. Three additions, one spec:

1. **`fetch_runs` table** — track each per-company fetch as a first-class record so trend queries can distinguish "posting removed" from "we didn't fetch" from "fetch failed."
2. **`description_text` column on `posting_snapshots`** — pull the description out of `raw_data` into a column so it's queryable and ready for embeddings later.
3. **Structured compensation columns on `posting_snapshots`** — capture salary ranges from sources that expose them as structured data.

## Context

The fetcher works end-to-end across all three adapters. Snapshots are accumulating. The shape of what we're storing is solid (append-only, raw_data preserved, identity vs observation cleanly split), but three gaps limit what trend queries can answer or what the AI knowledge-store goal can build on:

- **No fetch-run record.** Today, `posting_snapshots.fetched_at` is the only fetch-side trace. If Anthropic's fetch fails partway and Stripe's succeeds, there's no way to tell — both look like "we observed N postings on date X." That breaks the load-bearing trend question "did this posting still exist on date X?", because absence has two indistinguishable causes (posting removed vs we didn't see it).
- **Description sits in raw_data.** All three adapters preserve full wire payload, but the description body is the single highest-value text per posting (semantic search, role classification, comp extraction from prose). Querying it through `raw_data->>'content'` works but the path differs per adapter and isn't indexable cheaply.
- **Compensation hidden in raw_data.** Lever exposes `salaryRange` as a structured object on at least some boards; Greenhouse exposes pay ranges via `pay_input_ranges` or `metadata` entries; Ashby exposes `compensation` (tiered). For a market-intel tool, comp is among the highest-signal columns. Hidden today.

These three are bundled because they share a migration cadence, the same touch points (adapter → `domain.Posting` → fetcher write path → snapshot row), and the same testing pattern. Splitting them would mean three near-identical PRs.

## Design decisions

### `fetch_runs` granularity is per-company-per-invocation

The load-bearing trend question is "was posting X observed during a successful fetch of company Y on date Z?" That requires a per-company record. An outer "fetcher invocation" wrapping all per-company runs is convenient grouping but not load-bearing for the trend question.

**Decision: ship per-company `fetch_runs` only. Defer invocation-level grouping until a query asks for it.**

Schema:

```
fetch_runs (
    id              bigserial PK,
    company_id      bigint NOT NULL REFERENCES companies(id),
    started_at      timestamptz NOT NULL,
    completed_at    timestamptz,                    -- NULL while in_progress
    status          text NOT NULL CHECK (status IN ('in_progress','success','failed')),
    error_message   text,                           -- populated iff status = 'failed'; enforced at writer boundary, not by DB constraint
    postings_count  integer                         -- count observed; NULL while in_progress
)
```

`posting_snapshots` gains `fetch_run_id bigint REFERENCES fetch_runs(id)`. Every snapshot belongs to exactly one run; every successful run produces ≥0 snapshots.

**Why no `invocation_id` yet:** the fetcher today runs all companies in one process invocation. If the cron split changes (per-company crons, staggered schedules, manual runs), an invocation grouping becomes interesting. Until then, "all runs with `started_at` within a few seconds" is a fine ad-hoc query.

### Failure semantics: row exists, status = 'failed'

When a company's fetch fails (HTTP error, parse error, sanity-ceiling trip), the `fetch_runs` row is still written with `status = 'failed'` and `error_message` populated. Snapshots from a failed run are not written (the existing per-company error isolation already aborts the fetch on first error).

This means: a failed run produces a `fetch_runs` row with zero linked snapshots. That's the signal "we tried, we have a record of trying, but observed nothing." Distinct from "no row" (we didn't try).

**`postings_count` is set only on success.** On failure it stays NULL. Cheap denormalization that avoids `count(*)` joins on the hot trend path.

### Description: one text column, plain text only

Goal-aligned: the AI-knowledge-store goal needs embedding-ready text; semantic search wants tokens, not markup. HTML structure (lists, headers) is preserved verbatim in `raw_data` already, so a separate `description_html` column would be duplicative.

**Decision: one `description_text` column on `posting_snapshots`, nullable. HTML stripped at the adapter boundary using a minimal sanitizer.**

Per adapter:

| Source | Wire field | Conversion |
|---|---|---|
| Greenhouse | `content` (HTML, sometimes entity-encoded) | unescape entities → strip tags → collapse whitespace |
| Lever | `descriptionPlain` if present; else strip from `description` HTML | prefer plain when source provides it |
| Ashby | `descriptionHtml` (HTML) | strip tags → collapse whitespace |

Null when the adapter cannot produce a non-empty plain-text result. Don't fabricate.

**Sanitizer choice:** stdlib + a small helper, not an external dep. The conversion is "remove `<...>`, decode `&amp;` / `&lt;` / etc., collapse runs of whitespace." If real-world descriptions reveal richer markup that the simple stripper mangles, swap in `golang.org/x/net/html` for proper tokenization. Start simple.

**Length cap:** none in this spec. If pathological multi-MB descriptions appear, add a cap then. The 32 MiB body cap in `httpFetch` already bounds the worst case.

### Compensation: capture only what's structured at the source

Goal-aligned: trend analysis compounds over time. Garbage data poisons trends. Parsing comp from prose ("competitive salary," "$150k–$200k DOE") produces noise that looks like signal. Only structured fields get normalized columns.

**Schema:**

```
compensation_min       bigint,        -- whole currency units, no minor units
compensation_max       bigint,        -- inclusive upper bound
compensation_currency  text,          -- ISO 4217, 3-letter uppercase
compensation_period    text CHECK (compensation_period IN ('hour','day','week','month','year'))
```

All four nullable. The presence rule: when any of the four is non-null, the others should be too — but the schema doesn't enforce this (a CHECK like "all-or-nothing" rules out partial-info cases like "currency known, range unknown" that may surface later). Validation lives at the adapter boundary.

**Whole units, not minor units.** Salaries are quoted in dollars, not cents. `bigint` accommodates currencies with no minor unit (JPY, KRW) and lets the column type stay uniform across currencies without a `*_minor` suffix that implies cents universally.

**Per-adapter:**

| Source | Field | Notes |
|---|---|---|
| Lever | `salaryRange.{min, max, currency, interval}` | `interval` maps `"per-year-salary" → "year"`, `"per-hour-wage" → "hour"`, etc. via a small alias map; unknown intervals warn and produce NULL. |
| Greenhouse | `pay_input_ranges[0]` if present, else skip | Some boards expose this; many don't. Capture when available, NULL otherwise. |
| Ashby | `compensation.compensationTierSummary` | Free-text summary string, not structured min/max. **Skip in this spec.** Leave Ashby comp NULL — extraction is a future job, possibly LLM-driven. |

**Why skip Ashby comp:** the structured field is a tier summary string, not numeric min/max. Parsing it is prose extraction in disguise. Defer until either (a) Ashby exposes a numeric range field we missed, or (b) the LLM-extraction job lands as a separate spec.

**Currency normalization:** the wire string is uppercased and trimmed. Unknown formats (non-3-letter, lowercase variants) warn and produce NULL across all four columns rather than persisting partial data.

### Adapter contract: domain.Posting grows, contract style stays

`domain.Posting` adds:

```go
DescriptionText      *string
CompensationMin      *int64
CompensationMax      *int64
CompensationCurrency *string
CompensationPeriod   *string
```

`*int64` for compensation values to match the schema's `bigint` and the existing nullable-pointer convention. Adapters set all four comp fields together or none — partial sets are an adapter-side bug.

`fetch_run_id` does **not** belong on `domain.Posting`. The adapter doesn't know which run it's part of; the fetcher writer assigns it at insert time.

### Fetcher orchestration: bracket each company

Today, `cmd/fetcher` per-company flow is roughly: dispatch adapter → receive `[]domain.Posting` → write snapshots inside a transaction → log result. The change:

1. **Before** dispatching the adapter: insert a `fetch_runs` row with `status = 'in_progress'`, `started_at = now()`. Capture the returned id. This insert runs in its own short transaction that commits immediately, before the adapter call — so a mid-fetch crash leaves the row visible as `in_progress`.
2. **On adapter success:** open a *new* transaction for snapshot inserts, update the run row to `status = 'success'`, `completed_at = now()`, `postings_count = len(postings)`. Snapshots are inserted with `fetch_run_id` set.
3. **On adapter failure:** in a separate short transaction (the snapshot transaction never opened), update the run row to `status = 'failed'`, `completed_at = now()`, `error_message = err.Error()`. Use a fresh context derived from the parent `ctx` with a short timeout (e.g., 5 seconds) — not `workCtx`, which may already be expired if the failure was a timeout. If `ctx` itself is cancelled (shutdown), skip the failure update and let the orphan-reaper handle it.

**Why insert the run row before the adapter call:** if the process crashes mid-fetch, the row remains with `status = 'in_progress'` and no `completed_at`. A startup sweep (or a manual query) can identify orphans. Without this, a crash leaves no trace at all.

`started_at` and `completed_at` are Go-side `time.Now().UTC()` values passed as query parameters, matching the existing `fetched_at` convention.

**Why bundle the success update with the snapshot insert in one transaction:** snapshots and their owning run row commit atomically. No window where snapshots exist but the run still says `in_progress`.

**Crash recovery:** out of scope here. An `in_progress` row older than N minutes is an orphan; sweeping it is a separate, simple job. The spec just defends the invariant that orphans are *visible*.

### Migration ordering

Three logical changes, but one migration file. Splitting them across migrations means an intermediate state where (e.g.) `description_text` exists but `fetch_runs` doesn't, or vice versa — no intrinsic value, more files to apply. Single migration: `000005_fetch_runs_and_snapshot_enrichment.up.sql`.

The migration:
1. Creates `fetch_runs` (table + status check).
2. Adds `posting_snapshots.fetch_run_id` as **nullable** initially.
3. Adds `posting_snapshots.description_text`, `compensation_min`, `compensation_max`, `compensation_currency`, `compensation_period`.
4. Does **not** backfill existing snapshots' `fetch_run_id`.

The "existing snapshots have NULL `fetch_run_id`" question is in Open questions.

### Loose coupling: every column answers a query

| Column | Trend query it serves |
|---|---|
| `fetch_runs.status` | "Did we successfully fetch company X on date Y?" |
| `fetch_runs.completed_at` | "How long did each company's fetch take? Are any boards getting slower?" |
| `posting_snapshots.fetch_run_id` | "Of the postings observed in the last successful run for company X, which ones disappeared in the next successful run?" — i.e., removal detection. |
| `description_text` | "Find postings mentioning 'agentic'." (Embedding-ready feed for the future knowledge-store layer.) |
| `compensation_min/max/currency/period` | "How does senior backend comp range across AI startups vs large tech?" |

## Scope

### In scope

- Migration `000005_fetch_runs_and_snapshot_enrichment.{up,down}.sql` creating `fetch_runs` and adding the six new columns to `posting_snapshots`.
- New SQL queries: `InsertFetchRun`, `MarkFetchRunSuccess` (sets status, completed_at, postings_count), `MarkFetchRunFailed` (sets status, completed_at, error_message). Regenerate sqlc.
- Update `InsertPostingSnapshot` to write `fetch_run_id`, `description_text`, and the four compensation columns. Append all six to the column list and parameter list to keep existing positional parameters stable.
- Five new optional fields on `domain.Posting` (description + four comp).
- HTML-to-plain-text helper in `internal/ats` (package-private). Stdlib only. Single function: takes HTML bytes, returns trimmed plain text, returns empty string if input is empty or sanitizes to empty.
- Greenhouse adapter: populate `DescriptionText` from `content`. Populate compensation from `pay_input_ranges[0]` when present.
- Lever adapter: populate `DescriptionText` from `descriptionPlain` (preferred) or stripped `description`. Populate compensation from `salaryRange` when present, with interval alias map and warn on unknown intervals.
- Ashby adapter: populate `DescriptionText` from stripped `descriptionHtml`. Compensation: leave NULL (deferred).
- Fetcher write path orchestration: per-company `fetch_runs` row inserted before adapter call, finalized on success (in same tx as snapshot inserts) or failure (separate short tx).
- `cmd/fetcher/main.go` flow change to thread `fetch_run_id` into `buildSnapshotParams`.
- Per-adapter unit tests covering description extraction (with and without HTML), compensation parsing (Lever full set, Greenhouse range, Ashby NULL), unknown wire interval → warn + NULL, partial comp data → all-NULL with warn.
- Fetcher write-path test asserting that a successful run produces snapshots with non-NULL `fetch_run_id` and a `fetch_runs` row in `success`, and that a failed adapter call produces a `fetch_runs` row in `failed` with no linked snapshots.
- Documentation update to `agent-context/lib/project.md` adding `fetch_runs` to the snapshot/storage section: "Every fetch is recorded as a `fetch_runs` row; snapshots link to their run. Distinguishes 'posting removed' from 'fetch failed' from 'we didn't fetch.'"

### Out of scope

- Backfilling `fetch_run_id` on existing snapshots. Old rows stay NULL; trend queries handle NULL by either treating pre-fetch_runs snapshots as a single synthetic run or filtering them out. (See Open questions.)
- Crash-recovery sweep for orphan `in_progress` runs. The spec ensures orphans are *visible*; reaping them is a separate, simple cron.
- Invocation-level grouping (one outer id wrapping all per-company runs in a fetcher invocation). Defer until a query asks for it.
- LLM-driven extraction of comp from description prose. Future spec.
- Ashby compensation extraction. Stays NULL until either the wire shape changes or LLM extraction lands.
- A `description_html` column. Plain text covers embedding/search; HTML stays in `raw_data` for any UI need.
- Indexes on the new columns. Add when a query needs one.
- Vector embedding column. Different spec entirely.
- Repost detection logic. Schema supports it via `fetch_run_id` + gap detection; logic ships separately.

## Acceptance criteria

- `\d fetch_runs` after migration shows the columns enumerated above. Status check enforces the three values.
- `\d posting_snapshots` shows the new `fetch_run_id` (nullable bigint, FK to fetch_runs), `description_text`, and four compensation columns.
- Down migration drops the six columns and the `fetch_runs` table cleanly. Data loss is acceptable (pre-1.0 personal tool).
- A fetcher run against the watchlist produces one `fetch_runs` row per company, all with `status = 'success'`, `completed_at` non-NULL, `postings_count` matching the snapshot count.
- Forcing a Lever fetch failure (e.g., point at a 404 board token) produces one `fetch_runs` row with `status = 'failed'`, `error_message` populated, and **zero** snapshots linked to that run.
- A Greenhouse fixture with `content: "<p>Hello <b>world</b></p>"` produces `DescriptionText = "Hello world"`.
- A Lever fixture with `descriptionPlain: "Plain text"` and `description: "<p>HTML version</p>"` uses the plain string verbatim.
- A Lever fixture with no `descriptionPlain` and `description: "<p>HTML</p>"` produces `DescriptionText = "HTML"`.
- An Ashby fixture with `descriptionHtml: "<p>Role overview</p>"` produces `DescriptionText = "Role overview"`.
- A Lever fixture with `salaryRange: {min: 150000, max: 200000, currency: "USD", interval: "per-year-salary"}` produces `CompensationMin = 150000`, `CompensationMax = 200000`, `CompensationCurrency = "USD"`, `CompensationPeriod = "year"`.
- A Lever fixture with `salaryRange.interval = "per-fortnight"` (unknown) produces all four compensation fields NULL plus one `[lever] unknown interval` warn line.
- A Greenhouse fixture using the field names confirmed by the Task 4 spike produces `CompensationMin`, `CompensationMax`, `CompensationCurrency`, `CompensationPeriod` with correct values. If the spike finds minor-unit fields, the adapter divides by the appropriate factor before storing whole units.
- An Ashby fixture has `CompensationMin/Max/Currency/Period` all NULL regardless of `compensation` content (deferred).
- `go build ./...`, `go vet ./...`, `staticcheck ./...`, and `go test ./...` pass.
- Re-running `sqlc generate` against committed SQL produces no diff.
- `agent-context/lib/project.md` mentions `fetch_runs` in the storage section.

## Tasks

### Task 1 — Schema migration

Write `000005_fetch_runs_and_snapshot_enrichment.up.sql` and the down migration. Up creates `fetch_runs` (with the status check), adds `fetch_run_id bigint REFERENCES fetch_runs(id)` (nullable), and adds the five enrichment columns to `posting_snapshots` (`description_text text`, `compensation_min bigint`, `compensation_max bigint`, `compensation_currency text`, `compensation_period text` with check). Down drops them. Apply locally; verify with `\d`. Down migration must drop the new `posting_snapshots` columns first (removing the FK reference), then drop the `fetch_runs` table.

### Task 2 — SQL queries + sqlc regeneration

Add `InsertFetchRun(company_id, started_at) returns id`, `MarkFetchRunSuccess(id, completed_at, postings_count)`, `MarkFetchRunFailed(id, completed_at, error_message)`. Extend `InsertPostingSnapshot` to write the six new columns. Regenerate sqlc; commit the diff. New columns append at parameter positions $15–$20 in this order: `fetch_run_id`, `description_text`, `compensation_min`, `compensation_max`, `compensation_currency`, `compensation_period`. Do not reorder existing columns; `location_texts` at $14 stays in place.

### Task 3 — Domain field additions

Add `DescriptionText *string`, `CompensationMin *int64`, `CompensationMax *int64`, `CompensationCurrency *string`, `CompensationPeriod *string` to `domain.Posting`. Add a doc comment to the compensation fields stating the all-or-nothing invariant: adapters set all four together or none.

### Task 4 — HTML-to-plain-text helper + Greenhouse compensation spike

Add a package-private helper `htmlToPlainText(s string) string` in `internal/ats`. Stdlib only: decode common HTML entities (`&amp;`, `&lt;`, `&gt;`, `&quot;`, `&#39;`, numeric entities `&#NNN;`), strip tags via a regex or single-pass byte scan, collapse internal whitespace runs to single spaces, trim. Returns empty string for empty or all-markup input. Adapters set `DescriptionText = nil` when the helper returns an empty string; never set `DescriptionText` to a pointer to an empty string.

Spike Greenhouse's compensation field: fetch one job from a board known to expose pay (Stripe is a likely candidate). Confirm the field names (`pay_input_ranges`, `metadata` array entries with `value_type: "currency_range"`, etc.). Document findings inline in the adapter file before Task 5.

### Task 5 — Adapter updates

For each adapter, add a `decode*Description` step and (where in scope) `decode*Compensation` step that populate the new domain fields.

- **Greenhouse:** Add `Content string \`json:"content"\`` and `PayInputRanges []json.RawMessage \`json:"pay_input_ranges"\`` to the `ghJob` wire struct (exact sub-struct fields confirmed by Task 4 spike). (The adapter already fetches `?content=true`; this is a struct and parse change only, not a URL change.) `DescriptionText` from `content` via `htmlToPlainText`. Compensation from the field path confirmed in Task 4 spike.
- **Lever:** Add `Description string \`json:"description"\``, `DescriptionPlain string \`json:"descriptionPlain"\``, and `SalaryRange *leverSalaryRange \`json:"salaryRange"\`` to the `leverJob` wire struct, where `leverSalaryRange` is a new private struct `{ Min int64 \`json:"min"\`; Max int64 \`json:"max"\`; Currency string \`json:"currency"\`; Interval string \`json:"interval"\` }`. Use a pointer so wire-absent vs zero is distinguishable. `DescriptionText` from `descriptionPlain` if non-empty; else `htmlToPlainText(description)`. Compensation from `salaryRange.{min, max, currency, interval}`. Interval alias map: `"per-year-salary" → "year"`, `"per-month-salary" → "month"`, `"per-week-salary" → "week"`, `"per-day-wage" → "day"`, `"per-hour-wage" → "hour"`. These values are provisional based on prior observation; extend the map from `[lever] unknown interval` warns seen in live fetcher output. Unknown intervals warn (`[lever] unknown interval`) and produce all-NULL compensation.
- **Ashby:** Add `DescriptionHtml string \`json:"descriptionHtml"\`` to the `ashbyJob` wire struct. `DescriptionText` from `htmlToPlainText(descriptionHtml)`. Compensation: leave NULL. Add a comment stating the deferral and citing this spec.

### Task 6 — Fetcher orchestration

Modify `cmd/fetcher/main.go` per-company flow:

1. Insert `fetch_runs` row (`status = 'in_progress'`, `started_at = now()`). Capture id.
2. Call adapter.
3. On success: open the existing snapshot-write transaction, insert all snapshots with `fetch_run_id` set, update the run row to `success` + `completed_at` + `postings_count`, commit.
4. On failure: separate short transaction marks the run `failed` + `completed_at` + `error_message`. No snapshot writes.

Update `buildSnapshotParams` to the signature `(jobPostingID int64, fetchRunID int64, fetchedAt time.Time, p domain.Posting)`. The five new domain fields (`DescriptionText`, comp fields) are read from `p`; only `fetchRunID` is new as an explicit parameter. Add `nullInt64(v *int64) sql.NullInt64` helper alongside `nullStr` and `nullTime` in `cmd/fetcher/main.go`. fetch_runs lifecycle log lines (inserting a run, marking success/failed) use the `[fetcher]` subsystem tag.

### Task 7 — Tests

Per-adapter tests for description extraction and (where applicable) compensation parsing, against `httptest.Server` fixtures. Reuse existing testdata patterns. Add fixtures with: HTML-only description, plain+HTML description (Lever), structured compensation (Lever full, Greenhouse range), unknown-interval compensation (Lever warn case), partial-compensation wire data (asserts all-NULL output).

Fetcher write-path test: stand up a stub adapter that returns N postings, assert one `fetch_runs` row in `success` with `postings_count = N` and N snapshots all carrying that `fetch_run_id`. A second case where the stub returns an error asserts one `fetch_runs` row in `failed` and zero snapshots.

### Task 8 — Documentation

Update `agent-context/lib/project.md` snapshot/storage section: add a sentence on `fetch_runs` as the bracketing record for each per-company fetch, with the trend-query motivation. One paragraph; this is durable architecture.

## Sequencing

Task 1 first. Tasks 2 and 3 can proceed after 1. Task 4 is independent and can run in parallel with 1–3. Task 5 depends on 3 and 4. Task 6 depends on 2, 3, 5. Task 7 depends on 5, 6. Task 8 is independent.

Recommended order: 1 → 4 (parallel with 2, 3) → 5 → 6 → 7 → 8.

## Open questions

- **Existing snapshots have NULL `fetch_run_id`.**  Resolved: option (a). `fetch_run_id` is nullable permanently; pre-fetch_runs snapshots carry NULL. Trend queries must handle NULL by filtering or treating NULLs as pre-history. The column will not be tightened to NOT NULL without a separate backfill spec.
- **Greenhouse compensation field shape.** Pinned by the Task 4 spike. If Stripe / Anthropic / Figma all return empty here, the Greenhouse compensation pathway ships dormant (helper code in place, no live data) and waits for a board that exposes it.
- **Sanitizer fidelity.** The simple regex/byte-scan approach mishandles edge cases (script tags, CDATA, unbalanced tags). If real fixtures show this corrupts text in load-bearing ways, swap to `golang.org/x/net/html` tokenization. Decide after one round of real-fetcher output.
- **`error_message` size.** No cap in this spec. If a wrapped error message ever runs to KBs (it shouldn't — current wrapped errors are ~200 chars), add a truncation at the writer boundary.
- **In-progress orphan reaping.** Out of scope here, but worth noting: a startup sweep that marks `in_progress` runs older than N minutes as `failed` with `error_message = "orphaned (process crashed)"` would be a one-query follow-up. Not blocking this spec.
