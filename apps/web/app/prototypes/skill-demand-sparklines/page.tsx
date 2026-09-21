// Prototype — asked 2026-09-17.
//
// Question: does a Tufte table-graphic answer both halves of the original
// ask in one artifact — how skills attach to roles, and which way demand is
// moving — at a density the matrix never reached?
//
// One row per skill: name, corpus count, a word-sized 17-week series, the
// roles it most belongs to, and the direction of travel. No gridlines, no
// legend, no cell fills. Direct labels only.
//
// Design questions live in the knobs:
//   `?y=shared`   — one y-domain across every row. Comparable, but the long
//                   tail flatlines. `?y=row` (default) scales each row to
//                   itself, which is the sparkline convention and lies about
//                   magnitude unless the count column is read alongside.
//   `?sort=slope` — order by direction of travel rather than volume.
//   `?rows=60`    — Tufte would push this higher. See where it breaks.
//
// WHAT THIS SKETCH FOUND, and it is the important one:
//   A time series over a classified dimension does not measure demand. It
//   measures enrichment recency. Classified coverage by arrival week runs
//   88-100% through 2026-06-29, then 6.8% (07-06), 8.1% (07-20), and 0-2%
//   from August on. Every skill therefore flatlines to zero in the recent
//   weeks, and the chart reads as a market collapse that did not happen.
//   The global denominator cannot catch this: it reports 52% classified
//   while the last eight weeks are ~0%. A composition grouped by `week` over
//   a classified dimension needs a PER-WEEK coverage disclosure, not a
//   corpus-wide one. That is a grammar gap, not a chart bug.
//
//   Rendered here as a coverage band behind each series plus a coverage
//   strip above the table — the metadata gets the same graphical treatment
//   as the data, which is the only way a reader sees it in time.
//
// Notes for the spec:
//   - The weekly path counts postings by first-seen week, so this is an
//     ARRIVAL series, not open-count-at-week. Labelled as such; a different
//     question would need a different measure. Note this applies to the
//     DEMAND_TREND seed composition too — it reads as stock, it is arrivals.
//   - The first two weeks are corpus backfill (3,531 of 10,316 first-seens),
//     and 2026-07-20 absorbs a two-week fetch outage. None are demand.
//     Hidden by default, disclosed in the header, restorable via
//     `?bootstrap=show`.
//   - `shapeTemporal` cannot serve this. It splits segments at gaps, which
//     is exactly right, but `Segment` carries no series identity — so a
//     small-multiple caller cannot tell which skill a segment belongs to.
//     The grouping below is prototype-local and should not be copied; fix
//     the shaper instead.
//   - Weekly compositions take a different engine path than `count` over a
//     flat grouping, and carry no requisition sub-count, so none of the
//     150s pathology in the matrix prototype applies here.
//
// Not production. See agent-context/lib/web-guide.md § Prototypes.

import { scaleLinear, scaleUtc } from "d3-scale";
import { line } from "d3-shape";

import type { Composition } from "@/lib/composition";
import { runComposition } from "@/lib/db/measure-engine";

export const dynamic = "force-dynamic";

const SPARK_WIDTH = 132;
const SPARK_HEIGHT = 22;

interface Knobs {
  readonly y: "row" | "shared";
  readonly sort: "count" | "slope";
  readonly rows: number;
  readonly bootstrap: "hide" | "show";
}

interface Point {
  readonly week: Date;
  readonly value: number;
}

interface SkillRow {
  readonly skill: string;
  readonly total: number;
  readonly segments: readonly (readonly Point[])[];
  readonly peak: number;
  readonly last: number;
  readonly slope: number;
  readonly roles: readonly { readonly role: string; readonly lift: number }[];
}

function readKnobs(params: Record<string, string | string[] | undefined>): Knobs {
  const one = (key: string) => {
    const value = params[key];
    return Array.isArray(value) ? value[0] : value;
  };
  const parsed = Number.parseInt(one("rows") ?? "", 10);

  return {
    y: one("y") === "shared" ? "shared" : "row",
    sort: one("sort") === "slope" ? "slope" : "count",
    rows: Number.isFinite(parsed) ? Math.min(Math.max(parsed, 5), 120) : 40,
    bootstrap: one("bootstrap") === "show" ? "show" : "hide",
  };
}

// `all` rather than `open`: the open cohort drags in the requisition
// sub-count that makes a classified crossing unusable. See the matrix sketch.
const WEEKLY: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["skill", "week"],
  encoding: "line",
};

const SKILL_TOTALS: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["skill"],
  sort: "desc",
  encoding: "table",
};

const PAIRS: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["role", "skill"],
  encoding: "table",
};

const ROLE_TOTALS: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["role"],
  sort: "desc",
  encoding: "table",
};

// Postings per week, all of them — the coverage denominator.
const WEEKLY_ALL: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["week"],
  encoding: "line",
};

// Seniority is exactly one per classification, so summing it per week counts
// classified postings per week without double-counting the way role or skill
// would. A `classified` measure would say this directly; the grammar has none.
const WEEKLY_CLASSIFIED: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["seniority", "week"],
  encoding: "line",
};

async function run(label: string, composition: Composition) {
  const started = Date.now();
  const result = await runComposition(composition);
  console.log(`[spark] ${label}: ${Date.now() - started}ms`);
  if (!result.ok) throw new Error(result.error.message);
  return result.value;
}

function humanize(slug: string): string {
  return slug.replace(/-/g, " ");
}

export default async function SkillDemandSparklines({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const knobs = readKnobs(await searchParams);
  const weekly = await run("weekly", WEEKLY);
  const totals = await run("skillTotals", SKILL_TOTALS);
  const pairs = await run("pairs", PAIRS);
  const roleTotals = await run("roleTotals", ROLE_TOTALS);
  const weeklyAll = await run("weeklyAll", WEEKLY_ALL);
  const weeklyClassified = await run("weeklyClassified", WEEKLY_CLASSIFIED);

  // Per-week classified coverage. The corpus-wide denominator reports ~52%
  // while recent weeks sit near zero, so a weekly series needs its own.
  const allByWeek = new Map<string, number>();
  for (const row of weeklyAll.rows) {
    if (row.gap === true || row.keys.week == null) continue;
    allByWeek.set(row.keys.week, (allByWeek.get(row.keys.week) ?? 0) + row.value);
  }
  const classifiedByWeek = new Map<string, number>();
  for (const row of weeklyClassified.rows) {
    if (row.gap === true || row.keys.week == null) continue;
    classifiedByWeek.set(row.keys.week, (classifiedByWeek.get(row.keys.week) ?? 0) + row.value);
  }
  const coverageByWeek = new Map<string, number>();
  for (const [week, total] of allByWeek) {
    coverageByWeek.set(week, total === 0 ? 0 : (classifiedByWeek.get(week) ?? 0) / total);
  }

  const population = totals.denominator?.classified ?? 0;
  const skillTotal = new Map(totals.rows.map((row) => [row.keys.skill ?? "", row.value]));
  const roleTotal = new Map(roleTotals.rows.map((row) => [row.keys.role ?? "", row.value]));

  // Top roles per skill, by lift rather than count — the point of the column
  // is which roles this skill *belongs to*, not which roles are biggest.
  const rolesBySkill = new Map<string, { role: string; lift: number }[]>();
  for (const row of pairs.rows) {
    const role = row.keys.role ?? "";
    const skill = row.keys.skill ?? "";
    const expected = ((roleTotal.get(role) ?? 0) * (skillTotal.get(skill) ?? 0)) / (population || 1);
    if (expected <= 0 || row.value < 4) continue;
    const list = rolesBySkill.get(skill) ?? [];
    list.push({ role, lift: row.value / expected });
    rolesBySkill.set(skill, list);
  }

  // Gap weeks break the series rather than interpolating through it; absence
  // is data, and a sparkline that draws through a failed run is lying at 132px.
  const seriesBySkill = new Map<string, Point[][]>();
  const weeks = new Set<string>();
  for (const row of weekly.rows) {
    const skill = row.keys.skill ?? "";
    const weekKey = row.keys.week;
    if (weekKey == null) continue;
    weeks.add(weekKey);

    const segments = seriesBySkill.get(skill) ?? [[]];
    if (row.gap === true) {
      if (segments[segments.length - 1].length > 0) segments.push([]);
    } else {
      segments[segments.length - 1].push({
        week: new Date(`${weekKey}T00:00:00.000Z`),
        value: row.value,
      });
    }
    seriesBySkill.set(skill, segments);
  }

  const allWeeks = [...weeks].sort();
  // Weeks 1-2 are corpus backfill, not arrivals. Keeping them compresses every
  // other week into the baseline, so they are excluded by default and said so.
  const orderedWeeks = knobs.bootstrap === "show" ? allWeeks : allWeeks.slice(2);
  const firstWeek = orderedWeeks[0];
  const domain: [Date, Date] = [
    new Date(`${orderedWeeks[0]}T00:00:00.000Z`),
    new Date(`${orderedWeeks[orderedWeeks.length - 1]}T00:00:00.000Z`),
  ];

  const half = Math.floor(orderedWeeks.length / 2);
  const rows: SkillRow[] = [...seriesBySkill.entries()]
    .map(([skill, rawSegments]) => {
      const segments = rawSegments
        .map((segment) => segment.filter((point) => point.week >= new Date(`${firstWeek}T00:00:00.000Z`)))
        .filter((segment) => segment.length > 0);
      const points = segments.flat();
      const recent = points.filter((point) => point.week >= new Date(`${orderedWeeks[half]}T00:00:00.000Z`));
      const earlier = points.filter((point) => point.week < new Date(`${orderedWeeks[half]}T00:00:00.000Z`));
      const recentSum = recent.reduce((sum, point) => sum + point.value, 0);
      const earlierSum = earlier.reduce((sum, point) => sum + point.value, 0);

      return {
        skill,
        total: skillTotal.get(skill) ?? 0,
        segments,
        peak: Math.max(1, ...points.map((point) => point.value)),
        last: points.length > 0 ? points[points.length - 1].value : 0,
        slope: earlierSum === 0 ? (recentSum > 0 ? 1 : 0) : recentSum / earlierSum - 1,
        roles: (rolesBySkill.get(skill) ?? [])
          .sort((a, b) => b.lift - a.lift)
          .slice(0, 2),
      };
    })
    .filter((row) => row.total >= 10 && row.segments.length > 0)
    .sort((a, b) => (knobs.sort === "slope" ? b.slope - a.slope : b.total - a.total))
    .slice(0, knobs.rows);

  const sharedPeak = Math.max(1, ...rows.map((row) => row.peak));
  const x = scaleUtc().domain(domain).range([1, SPARK_WIDTH - 1]);

  // Contiguous runs of weeks whose classified coverage is too thin to read.
  // Shaded behind every series so "we did not look" never renders as "zero".
  const thinWeeks: { from: string; to: string }[] = [];
  for (const [index, week] of orderedWeeks.entries()) {
    if ((coverageByWeek.get(week) ?? 0) >= 0.5) continue;
    const previous = thinWeeks[thinWeeks.length - 1];
    if (previous != null && previous.to === orderedWeeks[index - 1]) previous.to = week;
    else thinWeeks.push({ from: week, to: week });
  }
  const coverageY = scaleLinear().domain([0, 1]).range([SPARK_HEIGHT - 2, 2]);
  const coveragePath =
    line<string>()
      .x((week) => x(new Date(`${week}T00:00:00.000Z`)))
      .y((week) => coverageY(coverageByWeek.get(week) ?? 0))(orderedWeeks) ?? undefined;

  const link = (patch: Record<string, string | number>) => {
    const query = new URLSearchParams({
      y: knobs.y,
      sort: knobs.sort,
      rows: String(knobs.rows),
      bootstrap: knobs.bootstrap,
      ...Object.fromEntries(Object.entries(patch).map(([key, value]) => [key, String(value)])),
    });
    return `/prototypes/skill-demand-sparklines?${query.toString()}`;
  };

  return (
    <main className="min-h-screen bg-surface-page p-8 text-content-primary">
      <header className="mb-6 max-w-content">
        <h1 className="text-2xl font-semibold">Skill demand</h1>
        <p className="mt-2 text-sm text-content-secondary">
          New postings per week, {orderedWeeks.length} weeks
          {knobs.bootstrap === "hide" && " (the first two, corpus backfill, are excluded)"}. The
          line breaks where a fetch run failed — absence is not zero.
        </p>
        <p className="mt-1 text-xs text-content-muted">
          {population.toLocaleString()} of {(totals.denominator?.total ?? 0).toLocaleString()} seen
          postings carry a classification. Skills under 10 postings are omitted.
        </p>
      </header>

      <nav className="mb-6 flex flex-wrap gap-x-6 gap-y-2 text-sm">
        <span className="flex gap-2">
          <span className="text-content-muted">order</span>
          <a className="underline" href={link({ sort: "count" })}>
            volume
          </a>
          <a className="underline" href={link({ sort: "slope" })}>
            direction
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">y-scale</span>
          <a className="underline" href={link({ y: "row" })}>
            per row
          </a>
          <a className="underline" href={link({ y: "shared" })}>
            shared
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">backfill weeks</span>
          <a className="underline" href={link({ bootstrap: "hide" })}>
            hide
          </a>
          <a className="underline" href={link({ bootstrap: "show" })}>
            show
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">rows</span>
          <a className="underline" href={link({ rows: 20 })}>
            20
          </a>
          <a className="underline" href={link({ rows: 40 })}>
            40
          </a>
          <a className="underline" href={link({ rows: 80 })}>
            80
          </a>
        </span>
      </nav>

      <table className="text-sm">
        <thead>
          <tr className="text-xs text-content-muted">
            <th className="pr-6 pb-2 text-left font-normal">skill</th>
            <th className="pr-4 pb-2 text-right font-normal">n</th>
            <th className="pr-4 pb-2 text-left font-normal">
              {orderedWeeks[0]?.slice(5)} → {orderedWeeks[orderedWeeks.length - 1]?.slice(5)}
            </th>
            <th className="pr-4 pb-2 text-right font-normal">last</th>
            <th className="pb-2 text-left font-normal">belongs to</th>
          </tr>
        </thead>
        <tbody>
          <tr className="text-xs text-content-muted">
            <td className="py-0.5 pr-6 whitespace-nowrap italic">classified coverage</td>
            <td className="py-0.5 pr-4 text-right tabular-nums">
              {Math.round((coverageByWeek.get(orderedWeeks[0]) ?? 0) * 100)}%
            </td>
            <td className="py-0.5 pr-4">
              <svg
                width={SPARK_WIDTH}
                height={SPARK_HEIGHT}
                viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`}
                role="img"
                aria-label="Share of each week's postings that carry a classification"
              >
                {thinWeeks.map((band) => (
                  <rect
                    key={band.from}
                    x={x(new Date(`${band.from}T00:00:00.000Z`))}
                    width={Math.max(
                      2,
                      x(new Date(`${band.to}T00:00:00.000Z`)) -
                        x(new Date(`${band.from}T00:00:00.000Z`)),
                    )}
                    y={0}
                    height={SPARK_HEIGHT}
                    className="fill-unavailable-subtle"
                  />
                ))}
                <path d={coveragePath} fill="none" strokeWidth={1} className="stroke-stale-ink" />
              </svg>
            </td>
            <td className="py-0.5 pr-4 text-right tabular-nums">
              {Math.round((coverageByWeek.get(orderedWeeks[orderedWeeks.length - 1]) ?? 0) * 100)}%
            </td>
            <td className="py-0.5 italic">
              shaded weeks are under 50% classified — not measurable demand
            </td>
          </tr>
          {rows.map((row) => {
            const peak = knobs.y === "shared" ? sharedPeak : row.peak;
            const y = scaleLinear()
              .domain([0, peak])
              .range([SPARK_HEIGHT - 2, 2]);
            const path = line<Point>()
              .x((point) => x(point.week))
              .y((point) => y(point.value));
            const endpoint = row.segments[row.segments.length - 1]?.at(-1);
            const direction =
              row.slope > 0.15
                ? "fill-trend-up-solid"
                : row.slope < -0.15
                  ? "fill-trend-down-solid"
                  : "fill-trend-flat-solid";

            return (
              <tr key={row.skill} className="align-middle hover:bg-surface-row-hover">
                <td className="py-0.5 pr-6 whitespace-nowrap">{humanize(row.skill)}</td>
                <td className="py-0.5 pr-4 text-right tabular-nums text-content-secondary">
                  {row.total}
                </td>
                <td className="py-0.5 pr-4">
                  <svg
                    width={SPARK_WIDTH}
                    height={SPARK_HEIGHT}
                    viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`}
                    role="img"
                    aria-label={`${humanize(row.skill)}, ${row.segments.flat().length} weeks plotted, last ${row.last}`}
                  >
                    {thinWeeks.map((band) => (
                      <rect
                        key={band.from}
                        x={x(new Date(`${band.from}T00:00:00.000Z`))}
                        width={Math.max(
                          2,
                          x(new Date(`${band.to}T00:00:00.000Z`)) -
                            x(new Date(`${band.from}T00:00:00.000Z`)),
                        )}
                        y={0}
                        height={SPARK_HEIGHT}
                        className="fill-unavailable-subtle"
                      />
                    ))}
                    {row.segments.map((segment, index) => (
                      <path
                        key={index}
                        d={path(segment) ?? undefined}
                        fill="none"
                        strokeWidth={1}
                        className="stroke-content-secondary"
                      />
                    ))}
                    {endpoint != null && (
                      <circle cx={x(endpoint.week)} cy={y(endpoint.value)} r={1.75} className={direction} />
                    )}
                  </svg>
                </td>
                <td className="py-0.5 pr-4 text-right tabular-nums text-content-secondary">
                  {row.last}
                </td>
                <td className="py-0.5 whitespace-nowrap text-content-secondary">
                  {row.roles.length === 0
                    ? <span className="text-content-muted">—</span>
                    : row.roles.map((entry, index) => (
                        <span key={entry.role}>
                          {index > 0 && <span className="text-content-muted">, </span>}
                          {humanize(entry.role)}
                          <span className="text-content-muted"> {entry.lift.toFixed(1)}×</span>
                        </span>
                      ))}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </main>
  );
}
