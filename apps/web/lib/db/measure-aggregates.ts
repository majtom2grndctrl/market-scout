import type { Fragment } from "postgres";

import type { Grouping } from "../composition";

import type { MeasureContext, MeasureRow } from "./measure-engine";
import { createMeasureScope } from "./measure-scope";

interface AggregateSqlRow {
  readonly keys: Record<string, string>;
  readonly value: number;
  // Present only when `selectAggregateRows` projected the column; see the
  // suppression rule there.
  readonly requisitions?: number;
}

interface RateSqlRow extends AggregateSqlRow {
  readonly gap: boolean;
}

// Aggregate measures share one grouped count query. `share` changes only the
// final projection, so its denominator is the result's assignment sum rather
// than the number of postings that happened to produce those assignments.
export async function runAggregateMeasure(context: MeasureContext): Promise<readonly MeasureRow[]> {
  switch (context.composition.measure) {
    case "count":
      return selectAggregateRows(context, false);
    case "share":
      return selectAggregateRows(context, true);
    case "delta":
      return selectDeltaRows(context);
    case "rate":
      return selectRateRows(context);
    case "age":
    case "lifespan":
      throw new Error(`distribution measure "${context.composition.measure}" reached the aggregate runner`);
  }
}

async function selectDeltaRows(context: MeasureContext): Promise<readonly MeasureRow[]> {
  const { sql, composition, now } = context;
  const window = composition.window;
  if (window == null) throw new Error("delta requires a validated window");

  // A week-grouped delta is a weekly time series: it must break its line on a
  // failed-run week, so it walks the same calendar + gap scaffold as rate
  // rather than a plain GROUP BY that silently omits empty and gap weeks.
  if (composition.groupBy?.includes("week")) return selectWeeklyDeltaRows(context);

  const scope = createMeasureScope(sql, composition);
  const where = scope.filters ?? sql`TRUE`;
  const keys = scope.keys ?? sql`'{}'::jsonb`;
  const grouping = scope.keys == null ? undefined : sql`GROUP BY ${scope.keys}`;
  const order = orderClause(sql, composition.sort);
  const limit = composition.limit == null ? undefined : sql`LIMIT ${composition.limit}`;

  const rows = await sql<AggregateSqlRow[]>`
    WITH current_grouped AS (
      SELECT
        ${keys} AS keys,
        count(*)::int AS value
      FROM open_postings_as_of(${now}) AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      ${scope.joins ?? sql``}
      WHERE ${where}
      ${grouping ?? sql``}
    ),
    previous_grouped AS (
      SELECT
        ${keys} AS keys,
        count(*)::int AS value
      FROM open_postings_as_of(${now} - ${window.weeks}::int * interval '1 week') AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      ${scope.joins ?? sql``}
      WHERE ${where}
      ${grouping ?? sql``}
    ),
    delta_rows AS (
      SELECT
        COALESCE(current_grouped.keys, previous_grouped.keys) AS keys,
        COALESCE(current_grouped.value, 0) - COALESCE(previous_grouped.value, 0) AS value
      FROM current_grouped
      FULL JOIN previous_grouped USING (keys)
    ),
    selected AS (
      SELECT keys, value
      FROM delta_rows
      ${order}
      ${limit ?? sql``}
    )
    SELECT keys, value
    FROM selected
    ${order}
  `;

  return rows.map((row) => ({ keys: row.keys, value: row.value }));
}

async function selectRateRows(context: MeasureContext): Promise<readonly MeasureRow[]> {
  const { sql, composition, now } = context;
  // Validation requires `week`; rate supplies its first-seen bucket rather
  // than using the reusable week source, which would add a second lifespan
  // join. Every remaining grouping belongs to a whole time-series key.
  const scope = createMeasureScope(sql, {
    ...composition,
    groupBy: composition.groupBy?.filter((grouping) => grouping !== "week"),
  });
  const companyFilters = (composition.filter ?? []).filter((filter) => filter.dim === "company");
  const nonCompanyFilters = (composition.filter ?? []).filter((filter) => filter.dim !== "company");
  const universeScope = createMeasureScope(sql, {
    ...composition,
    groupBy: [],
    ...(nonCompanyFilters.length > 0 && { filter: nonCompanyFilters }),
  });
  const where = scope.filters ?? sql`TRUE`;
  const universeWhere = universeScope.filters ?? sql`TRUE`;
  const seriesKeys = scope.keys ?? sql`'{}'::jsonb`;
  const companyGrouped = scope.groupBy.includes("company");
  const companyId = companyGrouped ? sql`posting.company_id` : sql`NULL::bigint`;
  const seriesOrder = rateSeriesOrderClause(sql, composition.sort);
  const seriesLimit = composition.limit == null ? undefined : sql`LIMIT ${composition.limit}`;
  const weekOnly = scope.groupBy.length === 0;
  const companyOnly = scope.groupBy.length === 1 && scope.groupBy[0] === "company";
  const explicitCompanyWhere = andFragments(
    sql,
    companyFilters.map((filter) => sql`company.id = ${filter.value}`),
  );

  const rows = await sql<RateSqlRow[]>`
    WITH company_universe AS (
      ${
        explicitCompanyWhere == null
          ? sql`
              SELECT DISTINCT company.id AS company_id, company.name AS company_name
              FROM ${universeScope.cohort} AS cohort
              JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
              JOIN companies AS company ON company.id = posting.company_id
              WHERE ${universeWhere}
            `
          : sql`
              SELECT company.id AS company_id, company.name AS company_name
              FROM companies AS company
              WHERE ${explicitCompanyWhere}
            `
      }
    ),
    successful_company_weeks AS (
      ${successfulCompanyWeeks(sql, now, sql`company_universe`)}
    ),
    rate_assignments AS (
      SELECT
        ${companyId} AS company_id,
        ${weekBucket(sql, sql`rate_lifespan.first_seen`)} AS week,
        ${seriesKeys} AS series_keys
      FROM all_seen_postings AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      JOIN posting_lifespans AS rate_lifespan
        ON rate_lifespan.job_posting_id = cohort.job_posting_id
      ${scope.joins ?? sql``}
      WHERE ${where}
        AND rate_lifespan.first_seen <= ${now}
    ),
    counted AS (
      SELECT company_id, week, series_keys, count(*)::int AS value
      FROM rate_assignments
      GROUP BY company_id, week, series_keys
    ),
    series AS (
      SELECT DISTINCT company_id, series_keys
      FROM rate_assignments
      WHERE NOT ${weekOnly || companyOnly}

      UNION ALL

      SELECT NULL::bigint AS company_id, '{}'::jsonb AS series_keys
      WHERE ${weekOnly}
        AND EXISTS (SELECT 1 FROM successful_company_weeks)

      UNION ALL

      SELECT
        company_universe.company_id,
        jsonb_build_object('company', company_universe.company_name) AS series_keys
      FROM company_universe
      WHERE ${companyOnly}
    ),
    ranked_series AS (
      SELECT
        series.company_id,
        series.series_keys,
        COALESCE(sum(counted.value), 0)::int AS total
      FROM series
      LEFT JOIN counted
        ON counted.company_id IS NOT DISTINCT FROM series.company_id
        AND counted.series_keys = series.series_keys
      GROUP BY series.company_id, series.series_keys
      ${seriesOrder}
      ${seriesLimit ?? sql``}
    ),
    ${weekCalendar(sql, sql`ranked_series`, companyGrouped, now)}
    ${weekSeriesSelect(sql, sql`counted`, companyGrouped, false)}
  `;

  return rows.map((row) => ({
    keys: row.keys,
    value: row.value,
    ...(row.gap && { gap: true }),
  }));
}

function andFragments(sql: MeasureContext["sql"], fragments: readonly Fragment[]): Fragment | undefined {
  const [first, ...rest] = fragments;
  if (first == null) return undefined;
  return rest.reduce((joined, fragment) => sql`${joined} AND ${fragment}`, first);
}

// Groupings that put one requisition's sibling postings in different rows.
// Under `market` that separation is the mechanism rather than an accident:
// listing a requisition once per location creates both the siblings and the
// markets that divide them, so a consumer totalling the series reads one job
// as several.
//
// A threshold, not a clean line — any grouping separates siblings when the
// classifier disagrees across them. Totalling the series over the dev corpus
// against its true distinct count puts `market` an order of magnitude past the
// rest, which is what earns it the only entry here:
//
//   market      value +48.0%   requisitions +48.7%
//   role        value  +3.3%   requisitions  +3.9%
//   seniority   value   0.0%   requisitions  +0.9%
//
// Read a per-row `requisitions` as the distinct count within that row, never
// as a term to total — it is no more additive than `value`. `seniority` is the
// one grouping where the two part, one term per posting keeping `value` exact
// while `requisitions` drifts; accepted at 0.9%.
//
// `company` cannot split a requisition at all: keys are board-scoped and the
// distinct count is company-keyed, so siblings share a row. `week` never
// reaches here — a week-grouped count walks `selectWeeklyAggregateRows`, which
// projects no requisition column.
const SIBLING_SPLITTING_GROUPINGS: readonly Grouping[] = ["market"];

async function selectAggregateRows(
  context: MeasureContext,
  normalize: boolean,
): Promise<readonly MeasureRow[]> {
  const { sql, composition } = context;

  // A week-grouped count/share is a weekly time series. A plain GROUP BY drops
  // every week with no rows — genuine zeros and failed-run gaps alike — so it
  // walks rate's calendar + gap scaffold instead, emitting an explicit row per
  // week in range and flagging the failed-run weeks.
  if (composition.groupBy?.includes("week")) return selectWeeklyAggregateRows(context, normalize);

  const scope = createMeasureScope(sql, composition);
  const where = scope.filters ?? sql`TRUE`;
  const grouping = scope.keys == null ? undefined : sql`GROUP BY ${scope.keys}`;
  const order = orderClause(sql, composition.sort);
  const limit = composition.limit == null ? undefined : sql`LIMIT ${composition.limit}`;

  // `posting_requisitions` answers "how many jobs, not listings" — but only for
  // a `count` over the open cohort, grouped so no row holds part of a
  // requisition. The view holds one row per *open* posting resolved against its
  // current snapshot: under `closed`/`all` an inner join would silently shrink
  // `value` itself, and a left join would report a fabricated 0. `share` is a
  // proportion, so an absolute second count beside it would mean nothing. All
  // three suppress in SQL rather than dropping the key from the mapped row, so
  // no suppressed row ever carries the column.
  //
  // Counting distinct over (company_id, requisition_key) rather than the key
  // alone is load-bearing: keys are board-scoped, so two companies can spell the
  // same requisition and a bare distinct count would merge them.
  const countsRequisitions =
    composition.measure === "count" &&
    composition.cohort === "open" &&
    !(composition.groupBy ?? []).some((grouping) => SIBLING_SPLITTING_GROUPINGS.includes(grouping));
  // Accepted cost, measured on the dev read-only DSN (EXPLAIN ANALYZE with
  // TIMING OFF, warmed, 3 runs each, 4,734 open postings): the ungrouped open
  // count goes 29.5 / 29.6 / 30.1 ms → 127.7 / 129.5 / 141.5 ms, and the
  // company-grouped one 37.5 / 37.7 / 38.3 ms → 143.7 / 150.1 / 156.1 ms.
  // `SELECT count(*) FROM posting_requisitions` alone is 81–92 ms, so the view
  // itself is the whole increment. No route reads this yet; `count`/`open` is
  // the default composition, so the cost arrives with the first one that does.
  // Recorded, not optimized: the second denominator is the point of the
  // feature, and ~100 ms buys it.
  const requisitionJoin = countsRequisitions
    ? sql`JOIN posting_requisitions AS requisition ON requisition.job_posting_id = cohort.job_posting_id`
    : sql``;
  // Counted over the same fanned rows as `value`: `scope.joins` expands a
  // posting into one row per taxonomy term, and both numbers must share that
  // denominator.
  const requisitionAggregate = countsRequisitions
    ? sql`, count(DISTINCT (requisition.company_id, requisition.requisition_key))::int AS requisitions`
    : sql``;
  const requisitionColumn = countsRequisitions ? sql`, requisitions` : sql``;

  const rows = await sql<AggregateSqlRow[]>`
    WITH grouped AS (
      SELECT
        ${scope.keys ?? sql`'{}'::jsonb`} AS keys,
        count(*)::int AS value
        ${requisitionAggregate}
      FROM ${scope.cohort} AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      ${scope.joins ?? sql``}
      ${requisitionJoin}
      WHERE ${where}
      ${grouping ?? sql``}
    ),
    selected AS (
      SELECT keys, value ${requisitionColumn}
      FROM grouped
      ${order}
      ${limit ?? sql``}
    )
    SELECT
      keys,
      ${normalize ? sql`value::double precision / NULLIF(sum(value) OVER (), 0)` : sql`value`} AS value
      ${requisitionColumn}
    FROM selected
    ${order}
  `;

  // Absent, never 0: a row that cannot supply the signal omits the key, so
  // "0 real" stays distinct from "0 unknown."
  return rows.map((row) => ({
    keys: row.keys,
    value: row.value,
    ...(row.requisitions != null && { requisitions: row.requisitions }),
  }));
}

// --- Shared weekly gap scaffold ---------------------------------------------
//
// Every weekly time series (rate, and week-grouped count/delta/share) must
// break its line on a failed-run week rather than draw through it. These
// helpers factor the minimal common core: the UTC week bucket, the now-bounded
// successful-run calendar, the calendar walk over each series, the gap flag,
// and the final row projection. Rate's series ranking, limit, and company
// universe stay in `selectRateRows` — they are rate-specific and not shared.

// UTC week bucket for a timestamp column or expression.
function weekBucket(sql: MeasureContext["sql"], column: Fragment): Fragment {
  return sql`date_trunc('week', ${column} AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`;
}

// The successful-run week calendar, scoped to a company-universe source and
// bounded by `now`. Kept inline rather than reading the `fetch_success_weeks`
// view because that view has no `now` bound, and as-of determinism requires
// `started_at <= now`. `universe` is any source exposing a `company_id` column
// (a CTE name or a parenthesized subquery).
function successfulCompanyWeeks(
  sql: MeasureContext["sql"],
  now: Fragment,
  universe: Fragment,
): Fragment {
  return sql`
    SELECT DISTINCT
      fetch_runs.company_id,
      ${weekBucket(sql, sql`fetch_runs.started_at`)} AS week
    FROM fetch_runs
    JOIN ${universe} AS scope_universe ON scope_universe.company_id = fetch_runs.company_id
    WHERE fetch_runs.status = 'success'
      AND fetch_runs.started_at <= ${now}
  `;
}

// Gap detection for the final projection: a week in range with no successful
// run is a gap. Granularity mirrors fetch-run success, which is company-scoped:
// per-(company, week) when `company` is grouped, otherwise scope-wide per week
// (applied to every group key present in that week).
function weekGap(sql: MeasureContext["sql"], companyGrouped: boolean): Fragment {
  return sql`
    NOT EXISTS (
      SELECT 1
      FROM successful_company_weeks
      WHERE successful_company_weeks.week = weeks.week
        ${companyGrouped ? sql`AND successful_company_weeks.company_id = weeks.company_id` : sql``}
    )
  `;
}

// The calendar walk: `series_bounds` + `weeks` CTEs. Each series runs from its
// earliest successful week (its own when company-grouped, the scope union
// otherwise) through the week containing `now`. `seriesSource` is the CTE
// supplying the retained series (`ranked_series` for rate, `series` elsewhere);
// it must expose `company_id` and `series_keys`. Depends on a
// `successful_company_weeks` CTE being defined earlier in the same query.
function weekCalendar(
  sql: MeasureContext["sql"],
  seriesSource: Fragment,
  companyGrouped: boolean,
  now: Fragment,
): Fragment {
  return sql`
    series_bounds AS (
      SELECT
        series_source.company_id,
        series_source.series_keys,
        min(successful_company_weeks.week) AS first_week,
        ${weekBucket(sql, now)} AS last_week
      FROM ${seriesSource} AS series_source
      JOIN successful_company_weeks
        ON ${companyGrouped ? sql`successful_company_weeks.company_id = series_source.company_id` : sql`TRUE`}
      GROUP BY series_source.company_id, series_source.series_keys
    ),
    weeks AS (
      SELECT
        series_bounds.company_id,
        series_bounds.series_keys,
        generate_series(first_week, last_week, interval '1 week') AS week
      FROM series_bounds
      WHERE first_week <= last_week
    )
  `;
}

// The final projection over the calendar: one row per (series, week), the flag
// carrying the gap signal while the value stays 0. `valueSource` is the CTE
// holding per-(company, week, series_keys) values (`counted` or `delta_rows`),
// exposing `company_id`, `week`, `series_keys`, `value`. `normalize` divides by
// the sum of real values across the whole result (global `share`); 0-valued gap
// and genuine-zero rows contribute nothing to that sum.
function weekSeriesSelect(
  sql: MeasureContext["sql"],
  valueSource: Fragment,
  companyGrouped: boolean,
  normalize: boolean,
): Fragment {
  const value = normalize
    ? sql`COALESCE(${valueSource}.value, 0)::double precision / NULLIF(sum(COALESCE(${valueSource}.value, 0)) OVER (), 0)`
    : sql`COALESCE(${valueSource}.value, 0)::int`;

  return sql`
    SELECT
      jsonb_build_object('week', to_char(weeks.week AT TIME ZONE 'UTC', 'YYYY-MM-DD'))
        || weeks.series_keys AS keys,
      ${value} AS value,
      ${weekGap(sql, companyGrouped)} AS gap
    FROM weeks
    LEFT JOIN ${valueSource}
      ON ${valueSource}.week = weeks.week
      AND ${valueSource}.company_id IS NOT DISTINCT FROM weeks.company_id
      AND ${valueSource}.series_keys = weeks.series_keys
    ORDER BY weeks.series_keys ASC, weeks.week ASC
  `;
}

// A week-grouped count/share. `week` buckets a posting by its first-seen week
// (as for rate); the remaining groupings form the non-week series. Unlike rate,
// no series ranking, limit, or company-universe seeding — series come from
// observed assignments (plus a single empty series when only `week` is grouped,
// so a scheduled-but-empty calendar still renders).
async function selectWeeklyAggregateRows(
  context: MeasureContext,
  normalize: boolean,
): Promise<readonly MeasureRow[]> {
  const { sql, composition, now } = context;
  const scope = createMeasureScope(sql, {
    ...composition,
    groupBy: composition.groupBy?.filter((grouping) => grouping !== "week"),
  });
  const where = scope.filters ?? sql`TRUE`;
  const seriesKeys = scope.keys ?? sql`'{}'::jsonb`;
  const companyGrouped = scope.groupBy.includes("company");
  const companyId = companyGrouped ? sql`posting.company_id` : sql`NULL::bigint`;
  const weekOnly = scope.groupBy.length === 0;

  const rows = await sql<RateSqlRow[]>`
    WITH company_scope AS (
      SELECT DISTINCT posting.company_id AS company_id
      FROM ${scope.cohort} AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      WHERE ${where}
    ),
    successful_company_weeks AS (
      ${successfulCompanyWeeks(sql, now, sql`company_scope`)}
    ),
    assignments AS (
      SELECT
        ${companyId} AS company_id,
        ${weekBucket(sql, sql`week_lifespan.first_seen`)} AS week,
        ${seriesKeys} AS series_keys
      FROM ${scope.cohort} AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      JOIN posting_lifespans AS week_lifespan
        ON week_lifespan.job_posting_id = cohort.job_posting_id
      ${scope.joins ?? sql``}
      WHERE ${where}
        AND week_lifespan.first_seen <= ${now}
    ),
    counted AS (
      SELECT company_id, week, series_keys, count(*)::int AS value
      FROM assignments
      GROUP BY company_id, week, series_keys
    ),
    series AS (
      SELECT DISTINCT company_id, series_keys FROM counted

      UNION

      SELECT NULL::bigint AS company_id, '{}'::jsonb AS series_keys
      WHERE ${weekOnly}
        AND EXISTS (SELECT 1 FROM successful_company_weeks)
    ),
    ${weekCalendar(sql, sql`series`, companyGrouped, now)}
    ${weekSeriesSelect(sql, sql`counted`, companyGrouped, normalize)}
  `;

  return rows.map((row) => ({
    keys: row.keys,
    value: row.value,
    ...(row.gap && { gap: true }),
  }));
}

// A week-grouped delta. The signed diff is computed per (series, week) across
// the two as-of snapshots, then LEFT JOINed onto the same calendar so failed-
// run weeks surface as gap rows rather than vanishing.
async function selectWeeklyDeltaRows(context: MeasureContext): Promise<readonly MeasureRow[]> {
  const { sql, composition, now } = context;
  const window = composition.window;
  if (window == null) throw new Error("delta requires a validated window");

  const scope = createMeasureScope(sql, {
    ...composition,
    groupBy: composition.groupBy?.filter((grouping) => grouping !== "week"),
  });
  const where = scope.filters ?? sql`TRUE`;
  const seriesKeys = scope.keys ?? sql`'{}'::jsonb`;
  const companyGrouped = scope.groupBy.includes("company");
  const companyId = companyGrouped ? sql`posting.company_id` : sql`NULL::bigint`;
  const weekOnly = scope.groupBy.length === 0;
  const previous = sql`${now} - ${window.weeks}::int * interval '1 week'`;

  const groupedSnapshot = (asOf: Fragment) => sql`
    SELECT
      ${companyId} AS company_id,
      ${weekBucket(sql, sql`week_lifespan.first_seen`)} AS week,
      ${seriesKeys} AS series_keys,
      count(*)::int AS value
    FROM open_postings_as_of(${asOf}) AS cohort
    JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
    JOIN posting_lifespans AS week_lifespan
      ON week_lifespan.job_posting_id = cohort.job_posting_id
    ${scope.joins ?? sql``}
    WHERE ${where}
      AND week_lifespan.first_seen <= ${now}
    GROUP BY
      ${companyId},
      ${weekBucket(sql, sql`week_lifespan.first_seen`)},
      ${seriesKeys}
  `;

  const rows = await sql<RateSqlRow[]>`
    WITH company_scope AS (
      SELECT DISTINCT posting.company_id AS company_id
      FROM open_postings_as_of(${now}) AS cohort
      JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
      WHERE ${where}
    ),
    successful_company_weeks AS (
      ${successfulCompanyWeeks(sql, now, sql`company_scope`)}
    ),
    current_grouped AS (
      ${groupedSnapshot(now)}
    ),
    previous_grouped AS (
      ${groupedSnapshot(previous)}
    ),
    delta_rows AS (
      SELECT
        COALESCE(current_grouped.company_id, previous_grouped.company_id) AS company_id,
        COALESCE(current_grouped.week, previous_grouped.week) AS week,
        COALESCE(current_grouped.series_keys, previous_grouped.series_keys) AS series_keys,
        COALESCE(current_grouped.value, 0) - COALESCE(previous_grouped.value, 0) AS value
      FROM current_grouped
      FULL JOIN previous_grouped
        ON current_grouped.company_id IS NOT DISTINCT FROM previous_grouped.company_id
        AND current_grouped.week = previous_grouped.week
        AND current_grouped.series_keys = previous_grouped.series_keys
    ),
    series AS (
      SELECT DISTINCT company_id, series_keys FROM delta_rows

      UNION

      SELECT NULL::bigint AS company_id, '{}'::jsonb AS series_keys
      WHERE ${weekOnly}
        AND EXISTS (SELECT 1 FROM successful_company_weeks)
    ),
    ${weekCalendar(sql, sql`series`, companyGrouped, now)}
    ${weekSeriesSelect(sql, sql`delta_rows`, companyGrouped, false)}
  `;

  return rows.map((row) => ({
    keys: row.keys,
    value: row.value,
    ...(row.gap && { gap: true }),
  }));
}

function orderClause(sql: MeasureContext["sql"], sort: "asc" | "desc" | undefined) {
  if (sort === "asc") return sql`ORDER BY value ASC, keys ASC`;
  if (sort === "desc") return sql`ORDER BY value DESC, keys ASC`;
  return sql`ORDER BY keys ASC`;
}

// A time series cannot apply LIMIT to its individual rows without severing its
// calendar. These modifiers therefore rank whole non-week series by their
// rate totals, then the outer query restores chronological week order.
function rateSeriesOrderClause(sql: MeasureContext["sql"], sort: "asc" | "desc" | undefined) {
  if (sort === "asc") return sql`ORDER BY total ASC, series_keys ASC, company_id ASC`;
  if (sort === "desc") return sql`ORDER BY total DESC, series_keys ASC, company_id ASC`;
  return sql`ORDER BY series_keys ASC, company_id ASC`;
}
