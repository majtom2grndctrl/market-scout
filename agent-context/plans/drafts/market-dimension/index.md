# Market Dimension

> Curate a discrete, first-class **market** dimension (major tech hubs + macro-regions) from the noisy per-posting location strings, derive it once in the read model, and add it to the composition grammar's closed vocabulary. Geography was excluded from the grammar on purpose (see [`research/composition-grammar.md`](../../../../research/composition-grammar.md)): raw job-posting location text is too noisy to expose as a grouping/filter. This makes location *reliable* by curation rather than exposing the raw text. Investigation notes in [`research.md`](./research.md).

## Goal

Add `market` as a grouping and filter dimension the agent can compose over, backed by a SQL derivation that maps noisy location strings to a curated set of tech hubs and macro-regions. The derivation is honest about coverage: named markets never silently stand in for the whole cohort.

## Scope

### In scope

- A seed table of markets (hubs and macro-regions), each with the location-string patterns that resolve to it, installed by a numbered migration.
- A read-model view that derives, per open posting, the market(s) its location resolves to — multi-match allowed, matching the raw location text rather than trusting the array to be atomic.
- An explicit representation of postings whose location resolves to no market, so a market grouping accounts for the whole cohort.
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
- [ ] `pnpm test:db` passes an assertion that a posting whose location text is `"US - Remote"` resolves to the remote-US market.
- [ ] `pnpm test:db` passes an assertion that a posting whose location text is `"N/A"` (or empty) produces the unmapped representation and no named-market row, proving unmapped locations are visible, not silently absent.
- [ ] `pnpm test:db` passes an assertion that `open_posting_taxonomy` returns the derived market rows under `term_kind = 'market'` for an open posting, and returns them for a posting that has **no** classification (market does not depend on classification).
- [ ] `pnpm test` passes the updated `requiresDenominator` suite: the `EXPECTED` table covers exactly `GROUPINGS` (now including `market`) and `requiresDenominator("market")` returns the decided value.
- [ ] `pnpm test` and `pnpm typecheck` pass with `market` present in `GROUPINGS` and `FILTER_DIMENSIONS`; the composition JSON schema exported by `COMPOSITION_JSON_SCHEMA` lists `market` among the `groupBy` and `filter.dim` enum members. *(review/grep gate: confirm the exported schema, not only that types compile.)*
- [ ] A reviewer can read the market seed rows in the migration and confirm every seed market names concrete patterns grounded in strings that occur in the corpus, and that the seed includes at least the hubs whose measured open-cohort counts exceed 100 (SF Bay Area, New York, Seattle, London, Singapore, India). *(review gate.)*
- [ ] The migration's `down` cleanly reverts the view and seed table, and the header carries a local performance-check comment in the convention of `000017`/`000019`. *(review gate.)*

## Tasks

### Task 1 — Market seed table and seed rows

Add a numbered migration (`000020`, the next free number) that creates a seed table naming each market and the patterns that resolve to it.

- Give each market a stable `slug` (e.g. `sf-bay-area`, `new-york`, `seattle`, `london`, `singapore`, `india`, `remote-us`) and a display `name`; the grammar filter value and the taxonomy `slug`/`name` come from these. Slugs are the durable identifier the composition link serializes.
- Give each market a `kind` discriminator distinguishing a metro **hub** from a **macro-region** (e.g. `US`, `Europe`), because the two answer different questions and a later denominator decision may treat them differently.
- Store the resolving patterns as data (a child table or an array column of patterns per market), not as branches baked into the view — a curated dictionary changes far more often than view logic, and data-driven patterns let a reviewer read the whole dictionary in one place.
- Match with word-boundary semantics (Postgres `~*` with `\y`), because bare substring matching maps `"Remote"` inside `"Vermont"` and `"SF"` inside unrelated tokens. The word-boundary form is what the workplace-type derivation in `000019` already relies on.
- Seed hubs grounded in real corpus strings, including their suburb spellings: fold `Everett`/`Bellevue`/`Kirkland`/`Kent`/`Bellingham, WA` into `seattle`, and `Mountain View`/`Fremont`/`Milpitas`/`Palo Alto`/`Menlo Park`/`Sunnyvale`/`San Jose`/`Oakland` into `sf-bay-area`. A hub that only matches its headline city name silently drops a large share of its metro (Everett alone is 106 open postings).
- Decide the macro-region and remote seed set per the Open questions below and the coverage target; whichever is chosen, the seed is complete for that decision — no `TODO` markets.

Do not:
- Hand-edit sqlc output or add a `queries/` entry for this; the derivation is read-model SQL consumed by views and the MCP query gateway, not a Go query.
- Add an agent-write path or MCP function for markets; the set is seeded, not runtime-minted.
- Store SQL text as a pattern value; patterns are regex/literal strings matched by the view, never executed.

### Task 2 — Market derivation view

Add, in the same migration, a view that resolves each open posting to its market(s).

- Join the open cohort the way `open_postings_display` does (open posting → its run-scoped current snapshot), because market must ride the same "open" definition every other read-model view uses; a second openness definition would drift.
- Match each market's patterns against the posting's raw location text (the scalar `location_text`, or `location_texts` joined), **not** against individual array elements as if atomic — the array's own elements carry concatenations (see `research.md`), so a per-element match understates multi-market postings.
- Emit one row per (posting, matched market): a posting matching two markets yields two rows, mirroring the many-to-many shape roles/skills already have in the taxonomy.
- Represent an unmapped posting explicitly per the Open questions decision (either an `unmapped` seed market it resolves to, or a coverage denominator carried by the grammar) — a posting with a location that matches nothing must remain countable, because a market grouping that drops it would imply a smaller cohort than exists.
- Carry a header comment with a local performance check (row count, warm `EXPLAIN (ANALYZE, TIMING OFF)` median) in the convention of the `000017` and `000019` headers, because these views run on every analysis read and the header is where the perf budget is recorded.

Do not:
- Alter, upsert, or write `posting_snapshots` — the derivation is read-only over append-only history.
- Auto-create a market row for an unrecognized location string; unmatched resolves to unmapped, never to a new market.

### Task 3 — Extend `open_posting_taxonomy` with a market term-kind

In the same migration, replace `open_posting_taxonomy` (via `CREATE OR REPLACE VIEW`) to add a `'market'` branch.

- Add a `UNION ALL` branch selecting `job_posting_id`, `'market'::text AS term_kind`, and the market `slug`/`name` from the derivation view, matching the existing four-column shape (`job_posting_id`, `term_kind`, `slug`, `name`).
- Join the market branch on `job_posting_id`, not `classification_id` — every existing branch joins through the current classification, but a market exists whether or not the posting is classified, so classification-joining it would hide markets for the ~65% of the cohort without a classification.
- Preserve the existing role/specialization/skill/dimension branches unchanged; this task only appends a branch.

Do not:
- Route the market branch through `open_postings_display.classification_id`; that is the mistake this task exists to avoid.
- Drop or reorder the existing branches; the taxonomy's four-column contract and its existing `term_kind` values are relied on by the web app and MCP consumers.

### Task 4 — Expose `market` in the composition grammar

Add `market` to the closed vocabulary and its rules in `apps/web/lib/composition/`.

- Add `"market"` to `GROUPINGS` in `vocabulary.ts`; declared order is canonical serialization order, so place it deliberately (a market is a corpus dimension like `company`, so grouping it near `company` reads naturally). Its position fixes how a market grouping stringifies in the deep link.
- Add `"market"` to `FILTER_DIMENSIONS`; it `satisfies readonly Grouping[]`, so a filter dimension must also be a grouping, and the value the model supplies is a market `slug`.
- Add a `market:` entry to `REQUIRES_DENOMINATOR` in `rules.ts` per the Open questions decision — the `Record<Grouping, boolean>` makes the missing entry a compile error, so this is not optional. The decision hinges on whether market is treated as full-corpus (present on ~99% of postings, derived from the snapshot not the classification) or as coverage-bearing (the unmapped residual).
- The zod enums in `schema.ts` build from these tuples, so no schema edit is needed; adding to the tuples threads `market` into `compositionSchema`, `COMPOSITION_JSON_SCHEMA`, `orderGroupings`, and `orderFilters` automatically. Verify rather than duplicate.

Do not:
- Add a parallel `market` enum or a bespoke schema branch; the whole design relies on `GROUPINGS`/`FILTER_DIMENSIONS` being the single source the schema derives from.
- Expose market values as free strings in the schema; the filter value is a market slug from the seed table, validated the same way other filter values are (exact value on the dimension).

### Task 5 — Update grammar unit tests

Update the composition test suite to reflect the new grouping.

- Add a `market:` line to the `EXPECTED: Record<Grouping, boolean>` table in `composition.test.ts` (near line 244) matching the `REQUIRES_DENOMINATOR` decision — the suite asserts `Object.keys(EXPECTED).sort() === [...GROUPINGS].sort()`, so the addition is mandatory for the suite to pass.
- Add or extend a case proving a `market` grouping/filter carries (or does not carry) the coverage denominator per the decision, so the intended coverage semantics are pinned by a test, not left implicit.

Do not:
- Weaken the `covers exactly the grouping vocabulary` assertion to accommodate the new member; update the `EXPECTED` table instead.

### Task 6 — Derivation db tests

Add a db test mirroring `read-model-views.db.test.ts` for the market derivation.

- Follow the established pattern exactly: seed via the owner DSN, read via the read-only DSN, `context.skip()` when either DSN is absent, and clean up marker-scoped rows in a `finally` block. A new pattern would diverge from the one the other read-model tests share.
- Cover, at minimum: a single-location posting resolving to one market; a suburb spelling folding into its hub; one concatenated location string resolving to two markets; and an unmapped/`N/A` location producing the unmapped representation and no named-market row.
- Assert market rows surface through `open_posting_taxonomy` under `term_kind = 'market'`, including for a posting with no classification row, proving the `job_posting_id` join.
- Clean up any market seed rows the test introduces, if the test seeds its own markets rather than relying on the migration's seeds — leave the shared market table as it was found.

Do not:
- Assert against live production location strings; seed deterministic fixtures so the test does not drift as the corpus changes.
- Add these cases to the non-db `test` suite; they need a live Postgres and belong under `test:db`.

## Sequencing

- **Phase 1 (SQL, one migration):** Tasks 1–3 land together in migration `000020` — seed table + seed rows, derivation view, and the `open_posting_taxonomy` replacement. They share one migration because the view depends on the seed table and the taxonomy depends on the view; splitting them would leave intermediate migrations that don't compose.
- **Phase 2 (grammar):** Task 4, after the Open questions denominator decision is made — it determines the `REQUIRES_DENOMINATOR` value.
- **Phase 3 (tests):** Tasks 5 and 6 in parallel — Task 5 (unit) depends only on Phase 2; Task 6 (db) depends only on Phase 1. Both are independent of each other.

## Boundary inventory

This feature crosses SQL ↔ TS. The market slug is the shared identifier.

| Concept | SQL side | TS / grammar side |
|---|---|---|
| The dimension | `term_kind = 'market'` in `open_posting_taxonomy` | `"market"` in `GROUPINGS` / `FILTER_DIMENSIONS` |
| A market's identity | seed table `slug` / `name` | filter value = market `slug`; taxonomy row `slug`/`name` |
| Coverage honesty | unmapped representation in the derivation view | `REQUIRES_DENOMINATOR["market"]` |
| Openness | `open_postings` run-scoped join (shared with other views) | cohort modifier (unchanged) |

## Open questions

- **How is a posting matching multiple markets represented?** The draft assumes multiple taxonomy rows (one per matched market), mirroring roles/skills. Confirm the measure engine's `share`/`count` semantics tolerate a posting counted under several markets (a market grouping would then sum to more than the cohort), and whether that is the intended reading or whether a primary-market tiebreak is wanted. This is the crux of the reliability angle.
- **Do unmapped locations get an explicit `unmapped` bucket, or a coverage denominator?** Measured hard-unmappable floor is ~0.8%, but the mapped fraction depends on dictionary breadth (~81% with hubs+remote alone, ~90%+ with macro-regions). Two honest routes: (a) seed an `unmapped` market so named + unmapped sum to the cohort, or (b) mark `market` as denominator-bearing so the engine reports mapped-vs-cohort coverage. They are mutually exclusive and drive the `REQUIRES_DENOMINATOR["market"]` value.
- **Does `market` require a denominator at all, given location is on the snapshot, not the classification?** Every current `REQUIRES_DENOMINATOR: true` dimension slices the ~35% classified corpus; market is present on ~99% of postings and does not depend on classification, which argues for `false`. But the unmapped residual is a *different* coverage gap the mechanism could still surface. Deciding `true` vs `false` here is the same decision as the bullet above and must be resolved before Task 4.
- **Are bare macro-regions (`United States`, `United Kingdom`, `Europe`, `North America`) admitted as markets, or left unmapped?** They are the single largest slice of the unmatched ~19% (≈820 open postings). Admitting them lifts coverage sharply but mixes hub-grain and region-grain values in one dimension; the `kind` discriminator exists to keep them distinguishable if admitted.
