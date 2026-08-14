# Workplace Type Derivation

## Goal

Derive remote/hybrid/onsite for postings whose ATS never supplied it, in the read
model, so both the web app and the agent's MCP query gateway read one definition.
`open_postings_display` serves 4,457 postings — the latest successful run per
company — and 1,609 of them carry an ATS-supplied `workplace_type` today.
Greenhouse, Workday, and Workable supply none at all. This raises coverage from
36.1% to 56.7%, 1,609 postings to 2,525, without touching the adapters and
without rewriting a single existing snapshot.

## Scope

### In scope

- One migration appending two derived columns to `open_postings_display`.
- Derivation from three sources in priority order: the ATS-supplied
  `workplace_type`, structured signals already sitting in `raw_data`, then a
  regex over a normalized `location_text`.
- A provenance column naming which source produced each value.
- Switching `posting_snapshots.raw_data` to `lz4` compression. Metadata-only,
  no row rewrite; it makes the tier-2 detoast roughly 3.5x cheaper as new
  snapshots land.
- `sqlc generate` and the `sqlc.yaml` overrides the new columns require.
- A `*.db.test.ts` case covering each derivation source and the abstain case.
- Threading both columns through `apps/web/lib/db/postings.ts` and rendering
  them on the postings page, keeping reported and derived values
  distinguishable.

### Out of scope

- Place extraction — splitting `location_text` into per-market place strings.
  The comma-ambiguous class is unresolvable without a city gazetteer, and a
  strip-and-split leaves ~430 garbage segments. See `research.md`.
- Description-prose extraction. Recovers at most 656 further postings — an upper
  bound measured before the `raw_data` tier existed, overlapping rows that tier
  now resolves. Needs an enrichment pass, not a view.
- Adapter changes. Adapters stay the raw-capture layer; `raw_data` is `jsonb` in
  the same table, so the view reaches these signals without one.
- Backfill or new stored columns. The derivation is computed on read.
- Exposing `location_texts` in any view.
- Materialized views or a refresh mechanism.
- A designed treatment for the postings page. It is a scaffold, and product
  screens are a stated non-goal — see `project.md` §Non-goals. Task 4 threads
  data through the shape already there.
- Filtering, sorting, or faceting postings by workplace type.

## Acceptance criteria

- [ ] The two columns are always both NULL or both non-NULL:
      `SELECT count(*) FROM open_postings_display WHERE (workplace_type_resolved
      IS NULL) <> (workplace_type_source IS NULL)` returns 0.
- [ ] A posting whose ATS supplied `workplace_type` returns that exact value,
      with `workplace_type_source` = `ats`, regardless of what its
      `location_text` says.
- [ ] A posting whose `location_text` contains `Remote-Friendly` and no other
      modality token resolves to NULL, not `remote`.
- [ ] A posting whose `location_text` is `US-Remote` resolves to `remote`, and
      one whose `location_text` is `Hybrid- Fremont, CA` resolves to `hybrid`.
      Both carry `workplace_type_source` = `location_text`.
- [ ] A posting whose `location_text` is `Onsite- Salem, OR` resolves to
      `onsite` with `workplace_type_source` = `location_text`.
- [ ] A posting whose `location_text` contains both a hybrid and a remote token
      resolves to `hybrid`.
- [ ] A Workable posting with `telecommuting` true in `raw_data` resolves to
      `remote` with `workplace_type_source` = `raw_data`.
- [ ] A posting whose `telecommuting` or `Remote` flag is false resolves to NULL
      for both columns, even when its `location_text` contains a modality token.
- [ ] A Greenhouse posting whose `Location Type` metadata entry is `On-Site`
      resolves to `onsite`, and one whose entry is `Hybrid (Travel-Required)`
      resolves to `hybrid`.
- [ ] A posting whose `Location Type` entry carries an unrecognized value and
      whose `location_text` is `US-Remote` resolves to `remote` with
      `workplace_type_source` = `location_text`.
- [ ] A Greenhouse posting whose `Job Posting Location` entry holds
      `["Illinois - Remote"]` resolves to `remote`, and one holding
      `["MD - Columbia - Headquarters"]` resolves to `onsite`.
- [ ] A posting whose `Job Posting Location` entry holds an empty array and whose
      `location_text` is `US-Remote` resolves to `remote` with
      `workplace_type_source` = `location_text`.
- [ ] A posting whose `Location Type` entry has a JSON null `value` and whose
      `location_text` is `US-Remote` resolves to `remote` with
      `workplace_type_source` = `location_text`.
- [ ] A Greenhouse posting whose `metadata` is JSON null rather than an array is
      skipped without raising an error.
- [ ] Leading, trailing, and non-breaking whitespace in `location_text` does not
      change the resolved value.
- [ ] The existing twelve columns of `open_postings_display` keep their current
      names, order, and types.
- [ ] `open_posting_taxonomy` is untouched by the migration, proven by the
      existing `it()` block in `read-model-views.db.test.ts` still passing
      unmodified.
- [ ] Inside the `.down.sql` verification transaction,
      `pg_get_viewdef('open_postings_display', true)` and
      `pg_get_viewdef('open_posting_taxonomy', true)` after the rebuild match the
      definitions captured before it, and
      `has_table_privilege('market_scout_readonly', <view>, 'SELECT')` is true
      for both.
- [ ] Migration 000019's header comment records an
      `EXPLAIN (ANALYZE, TIMING OFF)` median and the snapshot count it was
      measured at, so a later reader can tell growth from regression.
- [ ] `pg_attribute.attcompression` for `posting_snapshots.raw_data` is `l`. It
      is empty today, meaning the `pglz` default.
- [ ] No tier-2 branch expression references `current_snapshot.raw_data` outside
      the `rd` lateral. Review gate — grep the migration.
- [ ] `models.go` declares `WorkplaceTypeResolved sql.NullString` and
      `WorkplaceTypeSource sql.NullString` on the `OpenPostingsDisplay` struct.
- [ ] `OpenPostingsDisplayRow` declares both new fields and `listOpenPostings`
      selects both by name, so a row carries them at runtime.
- [ ] **Review gate, not a test.** The postings page renders the resolved value
      for a posting that has one, omits the line entirely for a posting that
      resolves to NULL, and labels a `location_text`-derived value differently
      from a reported one. `apps/web` has no React test stack and this plan does
      not add one — verify by reading the diff.
- [ ] `workplaceTypeLabel("hybrid", "ats")` returns `hybrid (reported)` and
      `workplaceTypeLabel("remote", "location_text")` returns
      `remote (derived from location)`.
- [ ] `workplaceTypeLabel` returns `null` for a mismatched pair — a non-null
      `resolved` with a null `source`, or the reverse.
- [ ] `workplaceTypeLabel` returns `null` for a `source` value it does not
      recognize.
- [ ] `pnpm test`, `pnpm test:db`, and `pnpm typecheck` pass from `apps/web/`,
      and `make check` passes from `apps/tools/`.

## Tasks

### Task 1: Migration 000019 — derived columns

Write `000019_workplace_type_derivation.up.sql` and its `.down.sql`, replacing
`open_postings_display`.

Two columns, appended after `seniority`, the current last column. Postgres
permits `CREATE OR REPLACE` only when the new definition is a superset with
identical leading columns.

| Column | Values |
|---|---|
| `workplace_type_resolved` | `remote` \| `hybrid` \| `onsite` \| NULL |
| `workplace_type_source` | `ats` \| `raw_data` \| `location_text` \| NULL |

Both columns are `text`.

Resolution order:

1. `posting_snapshots.workplace_type` — Ashby and Lever today; any adapter that
   starts populating it later, without a change here.
2. `raw_data` — four branches, tried in the order below. Reaches 419 view rows
   and resolves 405.
3. Regex over the normalized, `remote-friendly`-stripped `location_text`.
   Resolves 511 view rows.

Each branch produces one of four outcomes:

| Branch outcome | Effect |
|---|---|
| Resolves a value | Return it with `workplace_type_source` = `raw_data`. Stop. |
| Absent, JSON null `value`, or empty array | No information. Continue to the next branch, then to tier 3. |
| Present with an unrecognized non-null value | No usable information. Continue to the next branch, then to tier 3. |
| `false` boolean | Real information, but not separable. Continue through the remaining `raw_data` branches; if none resolves, return NULL and stop before tier 3. |

A `false` boolean stops before tier 3 because tier 3's only reliable signal is
remote-detection, which the flag has already contradicted. It does not suppress
the later `raw_data` branches — one of those may carry a three-value answer the
boolean cannot express.

Row counts in the branch tables below are corpus-wide, taken from `research.md`.
`open_postings_display` exposes 4,457 of the corpus's 8,115 postings, so the view
sees roughly half of each figure.

The `raw_data` branches, in order:

| # | Location | Key | `value_type` | Company today |
|---|---|---|---|---|
| 1 | `raw_data->'telecommuting'` | — | JSON boolean | Workable (Seeq, Vouched) |
| 2 | `metadata` entry | `Remote` | `yes_no` | Carbon Robotics |
| 3 | `metadata` entry | `Location Type` | `single_select` | Anthropic |
| 4 | `metadata` entry | `Job Posting Location` | `multi_select` | Tenable |

Every key is a per-company custom field. The four are disjoint across the
companies present today, so the order never actually arbitrates — it is fixed
only so the result stays deterministic if a company adds a second field.

Branches 1 and 2 are booleans, and they are one-sided. `true` resolves to
`remote`. `false` means only "not remote," which spans hybrid and onsite, so it
resolves NULL per the outcome table above. Every `false` row carries a bare place
string with no modality token, and bare-city strings skew hybrid in the labeled
corpus, so reading `false` as `onsite` would be wrong more often than right. This
is the failure that disqualified Ashby's `isRemote`. See `research.md`.

Branch 3, `Location Type`, is a genuine three-value enum. Match the value
case-insensitively as a prefix. Equality would resolve none of the 9 hybrid
rows — the stored value is `Hybrid (Travel-Required)`, not a bare token — and a
future `Hybrid (No Travel)` resolves for free.

| `value` prefix | Value today | `workplace_type_resolved` | Corpus rows |
|---|---|---|---|
| `On-Site` | `On-Site` | `onsite` | 657 |
| `Remote` | `Remote` | `remote` | 21 |
| `Hybrid` | `Hybrid (Travel-Required)` | `hybrid` | 9 |
| anything else | — | no match — falls through, see the outcome table | 0 |

Branch 4, `Job Posting Location`, holds an array of `Region - Modality [- City]`
slot strings. The modality token is not at a fixed position — compare
`Israel - Office` with `MD - Columbia - Headquarters` — so match tokens anywhere
in the slot rather than splitting on the hyphen:

| Slot matches | `workplace_type_resolved` | Corpus rows |
|---|---|---|
| `\yremote\y` | `remote` | 38 |
| `\y(office\|headquarters\|hq)\y` | `onsite` | 89 |
| neither | no match — falls through, see the outcome table | 0 |

Check `\yremote\y` across every slot before checking the office tokens. Every
posting carries exactly one slot today and none mixes the two, so the precedence
is untested by current data — it exists so a multi-slot posting stays
deterministic.

The fall-through cases are not rare: 110 `Location Type` entries carry a JSON
null `value`, and 5 `Job Posting Location` entries do — 4 JSON null, 1 an empty
array.

#### Query shape for tier 2

Detoast cost is **per access, not per row**. Every expression naming `raw_data`
decompresses the whole document again, so the obvious formulation — a `CASE`
whose branches each reference `current_snapshot.raw_data` — pays it four to eight
times per row. Measured against the current view's 28ms baseline: the direct-
reference form runs 750ms, the chain below runs 180ms, and the two return
identical rows.

```sql
-- One materialization, skipped entirely when tier 1 already answered.
LEFT JOIN LATERAL (
    SELECT r.doc->>'telecommuting' AS telecommuting,
           r.doc->'metadata'       AS metadata
    FROM (SELECT current_snapshot.raw_data || '{}'::jsonb AS doc
          WHERE current_snapshot.workplace_type IS NULL
          OFFSET 0) r
) AS rd ON TRUE

-- One aggregate pass over metadata yields every branch result at once.
LEFT JOIN LATERAL (
    SELECT bool_or(m.entry->>'name' = 'Remote'
                   AND m.entry->>'value' = 'true') AS remote_true,
           max(CASE WHEN m.entry->>'name' = 'Location Type'
                    THEN m.entry->>'value' END)    AS loc_type
           -- ...one aggregate per branch result
    FROM jsonb_array_elements(rd.metadata) m(entry)
    WHERE jsonb_typeof(rd.metadata) = 'array'
) AS md ON TRUE
```

`|| '{}'::jsonb` forces one fresh in-memory datum; `OFFSET 0` stops the planner
inlining the subquery and undoing that. Both are load-bearing and both look like
noise — comment them in the migration or they will be simplified away.

Narrowing what the lateral projects does not help. jsonb is stored as one TOASTed
value, so extracting a single key still decompresses the whole document. Only
reducing the number of accesses helps.

Normalization, applied before any match:

```sql
btrim(regexp_replace(translate(location_text, U&'\00A0', ' '), '\s+', ' ', 'g'))
```

The order is load-bearing: translate, then collapse, then trim. 168 postings
carry leading or trailing whitespace, 4 carry doubled spaces, and 1 carries a
U+00A0 that `btrim` alone will not remove.

Then strip the `remote-friendly` phrase from the normalized string, before any
modality match. In this corpus the phrase means an onsite role open to remote
candidates, and mapping it to `remote` drops precision from 95.2% to 80.1%:

```sql
regexp_replace(normalized, 'remote[- ]friendly', '', 'gi')
```

Match the stripped string with the case-insensitive operator `~*`, in this order.
Only three postings carry two modality tokens and `hybrid` is correct in all
three:

| Branch | Pattern |
|---|---|
| hybrid | `\yhybrid\y` |
| remote | `\yremote\y` |
| onsite | `\yon[- ]?site\y` |

Postgres `\y` is the word boundary and treats `-` as a non-word character, so
`US-Remote` matches `\yremote\y` and `Vancouver, BC (on-site)` matches
`\yon[- ]?site\y` without separate patterns. Every match uses `~*`; `~` is
case-sensitive and would match nothing in this Title-Cased corpus.

Build the onsite branch. It fires on 34 postings corpus-wide, none in the labeled
set, so its precision is unmeasurable.

- Use `CREATE OR REPLACE VIEW`. A `DROP` would fail on the dependent
  `open_posting_taxonomy` view or require `CASCADE` and a rebuild of it.
- Copy the twelve output columns, both laterals, and both `ORDER BY` clauses from
  `000018…up.sql`. A dropped tie-breaker silently reverts 000018.
- Add `posting_snapshots.raw_data` to the `current_snapshot` lateral's select
  list. That does not alter the outer view's column list, so `CREATE OR REPLACE`
  still succeeds.
- Compute the tier-2 branches through the lateral chain in "Query shape for tier
  2" above. Never reference `current_snapshot.raw_data` from a branch expression
  directly.
- The `.down.sql` cannot use `CREATE OR REPLACE VIEW`. Postgres refuses to drop
  columns from a view, and the rollback goes from fourteen columns back to
  twelve.
- Drop and rebuild both views instead: `DROP VIEW open_posting_taxonomy;`, then
  `DROP VIEW open_postings_display;`, then recreate `open_postings_display` from
  `000018…up.sql`'s body and `open_posting_taxonomy` from `000017…up.sql`'s body.
  Dependent first, or a bare `DROP` on the display view errors.
- The rebuilt views do not inherit the dropped views' ACL. They pick up `SELECT`
  for the read-only role from `ALTER DEFAULT PRIVILEGES` in
  `apps/tools/internal/db/setup/readonly_role.sql:71-74`, which applies only when
  the migration runs as the owner role that statement names.
- Give the `.down.sql` its own header comment describing the rollback, as
  `000018…down.sql` does. Do not copy 000018's header.
- Prove the `.down.sql` rebuilds both views by executing its body inside
  `BEGIN; … ROLLBACK;` against the local database. Capture
  `pg_get_viewdef(…, true)` for both views before the DDL, compare after, and
  assert `has_table_privilege('market_scout_readonly', …, 'SELECT')` on both —
  parsing alone proves nothing about the rebuilt definitions. Never run
  `go run ./cmd/migrate down` to test it — that verb is a full teardown of every
  migration, not a single step, and it strands the database `dirty` at the
  RESTRICT foreign keys in 000007 and 000008. See `developer-guide.md`
  §Migrations.
- Guard the Greenhouse `metadata` scan with `jsonb_typeof(...) = 'array'`.
  The field is JSON null rather than an array on 2,812 postings and an
  unguarded scan errors.
- Guard each entry's `value` separately before iterating it. Branch 4 calls
  `jsonb_array_elements_text` on `value`, which errors on the 4 entries whose
  `value` is JSON null. The `metadata`-array guard does not cover this.
- Match the modality patterns against the whole stripped string — never a
  substring, never a split segment. Matching the normalized string instead of the
  stripped one resolves `Remote-Friendly` to `remote`.
- Add `ALTER TABLE posting_snapshots ALTER COLUMN raw_data SET COMPRESSION lz4;`
  to the up migration. It is metadata-only — no existing row is rewritten, so it
  does not touch the append-only record. pglz decompression is roughly 3.5x
  slower than lz4 on these documents and is most of the tier-2 cost.
- Put `SET COMPRESSION pglz` in the `.down.sql`. Rows already written as lz4 stay
  lz4 and remain readable; the setting governs new writes only.
- Record an `EXPLAIN (ANALYZE, TIMING OFF)` median in 000019's header comment,
  with the snapshot count it was measured at, so a later reader can tell growth
  from regression. Note that the figure is the pglz number and will fall as the
  fetcher appends lz4-compressed snapshots. Do not wait for that to commit.
- If the measured median lands far above the roughly 180ms this shape produced in
  testing, the cause is almost certainly structural rather than a matter of
  degree: a tier-2 branch reaching `current_snapshot.raw_data` directly instead of
  going through `rd`, each such reference paying a fresh detoast. Grep the
  migration for `raw_data` before reaching for anything cleverer — that check is
  its own acceptance criterion, and it is the one that matters. The timing is
  evidence about whether you got the shape right, not a budget to hit.

Do not:

- Reorder, rename, retype, or remove any existing column.
- Split the normalized `location_text` on any delimiter.
- Add `hq` or `headquarters` to the `location_text` patterns. They belong to the
  `Job Posting Location` slot patterns only — see `research.md` for why the
  token was rejected for `location_text`.
- Claim the onsite `location_text` branch is validated. Its 34 postings sit
  outside the labeled set.
- Add `GRANT` statements. `CREATE OR REPLACE VIEW` preserves the view's existing
  ACL. See `apps/tools/internal/db/setup/readonly_role.sql:71-74`.
- Assume the up direction's `CREATE OR REPLACE` argument carries to the down
  direction. It does not — the down migration drops and rebuilds, and only the
  up direction preserves the ACL.
- Edit migrations 000017 or 000018.
- Narrow the view's correctness to buy speed. A company allowlist gating tier 2,
  a snapshot-age cutoff, or a materialized view all make the query faster and all
  make it quietly wrong later — a new company starts emitting a modality field and
  the view keeps working while silently ceasing to derive. Build the correct view
  in the shape above. If it later proves too slow for the app, that is a signal to
  revisit the read-model architecture, which is young enough to change, not a
  reason to compromise this view.

### Task 2: Regenerate sqlc output

Run `sqlc generate` from `apps/tools/` and commit the `models.go` diff alongside
the migration.

- Add an `overrides` entry in `sqlc.yaml` for each new column, keyed
  `open_postings_display.workplace_type_resolved` and
  `open_postings_display.workplace_type_source`, mirroring the existing
  `seniority` entry.
  sqlc cannot infer nullability for either expression. It emits `interface{}`, or
  a non-nullable `string` if the expression carries an explicit cast. Neither is
  usable; the override makes both `sql.NullString`.
- Read the `models.go` diff by eye before committing. A mistyped column name in
  an override is ignored silently, with exit 0.

Do not:

- Hand-edit `models.go`. It is sqlc output — see `developer-guide.md` §5.8.
- Edit the generated file to fix an override that did not take. Fix the override.

### Task 3: Database integration test

Add a second `it()` block to `apps/web/lib/db/read-model-views.db.test.ts`, with
its own marker and its own `finally` cleanup, covering every acceptance criterion
about view output. The rollback criterion belongs to Task 1, the `models.go`
criterion to Task 2, the `OpenPostingsDisplayRow` and postings-page criteria to
Task 4, and the command-gate criterion to Phase 3.

- Apply the migration with `go run ./cmd/migrate up` from `apps/tools/` before
  running the suite. The tests query the live schema, so an unapplied migration
  leaves them green against the old view.
- Seed fixtures through the owner DSN and read them back through the read-only
  DSN. Reading through the read-only role is how every other view test in this
  file reads, and it catches an accidental `DROP`/`CREATE` in the migration.
- Thread a unique marker through company name, board token, and source URLs, as
  the existing test does, so fixtures cannot collide across runs.
- Seed a `fetch_runs` row with status `success` and point every fixture
  snapshot's `fetch_run_id` at it, as the existing block does. The view filters
  to the latest successful run per company, so a snapshot without one never
  appears.
- Assert the view's shape against `information_schema.columns` ordered by
  `ordinal_position` — name and `data_type` for all fourteen columns. A
  `SELECT *` key check proves neither types nor the new columns' position.
- `open_posting_taxonomy` coverage stays in the existing `it()` block. The new
  block does not repeat it.
- Clean up in a `finally` block with FK-ordered deletes.

Seed one fixture per row. Every row names the signal, the `location_text`, and
the expected pair:

| Seeded signal | `location_text` | Expected resolved | Expected source |
|---|---|---|---|
| `workplace_type` = `onsite` | `US-Remote` | `onsite` | `ats` |
| `telecommuting` = true | `Berlin` | `remote` | `raw_data` |
| `telecommuting` = false | `US-Remote` | NULL | NULL |
| `Remote` = false | `US-Remote` | NULL | NULL |
| `Location Type` = `On-Site` | `Austin, TX` | `onsite` | `raw_data` |
| `Location Type` = `Hybrid (Travel-Required)` | `Austin, TX` | `hybrid` | `raw_data` |
| `Location Type` = `Flexible` | `US-Remote` | `remote` | `location_text` |
| `Location Type` value JSON null | `US-Remote` | `remote` | `location_text` |
| `Job Posting Location` = `["Germany - Remote"]` | `Munich` | `remote` | `raw_data` |
| `Job Posting Location` = `["Israel - Office"]` | `Tel Aviv` | `onsite` | `raw_data` |
| `Job Posting Location` = `[]` | `US-Remote` | `remote` | `location_text` |
| `metadata` is JSON null | `Hybrid- Fremont, CA` | `hybrid` | `location_text` |
| none | `Remote-Friendly, United States` | NULL | NULL |
| none | `US-Remote` | `remote` | `location_text` |
| none | `Hybrid- Fremont, CA` | `hybrid` | `location_text` |
| none | ` Hybrid- Fremont,  CA ` | `hybrid` | `location_text` |
| none | `Hybrid-<U+00A0>Fremont, CA` | `hybrid` | `location_text` |
| none | `Hybrid- Remote, CA` | `hybrid` | `location_text` |
| none | `Onsite- Salem, OR` | `onsite` | `location_text` |
| none | `Sacramento, CA` | NULL | NULL |

The `Remote-Friendly` row is the single highest-value assertion in the suite. The
` Hybrid- Fremont,  CA ` row must resolve identically to the clean
`Hybrid- Fremont, CA` row directly above it. The `<U+00A0>` row carries a literal
non-breaking space and must resolve identically too — it is the only fixture
that exercises the `translate` call.

Do not:

- Assert on generated sqlc code.
- Mock the database. See `testing-guide.md`.

### Task 4: Surface the derived value in the web app

Thread both columns from the view through to the postings page.

- Add `workplace_type_resolved` and `workplace_type_source` to the
  `OpenPostingsDisplayRow` interface in `apps/web/lib/db/postings.ts`, both
  typed `string | null`.
- Add both names to the `SELECT` list in `listOpenPostings`. That list
  enumerates columns explicitly. A name missing from it is absent at runtime
  while the interface still claims it, and `pnpm typecheck` cannot catch the gap.
- Split `listOpenPostings` the way `apps/web/lib/db/status.ts` splits
  `selectFetchHealth(sql: ISql)` from `getFetchHealth()`. `getSql()` calls
  next/server's `connection()`, which throws outside a request scope, so a
  `.db.test.ts` cannot reach the query without the seam.
- Assert the returned row's keys in a `.db.test.ts`, passing the read-only
  client directly. That is the only check that catches a name added to the
  interface but missing from the `SELECT`.
- Add `workplaceTypeLabel(resolved: string | null, source: string | null): string | null`
  to `apps/web/lib/format.ts`. That file holds the app's display formatting and
  already returns `string | null` for absent data.
- Return the resolved value verbatim, lowercase as stored. The sibling lines on
  the page render raw values too.
- Render `<p>Workplace: {label}</p>` in `apps/web/app/postings/page.tsx`, only
  when `workplaceTypeLabel` returns a non-null string. Omit the element entirely
  otherwise.
- This departs from the sibling lines' `?? ""` pattern deliberately. An empty
  `Workplace:` line asserts the field was checked and found absent, which the
  other columns can afford and this one cannot.
- Cover `workplaceTypeLabel` in a new `apps/web/lib/format.test.ts`. It is
  DB-free, so it belongs to `pnpm test`, not `pnpm test:db`.

The label the helper returns:

| `resolved` | `workplace_type_source` | Returns |
|---|---|---|
| non-null | `ats` or `raw_data` | `<resolved> (reported)` |
| non-null | `location_text` | `<resolved> (derived from location)` |
| any other combination | | `null` |

"Any other combination" covers both columns NULL, a mismatched pair the view
cannot produce but the signature allows, and an unrecognized `source` — which a
future adapter could introduce, since tier 1 is designed to pick up new adapters
without a change here.

`ats` and `raw_data` share a label because both are what the source said; the
column keeps them distinct for consumers needing the finer split. A
`location_text` value must read differently — it is a 95.2%-precision inference,
and `project.md`'s evidence trust tiers require a record mixing tiers to keep
them distinguishable.

Do not:

- Restyle the postings page, or add components to it.
- Add filtering, sorting, or faceting by workplace type.
- Treat `workplace_type` and `workplace_type_resolved` as interchangeable. The
  page renders the resolved column; the raw one still means "what the ATS said."

## Sequencing

**Phase 1 (sequential):** Task 1 — the view definition blocks everything.
**Phase 2 (concurrent):** Task 2, Task 3, Task 4 — all three consume the
migration and none consumes another.
**Phase 3 (sequential):** run the four gates — `pnpm test`, `pnpm test:db`, and
`pnpm typecheck` from `apps/web/`, and `make check` from `apps/tools/`. Phase 2's
agents each verify their own work; nothing else runs all four together.

## Why a provenance column

The provenance column exists because `project.md`'s evidence trust tiers require
that records mixing tiers keep them distinguishable. An ATS-supplied value is
ground truth; a regex match is an inference measured at 95.2% precision — but
measured on a labeled population that tier 1 and tier 2 resolve first, so
precision where tier 3 actually fires is unmeasured. Collapsing both into one
column would erase that difference for every consumer downstream.

`workplace_type` itself stays untouched and keeps meaning "what the ATS said."

## Boundary inventory

| Name | SQL column | JSON path | Go field (sqlc) | TS field |
|---|---|---|---|---|
| Resolved workplace type | `workplace_type_resolved` | — | `WorkplaceTypeResolved sql.NullString` | `workplace_type_resolved: string \| null` |
| Derivation provenance | `workplace_type_source` | — | `WorkplaceTypeSource sql.NullString` | `workplace_type_source: string \| null` |
| Workable remote flag | — | `current_snapshot.raw_data->>'telecommuting'` (JSON boolean) | — | — |
| Greenhouse metadata | — | `current_snapshot.raw_data->'metadata'` (array of `{id, name, value, value_type}`) | — | — |

Three `metadata` entry names are read, one per `value_type`: `Remote`
(`yes_no`, boolean `value`), `Location Type` (`single_select`, string `value`),
and `Job Posting Location` (`multi_select`, array-of-string `value`). Take a
boolean or string `value` with `->>'value'`, which returns `text` — compare a
boolean branch to the literals `'true'` and `'false'`, never to a SQL boolean;
iterate an array `value` with `jsonb_array_elements_text`, guarded by
`jsonb_typeof(...) = 'array'` — four `Job Posting Location` entries carry a JSON
null `value` and would error unguarded.

Task 4 adds the TS fields. `apps/web/lib/db/postings.ts` enumerates view columns
explicitly in both the interface and the `SELECT`, so both places need the name
or the field is silently absent at runtime.

## Open questions

1. **Workable's `telecommuting` is arguably an adapter mapping gap.** Reading it
   in the view is correct for the 48 existing postings, but a one-line adapter
   change would populate `workplace_type` properly going forward and make the
   view's Workable branch dead code. Same question, smaller, for Ashby's
   `address.postalAddress` and Greenhouse's `offices[].location` — both are
   unmapped place sources that a future place-extraction plan would want.

2. **Every Greenhouse modality field is per-company custom, and matching on the
   name string does not generalize.** The corpus carries 30 distinct
   company/field pairs across 13 companies; exactly three of them carry
   modality, and all three use a different name, a different `value_type`, and a
   different value shape. A fourth company naming its field differently grows
   coverage not at all, silently. Two options worth weighing later: match on
   field-name patterns rather than literals, or make the field-name-to-branch
   map a seeded table rather than SQL literals so onboarding a company is a data
   change. Neither belongs in this plan.

3. **`raw_data` holds place fields this plan deliberately ignores.** Carbon
   Robotics' `Office Location` and Tenable's `Country (For Website Only)` are
   structured place sources, as are Greenhouse `offices[].location` and Ashby
   `address.postalAddress`. A future place-extraction plan wants all four.
