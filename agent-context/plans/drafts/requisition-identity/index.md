# Requisition Identity

## Goal

Count postings and requisitions side by side; collapse only what the ATS itself identifies as one job. Some boards list a single requisition many times — one per location — so posting count overstates hiring for those companies. Extracting the ATS's own requisition key gives the read model a second honest denominator without discarding a single row.

## Scope

### In scope

- `requisition_key` column on `posting_snapshots`, populated by every ATS adapter from the platform's own job identifier.
- Backfill of `requisition_key` over existing snapshots from `raw_data`.
- `normalized_title` stored generated column on `posting_snapshots`.
- `posting_requisitions` read-model view mapping each posting to its requisition, falling back to posting identity when the platform exposes none.
- `template_peers` per-posting integer surfacing suspected template fan-out against the current snapshot.
- `count` measure returns requisition count alongside posting count.

### Out of scope

- Collapsing rows at fetch time. The fetcher writes what the board returns; grouping is derived.
- Template fan-out as a grouping key or vocabulary measure. Description text changes on 39.9% of postings during their observed life, so a content hash cannot anchor a time series.
- HTML-entity normalization. Zero instances in the corpus.
- A `requisitions` vocabulary measure competing with `count`. One measure, two numbers.
- Workday and Workable per-posting description fetch. Deferred separately; `template_peers` is 0 for NULL descriptions by design.

## Acceptance criteria

- [ ] Every adapter populates `requisition_key` when its platform exposes a job identifier, and leaves it NULL when the platform does not.
- [ ] After backfill, every Greenhouse snapshot with `internal_job_id` in `raw_data` carries a non-NULL `requisition_key`.
- [ ] `normalized_title` equals the title with leading and trailing whitespace removed and internal whitespace runs collapsed to single spaces.
- [ ] `posting_requisitions` returns exactly one row per open posting.
- [ ] A company whose postings each carry a distinct requisition key reports equal posting and requisition counts.
- [ ] Boulder Care's 49 postings sharing Greenhouse requisition `4458330008` report 49 postings and 1 requisition.
- [ ] `posting_requisitions` sets `requisition_source` to `ats` when the platform supplied the key and to `posting` when the row fell back to posting identity.
- [ ] `template_peers` is 0 for every posting whose current description is NULL or empty.
- [ ] A posting sharing its normalized title and description with N other currently-open postings at the same company reports `template_peers` of N.
- [ ] A `count` measure result carries a requisition count for each row alongside the posting count.
- [ ] `requisition_key` never appears in `GROUPINGS` or `FILTER_DIMENSIONS` in `apps/web/lib/composition/vocabulary.ts`.
- [ ] `migrate version` reports the same version with the dirty flag clear against both `market_scout` and `market_scout_test`.

## Tasks

### Task 1: Schema and backfill

Add `requisition_key text` and `normalized_title text` to `posting_snapshots` in one numbered migration, then backfill `requisition_key` from `raw_data` for existing rows.

- Put both columns in one migration. Two concurrent migrations collide on the sequence number.
- Make `normalized_title` a `STORED GENERATED` column over `title`. A generated column re-derives for all history when its expression changes, so the rule can never version across time the way a write-time rule would.
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

Add `posting_requisitions` and `template_peers` to the read model as SQL views in a numbered migration.

- Key `posting_requisitions` on the current snapshot's `requisition_key`, falling back to a posting-identity value when NULL, so every open posting yields exactly one row and a NULL-key platform degrades to one requisition per posting.
- Carry a `requisition_source` column naming which path produced the key, mirroring `workplace_type_source` in `000019`. Absent a source column, a fallback is indistinguishable from a real key.
- Compute `template_peers` as the count of other currently-open postings at the same company sharing both `normalized_title` and description hash, and 0 when the description is NULL or empty. Scoped to the current snapshot only, never across time.
- Derive "currently open" from `open_postings`, not by re-deriving fetch-run logic. That derivation lives in one place by design.

Do not:
- Expose `template_peers` as a grouping, filter, or measure. It is display-only, because a description hash changes identity on 39.9% of postings and the grammar permits any grouping to be paired with a time window.

### Task 4: Measure engine consumer

Return a requisition count alongside the posting count for the `count` measure in `apps/web/lib/db/measure-aggregates.ts`.

- Add the requisition count to `count` only. `delta`, `rate`, and `share` also route through `runAggregateMeasure`, and a second number on a ratio has no defined meaning.
- Mirror the existing `denominator` shape in `apps/web/lib/db/measure-engine.ts`, which already returns `classified` against `total` as a coverage pair. The same honesty contract covers fan-out.
- Count distinct requisitions within each grouped row, so a per-company row reports that company's requisitions rather than a corpus-wide total.
- Add a `.db.test.ts` case following `apps/web/lib/db/measure-engine.db.test.ts`.

## Sequencing

**Phase 1 (sequential):** Task 1 — every later task reads the two new columns.
**Phase 2 (concurrent):** Task 2, Task 3 — Task 2 touches Go adapters and sqlc output; Task 3 touches SQL views. Task 3 runs against Task 1's backfill, so it does not wait on Task 2.
**Phase 3 (sequential):** Task 4 — consumes the `posting_requisitions` view from Task 3.

## Boundary inventory

| Name | Go struct field | ATS source key | SQL column |
|---|---|---|---|
| Requisition key | `RequisitionKey` | Greenhouse `internal_job_id`; Workday `bulletFields[0]`; Workable `code`; Lever `id`; Ashby `id`; Gem `id` | `requisition_key` |
| Normalized title | — (generated) | — | `normalized_title` |
| Requisition source | — | — | `requisition_source` |
| Template peers | — | — | `template_peers` |

## Rough sketch

`posting_requisitions` resolves `COALESCE(current_snapshot.requisition_key, 'posting:' || job_posting_id)` so the fallback can never collide with a real key, with `requisition_source` set to `ats` or `posting` accordingly. `template_peers` uses `md5(description_text)` over the current snapshot per `(company_id, normalized_title)`, guarded to 0 where the description is NULL or empty.

## Open questions

- Should `template_peers` reach the posting-detail UI in this unit of work, or land as a read-model column with no consumer until a display task follows? The spec-session guidance argues for building the first consumer alongside the foundation, which would favour adding it.
- Gem has zero postings in the corpus today, so its `id` extraction ships unverified against live data.
- Whether `requisition_key` should later anchor repost detection. Our own first-observed timestamp is the settled anchor; a requisition key that survives a repost could sharpen it, but that is a separate question from counting.
