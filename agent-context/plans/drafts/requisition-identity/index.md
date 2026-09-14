# Requisition Identity

## Goal

Count postings and requisitions side by side; collapse only what the ATS itself identifies as one job. Some boards list a single requisition many times — one per location — so posting count overstates hiring for those companies. Extracting the ATS's own requisition key gives the read model a second honest denominator without discarding a single row.

## Scope

### In scope

- `requisition_key` column on `posting_snapshots`, populated by the three adapters whose platform distinguishes a job from a job post — Greenhouse, Workday, and Workable.
- Backfill of `requisition_key` over existing snapshots from `raw_data`, for those same three platforms.
- `posting_requisitions` read-model view mapping each posting to its requisition, falling back to posting identity when the platform exposes none.
- `count` measure returns requisition count alongside posting count.

### Out of scope

- Collapsing rows at fetch time. The fetcher writes what the board returns; grouping is derived.
- Template fan-out as a column, grouping, or measure. Its only case is a company whose posting and requisition counts already agree, so the pair discharges the honesty contract without it. Description text also changes on 39.9% of postings during their observed life, so a content hash could not anchor a time series even if one were wanted.
- `normalized_title`. Title is not a grammar grouping, and template fan-out was its only consumer.
- HTML-entity normalization. Zero instances in the corpus.
- A `requisitions` vocabulary measure competing with `count`. One measure, two numbers.
- Repost detection. Separating repost from fan-out requires the requisition key this spec delivers, so it is sequenced after, and the first-observed-timestamp anchor is settled architecture.
- Workday and Workable per-posting description fetch. Deferred separately.

## Acceptance criteria

- [ ] Greenhouse, Workday, and Workable adapters populate `requisition_key` when the platform supplies the identifier; Ashby, Lever, and Gem leave it NULL.
- [ ] After backfill, every Greenhouse snapshot carrying `internal_job_id` in `raw_data` has a non-NULL `requisition_key`.
- [ ] `posting_requisitions` returns exactly one row per open posting.
- [ ] `posting_requisitions` sets `requisition_source` to `ats` when the platform supplied the key and to `posting` when the row fell back to posting identity.
- [ ] An Ashby, Lever, or Gem company reports `requisition_source` of `posting` on every row, and equal posting and requisition counts.
- [ ] A Greenhouse company with two or more open postings sharing one `internal_job_id` reports a requisition count lower than its posting count, by exactly the number of surplus postings in each shared requisition.
- [ ] A `count` measure row carries a `requisitions` number alongside `value`.
- [ ] A week-grouped `count` row carries `requisitions` as null, never 0, so an absent number stays distinct from a real zero.
- [ ] `requisition_key` appears in neither `GROUPINGS` nor `FILTER_DIMENSIONS` in `apps/web/lib/composition/vocabulary.ts`.
- [ ] `migrate version` reports the same version with the dirty flag clear against both `market_scout` and `market_scout_test`.

## Tasks

### Task 1: Schema and backfill

Add `requisition_key text` to `posting_snapshots` in a numbered migration, then backfill it from `raw_data` for existing rows.

- Add a composite index on `(job_posting_id, requisition_key)`. The read-model view resolves the current snapshot per posting, so posting id leads.
- Backfill Greenhouse, Workday, and Workable rows only, from the wire keys in the Boundary inventory below, joining through `companies.ats` to pick the path. Ashby, Lever, and Gem expose no key distinct from posting identity, so their rows stay NULL.
- Read `raw_data` in the backfill. It is a one-time repair over history, so it is the right source even though adapters own extraction going forward.
- Apply to both `market_scout` and `market_scout_test`. `migrate` reads `DATABASE_URL` only and lands in one database per run — see `developer-guide.md` §2.

Do not:
- Add `requisition_key` to `job_postings`. It is an observed field like `title`, and a board that reassigns it should leave history intact.
- Make `requisition_key` a generated column. A stored generated column may only reference columns in its own row, and the extraction path depends on `companies.ats`, two joins away.
- Hand-edit any file under `apps/tools/internal/db/` that sqlc generates — see `developer-guide.md` §5.8.

### Task 2: Adapter extraction

Add `RequisitionKey *string` to `domain.Posting`, populate it in the three adapters whose platform distinguishes a job from a job post, and carry it through the snapshot write path.

- Populate `RequisitionKey` in Greenhouse, Workday, and Workable only. Those three expose an identifier distinct from posting identity; the ATS's job-versus-job-post distinction is the only authority on what counts as one job.
- Leave `RequisitionKey` nil in Ashby, Lever, and Gem. All three already assign the platform's `id` to `SourceID` (`ashby.go:149`, `lever.go:213`, `gem.go:154`), so a key copied from it would assert a distinction the platform does not make.
- Add an `InternalJobID int64` field tagged `json:"internal_job_id"` to the Greenhouse job struct in `apps/tools/internal/ats/greenhouse.go`, and convert it to string. The adapter parses only `id` today, which is the job-post id already used for `SourceID`.
- Add a `code` field to the Workable job struct in `apps/tools/internal/ats/workable.go`, which parses only `shortcode` today. Expect `requisition_key` NULL on roughly 83% of Workable rows — `code` is non-null on 86 of 498 snapshots.
- Treat an absent, empty, or null platform identifier as nil rather than an empty string, so the view's fallback branch stays distinguishable from a real key. Workday's `bulletFields` may be an empty array.
- Extract in the adapter, not from `raw_data` in SQL. Adapters own wire formats, and `apps/tools/internal/db/migrations/000019_workplace_type_derivation.up.sql` measured a 227.567ms median for the view that reads `raw_data`.
- Add the column to `InsertPostingSnapshot` in `apps/tools/internal/db/queries/fetcher.sql`, then run `sqlc generate` from `apps/tools/`.
- Add an adapter fixture test per platform asserting the extracted key, following the existing table-driven tests in `apps/tools/internal/ats/*_test.go`.

| Mirror | Don't mirror |
|---|---|
| `SourceID` — per-posting identifier, extracted in adapter, threaded through `domain.Posting` | `WorkplaceType` — has a SQL-side derivation layer; `RequisitionKey` has none |

Do not:
- Derive `RequisitionKey` from title, URL, or description. Only the platform's own identifier counts.
- Copy `SourceID` into `RequisitionKey` on any adapter. A key identical to posting identity carries no information and makes `requisition_source` a fiction.
- Use Greenhouse `requisition_id`. It is NULL on 57 postings and one board fills it with the literal string `See Opening ID`; `internal_job_id` is present on 100%.

### Task 3: Read-model view

Add `posting_requisitions` to the read model as a SQL view in a numbered migration.

- Key the view on the current snapshot's `requisition_key`, falling back to a posting-identity value when NULL, so every open posting yields exactly one row and a NULL-key platform degrades to one requisition per posting.
- Carry a `requisition_source` column naming which path produced the key, mirroring `workplace_type_source` in `apps/tools/internal/db/migrations/000019_workplace_type_derivation.up.sql`. Absent a source column, a fallback is indistinguishable from a real key.
- Derive "currently open" from `open_postings`, not by re-deriving fetch-run logic. That derivation lives in one place by design. `open_postings` carries only `job_posting_id` and `fetch_run_id`, so reach the current snapshot through the run-scoped lateral join that `open_postings_display` uses in `apps/tools/internal/db/migrations/000017_read_model_views.up.sql`.
- Apply to both `market_scout` and `market_scout_test`. `migrate` reads `DATABASE_URL` only and lands in one database per run — see `developer-guide.md` §2.
- Re-run `internal/db/setup/readonly_role.sql` against both databases after the migration, so the new view is readable on the read-only DSN — see `developer-guide.md` §2.

### Task 4: Measure engine consumer

Return a per-row requisition count for the `count` measure, joining `posting_requisitions` into the aggregate query.

- Add `readonly requisitions?: number` to `MeasureRow` in `apps/web/lib/db/measure-engine.ts`. Optional, because `measure-distributions.ts` also produces `MeasureRow` for `age` and `lifespan` and will not set it.
- Mirror the `gap` field's precedent in that same interface: a signal the row cannot supply is absent, never 0, so "0 real" stays distinct from "0 unknown."
- Join `posting_requisitions` on `job_posting_id` against the cohort in `selectAggregateRows` in `apps/web/lib/db/measure-aggregates.ts`, and count distinct resolved keys within each grouped row, so a per-company row reports that company's requisitions.
- Emit `requisitions` only when `composition.measure === "count"`. `selectAggregateRows(context, normalize)` serves `share` too, and a second number on a ratio has no defined meaning.
- Leave `requisitions` unset on every row from `selectWeeklyAggregateRows`, the path week-grouped `count` takes. `posting_requisitions` resolves against the current snapshot and cannot answer a historical week.
- Do not extend `createMeasureScope` in `apps/web/lib/db/measure-scope.ts`. It builds the cohort and filter fragments shared by every measure, and a `count`-only join does not belong there.
- Check `apps/web/lib/chart/shaping.ts`, the one consumer outside the db layer that reads `MeasureRow`, still compiles against the widened interface.
- Add a `.db.test.ts` case following `apps/web/lib/db/measure-engine.db.test.ts`, covering a Greenhouse fan-out company, a NULL-key company, and a week-grouped row asserting `requisitions` is absent. Seed each case as a fixture: these suites run against `market_scout_test`, which holds no production rows.

Do not:
- Add `requisition_key` or `requisitions` to `GROUPINGS` or `FILTER_DIMENSIONS` in `apps/web/lib/composition/vocabulary.ts`. The vocabulary is closed by design, and a second count the agent could group on would let it pick whichever number tells the better story.
- Emit `requisitions` as 0 when the number is unavailable. Absent and zero mean different things.

## Sequencing

**Phase 1 (sequential):** Task 1 — migration `000026`. Every later task reads the new column.
**Phase 2 (sequential):** Task 3 — migration `000027`. Kept apart from Task 2 because `sqlc.yaml` reads `internal/db/migrations/` as its schema source, so a half-written migration would poison Task 2's generated output.
**Phase 3 (concurrent):** Task 2, Task 4 — Go adapters against TypeScript measure engine, no shared files. Task 2 runs `sqlc generate` here, after both migrations have landed.

## Boundary inventory

Snapshot column and its wire sources:

| Name | Go struct field | ATS source key | SQL column |
|---|---|---|---|
| Requisition key | `RequisitionKey` | Greenhouse `internal_job_id`; Workday `bulletFields[0]`; Workable `code`. Ashby, Lever, and Gem supply none — nil. | `posting_snapshots.requisition_key` |

Columns `posting_requisitions` exposes, pinned here because Task 3 and Task 4 run in different phases:

| Column | Type | Meaning |
|---|---|---|
| `job_posting_id` | bigint | Join key against the cohort. One row per open posting. |
| `requisition_key` | text | Resolved key — the platform's, or the posting-identity fallback. Never NULL. |
| `requisition_source` | text | `ats` or `posting`. |

Measure-engine field:

| Name | TypeScript field | JSON key | Source |
|---|---|---|---|
| Requisition count | `MeasureRow.requisitions?: number` | `"requisitions"` | `count(DISTINCT posting_requisitions.requisition_key)` |

## Rough sketch

`posting_requisitions` resolves `COALESCE(current_snapshot.requisition_key, 'posting:' || job_posting_id)` so a fallback can never collide with a real key, with `requisition_source` set to `ats` or `posting` accordingly.

## Open questions

- Whether `requisition_key` should later anchor repost detection. Greenhouse `internal_job_id` is stable across every multi-snapshot posting measured (0 of 4,992 changed), unlike the ATS timestamps the architecture distrusts. But repost and fan-out cannot be separated until this spec ships, and the first-observed-timestamp anchor is settled architecture — so this is a later discussion, with evidence, not a scope question here.

## Known gaps outside this spec

- Supio (Gem) has never been fetched: zero fetch runs since it was added 2026-08-14, no error rows, adapter registered 2026-08-15. Gem extraction therefore ships untested against live data. This is a fetch-pipeline bug, not a spec risk.
