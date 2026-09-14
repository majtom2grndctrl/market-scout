# Requisition Identity

## Goal

Count postings and requisitions side by side; collapse only what the ATS itself identifies as one job. Some boards list a single requisition many times — one per location — so posting count overstates hiring for those companies. Extracting the ATS's own requisition key gives the read model a second honest denominator without discarding a single row.

## Scope

### In scope

- `requisition_key` column on `posting_snapshots`, populated by the three adapters whose platform distinguishes a job from a job post — Greenhouse, Workday, and Workable.
- Backfill of `requisition_key` over existing snapshots from `raw_data`, for those same three platforms.
- `posting_requisitions` read-model view mapping each posting to its requisition, falling back to posting identity when the platform exposes none.
- `count` measure returns requisition count alongside posting count, on the open cohort.

### Out of scope

- Collapsing rows at fetch time. The fetcher writes what the board returns; grouping is derived.
- Template fan-out as a column, grouping, or measure. Its only case is a company whose posting and requisition counts already agree, so the pair discharges the honesty contract without it. Description text also changes on 39.9% of postings during their observed life, so a content hash could not anchor a time series even if one were wanted.
- `normalized_title`. Title is not a grammar grouping, and template fan-out was its only consumer.
- HTML-entity normalization. Zero instances in the corpus.
- A `requisitions` vocabulary measure competing with `count`. One measure, two numbers.
- Repost detection. Separating repost from fan-out requires the requisition key this spec delivers, so it is sequenced after, and the first-observed-timestamp anchor is settled architecture.
- Workday and Workable per-posting description fetch. Deferred separately.
- Requisition counts on the `closed` and `all` cohorts. `posting_requisitions` resolves through the run-scoped current-snapshot lateral, which exists only for open postings; a closed posting would need a different derivation, unproven here. Those cohorts carry no `requisitions` key rather than a guess.

## Acceptance criteria

Each AC names how it is observed. Three are operator or review gates, not runnable tests — they are marked so, because an implementer reporting a pass on an unrunnable assertion is reporting nothing.

- [ ] Greenhouse and Workday adapters populate `requisition_key` from the platform identifier; Ashby, Lever, and Gem leave it nil. Workable populates it when `code` is a non-empty string and leaves it nil otherwise. *(`go test ./...`, adapter fixture tests.)*
- [ ] After backfill, every Greenhouse snapshot whose `raw_data` `internal_job_id` is a non-null number has a non-NULL `requisition_key`, and every snapshot where that key is JSON-null has `requisition_key` NULL. *(Operator gate: psql against both databases, result pasted into Task 1's completion report.)*
- [ ] `posting_requisitions` returns exactly one row per open posting, with `requisition_key` non-NULL on every row. *(Integration-tagged Go test under `apps/tools/internal/db/`, run with `go test -tags=integration ./...`.)*
- [ ] `posting_requisitions` sets `requisition_source` to `ats` when the platform supplied the key and to `posting` when the row fell back to posting identity. *(Same integration test.)*
- [ ] For a fixture Ashby or Lever company, every `posting_requisitions` row reports `requisition_source` of `posting`. *(Same integration test.)*
- [ ] For a fixture Greenhouse company with N open postings whose current snapshots carry M distinct non-NULL `requisition_key` values, `{measure: count, cohort: open, groupBy: [company]}` returns `value` N and `requisitions` M. Seed N=5, M=3. *(`pnpm test:db`.)*
- [ ] A `count` row over `cohort: open` carries a `requisitions` number alongside `value`; over `cohort: closed` and `cohort: all` it carries no `requisitions` key at all. *(`pnpm test:db`.)*
- [ ] A week-grouped `count` row has no `requisitions` key at all — absent, not null, not 0. *(`pnpm test:db`.)*
- [ ] `requisition_key` appears in neither `GROUPINGS` nor `FILTER_DIMENSIONS` in `apps/web/lib/composition/vocabulary.ts`. *(Review gate: coordinator reads the file. Negative existence — nothing runnable proves it.)*
- [ ] `migrate version` reports the same version with the dirty flag clear against both `market_scout` and `market_scout_test`. *(Operator gate: two shell invocations. This spec adds no routine, so the `executable_by_readonly` function-parity check in `developer-guide.md` §2 does not apply.)*

## Tasks

### Task 1: Schema and backfill

Add `requisition_key text` to `posting_snapshots` in a numbered migration, then backfill it from `raw_data` for existing rows.

- Backfill by joining through `companies.ats`, one branch per platform, extracting with `->>` so a JSON-null yields SQL NULL:
  - `greenhouse` → `nullif(raw_data->>'internal_job_id', '')`
  - `workday` → `nullif(raw_data->'bulletFields'->>0, '')`
  - `workable` → `nullif(raw_data->>'code', '')`
- Leave `ashby`, `lever`, and `gem` rows NULL. Those platforms expose no identifier distinct from posting identity.
- Do not sniff `raw_data` for the key instead of branching on `companies.ats`. Gem boards are Greenhouse-shaped and carry `internal_job_id` in their payload, so a sniffing backfill would populate Gem rows and break the NULL contract.
- Expect `internal_job_id` to be present-but-JSON-null on 356 Greenhouse snapshots across 20 postings. Those must land NULL, which `->>` gives you and `->` followed by a cast would not.
- Read `raw_data` in the backfill. It is a one-time repair over history, so it is the right source even though adapters own extraction going forward.
- Add no index. `idx_posting_snapshots_posting_fetched` on `(job_posting_id, fetched_at DESC)` already serves the current-snapshot lateral the read-model view uses, and a second index on an 84,500-row append-only table would be write amplification with no reader.
- Apply to both `market_scout` and `market_scout_test`. `migrate` reads `DATABASE_URL` only and lands in one database per run — see `developer-guide.md` §2.
- Paste two results into the completion report: `migrate version` from each database, and the backfill coverage query proving the second acceptance criterion.

Do not:
- Add `requisition_key` to `job_postings`. It is an observed field like `title`, and a board that reassigns it should leave history intact.
- Make `requisition_key` a generated column. A stored generated column may only reference columns in its own row, and the extraction path depends on `companies.ats`, two joins away.
- Hand-edit any file under `apps/tools/internal/db/` that sqlc generates — see `developer-guide.md` §5.8.

### Task 2: Adapter extraction

Add `RequisitionKey *string` to `domain.Posting`, populate it in the three adapters whose platform distinguishes a job from a job post, and carry it through the snapshot write path.

- Populate `RequisitionKey` in Greenhouse, Workday, and Workable only. Those three expose an identifier distinct from posting identity; the ATS's job-versus-job-post distinction is the only authority on what counts as one job.
- Leave `RequisitionKey` nil in Ashby, Lever, and Gem. All three already assign the platform's `id` to `SourceID` (`ashby.go:149`, `lever.go:213`, `gem.go:154`), so a key copied from it would assert a distinction the platform does not make.
- Greenhouse: add an `InternalJobID *int64` field tagged `json:"internal_job_id"` to the job struct in `apps/tools/internal/ats/greenhouse.go`, nil-check it, then format. A pointer is load-bearing — the key is present-but-JSON-null on 356 snapshots across 20 postings, and an `int64` would unmarshal those to `0` and format to the string `"0"`, a shared fake key merging 20 postings into one requisition.
- Workday: read the first element of the already-declared `BulletFields` slice on `wdJob` (`workday.go:73`), currently unread. Nil when the slice is empty; ignore any element beyond the first.
- Workable: add a `code` field to the job struct in `apps/tools/internal/ats/workable.go`, which parses only `shortcode` today. Expect nil on 96% of rows — `code` is non-empty on 20 of 498 snapshots, and 66 more carry it as an empty string.
- Treat an absent, empty, or JSON-null platform identifier as nil rather than an empty string, so the view's fallback branch stays distinguishable from a real key.
- Extract in the adapter, not from `raw_data` in SQL. Adapters own wire formats, and `apps/tools/internal/db/migrations/000019_workplace_type_derivation.up.sql` measured a 227.567ms median for the view that reads `raw_data`.
- Add `RequisitionKey: nullStr(p.RequisitionKey)` to `buildSnapshotParams` in `apps/tools/cmd/fetcher/main.go`, the sole construction site for `InsertPostingSnapshotParams` outside tests. Without it the change compiles clean and every row writes NULL — the adapters correct, the column present, the feature silently dead.
- Add the column to `InsertPostingSnapshot` in `apps/tools/internal/db/queries/fetcher.sql`, then run `sqlc generate` from `apps/tools/`. No call site breaks: every construction of the params struct uses named fields.
- Extend the `buildSnapshotParams` forwarding tests in `apps/tools/cmd/fetcher/main_test.go` to cover `RequisitionKey` nil and set. Those tests exist precisely so a new column cannot be silently dropped from the mapping.
- Add an adapter fixture test per platform asserting the extracted key, following the table-driven tests in `apps/tools/internal/ats/*_test.go`. Greenhouse and Workday fixtures already carry usable values.
- Record a new Workable fixture from the `seeq` board, which populates `code` on 5 of 51 postings — two of them sharing a code, so the fixture exercises fan-out and not just presence. The existing Workable fixture has `code` as JSON-null and tests only the nil half; the non-empty half is the one that produces a collapse. Do not hand-write the JSON — see `testing-guide.md` §4.

| Mirror | Don't mirror |
|---|---|
| `SourceID` — per-posting identifier, extracted in adapter, threaded through `domain.Posting` | `WorkplaceType` — has a SQL-side derivation layer; `RequisitionKey` has none |

Do not:
- Derive `RequisitionKey` from title, URL, or description. Only the platform's own identifier counts.
- Copy `SourceID` into `RequisitionKey` on any adapter. A key identical to posting identity carries no information and makes `requisition_source` a fiction.
- Use Greenhouse `requisition_id`. It is NULL on 57 postings and one board fills it with the literal string `See Opening ID`; `internal_job_id` is present on 100%.

### Task 3: Read-model view

Add `posting_requisitions` to the read model as a SQL view in a numbered migration.

Expose exactly these four columns, one row per open posting. Task 4 consumes them from a later phase and cannot read this text, so the names are contract:

| Column | Type | Meaning |
|---|---|---|
| `job_posting_id` | bigint | Join key against the measure cohort. |
| `company_id` | bigint | Scope for distinct-counting. Requisition keys are board-scoped, not global. |
| `requisition_key` | text | `COALESCE(current_snapshot.requisition_key, 'posting:' || open_postings.job_posting_id)`. Never NULL. |
| `requisition_source` | text | `ats` when the snapshot supplied the key, `posting` when it fell back. |

- Carry `company_id` so consumers count distinct over `(company_id, requisition_key)`. Greenhouse ids are board-scoped, Workday's are tenant-scoped, and Workable `code` is human-typed — live values are `2026-37`, `2026-92`, `SQ204`. There are no cross-company collisions among the current 5,700 keys, but two Workable boards numbering by year-week will collide on the first overlap, and an ungrouped count has no company boundary of its own.
- Prefix the fallback with `posting:` so a synthesized key can never collide with a real one.
- Derive "currently open" from `open_postings`, not by re-deriving fetch-run logic. That derivation lives in one place by design.
- Reach the current snapshot with the **run-scoped** lateral from `apps/tools/internal/db/migrations/000019_workplace_type_derivation.up.sql`: the lateral carries `AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id` and orders by `fetched_at DESC, id DESC`. Do not copy the lateral in `000017_read_model_views.up.sql` — it predates that predicate, and without it a snapshot from a later failed run can supply the row. The bug is invisible in a green fixture test.
- Mirror `workplace_type_source` in that same migration for the shape of `requisition_source`. Absent a source column, a fallback is indistinguishable from a real key.
- Add an integration-tagged test under `apps/tools/internal/db/` covering one row per open posting, `requisition_key` non-NULL on every row, both `requisition_source` values, and an Ashby or Lever company reporting `posting` throughout. Run with `go test -tags=integration ./...`. This task owns those assertions; Task 4's tests cover the measure, not the view.
- Apply to both `market_scout` and `market_scout_test`. `migrate` reads `DATABASE_URL` only and lands in one database per run — see `developer-guide.md` §2.
- Re-run `internal/db/setup/readonly_role.sql` against both databases after the migration. The script's `ALTER DEFAULT PRIVILEGES` already grants SELECT on a view created by the migration owner, so this reconfirms rather than repairs — a passing pre-check is expected, not evidence the grant is broken.

### Task 4: Measure engine consumer

Return a per-row requisition count for the `count` measure, joining `posting_requisitions` into the aggregate query.

- Add `readonly requisitions?: number` to `MeasureRow` in `apps/web/lib/db/measure-engine.ts`. Optional, because `measure-distributions.ts` also produces `MeasureRow` for `age` and `lifespan` and will not set it.
- Set the field by conditional spread, mirroring how `gap` is attached in that same file. A signal the row cannot supply is an absent key, never 0 and never null, so "0 real" stays distinct from "0 unknown."
- Emit `requisitions` only when `composition.measure === "count"` **and** `composition.cohort === "open"`. `posting_requisitions` covers open postings only: an inner join under another cohort would silently change `value` itself, and a left join would report a fabricated 0.
- Join `posting_requisitions` on `job_posting_id` in `selectAggregateRows` in `apps/web/lib/db/measure-aggregates.ts`, threading the new number through the `grouped` and `selected` CTEs and the outer projection, and widening the `AggregateSqlRow` interface. Count distinct over `(company_id, requisition_key)`, not the key alone — requisition keys are board-scoped, so a bare distinct count can merge two companies.
- Count over the same fanned rows `value` counts. `scope.joins` expands a posting into one row per taxonomy term when grouping by market, role, skill, specialization, or function, and `value` is `count(*)` over that expansion; a distinct count over the same rows keeps both numbers on one denominator.
- Leave `requisitions` unset on every row from `selectWeeklyAggregateRows`, the path week-grouped `count` takes. `posting_requisitions` resolves against the current snapshot and cannot answer a historical week.
- Do not extend `createMeasureScope` in `apps/web/lib/db/measure-scope.ts`. It builds the cohort and filter fragments shared by every measure, and a join valid for one measure and one cohort does not belong there.
- Update the six existing `toEqual` assertions on `count` rows in `apps/web/lib/db/measure-engine.db.test.ts`. They are exact-match and break the moment a `requisitions` key appears; a red suite here is expected, not a regression you introduced. Do not switch them to `toMatchObject` — exact matching is what pins the absent-versus-zero contract.
- Add `.db.test.ts` cases for a Greenhouse fan-out company, a NULL-key company, a `cohort: closed` row asserting `requisitions` is absent, and a week-grouped row asserting the same. Seed each as a fixture: these suites run against `market_scout_test`, which holds no production rows.
- Confirm `apps/web/lib/chart/shaping.ts` and `apps/web/components/chart/composition-chart.tsx` still compile — both read `MeasureRow`/`MeasureResult`. The widening is optional so neither should break. Gate on `pnpm typecheck`; Vitest does not typecheck.

Do not:
- Add `requisition_key` or `requisitions` to `GROUPINGS` or `FILTER_DIMENSIONS` in `apps/web/lib/composition/vocabulary.ts`. The vocabulary is closed by design, and a second count the agent could group on would let it pick whichever number tells the better story.
- Emit `requisitions` as 0 when the number is unavailable. Absent and zero mean different things.

## Sequencing

**Phase 1 (sequential):** Task 1 — migration `000027`. Every later task reads the new column.
**Phase 2 (sequential):** Task 3 — migration `000028`. Kept apart from Task 2 because `sqlc.yaml` reads `internal/db/migrations/` as its schema source, so a half-written migration would poison Task 2's generated output.
**Phase 3 (concurrent):** Task 2, Task 4 — Go adapters against TypeScript measure engine, no shared files. Task 2 runs `sqlc generate` here, after both migrations have landed.

## Boundary inventory

Snapshot column and its wire sources:

| Name | Go struct field | ATS source key | SQL column |
|---|---|---|---|
| Requisition key | `RequisitionKey` | Greenhouse `internal_job_id`; Workday `bulletFields[0]`; Workable `code`. Ashby, Lever, and Gem supply none — nil. | `posting_snapshots.requisition_key` |

`posting_requisitions` exposes `job_posting_id`, `company_id`, `requisition_key`, and `requisition_source`. The contract is restated inside both Task 3 and Task 4, since neither can read the other's text.

Measure-engine field:

| Name | TypeScript field | JSON key | Source |
|---|---|---|---|
| Requisition count | `MeasureRow.requisitions?: number` | `"requisitions"` | `count(DISTINCT (company_id, requisition_key))` over the cohort join |

## Rough sketch

`posting_requisitions` resolves `COALESCE(current_snapshot.requisition_key, 'posting:' || job_posting_id)` so a fallback can never collide with a real key, with `requisition_source` set to `ats` or `posting` accordingly, over the run-scoped current-snapshot lateral from `000019`.

`count` gains the second number only on `{measure: count, cohort: open}`. Every other composition — other cohorts, other measures, week-grouped counts — leaves the key off the row entirely.

## Open questions

None. Every fork is decided above.

## Follow-on, once this ships

- Whether `requisition_key` should anchor repost detection. Greenhouse `internal_job_id` is stable across every multi-snapshot posting measured — 0 of 4,992 changed — unlike the ATS timestamps the architecture distrusts. Repost and fan-out cannot be separated until this spec ships, and the first-observed-timestamp anchor is settled architecture, so this is a later discussion with evidence rather than a decision this spec waits on. Measurements in `research.md`.

## Known gaps outside this spec

- Supio (Gem) has never been fetched: zero fetch runs since it was added 2026-08-14, no error rows, adapter registered 2026-08-15. A fetch-pipeline bug, unrelated to this spec — Gem writes no `requisition_key` either way, so nothing here depends on it.
