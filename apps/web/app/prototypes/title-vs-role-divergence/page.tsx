// Prototype — asked 2026-09-21.
//
// Question: which canonical roles has the market agreed on a NAME for, and
// which ones is it still improvising a name for, one posting at a time?
//
// THE MEASURE — modal head share.
//
// A posting title is a base title plus a scope qualifier: "Software Engineer,
// Compute Infrastructure", "Account Executive — Mid-Market". Cut at the first
// comma or dash and what is left is the TITLE HEAD, the name the employer
// reached for before narrowing it. Group every open posting classified into a
// canonical role by its head, and ask what fraction of them share the single
// most common one.
//
//   high share  a settled, traditional name. 40 of 44 Field Sales
//               Representative postings say the same three words.
//   low share   no agreed name. 28 Support Engineer postings carry 15
//               different heads and the biggest of them is 5 postings.
//
// This is the taxonomy's own coverage seen from the other side. A canonical
// role with a low modal share is a real category the classifier had to
// assemble out of names nobody standardised — which is either evidence the
// role is genuinely emerging, or evidence the taxonomy is over-merging. The
// chart cannot tell those apart, and does not claim to.
//
// TWO NORMALISATION DEFECTS, both fixed in `query.ts`, both material:
//   1. Seniority fragmented the head. `staff software engineer` and
//      `software engineer` counted as different names for the same job.
//      Stripping the prefix moved Software Engineer 0.15 → 0.57 and Product
//      Engineer 0.16 → 0.46. Every number here is downstream of that fix, so
//      it is the first thing to distrust.
//   2. Trailing whitespace was not trimmed — `forward deployed engineer `
//      was its own head.
//
// KNOWN DEFECT, left in deliberately. The split treats a bare hyphen as a
// separator, so `Full-Stack Engineer` heads as `full`. Three postings in the
// current corpus, all singletons, none near a modal head. Fixing it means
// splitting only on a SPACED hyphen, which is a third change to a measure the
// numbers above are already sensitive to — recorded rather than silently
// applied.
//
// Knob: `?min=20` — the posting floor. See § Floor on the page.
//
// Not production. See agent-context/lib/web-guide.md § Prototypes.

import { scaleBand, scaleLinear } from "d3-scale";
import { max } from "d3-array";

import { Axis, type AxisValue } from "@/components/chart/primitives";
import { Meter } from "@/components/meter";
import { absoluteDate, exactCount, percentLabel, share } from "@/lib/format";

import {
  fetchBias,
  fetchCoverage,
  fetchRoleHeads,
  type Bias,
  type Coverage,
  type RoleHeadRow,
} from "./query";

export const dynamic = "force-dynamic";

const DEFAULT_FLOOR = 20;

/** One employer past this share of a role's postings and the role is that employer. */
const CONCENTRATION_FLAG = 0.6;

const PLOT_WIDTH = 440;
const ROLE_GUTTER = 250;
const HEAD_GUTTER = 370;
const ROW_HEIGHT = 30;
const MARGIN = { top: 8, right: HEAD_GUTTER, bottom: 40, left: ROLE_GUTTER };

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const floor = readFloor(await searchParams);
  const [rows, coverage, bias] = await Promise.all([
    fetchRoleHeads(floor),
    fetchCoverage(),
    fetchBias(),
  ]);

  return (
    <main className="mx-auto max-w-wide px-6 py-10">
      <header className="max-w-[65ch]">
        <p className="text-xs uppercase tracking-wide text-content-muted">
          Prototype · asked 2026-09-21
        </p>
        <h1 className="mt-2 font-display text-3xl tracking-tight text-content-primary">
          Title-vs-role divergence
        </h1>
        <p className="mt-3 text-content-secondary">
          Of the open postings classified into a canonical role, what fraction share the
          single most common <strong className="font-semibold">title head</strong> — the
          part of the raw posting title before the first comma or dash? A low share means
          the market has no agreed name for the work. A high share means a settled one.
        </p>
      </header>

      <CoveragePanel coverage={coverage} bias={bias} rows={rows} floor={floor} />

      <section className="mt-10">
        <h2 className="font-display text-xl tracking-tight text-content-primary">
          Roles by modal head share
        </h2>
        <p className="mt-1 text-sm text-content-muted">
          Ascending — most-improvised names first. One bar per role; the label to the
          right is that role&rsquo;s modal head.
        </p>
        {rows.length === 0 ? (
          <p className="mt-6 text-content-secondary">
            No role clears a floor of {floor} postings.
          </p>
        ) : (
          <DivergenceChart rows={rows} />
        )}
      </section>

      {rows.length > 0 && <RoleTable rows={rows} />}

      <FloorNote floor={floor} rows={rows} coverage={coverage} />
    </main>
  );
}

/* -------------------------------------------------------------------------- */
/* Coverage — first, not last. A number this partial has to arrive with its    */
/* denominator attached, and the denominators are measured here, not typed in. */
/* -------------------------------------------------------------------------- */

function CoveragePanel({
  coverage,
  bias,
  rows,
  floor,
}: {
  coverage: Coverage;
  bias: Bias;
  rows: readonly RoleHeadRow[];
  floor: number;
}) {
  const charted = rows.reduce((sum, row) => sum + row.postings, 0);
  const concentration = share(coverage.top3Postings, coverage.openPostings);
  const excluded = bias.platforms.filter((platform) => platform.classified === 0);
  const covered = bias.platforms.filter((platform) => platform.classified > 0);

  return (
    <section className="mt-8">
      <h2 className="font-display text-xl tracking-tight text-content-primary">
        What this chart can and cannot see
      </h2>

      <dl className="mt-4 grid gap-4 sm:grid-cols-3">
        <Meter
          label="Open postings carrying a classification"
          value={coverage.classifiedPostings}
          total={coverage.openPostings}
          noun="open postings"
        />
        <Meter
          label="Role assignments above the floor"
          value={charted}
          total={coverage.roleAssignments}
          noun="role assignments"
        />
        <Meter
          label="Canonical roles above the floor"
          value={rows.length}
          total={coverage.distinctRoles}
          noun="roles seen in the open cohort"
        />
      </dl>

      {/* The one-line version, placed where a reader who stops reading still
          gets it. The bullets below are the evidence for this sentence. */}
      <p className="mt-4 rounded-lg border border-edge bg-surface-sunken p-4 text-sm text-content-primary">
        <strong className="font-semibold">Read this chart as a biased sample.</strong> It
        describes older postings at over-drained companies on{" "}
        {joinNames(covered.map((platform) => titleCase(platform.ats)))} — not the open-posting corpus.
        Enrichment drains oldest-first and needs a description, so the classified subset
        skews old and omits whole ATS platforms and whole companies.
      </p>

      <ul className="mt-4 space-y-2 text-sm text-content-secondary">
        <li>
          <strong className="font-semibold text-content-primary">
            The classified subset is old, and that is the worst direction for this
            question.
          </strong>{" "}
          Median first-seen is {absoluteDate(bias.classifiedMedianFirstSeen)} for classified
          postings against {absoluteDate(bias.unclassifiedMedianFirstSeen)} for
          unclassified. Only {exactCount(bias.classifiedThisMonth)} classified postings were
          first seen this month, against {exactCount(bias.unclassifiedThisMonth)}{" "}
          unclassified. New titles appear in new postings, and new postings are nearly
          absent — so the chart understates exactly the improvisation it exists to find.
        </li>
        {excluded.length > 0 && (
          <li>
            <strong className="font-semibold text-content-primary">
              Whole ATS platforms are structurally excluded.
            </strong>{" "}
            Enrichment requires a description, and{" "}
            {excluded.map((platform, index) => (
              <span key={platform.ats}>
                {index > 0 ? (index === excluded.length - 1 ? " and " : ", ") : ""}
                {titleCase(platform.ats)} ({exactCount(platform.openPostings)} open)
              </span>
            ))}{" "}
            carry none — 0% classified, every posting invisible here. Those adapters return
            listing-level fields only.
          </li>
        )}
        <li>
          <strong className="font-semibold text-content-primary">
            The classified set&rsquo;s company mix is not the corpus&rsquo;s.
          </strong>{" "}
          {bias.biggestCompany && (
            <>
              {bias.biggestCompany.name} is the largest employer in the corpus at{" "}
              {exactCount(bias.biggestCompany.openPostings)} open postings but only{" "}
              {percentLabel(
                share(bias.biggestCompany.classified, bias.biggestCompany.openPostings),
              )}{" "}
              classified
              {bias.mostDrained && (
                <>
                  , while {bias.mostDrained.name} is{" "}
                  {percentLabel(
                    share(bias.mostDrained.classified, bias.mostDrained.openPostings),
                  )}
                </>
              )}
              .{" "}
            </>
          )}
          {exactCount(bias.zeroClassifiedCompanies)} of{" "}
          {exactCount(bias.companiesWithOpen)} companies with open postings have none
          classified at all. Company is a strong predictor of title convention, so this
          reweighting moves the measure directly.
        </li>
        <li>
          <strong className="font-semibold text-content-primary">
            The unclassified {percentLabel(1 - share(coverage.classifiedPostings, coverage.openPostings))} is
            not empty, it is unseen.
          </strong>{" "}
          Every share below is computed over the classified subset only. Nothing here
          says how the other{" "}
          {exactCount(coverage.openPostings - coverage.classifiedPostings)} open postings
          are named, and their titles could move any bar in either direction.
        </li>
        <li>
          <strong className="font-semibold text-content-primary">
            The corpus is concentrated.
          </strong>{" "}
          {coverage.topCompanies.map((company, index) => (
            <span key={company.name}>
              {index > 0 ? (index === coverage.topCompanies.length - 1 ? " and " : ", ") : ""}
              {company.name} ({percentLabel(share(company.postings, coverage.openPostings))})
            </span>
          ))}{" "}
          are {percentLabel(concentration)} of open postings between them. This is
          frontier-AI and infrastructure naming practice, not the general market.
        </li>
        <li>
          <strong className="font-semibold text-content-primary">
            The denominator is role assignments, not postings.
          </strong>{" "}
          {exactCount(coverage.classifiedPostings)} classified postings carry{" "}
          {exactCount(coverage.roleAssignments)} role assignments, so a posting classified
          into two roles is counted once in each.
        </li>
        <li>
          Roles below the {floor}-posting floor are omitted rather than shown at zero — a
          modal share over four postings is not a measurement.
        </li>
      </ul>
    </section>
  );
}

/* -------------------------------------------------------------------------- */
/* The chart. Hand-rendered SVG on d3-scale; one hue, because modal share is a */
/* neutral magnitude. An improvised name is not a failure and a settled one is */
/* not a success, so neither status nor trend colour has any standing here.    */
/* -------------------------------------------------------------------------- */

function DivergenceChart({ rows }: { rows: readonly RoleHeadRow[] }) {
  const height = rows.length * ROW_HEIGHT + MARGIN.top + MARGIN.bottom;
  const width = MARGIN.left + PLOT_WIDTH + MARGIN.right;

  const xScale = scaleLinear().domain([0, 1]).range([0, PLOT_WIDTH]);
  const yScale = scaleBand<string>()
    .domain(rows.map((row) => row.role))
    .range([0, rows.length * ROW_HEIGHT])
    // Leaves ~19px of bar in a 30px band: under the 24px cap, and the leftover
    // is the surface gap rather than a stroke drawn around each mark.
    .paddingInner(0.36);

  const ticks = [0, 0.25, 0.5, 0.75, 1].map((value) => ({
    value,
    label: percentLabel(value),
  }));

  // The widest bar owns the only inside label; everything else labels outside,
  // so no value is ever clipped by its own mark.
  const widest = max(rows, (row) => row.modalShare) ?? 0;

  return (
    <figure className="mt-6">
      <div className="overflow-x-auto">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          width={width}
          height={height}
          role="img"
          aria-label="Canonical roles ranked by modal title-head share, ascending"
          className="h-auto max-w-full"
        >
          <g transform={`translate(${MARGIN.left}, ${MARGIN.top})`}>
            {ticks.map((tick) => (
              <line
                key={tick.label}
                x1={xScale(tick.value)}
                x2={xScale(tick.value)}
                y1={0}
                y2={rows.length * ROW_HEIGHT}
                className="stroke-edge-hairline"
                strokeWidth={1}
              />
            ))}

            {rows.map((row) => {
              const y = yScale(row.role);
              if (y === undefined) return null;

              const barHeight = yScale.bandwidth();
              const barWidth = xScale(row.modalShare);
              const centre = y + barHeight / 2;
              const labelInside = row.modalShare === widest && barWidth > 56;

              return (
                <g key={row.role}>
                  <text
                    x={-12}
                    y={centre}
                    textAnchor="end"
                    dominantBaseline="middle"
                    className="fill-content-primary text-xs"
                  >
                    {row.role}
                  </text>
                  <path
                    d={barPath(barWidth, barHeight)}
                    transform={`translate(0, ${y})`}
                    className="fill-series-1"
                  />

                  <text
                    x={labelInside ? barWidth - 8 : barWidth + 8}
                    y={centre}
                    textAnchor={labelInside ? "end" : "start"}
                    dominantBaseline="middle"
                    className={
                      labelInside
                        ? "fill-on-solid text-xs tabular-nums"
                        : "fill-content-primary text-xs tabular-nums"
                    }
                  >
                    {percentLabel(row.modalShare)}
                  </text>

                  <text
                    x={PLOT_WIDTH + 24}
                    y={centre}
                    dominantBaseline="middle"
                    className="fill-content-secondary text-xs"
                  >
                    {row.modalHead}
                    <tspan className="fill-content-muted">
                      {"  "}
                      {row.modalN}/{row.postings}
                    </tspan>
                    {row.topCompanyShare >= CONCENTRATION_FLAG && (
                      <tspan className="fill-content-muted">
                        {"  · "}
                        {percentLabel(row.topCompanyShare)} one employer
                      </tspan>
                    )}
                  </text>
                </g>
              );
            })}

            <g transform={`translate(0, ${rows.length * ROW_HEIGHT})`}>
              <Axis
                orientation="bottom"
                scale={(value: AxisValue) => xScale(value as number)}
                ticks={ticks}
                length={PLOT_WIDTH}
              />
            </g>
          </g>
        </svg>
      </div>
      <figcaption className="mt-3 max-w-[65ch] text-sm text-content-muted">
        Bar length is the modal head&rsquo;s share of the role. The count beside each head
        is modal postings over the role&rsquo;s total. A role flagged{" "}
        <span className="text-content-secondary">one employer</span> draws more than{" "}
        {percentLabel(CONCENTRATION_FLAG)} of its postings from a single company, so its
        share measures that company&rsquo;s naming habit and its board&rsquo;s per-location
        fan-out rather than the market&rsquo;s.
      </figcaption>
    </figure>
  );
}

/** Rounded at the data end, square at the baseline, so the zero stays a zero. */
function barPath(width: number, height: number): string {
  const radius = Math.min(4, height / 2, Math.max(width, 0) / 2);

  if (width <= 0) return "";
  if (radius <= 0) return `M0,0 H${width} V${height} H0 Z`;

  return [
    "M0,0",
    `H${width - radius}`,
    `A${radius},${radius} 0 0 1 ${width},${radius}`,
    `V${height - radius}`,
    `A${radius},${radius} 0 0 1 ${width - radius},${height}`,
    "H0",
    "Z",
  ].join(" ");
}

/* -------------------------------------------------------------------------- */
/* The table view. Every number the chart encodes, plus the two it does not:   */
/* how many distinct heads the role spreads over, and employer concentration.  */
/* -------------------------------------------------------------------------- */

function RoleTable({ rows }: { rows: readonly RoleHeadRow[] }) {
  return (
    <section className="mt-10">
      <h2 className="font-display text-xl tracking-tight text-content-primary">
        Every row, as numbers
      </h2>
      <div className="mt-4 overflow-x-auto">
        <table className="w-full min-w-[46rem] border-collapse text-sm">
          <thead>
            <tr className="border-b border-edge text-left text-content-muted">
              <th scope="col" className="py-2 pr-4 font-medium">
                Canonical role
              </th>
              <th scope="col" className="py-2 pr-4 text-right font-medium">
                Postings
              </th>
              <th scope="col" className="py-2 pr-4 text-right font-medium">
                Heads
              </th>
              <th scope="col" className="py-2 pr-4 font-medium">
                Modal head
              </th>
              <th scope="col" className="py-2 pr-4 text-right font-medium">
                Modal n
              </th>
              <th scope="col" className="py-2 pr-4 text-right font-medium">
                Modal share
              </th>
              <th scope="col" className="py-2 text-right font-medium">
                Largest employer
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.role} className="border-b border-edge-hairline">
                <td className="py-2 pr-4 text-content-primary">{row.role}</td>
                <td className="py-2 pr-4 text-right tabular-nums text-content-secondary">
                  {row.postings}
                </td>
                <td className="py-2 pr-4 text-right tabular-nums text-content-secondary">
                  {row.distinctHeads}
                </td>
                <td className="py-2 pr-4 text-content-secondary">{row.modalHead}</td>
                <td className="py-2 pr-4 text-right tabular-nums text-content-secondary">
                  {row.modalN}
                </td>
                <td className="py-2 pr-4 text-right tabular-nums text-content-primary">
                  {percentLabel(row.modalShare)}
                </td>
                <td className="py-2 text-right tabular-nums text-content-secondary">
                  {percentLabel(row.topCompanyShare)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/* -------------------------------------------------------------------------- */

function FloorNote({
  floor,
  rows,
  coverage,
}: {
  floor: number;
  rows: readonly RoleHeadRow[];
  coverage: Coverage;
}) {
  const link = (value: number) => `?min=${value}`;

  return (
    <section className="mt-12 max-w-[65ch] border-t border-edge pt-6 text-sm text-content-secondary">
      <h2 className="font-display text-lg tracking-tight text-content-primary">
        The floor is a judgment call
      </h2>
      <p className="mt-2">
        A role earns a bar at <strong className="font-semibold">{floor} postings</strong>.
        Nothing in the data puts the boundary there. At this floor{" "}
        {rows.length} of {coverage.distinctRoles} roles appear; the other{" "}
        {coverage.distinctRoles - rows.length} are omitted rather than drawn at a share
        their base-n cannot support.
      </p>
      <p className="mt-2 text-content-muted">
        Try{" "}
        {[10, 20, 30, 50].map((value, index) => (
          <span key={value}>
            {index > 0 ? " · " : ""}
            <a className="text-content-link underline" href={link(value)}>
              {value}
            </a>
          </span>
        ))}
      </p>

      <h2 className="mt-8 font-display text-lg tracking-tight text-content-primary">
        What this sketch found
      </h2>
      <ol className="mt-2 list-decimal space-y-2 pl-5">
        <li>
          Seniority normalisation is not a detail, it is the measure. Before stripping the
          prefix, Software Engineer and Product Engineer both read ~0.15 and looked like
          the most improvised names in the corpus. After, they read 0.57 and 0.46. Any
          spec has to pin the normalisation rule, because the ranking is built on it.
        </li>
        <li>
          The two highest shares are artifacts. Field Sales Representative (0.91) and
          Business Development Representative (0.85) are one employer fanning a single
          requisition across locations. Posting count overstates them exactly the way
          project.md says it does — this measure needs the requisition denominator, or
          the concentration flag has to stay visible beside every bar.
        </li>
        <li>
          The bottom of the ranking is the actual finding. Support Engineer (0.18, 15
          heads over 28 postings) and Solutions Engineer (0.24, 23 heads over 54) are
          roles the market is naming per-posting. Forward Deployed Engineer at 0.29 is a
          name being invented in public.
        </li>
        <li>
          The sample is biased against the question. Enrichment drains oldest-first and
          requires a description, so the classified subset is months behind the corpus and
          missing whole platforms and companies. For &ldquo;which names are still being
          invented&rdquo; that is the worst available sample, and it is a reason to
          re-run this measure after enrichment catches up rather than to spec it on these
          numbers.
        </li>
        <li>
          The grammar cannot express this. There is no title dimension and no conditional
          share, so the whole measure lives in raw SQL in <code>query.ts</code>. A{" "}
          <code>title_head</code> grouping plus a conditional share measure would make it
          a composition instead of a sketch — the same gap the skill-to-title prototype
          reported.
        </li>
      </ol>
    </section>
  );
}

function titleCase(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function joinNames(names: readonly string[]): string {
  if (names.length <= 1) return names[0] ?? "no platform";
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

function readFloor(params: Record<string, string | string[] | undefined>): number {
  const raw = params.min;
  const value = Number(Array.isArray(raw) ? raw[0] : raw);
  return Number.isFinite(value) && value >= 1 ? Math.floor(value) : DEFAULT_FLOOR;
}
