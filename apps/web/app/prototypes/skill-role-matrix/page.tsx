// Prototype — asked 2026-09-17.
//
// Question: can one chart carry "how skills connect to roles", and does
// affinity-on-a-diverging-ramp actually read?
//
// Three sub-questions this sketch exists to answer, none of which a spec
// review can settle:
//   1. Does raw count fail the way we predict? (`?metric=count` — watch
//      whether hub skills like stakeholder-management flood every row.)
//   2. Does lift read as a relationship, or as noise? (`?metric=lift`)
//   3. Does base-N-as-cell-size keep thin cells visually quiet, or just
//      make the grid muddy? (`?channel=size` vs `?channel=colour`)
//
// Findings so far, all of which belong in the spec:
//   - The lift arithmetic below is the engine's job. A prototype computing
//     its own aggregate is exactly the invariant the grammar exists to
//     protect, so `affinity` has to become a measure before this ships.
//   - `count` over the *open* cohort grouped by two classified dimensions
//     takes ~150s. Not the crossing — the requisition sub-count that rides
//     along with count/open. `measure-aggregates.ts` benchmarked that cost
//     ungrouped and company-grouped only; a taxonomy crossing fans each
//     posting into many rows and `count(DISTINCT (record))` sorts every one
//     of them, per group. `?cohort=all` drops the sub-count and returns in
//     under a second. Engine bug, not a prototype problem.
//   - Fixed lift thresholds saturate: over a fragmented skill taxonomy most
//     live cells land above 3x, so the top bucket swallows the chart.
//     `?scale=quantile` buckets by rank instead. Open design question.
//   - Production would hand-render SVG per the charts decision. CSS grid
//     here is deliberate: the open question is legibility, not geometry.
//
// After looking at it (`?scale=fixed`):
//   - Base-N-as-size works. High-lift, well-evidenced cells read as solid
//     blocks; thin cells shrink to dots and stop shouting. Keep the channel.
//   - The diverging ramp is the wrong encoding for THIS selection. Picking
//     each skill's best role means every cell on screen is over-represented,
//     so the whole orange half of the scale goes unused. Either show all
//     roles per selected skill, or drop to a sequential ramp.
//   - Column selection is doing most of the work, and it is not part of the
//     grammar. Round-robin across roles produces the diagonal; a global
//     top-N produces one dense row and fifteen empty ones. A `matrix`
//     encoding cannot leave this unspecified.
//   - Off-diagonal is nearly empty at any honest floor, so the 2D form may
//     not be earning itself over small-multiple ranked bars. Open question.
//   - Data quality: the classifier emits role titles as skills — "product
//     management", "technical program manager", "solutions engineering".
//     Those inflate lift tautologically and are the strongest cells on the
//     diagonal. Some of this chart's signal is an artifact of that.
//     Near-duplicate slugs ("solution engineering" vs "solutions
//     engineering") sit in adjacent columns.
//
// Not production. See agent-context/lib/web-guide.md § Prototypes.

import type { Composition } from "@/lib/composition";
import { runComposition } from "@/lib/db/measure-engine";

export const dynamic = "force-dynamic";

// Static literals: Tailwind cannot see a class name built at runtime.
const DIVERGING = [
  "bg-delta-down-3",
  "bg-delta-down-2",
  "bg-delta-down-1",
  "bg-delta-mid",
  "bg-delta-up-1",
  "bg-delta-up-2",
  "bg-delta-up-3",
] as const;

const SEQUENTIAL = [
  "bg-ramp-1",
  "bg-ramp-2",
  "bg-ramp-3",
  "bg-ramp-4",
  "bg-ramp-5",
  "bg-ramp-6",
  "bg-ramp-7",
] as const;

const LIFT_EDGES = [0.5, 0.8, 0.95, 1.05, 1.5, 3] as const;

interface Knobs {
  readonly metric: "lift" | "count";
  readonly channel: "size" | "colour";
  readonly cohort: "all" | "open";
  readonly scale: "quantile" | "fixed";
  readonly roles: number;
  readonly skills: number;
  readonly minPair: number;
}

interface Cell {
  readonly role: string;
  readonly skill: string;
  readonly n: number;
  readonly lift: number;
}

function readKnobs(params: Record<string, string | string[] | undefined>): Knobs {
  const one = (key: string) => {
    const value = params[key];
    return Array.isArray(value) ? value[0] : value;
  };
  const int = (key: string, fallback: number, max: number) => {
    const parsed = Number.parseInt(one(key) ?? "", 10);
    return Number.isFinite(parsed) ? Math.min(Math.max(parsed, 1), max) : fallback;
  };

  return {
    metric: one("metric") === "count" ? "count" : "lift",
    channel: one("channel") === "colour" ? "colour" : "size",
    // Defaults to `all`: `open` is the honest question but currently costs
    // ~150s. See the header note.
    cohort: one("cohort") === "open" ? "open" : "all",
    scale: one("scale") === "fixed" ? "fixed" : "quantile",
    roles: int("roles", 16, 40),
    skills: int("skills", 32, 80),
    minPair: int("min", 6, 100),
  };
}

// Every read goes through the measure engine — prototypes get no SQL of their
// own. Where the engine cannot answer, that gap is the finding.
function compositions(cohort: Knobs["cohort"]) {
  return {
    pairs: {
      measure: "count",
      cohort,
      groupBy: ["role", "skill"],
      encoding: "table",
    } satisfies Composition,
    roleTotals: {
      measure: "count",
      cohort,
      groupBy: ["role"],
      sort: "desc",
      encoding: "table",
    } satisfies Composition,
    skillTotals: {
      measure: "count",
      cohort,
      groupBy: ["skill"],
      sort: "desc",
      encoding: "table",
    } satisfies Composition,
  };
}

async function run(label: string, composition: Composition) {
  const started = Date.now();
  const result = await runComposition(composition);
  console.log(`[proto] ${label}: ${Date.now() - started}ms`);
  // No error state by design: a prototype that cannot load is a prototype you fix.
  if (!result.ok) throw new Error(result.error.message);
  return result.value;
}

function humanize(slug: string): string {
  return slug.replace(/-/g, " ");
}

function bucket(value: number, edges: readonly number[]): number {
  let index = 0;
  while (index < edges.length && value > edges[index]) index += 1;
  return index;
}

export default async function SkillRoleMatrixPrototype({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const knobs = readKnobs(await searchParams);
  const plan = compositions(knobs.cohort);
  const pairResult = await run("pairs", plan.pairs);
  const roleResult = await run("roleTotals", plan.roleTotals);
  const skillResult = await run("skillTotals", plan.skillTotals);

  const population = pairResult.denominator?.classified ?? 0;
  const roleCount = new Map(roleResult.rows.map((row) => [row.keys.role ?? "", row.value]));
  const skillCount = new Map(skillResult.rows.map((row) => [row.keys.skill ?? "", row.value]));

  const cells: Cell[] = pairResult.rows
    .map((row) => {
      const role = row.keys.role ?? "";
      const skill = row.keys.skill ?? "";
      const expected =
        ((roleCount.get(role) ?? 0) * (skillCount.get(skill) ?? 0)) / (population || 1);
      return { role, skill, n: row.value, lift: expected > 0 ? row.value / expected : 0 };
    })
    .filter((cell) => cell.n >= knobs.minPair);

  const roles = roleResult.rows
    .map((row) => row.keys.role ?? "")
    .filter((role) => cells.some((cell) => cell.role === role))
    .slice(0, knobs.roles);

  const rank = new Map(roles.map((role, index) => [role, index]));

  // Column order is the whole readability experiment: sorting each skill to the
  // role it most belongs to should pull a diagonal out of the grid. If no
  // diagonal appears, the matrix is the wrong form and the sketch has earned
  // its keep by saying so.
  const anchored = new Map<string, { row: number; strength: number }>();
  for (const cell of cells) {
    const row = rank.get(cell.role);
    if (row === undefined) continue;
    const strength = knobs.metric === "count" ? cell.n : cell.lift;
    const current = anchored.get(cell.skill);
    if (current === undefined || strength > current.strength) {
      anchored.set(cell.skill, { row, strength });
    }
  }

  // Round-robin, not a global slice. Sorting every anchored skill by row and
  // taking the first N hands the whole budget to row 0, because the largest
  // role anchors the most skills — which is exactly how the first render
  // failed. Take a few per role instead, in role order, so a diagonal has a
  // chance to appear.
  const byRow = new Map<number, string[]>();
  for (const [skill, anchor] of anchored) {
    const bucketForRow = byRow.get(anchor.row) ?? [];
    bucketForRow.push(skill);
    byRow.set(anchor.row, bucketForRow);
  }
  for (const [, list] of byRow) {
    list.sort((a, b) => (anchored.get(b)?.strength ?? 0) - (anchored.get(a)?.strength ?? 0));
  }

  const skills: string[] = [];
  for (let depth = 0; skills.length < knobs.skills && depth < 12; depth += 1) {
    for (let row = 0; row < roles.length && skills.length < knobs.skills; row += 1) {
      const candidate = byRow.get(row)?.[depth];
      if (candidate !== undefined) skills.push(candidate);
    }
  }
  skills.sort(
    (a, b) =>
      (anchored.get(a)?.row ?? 0) - (anchored.get(b)?.row ?? 0) ||
      (anchored.get(b)?.strength ?? 0) - (anchored.get(a)?.strength ?? 0),
  );

  const shown = new Set(skills);
  const byKey = new Map(cells.map((cell) => [`${cell.role}|${cell.skill}`, cell]));
  const visible = cells.filter((cell) => shown.has(cell.skill) && rank.has(cell.role));
  const maxPair = Math.max(1, ...visible.map((cell) => cell.n));

  // Fixed thresholds saturate over this taxonomy, so the default buckets by
  // rank instead: six cut points at even quantiles of the lifts actually on
  // screen. Honest for reading structure, dishonest about absolute magnitude —
  // which is the trade the spec has to settle.
  const sortedLifts = visible.map((cell) => cell.lift).sort((a, b) => a - b);
  const edges =
    knobs.scale === "fixed" || sortedLifts.length < 7
      ? LIFT_EDGES
      : Array.from(
          { length: 6 },
          (_, index) => sortedLifts[Math.floor(((index + 1) / 7) * (sortedLifts.length - 1))],
        );

  const link = (patch: Record<string, string | number>) => {
    const query = new URLSearchParams({
      metric: knobs.metric,
      channel: knobs.channel,
      cohort: knobs.cohort,
      scale: knobs.scale,
      roles: String(knobs.roles),
      skills: String(knobs.skills),
      min: String(knobs.minPair),
      ...Object.fromEntries(Object.entries(patch).map(([key, value]) => [key, String(value)])),
    });
    return `/prototypes/skill-role-matrix?${query.toString()}`;
  };

  return (
    <main className="min-h-screen bg-surface-page p-8 text-content-primary">
      <header className="mb-6 max-w-content">
        <h1 className="text-2xl font-semibold">Skills &times; roles</h1>
        <p className="mt-2 text-sm text-content-secondary">
          {knobs.metric === "lift"
            ? "Affinity — how much more often this skill lands on this role than on an average posting. Diverging around 1.0."
            : "Raw posting count. Sequential ramp. Watch the hub skills take over."}
        </p>
        <p className="mt-1 text-xs text-content-muted">
          {population.toLocaleString()} of {(pairResult.denominator?.total ?? 0).toLocaleString()}{" "}
          {knobs.cohort === "open" ? " open" : " seen"} postings carry a classification. Cells
          under {knobs.minPair} postings are hidden.
        </p>
      </header>

      <nav className="mb-6 flex flex-wrap gap-x-6 gap-y-2 text-sm">
        <span className="flex gap-2">
          <span className="text-content-muted">metric</span>
          <a className="underline" href={link({ metric: "lift" })}>
            affinity
          </a>
          <a className="underline" href={link({ metric: "count" })}>
            count
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">base-N</span>
          <a className="underline" href={link({ channel: "size" })}>
            as size
          </a>
          <a className="underline" href={link({ channel: "colour" })}>
            ignored
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">cohort</span>
          <a className="underline" href={link({ cohort: "all" })}>
            all
          </a>
          <a className="underline" href={link({ cohort: "open" })}>
            open (slow)
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">scale</span>
          <a className="underline" href={link({ scale: "quantile" })}>
            quantile
          </a>
          <a className="underline" href={link({ scale: "fixed" })}>
            fixed
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

      <div className="overflow-x-auto">
        <div
          className="grid w-max gap-px"
          style={{ gridTemplateColumns: `11rem repeat(${skills.length}, 1.375rem)` }}
        >
          <div />
          {skills.map((skill) => (
            <div key={skill} className="relative h-40 text-xs text-content-secondary">
              <span className="absolute bottom-1 left-1/2 w-40 origin-bottom-left -rotate-90 whitespace-nowrap">
                {humanize(skill)}
              </span>
            </div>
          ))}

          {roles.map((role) => (
            <Row
              key={role}
              role={role}
              skills={skills}
              byKey={byKey}
              knobs={knobs}
              edges={edges}
              maxPair={maxPair}
              total={roleCount.get(role) ?? 0}
            />
          ))}
        </div>
      </div>

      <Legend metric={knobs.metric} edges={edges} />
    </main>
  );
}

function Row({
  role,
  skills,
  byKey,
  knobs,
  edges,
  maxPair,
  total,
}: {
  role: string;
  skills: readonly string[];
  byKey: Map<string, Cell>;
  knobs: Knobs;
  edges: readonly number[];
  maxPair: number;
  total: number;
}) {
  return (
    <>
      <div className="flex items-center justify-end gap-2 pr-3 text-xs">
        <span className="truncate text-content-primary">{humanize(role)}</span>
        <span className="tabular-nums text-content-muted">{total}</span>
      </div>
      {skills.map((skill) => {
        const cell = byKey.get(`${role}|${skill}`);
        if (cell === undefined) {
          return <div key={skill} className="h-6 bg-surface-sunken" />;
        }

        const swatch =
          knobs.metric === "lift"
            ? DIVERGING[bucket(cell.lift, edges)]
            : SEQUENTIAL[Math.min(6, Math.floor((cell.n / maxPair) ** 0.5 * 7))];

        const scale = knobs.channel === "size" ? Math.max(0.28, (cell.n / maxPair) ** 0.5) : 1;

        return (
          <div
            key={skill}
            className="flex h-6 items-center justify-center bg-surface-sunken"
            title={`${humanize(role)} × ${humanize(skill)} — ${cell.n} postings, ${cell.lift.toFixed(1)}× expected`}
          >
            <span
              className={`${swatch} block rounded-xs`}
              style={{ width: `${scale * 100}%`, height: `${scale * 100}%` }}
            />
          </div>
        );
      })}
    </>
  );
}

function Legend({ metric, edges }: { metric: Knobs["metric"]; edges: readonly number[] }) {
  const swatches = metric === "lift" ? DIVERGING : SEQUENTIAL;
  const labels =
    metric === "lift"
      ? [...edges.map((edge) => `${edge.toFixed(1)}×`), "higher"]
      : ["fewest", "", "", "", "", "", "most"];

  // A diverging ramp has to diverge around something. Quantile edges rarely
  // straddle 1.0, so claiming under/over-representation there would be a lie
  // the colours tell for us.
  const diverges = metric === "lift" && edges.some((edge) => edge < 1) && edges.some((edge) => edge > 1);

  return (
    <div className="mt-8 flex items-end gap-3 text-xs text-content-muted">
      <span>{metric !== "lift" ? "postings" : diverges ? "under-represented" : "lower"}</span>
      <span className="flex gap-px">
        {swatches.map((swatch, index) => (
          <span key={swatch} className="flex flex-col items-center gap-1">
            <span className={`${swatch} block h-4 w-12 rounded-xs`} />
            <span>{labels[index]}</span>
          </span>
        ))}
      </span>
      {metric === "lift" && <span>{diverges ? "over-represented" : "higher"}</span>}
    </div>
  );
}
