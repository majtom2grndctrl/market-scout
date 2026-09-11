# Requisition Identity

## Goal

Count postings and requisitions side by side; collapse only what the ATS itself identifies as one job. Some boards list a single requisition many times — one per location — so posting count overstates hiring for those companies. Extracting the ATS's own requisition key gives the read model a second honest denominator without discarding a single row.

## Scope

### In scope

- `requisition_key` column on `posting_snapshots`, populated by every ATS adapter from the platform's own job identifier.
- Backfill of `requisition_key` over existing snapshots from `raw_data`.
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

- [ ] Every adapter populates `requisition_key` when its platform exposes a job identifier, and leaves it NULL when the platform does not.
- [ ] After backfill, every Greenhouse snapshot carrying `internal_job_id` in `raw_data` has a non-NULL `requisition_key`.
- [ ] `posting_requisitions` returns exactly one row per open posting.
- [ ] `posting_requisitions` sets `requisition_source` to `ats` when the platform supplied the key and to `posting` when the row fell back to posting identity.
- [ ] A company whose postings each carry a distinct requisition key reports equal posting and requisition counts.
- [ ] Boulder Care's 49 postings sharing Greenhouse requisition `4458330008` report 49 postings and 1 requisition.
- [ ] A `count` measure result carries a requisition count for each row alongside the posting count.
- [ ] `requisition_key` appears in neither `GROUPINGS` nor `FILTER_DIMENSIONS` in `apps/web/lib/composition/vocabulary.ts`.
- [ ] `migrate version` reports the same version with the dirty flag clear against both `market_scout` and `market_scout_test`.

## Tasks

### Task 1: Schema and backfill

Add `requisition_key text` to `posting_snapshots` in a numbered migration, then backfill it from `raw_data` for existing rows.

- Index `requisition_key` alongside `job_posting_id`. The read-model view groups on it per company.
- Backfill each platform from its own key using the Boundary inventory below, joining through `companies.ats` to pick the path. Backfill is a one-time repair, so reading `raw_data` here is correct even though adapters own extraction going forward.
- Apply to both `market_scout` and `market_scout_test`. `migrate` reads `DATABASE_URL` only and lands in one database per run — see `developer-guide.md` §2.

Do not:
- Add `requisition_key` to `job_postings`. It is an observed field like `title`, and a board that reassigns it should leave history intact.
- Make `requisition_key` a generated column. A stored generated column may only reference columns in its own row, and the extraction path depends on `companies.ats`, two joins away.
- Hand-edit any file under `apps/tools/internal/db/` that sqlc generates — see `developer-guide.md` §5.8.

### Task 2: Adapter extraction

Add `RequisitionKey *string` to `domain.Posting`, populate it in all six adapters, and carry it through the snapshot write path.

- Extract from each platform's own identifier per the Boundary inventory. The ATS's job-versus-job-post distinction is the only authority on what counts as one job.
- Leave `RequisitionKey` nil when the platform exposes no job identifier. A nil is an honest absence; the view falls back to posting identity.
- Extract in the adapter, not from `raw_data` in SQL. Adapters own wire formats, and `000019` measured `raw_data` detoasting at 227ms against 33ms for a column.
- Add the column to `InsertPostingSnapshot` in `apps/tools/internal/db/queries/fetcher.sql`, then run `sqlc generate` from `apps/tools/`.

| Mirror | Don't mirror |
|---|---|
| `SourceID` — per-posting identifier, extracted in adapter, threaded through `domain.Posting` | `WorkplaceType` — has a SQL-side derivation layer; `RequisitionKey` has none |

Do not:
- Derive `RequisitionKey` from title, URL, or description. Only the platform's own identifier counts.
- Use Greenhouse `requisition_id`. It is NULL on 54 rows and one board fills it with the literal string `See Opening ID`; `internal_job_id` is present on 100%.

### Task 3: Read-model view

Add `posting_requisitions` to the read model as a SQL view in a numbered migration.

- Key the view on the current snapshot's `requisition_key`, falling back to a posting-identity value when NULL, so every open posting yields exactly one row and a NULL-key platform degrades to one requisition per posting.
- Carry a `requisition_source` column naming which path produced the key, mirroring `workplace_type_source` in `000019`. Absent a source column, a fallback is indistinguishable from a real key.
- Derive "currently open" from `open_postings`, not by re-deriving fetch-run logic. That derivation lives in one place by design.

### Task 4: Measure engine consumer

Return a requisition count alongside the posting count for the `count` measure in `apps/web/lib/db/measure-aggregates.ts`.

- Add the requisition count to `count` only. `delta`, `rate`, and `share` also route through `runAggregateMeasure`, and a second number on a ratio has no defined meaning.
- Mirror the existing `denominator` shape in `apps/web/lib/db/measure-engine.ts`, which already returns `classified` against `total` as a coverage pair. The same honesty contract covers fan-out.
- Count distinct requisitions within each grouped row, so a per-company row reports that company's requisitions rather than a corpus-wide total.
- Add a `.db.test.ts` case following `apps/web/lib/db/measure-engine.db.test.ts`.

## Sequencing

**Phase 1 (sequential):** Task 1 — every later task reads the new column.
**Phase 2 (concurrent):** Task 2, Task 3 — Task 2 touches Go adapters and sqlc output; Task 3 touches SQL views. Task 3 runs against Task 1's backfill, so it does not wait on Task 2.
**Phase 3 (sequential):** Task 4 — consumes the `posting_requisitions` view from Task 3.

## Boundary inventory

| Name | Go struct field | ATS source key | SQL column |
|---|---|---|---|
| Requisition key | `RequisitionKey` | Greenhouse `internal_job_id`; Workday `bulletFields[0]`; Workable `code`; Lever `id`; Ashby `id`; Gem `id` | `requisition_key` |
| Requisition source | — (derived in view) | — | `requisition_source` |

## Rough sketch

`posting_requisitions` resolves `COALESCE(current_snapshot.requisition_key, 'posting:' || job_posting_id)` so a fallback can never collide with a real key, with `requisition_source` set to `ats` or `posting` accordingly.

## Open questions

- Whether `requisition_key` should later anchor repost detection. Greenhouse `internal_job_id` is stable across every multi-snapshot posting measured (0 of 4,992 changed), unlike the ATS timestamps the architecture distrusts. But repost and fan-out cannot be separated until this spec ships, and the first-observed-timestamp anchor is settled architecture — so this is a later discussion, with evidence, not a scope question here.

## Known gaps outside this spec

- Supio (Gem) has never been fetched: zero fetch runs since it was added 2026-08-14, no error rows, adapter registered 2026-08-15. Gem extraction therefore ships untested against live data. This is a fetch-pipeline bug, not a spec risk.
