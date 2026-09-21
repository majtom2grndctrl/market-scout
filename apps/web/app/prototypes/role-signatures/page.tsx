// Prototype — asked 2026-09-17.
//
// Question: does the skill/role relationship read better as small multiples
// than as a matrix? The matrix sketch found an off-diagonal that is empty at
// any honest floor, which is Tufte's cue that the 2D form is carrying a
// one-dimensional finding — each role has a signature.
//
// One panel per role. Skills as a dot plot on a shared lift axis, drawn once
// at the top. Direct labels, no gridlines, no cell fills, no legend. Dot area
// carries base-N, the channel the matrix sketch confirmed works.
//
// What this is meant to settle against the matrix:
//   - Does a shared axis across panels support cross-role comparison better
//     than a shared colour ramp did?
//   - Does removing the empty off-diagonal lose anything real? The matrix
//     could show that a skill is absent from a role. A panel cannot.
//   - Do 90°-rotated column labels (matrix) cost more than the vertical
//     space these panels spend on horizontal ones?
//
// Knobs: `?axis=log|linear`, `?skills=8`, `?roles=16`, `?min=6`.
//
// After looking at it:
//   - It beats the matrix on every axis that matters. Same data, a fifth of
//     the ink, no empty off-diagonal, horizontal labels, one axis instead of
//     a colour legend. Cross-panel comparison works because the domain is
//     shared, which a colour ramp only pretended to do.
//   - What it gives up is real but small: the matrix could show that a skill
//     is ABSENT from a role. A panel only shows what is present. If absence
//     turns out to be the interesting half, that argues for the matrix.
//   - The biggest role has the flattest signature. `software engineer` (541)
//     tops out near 3x while `product manager` (203) reaches 20x, because a
//     role large enough to dominate the corpus pulls the average toward
//     itself. Lift is relative to "an average posting", and for the largest
//     role that average is largely itself. Worth stating in the spec: the
//     measure is weakest exactly where the data is thickest.
//   - Monochrome costs nothing here. Dot area carries base-N, position
//     carries lift, and no third channel was wanted — which frees colour for
//     something that actually needs it.
//
// Note: lift is computed here, which is the engine's job — see the matrix
// sketch. `affinity` has to become a measure before either form ships.
//
// Not production. See agent-context/lib/web-guide.md § Prototypes.

import { scaleLinear, scaleLog, scaleSqrt } from "d3-scale";

import type { Composition } from "@/lib/composition";
import { runComposition } from "@/lib/db/measure-engine";

export const dynamic = "force-dynamic";

const TRACK_WIDTH = 190;
const ROW_HEIGHT = 17;
// Label gutter, then the count hard against the right edge. A name longer than
// the gutter is truncated rather than allowed to collide with its own number.
const LABEL_GUTTER = 210;
const LABEL_CHARS = 27;

interface Knobs {
  readonly axis: "log" | "linear";
  readonly skills: number;
  readonly roles: number;
  readonly minPair: number;
}

interface Mark {
  readonly skill: string;
  readonly n: number;
  readonly lift: number;
}

function readKnobs(params: Record<string, string | string[] | undefined>): Knobs {
  const one = (key: string) => {
    const value = params[key];
    return Array.isArray(value) ? value[0] : value;
  };
  const int = (key: string, fallback: number, min: number, max: number) => {
    const parsed = Number.parseInt(one(key) ?? "", 10);
    return Number.isFinite(parsed) ? Math.min(Math.max(parsed, min), max) : fallback;
  };

  return {
    axis: one("axis") === "linear" ? "linear" : "log",
    skills: int("skills", 8, 3, 20),
    roles: int("roles", 16, 1, 40),
    minPair: int("min", 6, 1, 100),
  };
}

// `all`, not `open`: the open cohort carries the requisition sub-count that
// makes a two-dimension classified crossing take ~150s. See the matrix sketch.
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

const SKILL_TOTALS: Composition = {
  measure: "count",
  cohort: "all",
  groupBy: ["skill"],
  sort: "desc",
  encoding: "table",
};

async function run(label: string, composition: Composition) {
  const started = Date.now();
  const result = await runComposition(composition);
  console.log(`[panels] ${label}: ${Date.now() - started}ms`);
  if (!result.ok) throw new Error(result.error.message);
  return result.value;
}

function humanize(slug: string): string {
  return slug.replace(/-/g, " ");
}

function truncate(label: string): string {
  return label.length > LABEL_CHARS ? `${label.slice(0, LABEL_CHARS - 1)}…` : label;
}

export default async function RoleSignatures({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const knobs = readKnobs(await searchParams);
  const pairs = await run("pairs", PAIRS);
  const roleResult = await run("roleTotals", ROLE_TOTALS);
  const skillResult = await run("skillTotals", SKILL_TOTALS);

  const population = pairs.denominator?.classified ?? 0;
  const roleTotal = new Map(roleResult.rows.map((row) => [row.keys.role ?? "", row.value]));
  const skillTotal = new Map(skillResult.rows.map((row) => [row.keys.skill ?? "", row.value]));

  const marksByRole = new Map<string, Mark[]>();
  for (const row of pairs.rows) {
    const role = row.keys.role ?? "";
    const skill = row.keys.skill ?? "";
    if (row.value < knobs.minPair) continue;
    const expected = ((roleTotal.get(role) ?? 0) * (skillTotal.get(skill) ?? 0)) / (population || 1);
    if (expected <= 0) continue;
    const list = marksByRole.get(role) ?? [];
    list.push({ skill, n: row.value, lift: row.value / expected });
    marksByRole.set(role, list);
  }

  const panels = roleResult.rows
    .map((row) => row.keys.role ?? "")
    .filter((role) => (marksByRole.get(role)?.length ?? 0) > 0)
    .slice(0, knobs.roles)
    .map((role) => ({
      role,
      total: roleTotal.get(role) ?? 0,
      marks: (marksByRole.get(role) ?? [])
        .sort((a, b) => b.lift - a.lift)
        .slice(0, knobs.skills),
    }));

  const shownMarks = panels.flatMap((panel) => panel.marks);
  const maxLift = Math.max(2, ...shownMarks.map((mark) => mark.lift));
  const maxN = Math.max(1, ...shownMarks.map((mark) => mark.n));

  // One axis for every panel. That shared domain is the whole reason small
  // multiples beat sixteen independently-scaled charts.
  const x =
    knobs.axis === "log"
      ? scaleLog().domain([1, maxLift]).range([0, TRACK_WIDTH]).clamp(true)
      : scaleLinear().domain([0, maxLift]).range([0, TRACK_WIDTH]);
  const radius = scaleSqrt().domain([0, maxN]).range([0, 5.5]);

  const ticks = knobs.axis === "log" ? [1, 2, 5, 10, 25] : [0, maxLift / 2, maxLift];

  const link = (patch: Record<string, string | number>) => {
    const query = new URLSearchParams({
      axis: knobs.axis,
      skills: String(knobs.skills),
      roles: String(knobs.roles),
      min: String(knobs.minPair),
      ...Object.fromEntries(Object.entries(patch).map(([key, value]) => [key, String(value)])),
    });
    return `/prototypes/role-signatures?${query.toString()}`;
  };

  return (
    <main className="min-h-screen bg-surface-page p-8 text-content-primary">
      <header className="mb-6 max-w-content">
        <h1 className="text-2xl font-semibold">Role signatures</h1>
        <p className="mt-2 text-sm text-content-secondary">
          How much more often a skill lands on this role than on an average posting. One scale
          across every panel; dot area is how many postings stand behind the point.
        </p>
        <p className="mt-1 text-xs text-content-muted">
          {population.toLocaleString()} of {(pairs.denominator?.total ?? 0).toLocaleString()} seen
          postings carry a classification. Pairs under {knobs.minPair} postings are omitted.
        </p>
      </header>

      <nav className="mb-8 flex flex-wrap gap-x-6 gap-y-2 text-sm">
        <span className="flex gap-2">
          <span className="text-content-muted">axis</span>
          <a className="underline" href={link({ axis: "log" })}>
            log
          </a>
          <a className="underline" href={link({ axis: "linear" })}>
            linear
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">skills / panel</span>
          <a className="underline" href={link({ skills: 5 })}>
            5
          </a>
          <a className="underline" href={link({ skills: 8 })}>
            8
          </a>
          <a className="underline" href={link({ skills: 12 })}>
            12
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">floor</span>
          <a className="underline" href={link({ min: 3 })}>
            3
          </a>
          <a className="underline" href={link({ min: 6 })}>
            6
          </a>
          <a className="underline" href={link({ min: 15 })}>
            15
          </a>
        </span>
      </nav>

      <div className="mb-3">
        <p className="mb-1 text-xs text-content-muted">more often than an average posting →</p>
        <svg width={TRACK_WIDTH + LABEL_GUTTER} height={14} aria-hidden>
          {ticks.map((tick) => (
            <text
              key={tick}
              x={Math.min(TRACK_WIDTH, Math.max(8, x(tick)))}
              y={10}
              textAnchor="middle"
              className="fill-content-muted text-[10px]"
            >
              {Number.isInteger(tick) ? tick : tick.toFixed(1)}×
            </text>
          ))}
        </svg>
      </div>

      <div className="grid gap-x-10 gap-y-7 [grid-template-columns:repeat(auto-fill,minmax(27rem,1fr))]">
        {panels.map((panel) => (
          <section key={panel.role}>
            <h2 className="mb-1 flex items-baseline gap-2 text-sm">
              <span>{humanize(panel.role)}</span>
              <span className="tabular-nums text-content-muted">{panel.total}</span>
            </h2>
            <svg
              width={TRACK_WIDTH + LABEL_GUTTER}
              height={panel.marks.length * ROW_HEIGHT}
              role="img"
              aria-label={`${humanize(panel.role)}: ${panel.marks
                .map((mark) => `${humanize(mark.skill)} ${mark.lift.toFixed(1)}×`)
                .join(", ")}`}
            >
              {panel.marks.map((mark, index) => {
                const cy = index * ROW_HEIGHT + ROW_HEIGHT / 2;
                return (
                  <g key={mark.skill}>
                    <line
                      x1={0}
                      x2={TRACK_WIDTH}
                      y1={cy}
                      y2={cy}
                      className="stroke-edge-hairline"
                      strokeWidth={0.5}
                    />
                    <circle
                      cx={x(mark.lift)}
                      cy={cy}
                      r={Math.max(1.5, radius(mark.n))}
                      className="fill-content-primary"
                    />
                    <text
                      x={TRACK_WIDTH + 10}
                      y={cy + 3.5}
                      className="fill-content-secondary text-[11px]"
                    >
                      {truncate(humanize(mark.skill))}
                      <title>{humanize(mark.skill)}</title>
                    </text>
                    <text
                      x={TRACK_WIDTH + LABEL_GUTTER}
                      y={cy + 3.5}
                      textAnchor="end"
                      className="fill-content-muted text-[10px] tabular-nums"
                    >
                      {mark.n}
                    </text>
                  </g>
                );
              })}
            </svg>
          </section>
        ))}
      </div>
    </main>
  );
}
