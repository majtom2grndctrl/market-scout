# Market Dimension

> Curate a discrete, first-class **market** dimension (major tech hubs + macro-regions) from the noisy per-posting location strings, derive it once in the read model, and add it to the composition grammar's closed vocabulary. Geography was excluded from the grammar on purpose (see [`research/composition-grammar.md`](../../../../research/composition-grammar.md)): raw job-posting location text is too noisy to expose as a grouping/filter. This makes location *reliable* by curation rather than exposing the raw text. Investigation notes in [`research.md`](./research.md).

## Goal

Add `market` as a grouping and filter dimension the agent can compose over, backed by a SQL derivation that maps noisy location strings to a curated set of tech hubs and macro-regions. The derivation is honest about coverage: named markets never silently stand in for the whole cohort.

## Scope

### In scope

- A seed table of markets, each with the location-string patterns that resolve to it, installed by a numbered migration.
- Metro hubs, macro-regions (e.g. `us`, `europe`), and remote tiers (e.g. `remote-us`), tagged by a `kind` discriminator (`hub` / `region` / `remote`) so the grains stay distinguishable within one dimension. Macro-regions carry the ~19% of the cohort that names only a region; remote tiers carry the location strings that name only a remote scope.
- A read-model view that derives, per open posting, the market(s) its location resolves to — multi-match allowed, matching the raw location text rather than trusting the array to be atomic.
- A reserved `unmapped` market the derivation emits for a posting matching no seeded pattern, so every open posting appears at least once under a market grouping — named markets or `unmapped` — and none silently vanish. The guarantee is visibility, not summation: named markets over-count via multi-match, so bars do not sum to the cohort.
- A `'market'` branch added to `open_posting_taxonomy`, joining on `job_posting_id` (not classification), so markets exist for unclassified postings.
- `market` added to the grammar's `GROUPINGS`, `FILTER_DIMENSIONS`, and the `REQUIRES_DENOMINATOR` table, with the unit-test tables updated to match.
- Db tests for the derivation covering single-location, multi-location, overlapping-match, and unmapped-location cases.

### Out of scope

- Exposing raw `location_text` / `location_texts` as their own grammar dimension — the whole point is curation over exposure.
- City-level or country-complete geography. The dimension is a curated shortlist of markets, not a gazetteer. Long-tail locations resolve to a macro-region or to unmapped, never to a new auto-minted market.
- Sub-market or neighborhood granularity, and any distance/geocoding logic. Matching is string-pattern only.
- Backfilling a market onto closed or historical cohorts beyond what the existing `open_posting_taxonomy` / read-model views already expose. Market rides the same open-cohort join as the other taxonomy terms.
- Changing `posting_snapshots`. The derivation reads snapshots; it never writes or alters them (append-only invariant).
- Agent-minted markets at runtime. The market set is seeded and curated; unlike roles/skills, the classifier never introduces a new market.

## Acceptance criteria

- [ ] `pnpm test:db` passes a new derivation test asserting that a posting whose location text is `"San Francisco, CA"` resolves to exactly the `sf-bay-area` market and no other.
- [ ] `pnpm test:db` passes an assertion that a posting whose location text is `"Everett, WA"` resolves to the `seattle` market, proving suburb spellings fold into their hub rather than being dropped.
- [ ] `pnpm test:db` passes an assertion that a posting whose single location string is `"San Francisco, CA | New York City, NY"` resolves to **both** `sf-bay-area` and `new-york` (two rows), proving multi-market matching reads the concatenated string rather than trusting one array element per location.
- [ ] `pnpm test:db` passes an assertion that a posting whose location text is `"US - Remote"` resolves to the `remote-us` market.
- [ ] `pnpm test:db` passes an assertion that a posting whose location text is `"United States"` (a bare region) resolves to the `us` macro-region market, whose `open_posting_markets` row carries `kind = 'region'` (read from `open_posting_markets`, which surfaces `kind`; `open_posting_taxonomy` does not).
- [ ] `pnpm test:db` passes an assertion that a posting whose location text is `"N/A"` (or empty) resolves to exactly the reserved `unmapped` market (slug `unmapped`) and no named-market row, proving unmapped locations are a visible value, not silently absent.
- [ ] `pnpm test:db` passes an assertion that `open_posting_taxonomy` returns the derived market rows under `term_kind = 'market'` for an open posting, and returns them for a posting that has **no** classification (market does not depend on classification).
- [ ] `pnpm test` passes the updated `requiresDenominator` suite: the `EXPECTED` table covers exactly `GROUPINGS` (now including `market`) and `requiresDenominator("market")` returns `false` — location is present on ~99% of snapshots, so market carries no classification-coverage debt; the `unmapped` value carries dictionary-breadth honesty instead.
- [ ] `pnpm test` and `pnpm typecheck` pass with `market` present in `GROUPINGS` and `FILTER_DIMENSIONS`; the composition JSON schema exported by `COMPOSITION_JSON_SCHEMA` lists `market` among the `groupBy` and `filter.dim` enum members. *(review/grep gate: confirm the exported schema, not only that types compile.)*
- [ ] A reviewer can read the market seed rows in the migration and confirm every seed market names concrete patterns grounded in strings that occur in the corpus, and that the seed includes at least the hubs whose measured open-cohort counts exceed 100 (SF Bay Area, New York, Seattle, London, Singapore, India). *(review gate.)*
- [ ] The migration's `down` restores `open_posting_taxonomy` to its pre-000020 four-branch definition, then drops the derivation view and seed table (dependent before base), and the up header carries a local performance-check comment in the convention of `000017`/`000019`. *(review gate.)*

## Tasks

### Task 1 — Market seed table and seed rows

Add a numbered migration (`000020`, the next free number) that creates a seed table naming each market and the patterns that resolve to it.

- Give each market a stable lowercase-kebab `slug` (e.g. `sf-bay-area`, `new-york`, `seattle`, `london`, `singapore`, `india`, `remote-us`, `us`, `europe`). Slugs are the durable identifier the composition link serializes and the grammar filter value.
- Give each market a display `name`. The taxonomy row's `name` comes from it, so it is what a reader sees.
- Give each market a `kind` discriminator with one of three values — `hub`, `region`, or `remote` — because the dimension deliberately mixes three grains (metro, macro-region, remote scope) and a consumer must be able to tell them apart.
- Store the resolving patterns as data (a child table or an array column per market), not as branches baked into the view — a curated dictionary changes far more often than view logic, and data-driven patterns let a reviewer read the whole dictionary in one place.
- Match with word-boundary semantics (Postgres `~*` with `\y`), because bare substring matching maps `"Remote"` inside `"Vermont"`. The word-boundary form is what the workplace-type derivation in `000019` already relies on.
- Ground every seeded market's patterns in strings that actually occur in the corpus — this is the seeding contract for all listed slugs, not only the two worked below. `seattle` and `sf-bay-area` are worked in detail to show the suburb-folding depth expected; `new-york`, `london`, `singapore`, and `india` need the same grounding at their own scale.
- Harvest the real spellings first: query the live corpus over the read-only MCP (`select:mcp__market-scout-postgres__query`) — `SELECT location_text, count(*) FROM open_postings_display GROUP BY 1 ORDER BY 2 DESC` and the same over `unnest(location_texts)` from the run-scoped snapshots — and derive patterns from what appears, rather than inventing plausible city names. The patterns are only as honest as the strings they were read from.
- Fold suburb spellings into their hub: `Everett`/`Bellevue`/`Kirkland`/`Kent`/`Bellingham, WA` into `seattle`, and `Mountain View`/`Fremont`/`Milpitas`/`Palo Alto`/`Menlo Park`/`Sunnyvale`/`San Jose`/`Oakland` into `sf-bay-area`. A hub matching only its headline city drops a large share of its metro — `Everett` alone is 106 open postings.
- Seed macro-region markets (`us`, `europe`, and the region strings the corpus carries — `north america`, `united kingdom`, `emea`) tagged `kind = 'region'`, because bare-region strings are ~19% of the open cohort and admitting them lifts coverage from ~81% to ~90%+.
- Seed remote-tier markets (`remote-us`, matching the corpus's remote-scope spellings — `US - Remote`, `Remote - US`, `USA - Remote`, `US-Remote`, `United States - Remote`) tagged `kind = 'remote'`. A remote scope is a distinct grain from both a hub and a region, so it takes its own `kind` value rather than being forced into one of theirs.
- Seed the reserved `unmapped` market: a slug the derivation assigns when no seeded pattern matches (Task 2). It has no patterns of its own — it is the else. This is what keeps a location matching nothing countable rather than invisible.
- The seed is complete as delivered — no `TODO` markets.
- Write the `.down.sql` too: it restores `open_posting_taxonomy` to its pre-000020 four-branch definition, then drops `open_posting_markets` and the seed table (dependent before base), mirroring the down-ordering in `000017`/`000019`.

Do not:
- Hand-edit sqlc output or add a `queries/` entry for this; the derivation is read-model SQL consumed by views and the MCP query gateway, not a Go query.
- Add an agent-write path or MCP function for markets; the set is seeded, not runtime-minted.
- Store SQL text as a pattern value; patterns are regex/literal strings matched by the view, never executed.

### Task 2 — Market derivation view

Add, in the same migration, a view `open_posting_markets` that resolves each open posting to its market(s). Later phases read this view by that name and its columns `job_posting_id`, `slug`, `name`, `kind`.

- Resolve each open posting's current snapshot with a run-scoped lateral mirroring `open_postings_display`'s: filter `posting_snapshots` on `job_posting_id` and `fetch_run_id = open_postings.fetch_run_id`, `ORDER BY fetched_at DESC, id DESC LIMIT 1`. Copying the full tiebreak keeps market on the same "open" definition every other read-model view uses; a second definition would drift.
- Read `location_texts` (the `text[]` column) directly from `posting_snapshots` in that lateral, not from `open_postings_display` — the display view exposes only the scalar `location_text`, not the array.
- Run each market's patterns (`~*` with `\y`) against the full text of each unnested `location_texts` element. Multi-market matching falls out of this: one element can itself carry a concatenation, so matching its full text lets one string resolve to several markets.
- Emit one row per (posting, matched market) carrying `job_posting_id`, `slug`, `name`, and `kind` (from the seed table): a posting matching two markets yields two rows, mirroring the many-to-many shape roles/skills already have in the taxonomy. `kind` lives here on `open_posting_markets`, since the four-column `open_posting_taxonomy` (Task 3) cannot carry it.
- Emit the reserved `unmapped` market (slug `unmapped`) once per posting that matches no seeded pattern — resolve unmapped at **posting grain**, via `WHERE NOT EXISTS (a matched market for that posting)`, not per unnested element. A posting with several unmatched location strings must still yield exactly one `unmapped` row, never one per string.
- Carry a header comment with a local performance check (row count, warm `EXPLAIN (ANALYZE, TIMING OFF)` median) in the convention of the `000017` and `000019` headers, because these views run on every analysis read.

Do not:
- Require an element to *equal* a market name (exact `==`); patterns match *within* an element's full text, which is what lets a concatenated element resolve to several markets (see `research.md`).
- Emit `unmapped` at element grain; that yields one `unmapped` row per unmatched string instead of one per posting.
- Alter, upsert, or write `posting_snapshots` — the derivation is read-only over append-only history.
- Auto-create a named market row for an unrecognized location string; unmatched resolves to `unmapped`, never to a new market.

### Task 3 — Extend `open_posting_taxonomy` with a market term-kind

In the same migration, replace `open_posting_taxonomy` (via `CREATE OR REPLACE VIEW`) to add a `'market'` branch.

- Add a `UNION ALL` branch selecting `job_posting_id`, `'market'::text AS term_kind`, and `slug`/`name` from `open_posting_markets` (Task 2's view), matching the existing four-column shape (`job_posting_id`, `term_kind`, `slug`, `name`).
- Join the market branch on `job_posting_id`, not `classification_id` — every existing branch joins through the current classification, but a market exists whether or not the posting is classified, so classification-joining it would hide markets for the ~65% of the cohort without a classification.
- Preserve the existing role/specialization/skill/dimension branches unchanged; this task only appends a branch.

Do not:
- Select `kind` (or any fifth column) into the branch; the four-column shape is what keeps `CREATE OR REPLACE VIEW` legal, and `kind` lives on `open_posting_markets` for consumers that need it.
- Route the market branch through `open_postings_display.classification_id`; that is the mistake this task exists to avoid.
- Drop or reorder the existing branches; the taxonomy's four-column contract and its existing `term_kind` values are relied on by the web app and MCP consumers.

### Task 4 — Expose `market` in the composition grammar

Add `market` to the closed vocabulary and its rules in `apps/web/lib/composition/`.

- Add `"market"` to `GROUPINGS` in `vocabulary.ts`; declared order is canonical serialization order, so place it deliberately (a market is a corpus dimension like `company`, so grouping it near `company` reads naturally). Its position fixes how a market grouping stringifies in the deep link.
- Add `"market"` to `FILTER_DIMENSIONS`; it `satisfies readonly Grouping[]`, so a filter dimension must also be a grouping, and the value the model supplies is a market `slug`.
- Add `market: false` to `REQUIRES_DENOMINATOR` in `rules.ts` — the `Record<Grouping, boolean>` makes the missing entry a compile error, so this is not optional. `false` because location is present on ~99% of snapshots, so market carries none of the classification-coverage debt the `true` dimensions do; the `unmapped` value carries dictionary-breadth honesty instead.
- The zod enums in `schema.ts` build from these tuples, so no schema edit is needed; adding to the tuples threads `market` into `compositionSchema`, `COMPOSITION_JSON_SCHEMA`, `orderGroupings`, and `orderFilters` automatically. Verify rather than duplicate.

Do not:
- Add a parallel `market` enum or a bespoke schema branch; the whole design relies on `GROUPINGS`/`FILTER_DIMENSIONS` being the single source the schema derives from.
- Add market-specific filter-value validation. Filter values are free strings for every dimension (`schema.ts` validates `filter.value` as a non-empty string, never against a DB table); a market filter value is *semantically* a seed slug but stays a free string like `company` or `skill` — do not special-case it.

### Task 5 — Update grammar unit tests

Update the composition test suite to reflect the new grouping.

- Add `market: false` to the `EXPECTED: Record<Grouping, boolean>` table in `composition.test.ts` (near line 244) — the suite asserts `Object.keys(EXPECTED).sort() === [...GROUPINGS].sort()`, so the addition is mandatory for the suite to pass.
- Assert `requiresDenominator("market") === false`, pinning that a market grouping carries no coverage denominator rather than leaving it implicit.

Do not:
- Weaken the `covers exactly the grouping vocabulary` assertion to accommodate the new member; update the `EXPECTED` table instead.

### Task 6 — Derivation db tests

Add a db test mirroring `read-model-views.db.test.ts` for the market derivation.

- Follow the established pattern exactly: seed via the owner DSN, read via the read-only DSN, `context.skip()` when either DSN is absent, and clean up marker-scoped rows in a `finally` block. A new pattern would diverge from the one the other read-model tests share.
- Populate the `location_texts` (`text[]`) column on every fixture snapshot — that is the column `open_posting_markets` reads. The mirrored `read-model-views.db.test.ts` seeds only the scalar `location_text`; a fixture that leaves `location_texts` NULL resolves every posting to `unmapped` and fails the suite opaquely.
- Query `open_posting_markets` directly (by that name; columns `job_posting_id`, `slug`, `name`, `kind`) for the derivation assertions; query `open_posting_taxonomy` only for the `term_kind = 'market'` surfacing assertion, which carries no `kind`.
- Cover, at minimum: a single-location posting resolving to one market; a suburb spelling folding into its hub; one location string carrying a concatenation **in a single array element** resolving to two markets; a bare-region string resolving to a macro-region whose `open_posting_markets` row carries `kind = 'region'`; and an `N/A`/empty location producing exactly one `unmapped` market row and no named-market row.
- Assert market rows surface through `open_posting_taxonomy` under `term_kind = 'market'`, including for a posting with no classification row, proving the `job_posting_id` join.
- Clean up any market seed rows the test introduces, if the test seeds its own markets rather than relying on the migration's seeds — leave the shared market table as it was found.

Do not:
- Assert against live production location strings; seed deterministic fixtures so the test does not drift as the corpus changes.
- Add these cases to the non-db `test` suite; they need a live Postgres and belong under `test:db`.

## Sequencing

- **Phase 1 (SQL, one migration):** Tasks 1–3 land together in migration `000020` — seed table + seed rows, derivation view, and the `open_posting_taxonomy` replacement. They share one migration because the view depends on the seed table and the taxonomy depends on the view; splitting them would leave intermediate migrations that don't compose.
- **Phase 2 (grammar):** Task 4 — adds `market` to the vocabulary with `REQUIRES_DENOMINATOR["market"] = false` (decided below).
- **Phase 3 (tests):** Tasks 5 and 6 in parallel — Task 5 (unit) depends only on Phase 2; Task 6 (db) depends only on Phase 1. Both are independent of each other.

## Boundary inventory

This feature crosses SQL ↔ TS. The market slug is the shared identifier.

| Concept | SQL side | TS / grammar side |
|---|---|---|
| The dimension | `term_kind = 'market'` in `open_posting_taxonomy` | `"market"` in `GROUPINGS` / `FILTER_DIMENSIONS` |
| A market's identity | seed table `slug` / `name` | filter value = market `slug`; taxonomy row `slug`/`name` |
| Market grain | seed table + derivation view `kind` (`hub`/`region`/`remote`) | not exposed in the grammar; read from the derivation view |
| Coverage honesty | `unmapped` market row in the derivation view | `REQUIRES_DENOMINATOR["market"] = false` |
| Openness | `open_postings` run-scoped join (shared with other views) | cohort modifier (unchanged) |

## Decisions

- **Multi-market postings emit multiple rows, no tiebreak.** A posting in London and New York counts under both, mirroring how `role`/`skill` already behave in the taxonomy. Named-market bars therefore over-count and do not sum to the cohort — the honesty guarantee is that no posting is invisible, carried by the `unmapped` value.
- **Unmapped is an explicit `unmapped` market value, and `market` needs no denominator (`false`).** Location is present on ~99% of snapshots — market has none of the classification-coverage debt the `true` dimensions carry, so `requiresDenominator` (which means "fraction of the ~35% classified corpus") would misdescribe it. Dictionary-breadth honesty lives in the visible, groupable, filterable `unmapped` value instead of a scalar. Keeps the denominator concept about classification only.
- **Three grains in one dimension, tagged by `kind` (`hub`/`region`/`remote`).** Bare-region strings are ~19% of the open cohort; admitting `us`/`europe`/etc. lifts coverage from ~81% to ~90%+ and shrinks `unmapped` toward the ~0.8% hard floor. Remote-scope strings are their own grain, neither hub nor region. `kind` keeps all three distinguishable within the one dimension, surfaced on the derivation view (not the four-column taxonomy).

## Cross-spec seam

- **measure-engine `share` normalization.** Because multi-market postings count under several markets, a `share` grouped by `market` must normalize over the sum of market assignments, not the posting count — the same treatment any multi-valued taxonomy dimension (`skill`, `role`) already needs. measure-engine owns that normalization; this spec only guarantees the many-rows-per-posting shape it consumes.
