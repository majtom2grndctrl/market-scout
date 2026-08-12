# Workplace Type Derivation

## Goal

Derive remote/hybrid/onsite for postings whose ATS never supplied it, in the read
model, so both the web app and the agent's MCP query gateway read one definition.
Today 67% of postings have a NULL `workplace_type` — Greenhouse, Workday, and
Workable supply none at all, and Greenhouse alone is 56% of the corpus. This
raises coverage from 32.9% to roughly 55% without touching the adapters or the
append-only snapshot record.

## Scope

### In scope

- One migration appending two derived columns to `open_postings_display`.
- Derivation from three sources in priority order: the ATS-supplied
  `workplace_type`, structured signals already sitting in `raw_data`, then a
  regex over a normalized `location_text`.
- A provenance column naming which source produced each value.
- `sqlc generate` and the `sqlc.yaml` overrides the new columns require.
- A `*.db.test.ts` case covering each derivation source and the abstain case.

### Out of scope

- Place extraction — splitting `location_text` into per-market place strings.
  The comma-ambiguous class is unresolvable without a city gazetteer, and a
  strip-and-split leaves ~430 garbage segments. See `research.md`.
- Description-prose extraction. Recovers ~656 further postings but needs an
  enrichment pass, not a view.
- Adapter changes. Adapters stay the raw-capture layer; `raw_data` is `jsonb` in
  the same table, so the view reaches these signals without one.
- Backfill or new stored columns. The derivation is computed on read.
- Exposing `location_texts` in any view.
- Materialized views or a refresh mechanism.

## Acceptance criteria

- [ ] `open_postings_display` returns a resolved workplace type and a provenance
      value for every posting where any of the three sources yields an answer,
      and NULL for both columns where none does.
- [ ] A posting whose ATS supplied `workplace_type` returns that exact value,
      with provenance naming the ATS field, regardless of what its
      `location_text` says.
- [ ] A posting whose `location_text` contains `Remote-Friendly` and no other
      modality token resolves to NULL, not `remote`.
- [ ] A posting whose `location_text` is `US-Remote` resolves to `remote`, and
      one whose `location_text` is `Hybrid- Fremont, CA` resolves to `hybrid`.
- [ ] A posting whose `location_text` contains both a hybrid and a remote token
      resolves to `hybrid`.
- [ ] A Workable posting with `telecommuting` true in `raw_data` resolves to
      `remote` with provenance naming `raw_data`.
- [ ] A Greenhouse posting carrying a `Location Type` metadata entry resolves to
      that entry's value, and a Greenhouse posting whose `metadata` is JSON null
      rather than an array is skipped without raising an error.
- [ ] Leading, trailing, and non-breaking whitespace in `location_text` does not
      change the resolved value.
- [ ] The existing twelve columns of `open_postings_display` keep their current
      names, order, and types, and `open_posting_taxonomy` continues to return
      the same rows it does today.
- [ ] Rolling the migration back restores the view exactly as migration 000018
      left it, including both `id DESC` tie-breakers.
- [ ] `sqlc generate` produces a committed `models.go` diff in which both new
      columns are nullable.
- [ ] `pnpm test:db` passes from `apps/web/`, and `make check` passes from
      `apps/tools/`.

## Tasks

### Task 1: Migration 000019 — derived columns

Write `000019_workplace_type_derivation.up.sql` and its `.down.sql`, replacing
`open_postings_display`.

- Use `CREATE OR REPLACE VIEW`. A `DROP` would fail on the dependent
  `open_posting_taxonomy` view or require `CASCADE` and a rebuild of it.
- Append the two new columns after `seniority`, the current last column.
  Postgres only permits `CREATE OR REPLACE` when the new definition is a
  superset with identical leading columns.
- Copy the twelve existing columns, both laterals, and both `ORDER BY` clauses
  from `000018…up.sql` verbatim. A dropped tie-breaker silently reverts 000018.
- Make the `.down.sql` a verbatim copy of `000018_read_model_view_tie_breakers.up.sql`.
  That is the house pattern for a migration that replaces a view — see that
  migration's own `.down.sql`.
- Normalize `location_text` before matching: strip U+00A0, collapse whitespace
  runs, then trim. 168 postings carry stray whitespace and one carries a
  non-breaking space that `btrim` alone will not remove.
- Strip `remote[- ]friendly` from the normalized string before any modality
  match. In this corpus the phrase means an onsite role open to remote
  candidates; mapping it to `remote` drops precision from 95.2% to 80.1%.
- Order the modality branches hybrid, then remote, then onsite. Only three
  postings carry two tokens and `hybrid` is correct in all three.
- Guard the Greenhouse `metadata` scan with `jsonb_typeof(...) = 'array'`.
  The field is JSON null rather than an array on 2,812 postings and an
  unguarded scan errors.

Do not:

- Reorder, rename, retype, or remove any existing column.
- Split `location_text` on `\yOR\y`. It means Oregon in 100 rows and "or" in 12,
  and the two are not separable.
- Add `GRANT` statements. The read-only role reaches new views through
  `ALTER DEFAULT PRIVILEGES` in `readonly_role.sql`.
- Edit migrations 000017 or 000018.

| Mirror | Don't mirror |
|---|---|
| `000018…down.sql` — full verbatim restore of the prior view | `000017…down.sql` — plain `DROP`, correct only for a migration that created the views |

### Task 2: Regenerate sqlc output

Run `sqlc generate` from `apps/tools/` and commit the `models.go` diff alongside
the migration.

- Add an `overrides` entry in `sqlc.yaml` for each new column. sqlc infers
  columns computed inside a `LEFT JOIN LATERAL` as non-nullable regardless of
  their real nullability, and a Go read of a NULL would panic.
- Read the `models.go` diff by eye before committing. A mistyped column name in
  an override is ignored silently, with exit 0.

Do not hand-edit `models.go` — it is sqlc output. See `developer-guide.md` §5.8.

### Task 3: Database integration test

Add a case to `apps/web/lib/db/read-model-views.db.test.ts`, or a sibling
`*.db.test.ts`, covering every acceptance criterion that names a derivation
source or an abstain.

- Seed fixtures through the owner DSN and read them back through the read-only
  DSN. Reading through the read-only role is what proves the grant reached the
  replaced view.
- Thread a unique marker through company name, board token, and source URLs, as
  the existing test does, so fixtures cannot collide across runs.
- Include a fixture whose `location_text` is `Remote-Friendly, United States`
  and assert the resolved value is NULL. This is the single highest-value
  assertion in the suite.
- Include a Greenhouse-shaped fixture whose `raw_data.metadata` is JSON null,
  to prove the guard holds.
- Clean up in a `finally` block with FK-ordered deletes.

Do not assert on generated sqlc code, and do not mock the database. See
`testing-guide.md`.

## Sequencing

**Phase 1 (sequential):** Task 1 — the view definition blocks everything.
**Phase 2 (concurrent):** Task 2, Task 3 — both consume the migration, neither consumes the other.

## Rough sketch

Two columns, appended after `seniority`:

| Column | Values |
|---|---|
| `workplace_type_resolved` | `remote` \| `hybrid` \| `onsite` \| NULL |
| `workplace_type_source` | `ats` \| `raw_data` \| `location_text` \| NULL |

The provenance column exists because `project.md`'s evidence trust tiers require
that records mixing tiers keep them distinguishable. An ATS-supplied value is
ground truth; a regex match is a 95%-precision inference. Collapsing both into
one column would erase that difference for every consumer downstream.

`workplace_type` itself stays untouched and keeps meaning "what the ATS said."

Resolution order, first hit wins:

1. `posting_snapshots.workplace_type` — Ashby, Lever, and Gem once it ships.
2. `raw_data`: Workable `telecommuting`, Greenhouse `metadata` entries named
   `Location Type` or `Remote`.
3. Regex over the normalized, `remote-friendly`-stripped `location_text`.

Postgres `\y` is the word boundary and treats `-` as one, so `US-Remote` and
`United States-Remote` both match without a separate pattern.

The onsite regex branch fires on 34 postings, none of them in the labeled set.
Ship it, but no acceptance criterion can validate it — do not claim it works.

## Boundary inventory

| Name | SQL column | JSON path | Go field (sqlc) | TS field |
|---|---|---|---|---|
| Resolved workplace type | `workplace_type_resolved` | — | `WorkplaceTypeResolved sql.NullString` | `workplace_type_resolved: string \| null` |
| Derivation provenance | `workplace_type_source` | — | `WorkplaceTypeSource sql.NullString` | `workplace_type_source: string \| null` |
| Workable remote flag | — | `raw_data->>'telecommuting'` | — | — |
| Greenhouse metadata | — | `raw_data->'metadata'` (array of `{name, value}`) | — | — |

The TS column is listed because `apps/web/lib/db/postings.ts` enumerates view
columns explicitly. It keeps compiling untouched; adding the field there is
optional and not required by any acceptance criterion.

## Open questions

1. **Should `postings.ts` and the postings page surface the derived value?**
   The plan makes the columns available and stops there. Rendering them is a
   separate frontend decision.

2. **Workable's `telecommuting` is arguably an adapter mapping gap.** Reading it
   in the view is correct for the 48 existing postings, but a one-line adapter
   change would populate `workplace_type` properly going forward and make the
   view's Workable branch dead code. Same question, smaller, for Ashby's
   `address.postalAddress` and Greenhouse's `offices[].location` — both are
   unmapped place sources that a future place-extraction plan would want.

3. **The Greenhouse `metadata` field names are per-company custom.** `Location
   Type` is one company's field, `Remote` another's. Matching on the name string
   works for the three companies present today but will not generalize. If a
   fourth company names its field differently, coverage silently does not grow.
