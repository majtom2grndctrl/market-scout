# Measure Engine

> Brief — decisions and behavior. Rationale and live numbers in `research.md`. Roadmap: Compositional Analysis Surface → Grammar.

## Goal

Compute a validated `Composition` into typed rows plus a cohort-specific coverage denominator. The engine is the other half of the grammar's boundary: the grammar validates and hands over a typed value; the engine reads Postgres and hands back numbers. The model never sees SQL; a number only reaches the model after the engine computed it. Lives in `apps/web/lib/db`, over the read-model views.

## Scope

### In scope

- `runComposition(composition): Promise<MeasureResult>` — the single entry point. Input is a `Composition` the caller already validated with `validate()`; the engine re-validates and refuses an invalid crossing rather than trusting the caller.
- All six measures: `count`, `delta`, `share`, `rate` (aggregate) and `age`, `lifespan` (per-posting distribution).
- Cohort membership: `open`, `closed`, `all`, plus as-of-open reconstruction for `delta`.
- Grouping by every vocabulary grouping, including `week` (UTC weekly buckets) and the `function` → role-dimension mapping.
- Filters on every filter dimension, applied as parameters, never string-built.
- A **read-model migration** (Go side) that defines closed/all cohort membership, the per-posting lifespan primitive, a cohort-agnostic taxonomy, and parameterized as-of-open — so openness stays defined once in SQL and the engine never reproduces it.
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

- [ ] `runComposition` accepts a validated `Composition` and returns a `MeasureResult` with typed rows for each of the six measures.
- [ ] A composition that fails `validate()` is refused by `runComposition` with the grammar's `GrammarError`, and no query runs.
- [ ] `count` over the `open` cohort with no grouping returns a total that equals the `open_postings` view's row count.
- [ ] `delta` over a window returns the signed difference between open-count-now and open-count-as-of-(now − window), computed from the read model's as-of-open definition, not a re-implemented one.
- [ ] `rate` returns one row per UTC week in range; a week with no successful run in scope is present as a gap row, distinguishable from a week whose count is zero.
- [ ] `lifespan` returns one row per closed posting carrying its duration in days; a posting seen in exactly one successful run follows the single-run rule stated in the sketch.
- [ ] `age` returns one row per open posting carrying its current age in days.
- [ ] `share` returns per-group values that sum to 1.0 (within floating tolerance) across the groups in each composition.
- [ ] A composition whose grouping or filter `requiresDenominator` returns a denominator giving classified count and cohort total; the open-cohort denominator reflects the cohort actually queried, not a corpus-wide constant.
- [ ] A composition that requires no denominator returns none.
- [ ] Grouping by `function` resolves to role dimensions; grouping by `role`/`specialization`/`skill` resolves to the matching taxonomy terms for the composition's cohort, not the open-only view.
- [ ] Filter values reach SQL as bound parameters; grouping keys resolve through a fixed lookup, so no model-supplied string is concatenated into SQL text.
- [ ] The migration header records an `EXPLAIN (ANALYZE, TIMING OFF)` median for the lifespan primitive, matching the perf-check convention in migrations 000017 and 000019; no new view reads `raw_data`.
- [ ] `pnpm test:db` passes with the new fixtures; `pnpm typecheck` passes.

## Tasks

### Task 1: Read-model cohort and lifespan primitives (migration)

Add a numbered migration under `apps/tools/internal/db/migrations/` (up + down) that generalizes the read model past open-only. Mirror the existing view migrations' shape and the header-comment convention that records a local performance check.

- Define closed and all cohort membership so the engine reads each cohort, never reconstructs it. `open` already exists as `open_postings`; closed = seen in a success run but absent from open; all = ever seen in a success run. A cohort a view defines once cannot drift from the engine's copy.
- Add a per-posting lifespan primitive keyed by `job_posting_id`: first-seen and last-seen from `fetch_runs.started_at` across success runs, and a closed flag. `started_at` is the run's real timestamp — one per run, per `research.md`.
- Add a cohort-agnostic taxonomy keyed by `job_posting_id` — role, specialization, skill, and dimension terms for any seen posting. `open_posting_taxonomy` joins `open_postings_display`, so it cannot serve closed or all cohorts.
- Add parameterized as-of-open as a SQL function taking a `timestamptz`, returning the open `job_posting_id` set as of that instant. `add_company`/`save_enrichment` (migrations 000010–000015) are the precedent for a function in a migration. A plain view cannot take the date parameter `delta` needs.
- Confirm `market_scout_readonly` can read the new views/function, matching the grant note in migration 000017's header.
- Record an `EXPLAIN (ANALYZE, TIMING OFF)` median in the migration header, as 000017 and 000019 do. The lifespan primitive's `GROUP BY job_posting_id` with `min`/`max` over every snapshot is the heaviest new query and the one to measure; add an index if it regresses. `000019` reached 227ms only because it detoasts `raw_data` JSONB per row — these views touch ids and timestamps only, so they should land near 000017's 33–47ms, not that.

Do not:
- Hand-edit sqlc output. If sqlc regeneration is required, regenerate it (`developer-guide.md` §5.8).
- Redefine `open_postings`. Build closed/all/as-of on top of the existing open definition.
- Join `raw_data` into any new view. Workplace-type derivation is the one heavy consumer and it already exists; the cohort and lifespan views have no reason to detoast.

### Task 2: Engine scaffold — dispatch, fragments, types, denominator

Create the engine module under `apps/web/lib/db/` (sibling to `postings.ts`). Take the `postgres` connection as an argument the way `selectOpenPostings(sql)` does, so the db test drives it on the read-only client; add a `runComposition(composition)` wrapper that resolves `getSql()` for request scope. This task builds the seams the measure modules fill and lands `count` and `share` to exercise them.

- Re-validate with `validate()` at entry; on `{ ok: false }` return its `GrammarError` and run no query. The engine defends its own boundary; a caller that skipped validation must still be refused.
- Build cohort, grouping, and filter as composable SQL fragments from fixed lookups keyed by the vocabulary enums. A grouping or filter dimension maps to its source through a table in code, never a string interpolated from the composition.
- Map groupings to sources: `company` → company id/name; `role`/`specialization`/`skill` → cohort-agnostic taxonomy terms (Task 1); `function` → role dimensions; `seniority` → the classification seniority column; `week` → `date_trunc('week', started_at)` at UTC.
- Pass filter values as bound query parameters. A model-supplied value never enters SQL text.
- Define `MeasureResult` and its row type once, shared by all measures (see sketch). Aggregate and distribution measures share one row shape so the chart seam is single.
- Compute the denominator here: when `validate()` reports `requiresDenominator`, return classified count and total *for the composition's cohort*. Open is 24% classified, closed differs — the denominator is cohort-scoped, never a constant.
- Land `count` and `share` over the fragments as the first two measures.

Do not:
- Import `postgres` types the composition module forbids; keep the grammar module pure and one-directional (engine imports grammar, never the reverse).
- Concatenate any grouping key or filter value into SQL text.

### Task 3: Aggregate measures — delta, rate

Add `delta` and `rate` to the engine, over Task 2's fragments and Task 1's views. Separate file from Task 4 so the two measure sets land concurrently without sharing edits.

- `delta`: signed change = open-count-now − open-count-as-of-(now − `window`), via Task 1's as-of-open function. Grouping splits the delta per group. The window is required and already validated; read it, never default it.
- `rate`: new postings per UTC week = first-seen (Task 1 lifespan primitive) bucketed by week over the `all` cohort. Emit one row per week in range.
- A week with no successful run in scope is a gap row, flagged distinctly from a genuine zero-count week. Absence is data; smoothing it would lie (`research.md`, and the 07-13 gap live).

### Task 4: Distribution measures — age, lifespan

Add `age` and `lifespan` to the engine as per-posting distributions. Separate file from Task 3.

- `age`: one row per open posting, value = days from first-seen to now. Open cohort only (fixed by the grammar).
- `lifespan`: one row per closed posting, value = last-seen − first-seen in days, from Task 1's lifespan primitive. Closed cohort only.
- Single-run rule: a posting seen in exactly one success run has last-seen == first-seen, so lifespan is 0 days. Return it as a 0-day row, not dropped — 431 postings live carry this shape and excluding them would bias the distribution short.
- Return raw per-posting values. Bin edges are the histogram chart's call, not the engine's.
- With one grouping, tag each row with its group for small multiples; the engine does not aggregate the distribution.

### Task 5: Database tests

Add `measure-engine.db.test.ts` under `apps/web/lib/db/`. Mirror `read-model-views.db.test.ts`: seed through the owner DSN with a per-run marker, read through the read-only DSN, `context.skip()` when either DSN is absent, clean up by marker.

- Seed a small deterministic fixture: a company, several success runs across weeks, a failed run, postings that stay open, postings that close, one posting seen in a single run, and classified plus unclassified postings.
- Assert each measure against the fixture: `count` total, `delta` sign and magnitude across the window, `rate` per-week including the gap week, `age` and `lifespan` values, `share` summing to 1.0.
- Assert the denominator is cohort-scoped: classified-count and total for open differ from closed on the same fixture.
- Assert an invalid crossing is refused before any query runs.

## Sequencing

**Phase 1 (sequential):** Task 1 — migration defines the views/function every measure reads.
**Phase 2 (sequential):** Task 2 — engine scaffold, fragments, types, denominator, and `count`/`share`; defines the seams Tasks 3–4 fill.
**Phase 3 (concurrent):** Task 3 (delta, rate) and Task 4 (age, lifespan) — separate files, both consume Phase 2's fragments and Phase 1's views.
**Phase 4 (sequential):** Task 5 — db tests exercise all six measures and the denominator once they exist.

## Rough sketch

One shared result shape across measures keeps the engine↔chart seam single:

```ts
// Proposed design — remove after implementation.
interface MeasureRow {
  keys: Partial<Record<Grouping, string>>; // {} for an ungrouped total; per-posting rows carry their grouping
  value: number;                           // count, signed delta, share (0–1), or duration in days
  gap?: boolean;                           // rate: week with no successful run in scope
}
interface MeasureResult {
  measure: Measure;
  cohort: Cohort;
  groupBy: readonly Grouping[];
  rows: readonly MeasureRow[];
  denominator?: { classified: number; total: number }; // present iff requiresDenominator
}
```

Cohort membership, lifespan, and taxonomy come from Task 1's views; the engine assembles measure SQL from fixed grouping/filter/cohort fragments and computes only the arithmetic a view cannot — differencing (`delta`), normalizing (`share`), weekly bucketing (`rate`), and duration (`age`, `lifespan`). As-of-open for `delta` uses Task 1's function so the "now" branch reproduces `open_postings` exactly (`research.md`).

## Boundary inventory

Cross-boundary names are SQL column ↔ TS interface field (no Go struct; the web app reads views through `postgres` tagged templates, not sqlc). Grouping keys map to sources through a fixed code lookup:

| Grouping | Source (Task 1 / existing) | Cohort-safe? |
|---|---|---|
| `company` | `companies.id` / `.name` | yes |
| `role` `specialization` `skill` | cohort-agnostic taxonomy (Task 1) | yes |
| `function` | role dimensions via taxonomy `term_kind = 'dimension'` | yes |
| `seniority` | latest classification's seniority column | yes |
| `week` | `date_trunc('week', fetch_runs.started_at)` UTC | yes |

## Open questions

- **Read-model boundary (needs sign-off before promotion).** This spec puts cohort membership, lifespan, taxonomy, and as-of-open in a SQL migration (Task 1), keeping "openness defined once in SQL" — the settled read-model invariant. The alternative is the engine building those CTEs in TS, which duplicates the open definition and risks drift. Recommendation: the migration. Confirm before promoting.
- **Company filter key.** Filter on `company` matches by name today; `agent-readable-composition-state` (later) fixes the reader-facing identifier. If it lands as id, the filter lookup changes. Low blast radius — one fragment.
- **Histogram binning owner.** This spec returns raw per-posting durations and leaves bin edges to `chart-primitives`. If that spec expects pre-binned counts, `age`/`lifespan` output shape shifts. Recommendation: raw values (engine = data, chart = geometry).
</content>
