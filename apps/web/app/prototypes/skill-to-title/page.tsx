// Prototype — asked 2026-09-17.
//
// Question: invert the hierarchy. Skill at the top, job title at the leaf.
// What does one skill open, and how firmly is it required at each door?
//
// THE CENTRAL MOVE — replace lift with P(skill | title).
//
// The first three sketches all ranked by lift, and all three hit the same
// wall: lift is a ratio against an expectation, it is unbounded above, it is
// weakest exactly where the data is thickest, and every value in an
// anchor-best selection sits on one side of 1.0 so a diverging ramp is the
// wrong instrument. Lift also answers a question nobody asked out loud —
// "is this skill over-represented here" — when the question a person
// actually has is "if I apply for this job, will they want this?"
//
// That question is a conditional probability, and it is bounded, one-sided,
// and directly readable: Kubernetes appears in 88% of site-reliability-
// engineer postings. Figma in 70% of product-designer postings. No legend
// explains a percentage.
//
// Lift does not vanish — it becomes GEOMETRY instead of arithmetic. Each
// bar's zero is 0%, and a vertical reference line sits at the skill's
// corpus-wide base rate P(skill). A bar reaching past the line is a role
// that wants this skill more than the corpus does; the distance past it IS
// the lift, read off the same axis as the probability. One mark, both
// readings, no second colour channel.
//
// ONE AXIS, THREE ALTITUDES.
//
// The 0-100% axis is shared by all three levels of the drill, which is what
// makes the hierarchy legible rather than merely nested:
//
//   index    a word-sized strip per skill — one tick per role that demands
//            it, placed at that role's requirement rate, sized by base-N.
//            Ticks crowding right = a gate. Crowding left = ambient.
//   roles    the same distribution, exploded: one bar per role.
//   titles   each role's bar splits into its constituent job titles, on the
//            same axis, indented under their parent.
//
// The index glyph and the detail chart are THE SAME MARK at two scales, so
// drilling in is a zoom, not a context switch. That is the structural idea
// this sketch exists to test.
//
// WHAT IS A JOB TITLE HERE. The corpus stores raw posting titles, but they
// are unreachable: `lib/db/postings.ts` returns titles only for the open
// cohort with no taxonomy join, and the measure engine has no title
// grouping. So a title is RECONSTRUCTED from the taxonomy, and `?leaf=`
// picks which decomposition to test:
//   `?leaf=seniority`    (default) "Staff Infrastructure Engineer"
//   `?leaf=specialization`         "Software Engineer, Compute Infrastructure"
// Both are legitimate readings of "job title" and they answer different
// questions. Comparing them is half the point of the knob.
//
// Other knobs:
//   `?skill=kubernetes`  which skill is drilled. Default: the highest-grip
//                        skill above the floor.
//   `?drill=roles`       skip the leaf level. The leaf is the only expensive
//                        thing on the page — see finding 8.
//   `?min=25`            corpus floor for a skill to appear in the index.
//   `?floor=8`           base-N floor for a role or title to earn a bar.
//   `?strong=20`         base-N floor for a role to be allowed to define a
//                        skill's grip. Deliberately higher than `floor`.
//   `?grip=0.6&reach=20` the two thresholds that cut the index into four
//                        readings. Exposed precisely because they are
//                        arbitrary — see finding 3.
//   `?sort=grip|reach|n` index order.
//
// WHAT THIS SKETCH FOUND
//
// 1. The requirement rate climbs with seniority inside a single role, and
//    that gradient is the drill-down's actual payoff. Kubernetes across
//    infrastructure-engineer: mid 32% (7/22), senior 59% (26/44), staff 88%
//    (15/17). Across software-engineer: junior 13%, mid 21%, senior 31%,
//    principal 38%. Neither the skill-level number nor the role-level number
//    contains that shape — it exists only at the leaf, and it is the first
//    reading across these four sketches that a person could act on. It also
//    says the leaf is not decoration: a drill that stopped at the role would
//    have reported one number, 55%, for a skill that ranges 32→88 inside it.
//
//    And the SHAPE of that gradient separates kinds of skill, which is the
//    part worth chasing. Inside software-engineer, kubernetes climbs
//    13/21/31/27/38 across junior→principal while typescript stays flat at
//    29/24/29/23/38. One is a thing you are expected to grow into; the other
//    is a thing you either work in or do not. No aggregate over the role can
//    tell those apart, and nothing above the leaf even has the shape to try.
//
// 2. Two summary statistics separate skills into kinds the corpus genuinely
//    contains:
//      grip  = max P(skill | role) over roles above `strong` — how hard the
//              most-demanding real role insists.
//      reach = exp(H(P(role | skill))) — the effective number of roles the
//              skill spreads across. Perplexity rather than a raw count, so
//              40 roles with one dominant does not read as 40 even.
//    kubernetes   grip .88 reach 15 — a narrow gate
//    project mgmt grip .85 reach 65 — required hard, everywhere: a baseline
//    communication grip .35 reach 62 — named everywhere, required nowhere
//    figma        grip .70 reach  5 — narrow, and firmly required
//    A candidate reads those four rows completely differently. Lift ranks
//    them almost identically.
//
// 3. The four-way cut on those two numbers is thresholds on a continuum, and
//    nothing in the data puts a boundary at .6 or at 20. The labels are a
//    reading aid, not a finding. They are knobs so that moving one and
//    watching a skill change category costs a click — the honest way to show
//    a taxonomy the data does not support. A spec should ship the two
//    numbers and probably not the four words.
//
// 4. `share` cannot express any of this. It normalizes over the whole
//    result's assignment sum, so `share` grouped by [role, skill] yields
//    P(role AND skill) across all pairs — not P(skill | role). The grammar
//    has no conditional share: no way to name the grouping a proportion is
//    taken WITHIN. Every rate on this page is therefore computed in-page
//    from two counts, the same invariant leak the first three sketches
//    reported for lift. One conditional measure would close both.
//
// 5. The grammar has no drill. This page issues one composition for the
//    index and a second, filtered one for the selected skill, and nothing in
//    the vocabulary ties parent to child — the filter dimension happening to
//    match the parent's grouping is a convention this file holds, not a
//    contract the engine checks.
//
// 6. Role-titles-as-skills, already reported by the matrix sketch, is far
//    more visible here. `program-management` sits at 81% grip, and the role
//    it grips is technical-program-manager. That is not a requirement, it is
//    the job's name restated. Requirement-rate framing makes the tautology
//    obvious in a way lift did not: a skill cannot be 81% "required" by the
//    role it is a synonym for. Same for `financial-modeling`, `ux-design`,
//    `interaction-design` (reach 2).
//
// 7. `function` is unusable as a top-level cut. 24,423 assignments over
//    5,337 postings — 4.6 each — with `operations` on 4,989 of them. It is
//    not a partition of the corpus. Not used here; noted so the next sketch
//    does not try.
//
// 8. THE ENGINE CANNOT SERVE A DRILL-DOWN TODAY, and this is the finding to
//    carry forward. A drill needs P(skill | role, leaf), which has exactly
//    two spellings in the grammar, and both fail:
//
//      a. Filter the skill, group by [role, leaf]. Measured on the dev
//         read-only DSN: the same grouping unfiltered returns in 853ms;
//         filtered to one skill it takes 28,420ms, and the identical SQL
//         exceeds the MCP client's timeout outright. The cause is structural,
//         not a missing index — `GROUPING_SOURCES.skill.filter` emits a
//         correlated `EXISTS` against `posting_taxonomy`, which is a view
//         that re-derives the taxonomy, so the planner re-evaluates it per
//         candidate row. Every classified filter dimension is built this way,
//         so this is not about skills.
//
//      b. Cross all three at once — groupBy [role, skill, seniority], no
//         filter, which would serve every skill's drill from one result.
//         Times out with no rows. The matrix sketch reported a 150s two-way
//         crossing and blamed the requisition sub-count; this is `cohort:
//         "all"`, which carries no requisitions, so a three-way classified
//         crossing is independently unaffordable.
//
//    The page keeps (a) and shows its cost in the timing strip rather than
//    routing around it. `?drill=roles` omits the leaf and the page is
//    instant, which is the measurement: the leaf level IS the 28 seconds.
//
// 9. A maximum is not a robust statistic, and the base-N floor that is right
//    for drawing a bar is far too low for taking a max over. At `floor=8`,
//    four unrelated skills reported grip 1.00, each off a single 8-posting
//    role — a number that said nothing about any of them. Hence a separate,
//    higher `strong` floor, AND the role that produced the grip rendered
//    beside it. Any summary statistic this surface ships needs its source
//    visible; that is cheaper than making the statistic robust and it is
//    what makes a wrong one catchable on sight.
//
// 10. Engine-level, cheap to fix, independent of everything above:
//     `runCompositionWith` awaits its rows and then its denominator in
//     sequence. They are independent queries. A page at this altitude issues
//     five compositions, so it pays five avoidable round trips, and each
//     composition re-derives the cohort view from scratch — the `?drill=roles`
//     timings show three compositions costing ~1s each for work whose SQL
//     measures 300-400ms.
//
// 11. The bar's ramp fill is a redundant encoding — length already carries the
//     rate. Kept anyway, and `?ink=flat` is there to argue with: across ~60
//     stacked rows the dark/pale contrast is what makes a high-rate bar
//     findable without reading left to right, and Tufte's objection is to
//     junk, not to redundancy that shortens a search. Flip the knob and judge
//     it; a spec should not inherit this choice untested.
//
// Deliberately NOT here: time. The sparkline sketch established that a series
// over a classified dimension measures enrichment recency, not demand. Every
// number on this page is a proportion taken inside the classified slice,
// which is the one reading that stays honest while coverage is uneven — the
// coverage term cancels. Said in the header rather than assumed.
//
// Not production. See agent-context/lib/web-guide.md § Prototypes.

import type { Composition, Grouping } from "@/lib/composition";
import { runComposition } from "@/lib/db/measure-engine";

export const dynamic = "force-dynamic";

const AXIS_WIDTH = 420;
const STRIP_WIDTH = 150;
const STRIP_HEIGHT = 16;

// Ranked low to high so a title sorts into a ladder rather than alphabet.
// `unknown` is kept and placed last: a classification that declined to say is
// data, and dropping it would quietly shrink every role's denominator.
const SENIORITY_ORDER = [
  "intern",
  "junior",
  "mid",
  "senior",
  "staff",
  "lead",
  "principal",
  "director",
  "unknown",
] as const;

type Leaf = "seniority" | "specialization";

interface Knobs {
  readonly skill: string | undefined;
  readonly leaf: Leaf;
  readonly min: number;
  readonly floor: number;
  readonly grip: number;
  readonly reach: number;
  readonly strong: number;
  readonly drill: "roles" | "titles";
  readonly ink: "ramp" | "flat";
  readonly sort: "grip" | "reach" | "n";
}

interface IndexRow {
  readonly skill: string;
  readonly n: number;
  readonly grip: number;
  // Which role produces the grip. A max is not a robust statistic, so the
  // number is only trustworthy if its source is visible beside it — see
  // finding 9.
  readonly gripRole: string;
  readonly reach: number;
  readonly ticks: readonly { readonly rate: number; readonly n: number }[];
}

interface TitleRow {
  readonly label: string;
  // The raw leaf slug, kept alongside the composed label because the ladder
  // sorts seniority by rank and the label it renders no longer contains it.
  readonly leafValue: string;
  readonly rate: number;
  readonly n: number;
  readonly base: number;
}

interface RoleRow extends TitleRow {
  readonly titles: readonly TitleRow[];
}

function readKnobs(params: Record<string, string | string[] | undefined>): Knobs {
  const one = (key: string) => {
    const value = params[key];
    return Array.isArray(value) ? value[0] : value;
  };
  const num = (key: string, fallback: number, low: number, high: number) => {
    const parsed = Number.parseFloat(one(key) ?? "");
    return Number.isFinite(parsed) ? Math.min(Math.max(parsed, low), high) : fallback;
  };
  const sort = one("sort");

  return {
    skill: one("skill"),
    leaf: one("leaf") === "specialization" ? "specialization" : "seniority",
    min: num("min", 25, 5, 500),
    floor: num("floor", 8, 2, 100),
    grip: num("grip", 0.6, 0, 1),
    reach: num("reach", 20, 1, 300),
    strong: num("strong", 20, 2, 500),
    drill: one("drill") === "roles" ? "roles" : "titles",
    ink: one("ink") === "flat" ? "flat" : "ramp",
    sort: sort === "reach" || sort === "n" ? sort : "grip",
  };
}

// Every composition is `count` over `all`. Two reasons, both structural:
// `open` drags in the requisition sub-count, which turns a two-way classified
// crossing into a ~150s query (matrix sketch), and a proportion needs both
// halves drawn from the same cohort or the rate is a ratio of two populations.
function counting(groupBy: readonly Grouping[], filter?: Composition["filter"]): Composition {
  return {
    measure: "count",
    cohort: "all",
    groupBy,
    ...(filter != null && { filter }),
    encoding: "table",
  };
}

// Timings go on the page, not only to the dev console. A drill-down surface
// issues one composition per level and the cost compounds invisibly; a sketch
// that hides its own latency cannot report on it. See finding 8.
type Timing = { label: string; ms: number };

function runner(into: Timing[]) {
  return async function run(label: string, composition: Composition) {
    const started = Date.now();
    const result = await runComposition(composition);
    into.push({ label, ms: Date.now() - started });
    if (!result.ok) throw new Error(result.error.message);
    return result.value;
  };
}

// Slugs are lowercase alphanumeric-and-hyphen, so "/" cannot occur inside one
// and a joined key is unambiguous.
function titleKey(left: string | undefined, right: string | undefined): string {
  return `${left ?? ""}/${right ?? ""}`;
}

function humanize(slug: string): string {
  return slug.replace(/-/g, " ");
}

// Title Case, because the leaf is claiming to be a job title and a lowercase
// slug does not read as one. The whole point of the leaf row is that it looks
// like something a person could paste into a job search.
function titleCase(slug: string): string {
  return humanize(slug).replace(/\b[a-z]/g, (letter) => letter.toUpperCase());
}

// "Staff Infrastructure Engineer" / "Software Engineer, Compute Infrastructure".
// Two different claims about what a title is made of, which is the knob.
function composeTitle(role: string, leafValue: string, leaf: Leaf): string {
  if (leaf === "specialization") return `${titleCase(role)}, ${titleCase(leafValue)}`;
  if (leafValue === "unknown") return `${titleCase(role)} (level unstated)`;
  return `${titleCase(leafValue)} ${titleCase(role)}`;
}

function leafRank(leaf: Leaf, value: string): number {
  if (leaf !== "seniority") return 0;
  const index = SENIORITY_ORDER.indexOf(value as (typeof SENIORITY_ORDER)[number]);
  return index === -1 ? SENIORITY_ORDER.length : index;
}

// exp(Shannon entropy) over P(role | skill). A skill split evenly across ten
// roles scores 10; one concentrated in a single role with a long thin tail
// scores near 1. A raw distinct-role count cannot tell those apart, and the
// distinction is the whole difference between "portable" and "specialised".
function perplexity(counts: readonly number[]): number {
  const total = counts.reduce((sum, value) => sum + value, 0);
  if (total === 0) return 0;
  const entropy = counts.reduce((sum, value) => {
    const share = value / total;
    return share <= 0 ? sum : sum - share * Math.log(share);
  }, 0);
  return Math.exp(entropy);
}

// Seven buckets, sequential. Requirement rate is a magnitude with a floor and
// a ceiling and no meaningful midpoint, so it takes `ramp-*`. The diverging
// family would have to invent a middle, which is exactly the mistake the
// matrix sketch made with lift.
const RAMP = [
  "fill-ramp-1",
  "fill-ramp-2",
  "fill-ramp-3",
  "fill-ramp-4",
  "fill-ramp-5",
  "fill-ramp-6",
  "fill-ramp-7",
] as const;

function rampClass(rate: number, ink: Knobs["ink"]): string {
  if (ink === "flat") return "fill-observed";
  return RAMP[Math.min(6, Math.max(0, Math.floor(rate * 7)))];
}

function reading(row: IndexRow, knobs: Knobs): string {
  const required = row.grip >= knobs.grip;
  const broad = row.reach >= knobs.reach;
  if (required && broad) return "baseline";
  if (required) return "gate";
  if (broad) return "ambient";
  return "edge";
}

export default async function SkillToTitle({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const knobs = readKnobs(await searchParams);
  const timings: Timing[] = [];
  const run = runner(timings);
  const started = Date.now();

  const [pairs, skillTotals, roleTotals] = await Promise.all([
    run("pairs", counting(["role", "skill"])),
    run("skillTotals", counting(["skill"])),
    run("roleTotals", counting(["role"])),
  ]);

  const population = skillTotals.denominator?.classified ?? 0;
  const corpus = skillTotals.denominator?.total ?? 0;
  const skillN = new Map(skillTotals.rows.map((row) => [row.keys.skill ?? "", row.value]));
  const roleN = new Map(roleTotals.rows.map((row) => [row.keys.role ?? "", row.value]));

  // One pass over every (role, skill) pair builds the whole index: the tick
  // distribution, grip, and the counts perplexity needs.
  const bySkill = new Map<
    string,
    { ticks: { rate: number; n: number }[]; roleCounts: number[]; grip: { rate: number; role: string } }
  >();
  for (const row of pairs.rows) {
    const skill = row.keys.skill ?? "";
    const role = row.keys.role ?? "";
    const base = roleN.get(role) ?? 0;
    if (base === 0) continue;

    const entry = bySkill.get(skill) ?? { ticks: [], roleCounts: [], grip: { rate: 0, role: "" } };
    entry.roleCounts.push(row.value);
    // A rate over three postings is noise, not a requirement. Excluded from
    // the strip; still counted toward reach, where thin roles are the tail
    // that perplexity exists to weigh correctly.
    const rate = row.value / base;
    if (base >= knobs.floor) entry.ticks.push({ rate, n: row.value });
    // Grip takes a HIGHER floor than a bar does, and the two floors being
    // different is itself the finding: a maximum over thin roles is dominated
    // by them. At the bar floor of 8, four separate skills reported grip 1.00
    // off a single 8-posting role, which said nothing about any of them.
    if (base >= knobs.strong && rate > entry.grip.rate) entry.grip = { rate, role };
    bySkill.set(skill, entry);
  }

  const index: IndexRow[] = [...bySkill.entries()]
    .map(([skill, entry]) => ({
      skill,
      n: skillN.get(skill) ?? 0,
      grip: entry.grip.rate,
      gripRole: entry.grip.role,
      reach: perplexity(entry.roleCounts),
      ticks: entry.ticks.sort((a, b) => a.rate - b.rate),
    }))
    .filter((row) => row.n >= knobs.min && row.ticks.length > 0)
    .sort((a, b) =>
      knobs.sort === "n" ? b.n - a.n : knobs.sort === "reach" ? b.reach - a.reach : b.grip - a.grip,
    );

  const selected = index.find((row) => row.skill === knobs.skill)?.skill ?? index[0]?.skill;

  // The drill. Two more compositions, both filtered to the selected skill —
  // the numerator — against the unfiltered role×leaf counts that are its
  // denominator. Nothing in the grammar states that relationship; see note 5.
  const leaf: Grouping = knobs.leaf;
  const [titleTotals, titleWithSkill] =
    knobs.drill === "roles" || selected == null
      ? [undefined, undefined]
      : await Promise.all([
          run("titleTotals", counting(["role", leaf])),
          run("titleWithSkill", counting(["role", leaf], [{ dim: "skill", value: selected }])),
        ]);

  const titleN = new Map<string, number>();
  for (const row of titleTotals?.rows ?? []) {
    titleN.set(titleKey(row.keys.role, row.keys[leaf]), row.value);
  }

  const roleBuckets = new Map<string, TitleRow[]>();
  for (const row of titleWithSkill?.rows ?? []) {
    const role = row.keys.role ?? "";
    const leafValue = row.keys[leaf] ?? "";
    const base = titleN.get(titleKey(role, leafValue)) ?? 0;
    if (base < knobs.floor) continue;

    const list = roleBuckets.get(role) ?? [];
    list.push({
      label: composeTitle(role, leafValue, knobs.leaf),
      leafValue,
      rate: row.value / base,
      n: row.value,
      base,
    });
    roleBuckets.set(role, list);
  }

  // Keyed lookup of the unfiltered pair counts the index query already holds.
  // A role's own rate is not the sum of its titles' — a posting classified for
  // the role but not for the leaf dimension belongs to one and not the other,
  // so rolling the leaf rates up would quietly drop it.
  const pairN = new Map<string, number>();
  for (const row of pairs.rows) {
    pairN.set(titleKey(row.keys.role, row.keys.skill), row.value);
  }

  const selectedN = selected == null ? 0 : (skillN.get(selected) ?? 0);
  const baseRate = population === 0 ? 0 : selectedN / population;

  // With the leaf off, the ladder's roles come from the pair crossing already
  // in hand; with it on, from the filtered query, which also supplies titles.
  // The two lists are not identical, and the difference is honest rather than
  // a bug: a role whose postings are spread so thinly across levels that no
  // single title clears `floor` has no leaf to show, so it drops out of the
  // title view while remaining in the roles view. Toggling `?drill=` is
  // therefore also a way to see which roles the leaf decomposition cannot
  // resolve.
  const ladderRoles =
    knobs.drill === "titles"
      ? [...roleBuckets.keys()]
      : pairs.rows.filter((row) => row.keys.skill === selected).map((row) => row.keys.role ?? "");

  const ladder: RoleRow[] = ladderRoles
    .map((role) => {
      const titles = roleBuckets.get(role) ?? [];
      const base = roleN.get(role) ?? 0;
      // Both halves of the role row come from the same pair. Summing the
      // titles' numerators against the role's denominator mixes two
      // populations — it rendered "18/25" beside a bar drawn at 100%.
      const n = pairN.get(titleKey(role, selected)) ?? 0;
      return {
        label: titleCase(role),
        leafValue: "",
        rate: base === 0 ? 0 : n / base,
        n,
        base,
        titles: titles.sort(
          (a, b) =>
            leafRank(knobs.leaf, a.leafValue) - leafRank(knobs.leaf, b.leafValue) ||
            b.rate - a.rate,
        ),
      };
    })
    .filter((row) => row.base >= knobs.floor)
    .sort((a, b) => b.rate - a.rate)
    .slice(0, 14);

  const link = (patch: Record<string, string | number>) => {
    const query = new URLSearchParams({
      ...(selected != null && { skill: selected }),
      leaf: knobs.leaf,
      drill: knobs.drill,
      ink: knobs.ink,
      strong: String(knobs.strong),
      min: String(knobs.min),
      floor: String(knobs.floor),
      grip: String(knobs.grip),
      reach: String(knobs.reach),
      sort: knobs.sort,
      ...Object.fromEntries(Object.entries(patch).map(([key, value]) => [key, String(value)])),
    });
    return `/prototypes/skill-to-title?${query.toString()}`;
  };

  const x = (rate: number) => rate * AXIS_WIDTH;
  const selectedRow = index.find((row) => row.skill === selected);

  return (
    <main className="min-h-screen bg-surface-page p-8 text-content-primary">
      <header className="mb-6 max-w-content">
        <h1 className="text-2xl font-semibold">What does a skill open?</h1>
        <p className="mt-2 text-sm text-content-secondary">
          Every number here is a share of postings that demand the skill — never a count of
          postings and never a trend. A proportion taken inside the classified slice cancels the
          coverage term, which is the one reading that survives uneven enrichment.
        </p>
        <p className="mt-1 text-xs text-content-muted">
          {population.toLocaleString()} of {corpus.toLocaleString()} seen postings carry a
          classification. Skills under {knobs.min} postings and titles under {knobs.floor}{" "}
          are omitted. Titles are reconstructed from the taxonomy — the corpus&rsquo;s raw posting
          titles are not reachable from the measure engine.
        </p>
      </header>

      <p className="mb-6 text-xs text-content-muted">
        {timings.map((timing) => `${timing.label} ${timing.ms}ms`).join(" · ")} · total{" "}
        {Date.now() - started}ms over {timings.length} compositions.{" "}
        {knobs.drill === "titles"
          ? "titleWithSkill is the drill, and it is the whole cost — a filter on a classified dimension is a correlated EXISTS over a view. See finding 8; ?drill=roles omits it."
          : "The leaf level is off, so no filtered composition ran. ?drill=titles pays ~30s for it."}
      </p>

      <nav className="mb-8 flex flex-wrap gap-x-8 gap-y-2 text-sm">
        <span className="flex gap-2">
          <span className="text-content-muted">title is</span>
          <a className={knobs.leaf === "seniority" ? "underline" : "text-content-secondary underline"} href={link({ leaf: "seniority" })}>
            level + role
          </a>
          <a className={knobs.leaf === "specialization" ? "underline" : "text-content-secondary underline"} href={link({ leaf: "specialization" })}>
            role + specialization
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">drill</span>
          <a className="underline" href={link({ drill: "titles" })}>
            to titles
          </a>
          <a className="underline" href={link({ drill: "roles" })}>
            roles only (fast)
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">bar ink</span>
          <a className="underline" href={link({ ink: "ramp" })}>
            ramp
          </a>
          <a className="underline" href={link({ ink: "flat" })}>
            flat
          </a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">index order</span>
          <a className="underline" href={link({ sort: "grip" })}>grip</a>
          <a className="underline" href={link({ sort: "reach" })}>reach</a>
          <a className="underline" href={link({ sort: "n" })}>volume</a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">skill floor</span>
          <a className="underline" href={link({ min: 10 })}>10</a>
          <a className="underline" href={link({ min: 25 })}>25</a>
          <a className="underline" href={link({ min: 80 })}>80</a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">title floor</span>
          <a className="underline" href={link({ floor: 5 })}>5</a>
          <a className="underline" href={link({ floor: 8 })}>8</a>
          <a className="underline" href={link({ floor: 20 })}>20</a>
        </span>
      </nav>

      <div className="flex flex-wrap items-start gap-12">
        <section className="min-w-0">
          <h2 className="mb-1 text-sm font-semibold">Skills</h2>
          <p className="mb-3 max-w-[30rem] text-xs text-content-muted">
            One tick per role, placed at the share of that role&rsquo;s postings demanding the
            skill, sized by how many. Ticks crowding right is a skill a job insists on; crowding
            left is one a job merely mentions.
          </p>
          <table className="text-sm">
            <thead>
              <tr className="text-xs text-content-muted">
                <th className="pr-4 pb-2 text-left font-normal">skill</th>
                <th className="pr-4 pb-2 text-right font-normal">n</th>
                <th className="pr-4 pb-2 text-left font-normal">0% → 100% of a role</th>
                <th className="pr-3 pb-2 text-right font-normal">grip</th>
                <th className="pr-4 pb-2 text-left font-normal">strongest in</th>
                <th className="pr-4 pb-2 text-right font-normal">reach</th>
                <th className="pb-2 text-left font-normal">reads as</th>
              </tr>
            </thead>
            <tbody>
              {index.slice(0, 44).map((row) => (
                <tr
                  key={row.skill}
                  className={
                    row.skill === selected
                      ? "bg-surface-row-selected align-middle"
                      : "align-middle hover:bg-surface-row-hover"
                  }
                >
                  <td className="py-0.5 pr-4 whitespace-nowrap">
                    <a className="underline" href={link({ skill: row.skill })}>
                      {humanize(row.skill)}
                    </a>
                  </td>
                  <td className="py-0.5 pr-4 text-right tabular-nums text-content-secondary">
                    {row.n}
                  </td>
                  <td className="py-0.5 pr-4">
                    <svg
                      width={STRIP_WIDTH}
                      height={STRIP_HEIGHT}
                      viewBox={`0 0 ${STRIP_WIDTH} ${STRIP_HEIGHT}`}
                      role="img"
                      aria-label={`${humanize(row.skill)} demanded by ${row.ticks.length} roles, strongest ${Math.round(row.grip * 100)} percent`}
                    >
                      <line
                        x1={0}
                        x2={STRIP_WIDTH}
                        y1={STRIP_HEIGHT - 1}
                        y2={STRIP_HEIGHT - 1}
                        strokeWidth={1}
                        className="stroke-edge-hairline"
                      />
                      {row.ticks.map((tick, position) => (
                        <circle
                          key={position}
                          cx={Math.min(STRIP_WIDTH - 1.5, Math.max(1.5, tick.rate * STRIP_WIDTH))}
                          cy={STRIP_HEIGHT / 2 - 1}
                          r={Math.min(3.2, 0.9 + Math.sqrt(tick.n) / 4)}
                          fillOpacity={0.55}
                          className="fill-observed"
                        />
                      ))}
                    </svg>
                  </td>
                  <td className="py-0.5 pr-3 text-right tabular-nums text-content-secondary">
                    {row.grip.toFixed(2)}
                  </td>
                  <td className="max-w-[11rem] truncate py-0.5 pr-4 text-content-muted">
                    {humanize(row.gripRole)}
                  </td>
                  <td className="py-0.5 pr-4 text-right tabular-nums text-content-secondary">
                    {row.reach.toFixed(0)}
                  </td>
                  <td className="py-0.5 whitespace-nowrap text-content-muted">
                    {reading(row, knobs)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="mt-3 max-w-[30rem] text-xs text-content-muted">
            grip is the strongest single role&rsquo;s share; reach is the effective number of
            roles, exp(H) rather than a raw count. The four readings cut those two at{" "}
            {knobs.grip} and {knobs.reach} — thresholds on a continuum, not boundaries the data
            contains. Move them with <code>?grip=</code> and <code>?reach=</code> and watch a
            skill change category; that is the honest disclosure. Grip only counts roles with{" "}
            {knobs.strong}+ postings (<code>?strong=</code>) and names the role it came from,
            because a maximum is not a robust statistic.
          </p>
        </section>

        <section className="min-w-0 flex-1">
          <h2 className="mb-1 text-sm font-semibold">
            {selected == null ? "No skill" : humanize(selected)}
          </h2>
          <p className="mb-4 max-w-[38rem] text-xs text-content-muted">
            {selectedN.toLocaleString()} classified postings name it —{" "}
            {Math.round(baseRate * 100)}% of the corpus, marked by the dashed line. A bar past
            that line is a job that wants this skill more than the market at large; the distance
            past it is the lift, read off the same axis instead of a second scale.
            {selectedRow != null && (
              <>
                {" "}
                grip {selectedRow.grip.toFixed(2)} · reach {selectedRow.reach.toFixed(0)} · reads
                as {reading(selectedRow, knobs)}.
              </>
            )}
          </p>

          <div className="mb-2 flex text-xs text-content-muted">
            <span className="w-[22rem]" />
            <svg width={AXIS_WIDTH} height={14} viewBox={`0 0 ${AXIS_WIDTH} 14`} aria-hidden>
              {[0, 0.25, 0.5, 0.75, 1].map((mark) => (
                <text
                  key={mark}
                  x={x(mark)}
                  y={10}
                  textAnchor={mark === 0 ? "start" : mark === 1 ? "end" : "middle"}
                  className="fill-content-muted"
                  fontSize={10}
                >
                  {mark * 100}%
                </text>
              ))}
            </svg>
          </div>

          {ladder.map((role) => (
            <div key={role.label} className="mb-3">
              <Bar
                label={role.label}
                rate={role.rate}
                n={role.n}
                base={role.base}
                baseRate={baseRate}
                ink={knobs.ink}
                emphasis
              />
              {role.titles.map((title) => (
                <Bar
                  key={title.label}
                  label={title.label}
                  rate={title.rate}
                  n={title.n}
                  base={title.base}
                  baseRate={baseRate}
                  ink={knobs.ink}
                />
              ))}
            </div>
          ))}
          {ladder.length === 0 && (
            <p className="text-sm text-content-muted">
              No role clears the {knobs.floor}-posting floor for this skill.
            </p>
          )}
        </section>
      </div>
    </main>
  );
}

// Bar height carries base-N, which is the finding the matrix and the small
// multiples both confirmed: a thin mark goes visually quiet without being
// hidden, so a 3-of-4 rate cannot shout as loudly as a 70-of-226.
function Bar({
  label,
  rate,
  n,
  base,
  baseRate,
  ink,
  emphasis = false,
}: {
  label: string;
  rate: number;
  n: number;
  base: number;
  baseRate: number;
  ink: Knobs["ink"];
  emphasis?: boolean;
}) {
  const height = emphasis ? 14 : Math.min(11, 4 + Math.sqrt(base));
  const rowHeight = emphasis ? 18 : 15;

  return (
    <div className="flex items-center hover:bg-surface-row-hover">
      <span
        className={`w-[22rem] truncate pr-4 text-right text-xs ${
          emphasis ? "font-medium text-content-primary" : "text-content-secondary"
        }`}
      >
        {label}
      </span>
      <svg
        width={AXIS_WIDTH}
        height={rowHeight}
        viewBox={`0 0 ${AXIS_WIDTH} ${rowHeight}`}
        role="img"
        aria-label={`${label}: ${Math.round(rate * 100)} percent of ${base} postings`}
      >
        <rect
          x={0}
          y={(rowHeight - height) / 2}
          width={Math.max(1, rate * AXIS_WIDTH)}
          height={height}
          className={rampClass(rate, ink)}
        />
        <line
          x1={baseRate * AXIS_WIDTH}
          x2={baseRate * AXIS_WIDTH}
          y1={0}
          y2={rowHeight}
          strokeWidth={1}
          strokeDasharray="2 2"
          className="stroke-edge-strong"
        />
      </svg>
      <span className="pl-3 text-xs tabular-nums text-content-secondary">
        {Math.round(rate * 100)}%
      </span>
      <span className="pl-2 text-xs tabular-nums text-content-muted">
        {n}/{base}
      </span>
    </div>
  );
}
