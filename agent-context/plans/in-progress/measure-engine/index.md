# Measure Engine

> Brief — decisions and behavior. Rationale and live numbers in `research.md`. Roadmap: Compositional Analysis Surface → Grammar.

## Goal

Compute a validated `Composition` into typed rows plus a cohort-specific coverage denominator. The engine is the other half of the grammar's boundary: the grammar validates and hands over a typed value; the engine reads Postgres and hands back numbers. The model never sees SQL; a number only reaches the model after the engine computed it. Lives in `apps/web/lib/db`, over the read-model views.

## Scope

### In scope

- `runComposition(composition): Promise<Result<MeasureResult, GrammarError>>` — the single entry point. Input is a `Composition` the caller already validated with `validate()`; the engine re-validates and refuses an invalid crossing rather than trusting the caller. It returns the grammar's existing `Result<T, E>` union (`errors.ts`), never throwing on a refusal — so a caller narrates "no" the same way it does for `validate()`.
- All six measures: `count`, `delta`, `share`, `rate` (aggregate) and `age`, `lifespan` (per-posting distribution).
- Cohort membership: `open`, `closed`, `all`, plus as-of-open reconstruction for `delta`.
- Grouping by every vocabulary grouping, including `week` (UTC weekly buckets), the `function` → role-dimension mapping, and `market` — a multi-valued grouping resolved from the curated market derivation, not classification.
- Filters on every filter dimension, applied as parameters, never string-built.
- A **read-model migration** (Go side) that defines closed/all cohort membership, the per-posting lifespan primitive, a cohort-agnostic taxonomy (role/specialization/skill/function **and market**), and parameterized as-of-open — so openness stays defined once in SQL and the engine never reproduces it. `market-dimension` wired `market` only into the open-cohort views (`open_posting_markets`, the `term_kind = 'market'` branch of `open_posting_taxonomy`); a cohort-agnostic market source is the same debt this migration already pays for the classified dimensions.
- Multi-valued `share` normalization for `market`, `role`, `specialization`, and `skill` — a posting counts under every group it belongs to, so `share` normalizes over the sum of group assignments, not the posting count. `market-dimension` assigns this treatment to this spec (its *Cross-spec seam*).
- Cohort-specific coverage denominator, returned inline, present only when the composition `requiresDenominator`.
- Failed / absent weeks returned as gap rows, never interpolated.
- `.db.test.ts` coverage with seeded deterministic fixtures.

### Out of scope

- The chart layer — `chart-primitives`. The engine returns data; the chart owns scales, geometry, and histogram bin edges.
- Registering `runComposition` as a tool / streaming it — `chat-transport`.
- The `/view` route, deep-link wiring, and turning rows into a rendered composition — frame and route specs.
- Composite and co-occurrence measures — absent from the grammar; not here.
- Interestingness scoring — `composition-quality-evals`.
- The deterministic demo fixture corpus — `demo-fixture-corpus`. This spec seeds its own throwaway test fixtures; it does not build the shared corpus.

## Acceptance criteria

- [ ] `runComposition` accepts a validated `Composition` and returns `{ ok: true, value }` wrapping a `MeasureResult` with typed rows for each of the six measures.
- [ ] A composition that fails `validate()` is refused by `runComposition` returning `{ ok: false, error }` where `error` is the grammar's `GrammarError` (not a thrown exception), and no query runs — verified by a `sql` proxy that counts invocations (zero on the refusal path).
- [ ] `count` over the `open` cohort with no grouping returns a total equal to a direct `SELECT count(*) FROM open_postings` on the same client — asserted by equality, not a hardcoded number.
- [ ] `delta` over a window returns the signed difference between open-count-now and open-count-as-of-(now − window), computed from the read model's as-of-open definition, not a re-implemented one.
- [ ] `rate` requires `week` in `groupBy` and returns one row per UTC week in range. A week with no successful run in the series calendar is present as a gap row, distinguishable from a week whose count is zero.
- [ ] A week-grouped `count`, `delta`, or `share` returns one explicit row per UTC week in range — empty weeks included — flagging a week with no successful run in scope as a gap row (`value: 0`, `gap: true`), distinguishable from a genuine-zero week that had a successful run. Gap granularity is per-(company, week) when `company` is grouped, otherwise scope-wide per week applied to every group key in that week; a `share` gap row contributes nothing to the normalizer, which still sums to 1.0 over the real values.
- [ ] `lifespan` returns one row per closed posting carrying its duration in days; a posting seen in exactly one successful run follows the single-run rule stated in Task 4 (last-seen == first-seen yields a 0-day row).
- [ ] `age` returns one row per open posting carrying its current age in days.
- [ ] A grouped `delta` returns one signed row per group; a grouped `age`/`lifespan` tags each row with its group (small multiples).
- [ ] `share` returns per-group values that sum to 1.0 (within floating tolerance) across the groups in each composition, including for a multi-valued grouping (`market`, `role`, `specialization`, `skill`) where a posting counts under several groups — proving the normalizer is the sum of group assignments, not the posting count.
- [ ] A two-grouping `share` (e.g. `stacked_bar`) normalizes globally: every cell the composition produces sums to 1.0 across the whole result, not per outer-group panel.
- [ ] `count` grouped by `market` over the `closed` cohort returns one row per matched market for closed postings, resolved from the cohort-agnostic market source rather than the open-only `open_posting_markets`; a closed posting matching no seed pattern appears under exactly one `unmapped` row.
- [ ] A composition whose grouping or filter `requiresDenominator` returns a denominator giving classified count and cohort total; the open-cohort denominator reflects the cohort actually queried, not a corpus-wide constant.
- [ ] A composition that requires no denominator returns none.
- [ ] Grouping by `function` resolves to role dimensions; grouping by `role`/`specialization`/`skill` resolves to the matching taxonomy terms for the composition's cohort, not the open-only view.
- [ ] Grouping by `seniority` resolves to the latest classification's seniority for each posting in the composition's cohort, with unclassified postings absent from the grouped rows.
- [ ] Filter values reach SQL as bound parameters; grouping keys resolve through a fixed lookup, so no model-supplied string is concatenated into SQL text. *(Review/grep gate; a filter value carrying SQL metacharacters treated as a literal is the one runnable half.)*
- [ ] The migration header records an `EXPLAIN (ANALYZE, TIMING OFF)` median for the lifespan primitive, matching the perf-check convention in migrations 000017 and 000019; no new view reads `raw_data`. *(Review/grep gate, not a `pnpm test` assertion.)*
- [ ] `pnpm test:db` passes with the new fixtures; `pnpm typecheck` passes.

## Tasks

### Task 1: Read-model cohort and lifespan primitives (migration)

Add a numbered migration under `apps/tools/internal/db/migrations/` (up + down) that generalizes the read model past open-only. Mirror the existing view migrations' shape and the header-comment convention that records a local performance check.

- Define closed and all cohort membership so the engine reads each cohort, never reconstructs it. `open` already exists as `open_postings`; closed = seen in a success run but absent from open; all = ever seen in a success run. A cohort a view defines once cannot drift from the engine's copy.
- Add a per-posting lifespan primitive keyed by `job_posting_id`: first-seen and last-seen from `fetch_runs.started_at` across success runs, and a closed flag. `started_at` is the run's real timestamp — one per run, per `research.md`.
- Add a latest-classification primitive keyed by `job_posting_id`: the single newest classification per posting (`ORDER BY classified_at DESC, id DESC LIMIT 1`), mirroring `open_postings_display`'s lateral. Superseded classifications coexist per posting; a join across all of them double-counts terms. The taxonomy and seniority below both derive from this, so the latest-only rule is stated once.
- Add a cohort-agnostic taxonomy keyed by `job_posting_id` — role, specialization, skill, dimension, **and market** terms for any seen posting. Role/specialization/skill/dimension resolve through the latest-classification primitive; market resolves through the cohort-agnostic market derivation below (it never joins classification). `open_posting_taxonomy` joins `open_postings_display`, so it cannot serve closed or all cohorts; the term-junction tables (`job_posting_roles`, `job_posting_specializations`, `job_posting_skills`) key on `classification_id`, which is why the latest-classification primitive is the join root. Match the four-column shape (`job_posting_id`, `term_kind`, `slug`, `name`) `open_posting_taxonomy` fixed, so the engine reads one taxonomy contract across cohorts.
- Add a cohort-agnostic market derivation keyed by `job_posting_id`, mirroring `open_posting_markets` (migrations 000020–000021) but over any seen posting's latest success-run snapshot rather than the open one. `open_posting_markets` gets its `fetch_run_id` for free from `open_postings`; the cohort-agnostic mirror has no such per-posting run id, so it must derive the snapshot itself — a per-posting lateral over `posting_snapshots` joined to `fetch_runs` where `status = 'success'`, `ORDER BY fetched_at DESC, id DESC LIMIT 1`. Note `latest_successful_fetch_runs` (000017) is keyed **per company**, not per posting, so it is not a drop-in here. Read `location_texts` from that snapshot, match each unnested element against `market_seeds.patterns` with `\y` word boundaries, emit one row per matched market carrying `slug`/`name`/`kind`, and emit the reserved `unmapped` market once (posting grain, `WHERE NOT EXISTS` a match) for a posting matching nothing. `open_posting_markets` is open-only because it joins `open_postings`; a closed-cohort `market` grouping has no source without this. Reuse `market_seeds` — never a second seed dictionary, which would drift from the curated one.
- Add a success-week calendar keyed by `company_id` and UTC week: the distinct successful-run weeks (`date_trunc('week', fetch_runs.started_at)`). `rate` scopes this calendar to its company universe, so it can distinguish a gap (no successful run in scope) from a genuine zero without the engine reading raw snapshots.
- Add parameterized as-of-open as a plain `LANGUAGE sql` read function in `public` taking a `timestamptz`, returning the open `job_posting_id` set as of that instant. A plain view cannot take the date parameter `delta` needs. Cite `add_company`/`save_enrichment` (migrations 000010–000015) only for the CREATE-FUNCTION-in-migration mechanics — **not** their grant posture: those are `SECURITY DEFINER` functions in the `mcp` schema with `REVOKE ALL ... FROM PUBLIC` because they gate agent writes. This function is a read; it stays in `public` and is not `SECURITY DEFINER`. `setup/readonly_role.sql` grants `EXECUTE` explicitly to `market_scout_readonly`; the owner’s global default privileges revoke `PUBLIC EXECUTE`, and the migration revokes it explicitly for posture parity.
- Confirm `market_scout_readonly` can read the new views/function. The real gate is Task 5 reading them on the read-only DSN; migration 000017's header documents that role grants are provisioned outside migrations.
- Record an `EXPLAIN (ANALYZE, TIMING OFF)` median in the migration header, as 000017 and 000019 do. The lifespan primitive's `GROUP BY job_posting_id` with `min`/`max` over every snapshot is the heaviest new query and the one to measure; add an index if it regresses. `000019` reached 227ms only because it detoasts `raw_data` JSONB per row — these views touch ids and timestamps only, so they should land near 000017's 33–47ms, not that.

Task 1 produces (the objects later phases reference by name, since they cannot read this task's text):

| Object | Shape | Read by |
|---|---|---|
| `open_postings` (pre-existing, not built here) | cohort membership: `job_posting_id` | count, age, delta, denominator |
| `closed_postings`, `all_seen_postings` | cohort membership: `job_posting_id` | count, share, denominator |
| `posting_lifespans` | `job_posting_id`, `first_seen`, `last_seen`, `is_closed` | age, lifespan, rate |
| `latest_classifications` | `job_posting_id`, `classification_id`, `seniority`, `classified_at` | seniority grouping, denominator |
| `posting_markets` | `job_posting_id`, `slug`, `name`, `kind` | market grouping/filter (cohort-agnostic; feeds the market branch of `posting_taxonomy`) |
| `posting_taxonomy` | `job_posting_id`, `term_kind`, `slug`, `name` | role/specialization/skill/function/**market** groupings |
| `fetch_success_weeks` | `company_id`, `week` | rate availability and gap detection |
| `open_postings_as_of(timestamptz)` | returns `job_posting_id` set | delta |

Names are the plan's to set; if the implementer picks different ones, update this table so Phase 2–4 stay anchored.

Do not:
- Hand-edit sqlc output. If sqlc regeneration is required, regenerate it (`developer-guide.md` §5.8).
- Redefine `open_postings`. Build closed/all/as-of on top of the existing open definition.
- Join `raw_data` into any new view. Workplace-type derivation is the one heavy consumer and it already exists; the cohort and lifespan views have no reason to detoast.
- Seed a second market dictionary or re-derive `open_posting_markets`. Read `market_seeds`; the cohort-agnostic market derivation is a new keying of the same patterns, not a copy of the dictionary.

### Task 2: Engine scaffold — dispatch, fragments, types, denominator

Create the engine module under `apps/web/lib/db/` (sibling to `postings.ts`). Name the connection-taking core `runCompositionWith(sql, composition, opts?)`, mirroring `selectOpenPostings(sql)`, so the db test drives it on the read-only client; `runComposition(composition)` resolves `getSql()` and delegates to it. This task builds the seams the measure modules fill and lands `count` and `share` to exercise them. The objects this task reads come from Task 1's produced-objects table (`closed_postings`, `all_seen_postings`, `posting_lifespans`, `latest_classifications`, `posting_taxonomy`, `fetch_success_weeks`, `open_postings_as_of`).

- Re-validate with `validate()` at entry; on `{ ok: false }` return `{ ok: false, error }` (the grammar's `Result<MeasureResult, GrammarError>` union from `errors.ts`) and run no query. The engine returns the refusal, never throws — a caller that skipped validation must still be refused, and it narrates "no" the same way it does for `validate()`.
- Accept a `Composition` that came from `parseComposition` — the normalized, canonical form. `validate()` orders groupings and filters internally, so it is robust either way; the precondition matters because the engine's SQL keys off the composition directly and a duplicated `groupBy` would emit a duplicated grouping column.
- Thread a clock through `opts.now?: Date`, defaulting to SQL `now()` when absent. `delta` (as-of window), `age` (first-seen → now), and `rate` (range end) all depend on "now"; a fixed `now` is what lets Task 5 assert deterministic magnitudes against fixed-date fixtures.
- Build cohort, grouping, and filter as composable SQL fragments from fixed lookups keyed by the vocabulary enums. A grouping or filter dimension maps to its source through a table in code, never a string interpolated from the composition.
- Map groupings to sources: `company` → the base `companies` dimension table's `id`/`name` (a stable one-row-per-company table, read directly — not a Task 1 view); `role`/`specialization`/`skill` → `posting_taxonomy` terms by `term_kind`; `market` → `posting_taxonomy` where `term_kind = 'market'` (multi-valued — a posting appears under each matched market, or once under `unmapped`); `function` → `posting_taxonomy` where `term_kind = 'dimension'`; `seniority` → `latest_classifications.seniority`; `week` → the UTC week of the posting's `posting_lifespans.first_seen`.
- `week` grouping buckets by first-seen (posting entry week) for every measure, so a posting seen across many runs resolves to one week and needs no per-run disambiguation. `rate` is the canonical week-grouped measure; a week-grouped `count`/`delta`/`share` is the same inflow bucketing applied to that measure. Every such series is a weekly time series and shares rate's calendar-and-gap treatment: one explicit row per week in range, empty weeks included, with failed/absent-run weeks flagged as gaps. Factor the minimal calendar-generation + successful-weeks + gap-flag core into a shared helper both `rate` and the week-grouped `count`/`delta`/`share` branches call; rate's series-ranking, limit, and company-universe stay in the rate path. Gap granularity follows fetch-run success (company-scoped): per-(company, week) when `company` is grouped, else scope-wide per week across every group key.
- Filter dimensions are the grammar's `FILTER_DIMENSIONS` — the grouping set minus `week` — each filtering against the same source its grouping uses (Boundary inventory). The `company` filter matches `companies.id`; a user-facing name is resolved to that id upstream (`agent-readable-composition-state`), not here.
- Pass filter values as bound query parameters. A model-supplied value never enters SQL text.
- Define `MeasureResult` and its row type once, shared by all measures — Tasks 3, 4, and 5 all build against this exact contract, so pin it here:

  ```ts
  interface MeasureRow {
    keys: Partial<Record<Grouping, string>>; // {} for an ungrouped total; per-posting and grouped rows carry their grouping key(s)
    value: number;                           // count, signed delta, share (0–1), or duration in days
    gap?: boolean;                           // any weekly series (rate, and week-grouped count/delta/share): week with no successful run in scope; value is 0, the flag is the signal
  }
  interface MeasureResult {
    measure: Measure;
    cohort: Cohort;
    groupBy: readonly Grouping[];
    rows: readonly MeasureRow[];
    denominator?: { classified: number; total: number }; // present iff requiresDenominator
  }
  ```

  Aggregate and distribution measures share one row shape so the chart seam is single. `keys` is a map (grouping → value), not an array, so a two-grouping row carries both keys unambiguously.
- Compute the denominator here: when `validate()` reports `requiresDenominator`, return classified count and total *for the composition's cohort*. Open is 24% classified, closed differs — the denominator is cohort-scoped, never a constant. `validate()` already folds filter dimensions into `requiresDenominator`, so a filter on a classified dimension triggers the denominator too — no separate path.
- Land `count` and `share` over the fragments as the first two measures. When `week` is grouped, both take the weekly-series branch (shared gap scaffold above): a plain `GROUP BY` silently omits empty weeks, erasing gap and genuine-zero alike, so emit an explicit row per week in range and flag the gaps. A `share` gap row carries `value: 0` and must not distort the normalizer — 0 contributes nothing to the assignment sum, which still totals 1.0 over the real values.
- Normalize `share` over the sum of the composition's own group values, not the posting count: `share(cell) = value(cell) / Σ value(all cells)`. For a multi-valued grouping (`market`, `role`, `specialization`, `skill`) a posting counts under several groups, so the assignment sum exceeds the posting count — dividing by posting count would make shares sum past 1.0. Dividing by the group-value sum keeps them summing to 1.0 for single- and multi-valued groupings alike.
- With two groupings (a `stacked_bar` share), `Σ` runs over every cell the composition produces — the whole result sums to 1.0, not each outer-group panel. One rule covers one or two groupings.

Do not:
- Import `postgres` types the composition module forbids; keep the grammar module pure and one-directional (engine imports grammar, never the reverse).
- Concatenate any grouping key or filter value into SQL text.
- Normalize `share` over posting count for a multi-valued grouping; the denominator is the sum of group assignments, so shares still total 1.0.

### Task 3: Aggregate measures — delta, rate

Add `delta` and `rate` to the engine, over Task 2's fragments and Task 1's views. Separate file from Task 4 so the two measure sets land concurrently without sharing edits.

- `delta`: signed change = open-count-at-`now` − open-count-as-of-(`now` − `window`), both via `open_postings_as_of`, where `now` is `opts.now` (Task 2's clock). Grouping splits the delta per group. The window is required and already validated; read it, never default it. When `week` is grouped, `delta` is a weekly time series: diff each series per (series, week) across the two snapshots, then walk the shared gap scaffold so failed/absent-run weeks surface as gap rows (`value: 0`, `gap: true`) rather than vanishing behind the two-snapshot `FULL JOIN`, and a genuinely-zero-diff week on a successful run stays ungapped.
- `rate` requires `week` in `groupBy`. New postings bucket by `posting_lifespans.first_seen` UTC week over the `all` cohort; availability comes separately from successful fetch runs. Its company universe is an explicit company filter when one is present, otherwise companies with historically matching postings under the non-company filters. The range is that universe's earliest available week through the UTC week containing `now`; a future run never advances the calendar.
- A company-grouped series uses its own availability calendar. Other series use the universe's union calendar. Week-only rate seeds one series when availability exists; company-plus-week seeds every universe company, including a zero-posting company; other non-company series use observed assignments only. Whole series are ranked and limited before week expansion, then each retained series returns chronologically.
- Gap rows come from the successful-run week calendar: a week in range absent from the applicable calendar had no successful run in scope, so it is a gap, distinct from a week that had a run but zero new postings. Absence is data; smoothing it would lie (`research.md`, and the 07-13 gap live). Build that calendar inline, `now`-scoped (`started_at <= now`), rather than reading the `fetch_success_weeks` view — the view has no `now` bound, and as-of determinism (`opts.now`) needs one. This same inline calendar + gap-flag core is the shared helper the week-grouped `count`/`delta`/`share` branches reuse (Task 2).
- A gap row carries `value: 0` with `gap: true`; consumers distinguish it from a real zero by the flag, never the value. This holds for every weekly series — `rate` and week-grouped `count`/`delta`/`share` alike.

### Task 4: Distribution measures — age, lifespan

Add `age` and `lifespan` to the engine as per-posting distributions. Separate file from Task 3.

- `age`: one row per open posting, value = days from `posting_lifespans.first_seen` to `now` (Task 2's clock). Open cohort only (fixed by the grammar) — `posting_lifespans` is keyed for every seen posting, so join it to the `open_postings` membership object to restrict to open.
- `lifespan`: one row per closed posting, value = `posting_lifespans.last_seen` − `first_seen` in days. Closed cohort only.
- Single-run rule: a posting seen in exactly one success run has last-seen == first-seen, so lifespan is 0 days. Return it as a 0-day row, not dropped — 431 postings live carry this shape and excluding them would bias the distribution short.
- Return raw per-posting values. Bin edges are the histogram chart's call, not the engine's.
- With one grouping, tag each row with its group for small multiples; the engine does not aggregate the distribution.

### Task 5: Database tests

Add `measure-engine.db.test.ts` under `apps/web/lib/db/`. Mirror `read-model-views.db.test.ts`: seed through the owner DSN with a per-run marker, read through the read-only DSN, `context.skip()` when either DSN is absent, clean up by marker. The test drives `runCompositionWith` directly on the read-only client.

- Seed a small deterministic fixture: a company, several success runs across weeks, a failed run, postings that stay open, postings that close, one posting seen in a single run, classified plus unclassified postings, and a superseded second classification on one posting so latest-classification selection is exercised.
- Populate `location_texts` on fixture snapshots so market resolves: at least one posting matching a seed market, one whose single string carries two markets (proving multi-valued rows), one matching nothing (proving `unmapped`), and one that closes (proving the cohort-agnostic market source serves the `closed` cohort, not just open). Seed no new `market_seeds`; rely on the migration's dictionary and match strings it already carries.
- Assert `count` grouped by `market` over `closed` returns market rows for closed postings, and `share` grouped by `market` sums to 1.0 across the multi-valued groups — pinning the assignment-sum normalizer.
- Pass a fixed `opts.now` on every call so `delta`, `age`, and `rate` are deterministic; seed run timestamps relative to that fixed `now`, not the wall clock.
- Assert each measure against the fixture: `count` total, `delta` sign and magnitude across the window, `rate` per-week including the gap week, `age` and `lifespan` values, `share` summing to 1.0.
- Assert the weekly-series gap parity: a week-grouped `count` (scope-wide and per-`company`), `delta`, and `share` (over a non-company grouping) each emit an explicit row per week in range, flag the failed-run week (`2099-01-19`) and the no-run-yet week (`2099-02-02`) as `value: 0, gap: true`, and leave a genuine-zero week that had a successful run (`2099-01-12`) ungapped. For the per-`company` case the gap is per-(company, week); for the non-company cases it is scope-wide, applied across every group key, and a `share` normalizer still sums to 1.0. Two distinct `count` assertions: the ungrouped engine `count` equals a direct `SELECT count(*) FROM open_postings` on the same client (a **global** consistency check, both sides unfiltered — not a fixture magnitude); the fixture-magnitude assertions filter by the seeded company so they read only the fixture's rows.
- Assert rate refuses a composition without `week`; ignores a successful run later than injected `now`; seeds an explicitly filtered zero-posting company; returns no rows for an empty availability calendar; and uses the union availability calendar for non-company filters that match several companies.
- Assert the denominator is cohort-scoped: classified-count and total for open differ from closed on the same fixture.
- Assert an invalid crossing is refused and no query runs — drive the core with a `sql` proxy that counts invocations and assert zero calls on the refusal path.

## Sequencing

**Phase 1 (sequential):** Task 1 — migration defines the views/function every measure reads.
**Phase 2 (sequential):** Task 2 — engine scaffold, fragments, types, denominator, and `count`/`share`; defines the seams Tasks 3–4 fill.
**Phase 3 (concurrent):** Task 3 (delta, rate) and Task 4 (age, lifespan) — separate files, both consume Phase 2's fragments and Phase 1's views.
**Phase 4 (sequential):** Task 5 — db tests exercise all six measures and the denominator once they exist.

## Rough sketch

One shared result shape across measures keeps the engine↔chart seam single. The `MeasureRow` / `MeasureResult` interfaces are pinned in Task 2 — the single source; do not re-declare them. The connection-taking core:

```ts
// Proposed design — remove after implementation.
runCompositionWith(sql, composition, opts?: { now?: Date }): Promise<Result<MeasureResult, GrammarError>>
// Refusal returns { ok: false, error }; success returns { ok: true, value }. Never throws on a refusal.
// now defaults to SQL now(); tests pass a fixed value for deterministic delta/age/rate.
```

Cohort membership, lifespan, and taxonomy come from Task 1's views; the engine assembles measure SQL from fixed grouping/filter/cohort fragments and computes only the arithmetic a view cannot — differencing (`delta`), normalizing (`share`), weekly bucketing (`rate`), and duration (`age`, `lifespan`). As-of-open for `delta` uses Task 1's function so the "now" branch reproduces `open_postings` exactly (`research.md`).

## Boundary inventory

Cross-boundary names are SQL column ↔ TS interface field (no Go struct; the web app reads views through `postgres` tagged templates, not sqlc). Grouping keys map to sources through a fixed code lookup:

| Grouping | Source (Task 1 / existing) | Cohort-safe? |
|---|---|---|
| `company` | `companies.id` / `.name` | yes |
| `role` `specialization` `skill` | cohort-agnostic taxonomy (Task 1) — multi-valued | yes |
| `market` | cohort-agnostic market derivation (Task 1), `term_kind = 'market'` — multi-valued, incl. `unmapped` | yes |
| `function` | role dimensions via taxonomy `term_kind = 'dimension'` | yes |
| `seniority` | latest classification's seniority column | yes |
| `week` | UTC week of `posting_lifespans.first_seen`; `fetch_success_weeks` defines rate availability, not emitted bucket | yes |

## Cross-spec decisions

- **Read-model boundary — migration.** Cohort membership, lifespan, taxonomy, and as-of-open live in SQL (Task 1), keeping "openness defined once in SQL" — the engine-built-CTE alternative duplicates the open definition and risks drift.
- **Company filter key — `companies.id`.** The `company` filter matches the stable, unique id, not the name. Resolving a user-facing company name to that id is the model-facing layer's job, deferred to `agent-readable-composition-state`; the engine takes the canonical id.
- **Distribution shape — raw per-posting durations.** `age`/`lifespan` return one row per posting; `chart-primitives` owns bin edges, since bin width interacts with scale and viewBox. Faithful to the grammar, which defines these as distributions, not aggregates. Forward seam: binned counts a reader narrates are projected in `agent-readable-composition-state`, not computed here.
- **Multi-valued `share` normalization — this spec owns it.** `market-dimension` (its *Cross-spec seam*) defers to this engine the rule that `share` over a multi-valued grouping normalizes over the sum of group assignments, not the posting count — the same treatment `role`, `specialization`, and `skill` already need. Stated as `share(cell) = value(cell) / Σ value(all cells)`, one rule covers single- and multi-valued groupings and one or two grouping dimensions (global, not per-panel).
- **Cohort-agnostic market — built here.** `market-dimension` wired `market` only into the open-cohort views and deferred closed/historical cohorts by design. The grammar admits `market` grouping over any cohort, so this engine's Task 1 supplies the cohort-agnostic market source, exactly as it already supplies a cohort-agnostic taxonomy because `open_posting_taxonomy` is open-only. It reuses `market_seeds`; it does not re-seed or re-derive the open view.

## Dependency

Depends on `market-dimension` (migrations 000020–000022) being landed: Task 1 reads `market_seeds` — whose seed dictionary completes at 000022 (`000022_market_seed_city_folds` appends suburb folds) — and mirrors `open_posting_markets` (view shape finalized at 000021), and Task 2 maps the `market` grouping the grammar now exposes. `market` is already present in `GROUPINGS`, `FILTER_DIMENSIONS`, and `REQUIRES_DENOMINATOR` (`false`), so no grammar edit belongs here — the engine consumes that vocabulary, it does not extend it.
</content>
