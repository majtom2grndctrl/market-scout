// Prototype — asked 2026-09-17.
//
// Question: given a profile — past titles, claimed skills, a stated goal —
// which skills should this screen put in front of THIS person, and in what
// order? `skill-to-title` ranked skills for nobody in particular. A corpus
// has no priorities; a person does.
//
// Profiles are mocked in `../_personas.ts`. Market Scout has no profile
// feature; this sketch assumes one and four filled-out examples.
//
// THE MEASURE — weighted coverage, not a match count.
//
// For a target role R, its demand is the set of skills appearing in at least
// `?demand=` of R's postings, each weighted by its requirement rate:
//
//   fit(R) = Σ p(s|R) for s you have and R demands
//            ─────────────────────────────────────
//            Σ p(s|R) for everything R demands
//
// Weighting by rate is the whole point. Matching python at 0.91 of
// data-scientist postings is not the same event as matching `r` at 0.19, and
// a count of matched skills says it is. It also makes the score robust to
// profile length: listing twenty niche skills cannot inflate it, because the
// denominator is the role's demand and not the person's inventory.
//
// That reasoning is half right and finding 1 is where it breaks: weighting
// fixes comparability WITHIN a target and does nothing for comparability
// ACROSS targets, because the denominator still grows with however many skills
// the classifier happened to name for that role. The top-K column is the
// repair; both numbers are shown so the disagreement stays visible.
//
// THE DISCLOSURE, which is the reason this sketch exists.
//
// `grip` — the statistic `skill-to-title` used for skills — runs in both
// directions, and nobody noticed until a profile was pointed at it. Applied
// to a ROLE it asks how hard that role insists on anything at all:
//
//   data-scientist    grip 0.91  (python)              — has a gate
//   product-designer  grip 0.70  (figma)               — has a gate
//   software-engineer grip 0.42  (python)              — soft
//   product-manager   grip 0.38  (stakeholder-mgmt)    — demands nothing
//   general-application grip 0.06                      — degenerate
//
// product-manager spans 203 postings and only FOUR skills clear 25%. A
// coverage score against it is nearly meaningless: it cannot be prepared for,
// so a high number mostly reports that the scorer's skills are common. The
// screen therefore never shows a fit score without the target's grip beside
// it, and says so in words below the bar rather than in a legend.
//
// This is the difference between a tool and a horoscope, and it is entirely
// invisible without personas — which is what the personas were for.
//
// Knobs:
//   `?persona=maya|dev|priya|tom`
//   `?target=product-engineer`  override the persona's own goal and explore
//   `?demand=0.1`               rate at which a role counts as demanding a skill
//   `?floor=25`                 min postings for a role to be rankable
//   `?core=5`                   how many of a role's heaviest asks are "core"
//   `?show=all`                 include roles the persona already holds
//
// WHAT THIS SKETCH FOUND
//
// 1. WEIGHTED COVERAGE IS NOT COMPARABLE ACROSS TARGETS, and the personas
//    caught it within a minute of being pointed at the screen. The prediction
//    written into `_personas.ts` was that Tom would score deceptively HIGH
//    against an undefined target. He scores 27%. Maya scores 29%. Those two
//    numbers are indistinguishable and describe opposite situations:
//
//      Maya   29%  holds product-engineer's top TWO of twenty (typescript
//                  0.65, react 0.50). Genuinely close; the score is dragged
//                  down by an eighteen-skill tail she will never be asked
//                  about in full.
//      Tom    27%  holds one of product-manager's thirteen, and the twelve he
//                  lacks ARE the job (product-thinking, product-management,
//                  product-judgment).
//      Priya  60%  holds data-scientist's top two of eleven — the same
//                  achievement as Maya's, scored at double, purely because
//                  data-scientist's tail is shorter.
//
//    The denominator is the role's total demand mass, so the score moves with
//    how verbose a role's classification happens to be — an artefact of the
//    classifier, not of the person. Fixed here by reporting rank alongside
//    mass: "2 of the top 5" is the same claim whether the role names ten more
//    skills or thirty. `?core=` sets the K, because 5 is a guess.
//
//    Worth stating plainly: the original measure was wrong, the personas are
//    what made it wrong out loud, and no corpus-wide sketch would have
//    surfaced it — there was nothing to be wrong ABOUT until a profile
//    existed. That is the argument for personas as a working tool and not
//    just as product framing.
//
// 1b. grip still earns its column, just not for the reason predicted. It does
//    not separate Priya from Tom by score — their scores differ anyway. It
//    separates what the scores MEAN: Priya's gap list is one real thing
//    (statistical-analysis, 55%), Tom's target has no requirement above 38%,
//    so there is nothing he can learn that would demonstrably qualify him.
//    A screen that printed 27% and 60% without that sentence would send Tom
//    to study a curriculum that does not exist.
//
// 1c. The trap DID fire — in the recommendation list rather than at the
//    stated target, which is worse, because that list is the part a person
//    would act on. Ranked by weighted coverage, Tom's third-best suggestion
//    is operations-associate: 63%, grip 0.32, a role that insists on nothing.
//    Ranked fifth-from-top is strategy-operations-lead at 47% — but 4 of its
//    top 5 held, and grip 0.66. The lower-scoring role is the better move by
//    every reading that survives scrutiny, and fit alone inverts them.
//    The sort is deliberately LEFT on weighted coverage so the inversion is
//    visible on the page rather than quietly corrected; a spec should
//    probably rank on core-held with grip as a gate, and that is a claim to
//    test, not to assume.
//
// 2. "What stops counting" turned out to matter more than the gap list. Maya
//    carries 2.49 of weighted demand at product-designer that is worth
//    essentially nothing at product-engineer — four skills, her whole craft.
//    A gap list alone says "learn full-stack-engineering"; it does not say
//    "and four things you are best at stop being asked about." The second
//    sentence is the one that changes a decision, and no corpus-wide view of
//    skills can produce it because it needs a FROM as well as a TO.
//
// 3. A profile makes the taxonomy's collisions expensive rather than
//    cosmetic. `design-engineer` in this corpus is mostly a MECHANICAL role
//    (gd-and-t 0.35, cad-software 0.29, materials-engineering 0.24) with web
//    skills mixed in (typescript 0.24, react 0.24) — two unrelated jobs under
//    one slug. Maya ranks against it on her React and would be told a
//    mechanical-engineering job fits her. Corpus-wide sketches could file
//    taxonomy hygiene as tidy-up; pointed at a person it produces a wrong
//    answer with a confident number on it.
//
// 4. The role-title-as-skill problem gets worse here, not better. With
//    `?show=all`, Tom's top two are the jobs he already has — program-manager
//    79% and technical-program-manager 68% — inflated by `program-management`
//    and `project-management`, which are his current job titles restated as
//    qualifications for it. "Roles you already fit" is therefore unusable as a
//    ranking until aliasing lands, and `show=new` (the default) hides the
//    symptom without fixing the cause: the same tautological skills still
//    inflate every ADJACENT role that shares them.
//
// 5. The level in a goal is not applied, and cannot be. Dev wants
//    infrastructure-engineer at STAFF, and staff wants different things than
//    mid does (kubernetes 32/59/88 across mid/senior/staff). Answering that
//    needs P(skill | role, seniority) — the three-way classified crossing that
//    times out, and the filtered path that costs 28s per skill. So every gap
//    on this page is role-level, disclosed in one line under the target
//    rather than silently. A profile-driven product cannot ship this way: a
//    goal without a level is barely a goal.
//
// 6. Cheap and worth saying: this page needs no filter and no crossing beyond
//    the [role, skill] pair, so it runs in ~2s where the title drill runs in
//    ~30. Everything a profile actually needs is available today. It is the
//    LEVEL, not the profile, that is blocked.
//
// Not production. See agent-context/lib/web-guide.md § Prototypes.

import type { Composition, Grouping } from "@/lib/composition";
import { runComposition } from "@/lib/db/measure-engine";

import { PERSONAS, PROFILE_CAVEATS, personaById, titleOf } from "../_personas";

export const dynamic = "force-dynamic";

const AXIS_WIDTH = 300;

// How many of a role's heaviest requirements count as its core. Five is a
// guess, exposed as `?core=` so it can be argued with rather than defended.
const DEFAULT_CORE = 5;

interface Knobs {
  readonly persona: string | undefined;
  readonly target: string | undefined;
  readonly demand: number;
  readonly floor: number;
  readonly core: number;
  readonly show: "new" | "all";
}

interface FitRow {
  readonly role: string;
  readonly base: number;
  readonly fit: number;
  readonly matched: number;
  readonly demanded: number;
  // Of the role's TOP_K heaviest requirements, how many are held. Rank-based
  // rather than mass-based, and comparable across targets in a way the
  // weighted score is not — see finding 1.
  readonly core: number;
  readonly grip: number;
  readonly gripSkill: string;
}

interface GapRow {
  readonly skill: string;
  readonly rate: number;
  readonly held: boolean;
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

  return {
    persona: one("persona"),
    target: one("target"),
    demand: num("demand", 0.1, 0.01, 0.9),
    floor: num("floor", 25, 5, 500),
    core: num("core", DEFAULT_CORE, 1, 20),
    show: one("show") === "all" ? "all" : "new",
  };
}

// `count` over `all`, same as every other sketch: `open` drags in the
// requisition sub-count that makes a classified crossing unusable, and a
// proportion needs both halves drawn from one cohort.
function counting(groupBy: readonly Grouping[]): Composition {
  return { measure: "count", cohort: "all", groupBy, encoding: "table" };
}

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

function humanize(slug: string): string {
  return slug.replace(/-/g, " ");
}

// One-sided magnitude with a floor and a ceiling and no meaningful midpoint,
// so it takes the sequential ramp. Same reasoning as `skill-to-title`.
const RAMP = [
  "fill-ramp-1",
  "fill-ramp-2",
  "fill-ramp-3",
  "fill-ramp-4",
  "fill-ramp-5",
  "fill-ramp-6",
  "fill-ramp-7",
] as const;

function rampClass(rate: number): string {
  return RAMP[Math.min(6, Math.max(0, Math.floor(rate * 7)))];
}

// The sentence under a fit score. Deliberately prose and deliberately not a
// legend: the caveat has to arrive at the same moment as the number, and a
// reader who has to look up what "grip 0.38" means will not look it up.
function gripReading(grip: number, gripSkill: string, base: number): string {
  if (grip >= 0.6) {
    return `This role has a hard requirement — ${humanize(gripSkill)} appears in ${Math.round(grip * 100)}% of its postings. A gap here is a real gap.`;
  }
  if (grip >= 0.45) {
    return `Moderately defined. Its strongest requirement, ${humanize(gripSkill)}, reaches ${Math.round(grip * 100)}% of postings — enough to prepare for, not enough to gate on.`;
  }
  return `This target demands nothing in particular: across ${base} postings its strongest requirement, ${humanize(gripSkill)}, reaches only ${Math.round(grip * 100)}%. A high coverage here mostly says your skills are common — it is not evidence you are ready.`;
}

export default async function PersonaSkillGap({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const knobs = readKnobs(await searchParams);
  const persona = personaById(knobs.persona);
  const timings: Timing[] = [];
  const run = runner(timings);
  const started = Date.now();

  const [pairs, roleTotals, skillTotals] = await Promise.all([
    run("pairs", counting(["role", "skill"])),
    run("roleTotals", counting(["role"])),
    run("skillTotals", counting(["skill"])),
  ]);

  const roleN = new Map(roleTotals.rows.map((row) => [row.keys.role ?? "", row.value]));
  const population = skillTotals.denominator?.classified ?? 0;
  const corpus = skillTotals.denominator?.total ?? 0;

  // P(skill | role) for every pair, which is everything this page needs. No
  // filter, no third grouping — see finding 6.
  const demandByRole = new Map<string, { skill: string; rate: number }[]>();
  for (const row of pairs.rows) {
    const role = row.keys.role ?? "";
    const base = roleN.get(role) ?? 0;
    if (base < knobs.floor) continue;

    const list = demandByRole.get(role) ?? [];
    list.push({ skill: row.keys.skill ?? "", rate: row.value / base });
    demandByRole.set(role, list);
  }

  const have = new Set(persona.have);
  const heldRoles = new Set(persona.past.map((entry) => entry.role));
  const target = knobs.target ?? persona.target.role;

  const fits: FitRow[] = [...demandByRole.entries()]
    .map(([role, demand]) => {
      const relevant = demand
        .filter((entry) => entry.rate >= knobs.demand)
        .sort((a, b) => b.rate - a.rate);
      const total = relevant.reduce((sum, entry) => sum + entry.rate, 0);
      const covered = relevant
        .filter((entry) => have.has(entry.skill))
        .reduce((sum, entry) => sum + entry.rate, 0);
      // Grip is taken over the role's FULL demand, not the floored set: the
      // question "does this role insist on anything" must not be answerable by
      // moving the floor.
      const strongest = demand.reduce(
        (best, entry) => (entry.rate > best.rate ? entry : best),
        { skill: "", rate: 0 },
      );

      return {
        role,
        base: roleN.get(role) ?? 0,
        fit: total === 0 ? 0 : covered / total,
        matched: relevant.filter((entry) => have.has(entry.skill)).length,
        demanded: relevant.length,
        core: relevant.slice(0, knobs.core).filter((entry) => have.has(entry.skill)).length,
        grip: strongest.rate,
        gripSkill: strongest.skill,
      };
    })
    .filter((row) => row.demanded > 0 && (knobs.show === "all" || !heldRoles.has(row.role)))
    .sort((a, b) => b.fit - a.fit)
    .slice(0, 12);

  const targetDemand = (demandByRole.get(target) ?? [])
    .filter((entry) => entry.rate >= knobs.demand)
    .sort((a, b) => b.rate - a.rate);
  const targetFit = fits.find((row) => row.role === target);
  const targetBase = roleN.get(target) ?? 0;
  const targetStrongest = (demandByRole.get(target) ?? []).reduce(
    (best, entry) => (entry.rate > best.rate ? entry : best),
    { skill: "", rate: 0 },
  );
  const targetTotal = targetDemand.reduce((sum, entry) => sum + entry.rate, 0);
  const targetCovered = targetDemand
    .filter((entry) => have.has(entry.skill))
    .reduce((sum, entry) => sum + entry.rate, 0);

  const gap: GapRow[] = targetDemand.map((entry) => ({
    skill: entry.skill,
    rate: entry.rate,
    held: have.has(entry.skill),
  }));

  // What stops counting: skills the persona holds that carry weight at the
  // role they are leaving and none at the role they are going to. This is
  // finding 2, and it needs a FROM as well as a TO, which is the one thing a
  // corpus-wide view of skills structurally cannot supply.
  const fromRole = persona.past[0]?.role ?? "";
  const fromDemand = new Map(
    (demandByRole.get(fromRole) ?? []).map((entry) => [entry.skill, entry.rate]),
  );
  const targetRate = new Map(
    (demandByRole.get(target) ?? []).map((entry) => [entry.skill, entry.rate]),
  );
  const stranded = persona.have
    .map((skill) => ({
      skill,
      was: fromDemand.get(skill) ?? 0,
      now: targetRate.get(skill) ?? 0,
    }))
    .filter((entry) => entry.was >= knobs.demand && entry.now < knobs.demand)
    .sort((a, b) => b.was - a.was);
  const strandedWeight = stranded.reduce((sum, entry) => sum + entry.was, 0);

  const link = (patch: Record<string, string | number>) => {
    const query = new URLSearchParams({
      persona: persona.id,
      demand: String(knobs.demand),
      floor: String(knobs.floor),
      core: String(knobs.core),
      show: knobs.show,
      ...(knobs.target != null && { target: knobs.target }),
      ...Object.fromEntries(Object.entries(patch).map(([key, value]) => [key, String(value)])),
    });
    return `/prototypes/persona-skill-gap?${query.toString()}`;
  };

  return (
    <main className="min-h-screen bg-surface-page p-8 text-content-primary">
      <header className="mb-6 max-w-content">
        <h1 className="text-2xl font-semibold">What should this person learn next?</h1>
        <p className="mt-2 text-sm text-content-secondary">
          The same requirement rates as the skill drill, pointed at a profile. Coverage is
          weighted by how often each skill is actually asked for, so matching a skill 91% of
          postings want is not the same event as matching one 19% want.
        </p>
        <p className="mt-1 text-xs text-content-muted">
          {population.toLocaleString()} of {corpus.toLocaleString()} seen postings carry a
          classification. Profiles are mock data — Market Scout has no profile feature.
        </p>
      </header>

      <p className="mb-6 text-xs text-content-muted">
        {timings.map((timing) => `${timing.label} ${timing.ms}ms`).join(" · ")} · total{" "}
        {Date.now() - started}ms over {timings.length} compositions. No filter and no third
        grouping — everything a profile needs is one crossing away.
      </p>

      <nav className="mb-6 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
        <span className="text-content-muted">profile</span>
        {PERSONAS.map((entry) => (
          <a
            key={entry.id}
            className={
              entry.id === persona.id
                ? "rounded-sm bg-accent-solid px-2 py-0.5 text-on-solid"
                : "px-2 py-0.5 underline"
            }
            href={`/prototypes/persona-skill-gap?persona=${entry.id}`}
          >
            {entry.name}
          </a>
        ))}
        <span className="ml-4 flex gap-2">
          <span className="text-content-muted">demand floor</span>
          <a className="underline" href={link({ demand: 0.05 })}>5%</a>
          <a className="underline" href={link({ demand: 0.1 })}>10%</a>
          <a className="underline" href={link({ demand: 0.25 })}>25%</a>
        </span>
        <span className="flex gap-2">
          <span className="text-content-muted">roles</span>
          <a className="underline" href={link({ show: "new" })}>new to them</a>
          <a className="underline" href={link({ show: "all" })}>all</a>
        </span>
      </nav>

      <section className="mb-8 max-w-content rounded-sm border border-edge bg-surface-raised p-4">
        <h2 className="text-sm font-semibold">
          {persona.name} — {titleOf(persona.past[0]?.role ?? "", persona.past[0]?.level)}
        </h2>
        <p className="mt-1 text-sm text-content-secondary">“{persona.motive}”</p>
        <dl className="mt-3 grid grid-cols-[6rem_1fr] gap-x-4 gap-y-1 text-xs">
          <dt className="text-content-muted">history</dt>
          <dd className="text-content-secondary">
            {persona.past
              .map((entry) => `${titleOf(entry.role, entry.level)}, ${entry.years}y`)
              .join(" ← ")}
          </dd>
          <dt className="text-content-muted">claims</dt>
          <dd className="flex flex-wrap gap-1">
            {persona.have.map((skill) => (
              <span
                key={skill}
                className="rounded-sm bg-observed px-1.5 py-0.5 text-on-solid"
              >
                {humanize(skill)}
              </span>
            ))}
          </dd>
          <dt className="text-content-muted">goal</dt>
          <dd className="text-content-secondary">
            {titleOf(persona.target.role, persona.target.level)}
          </dd>
        </dl>
        <p className="mt-3 border-t border-edge-hairline pt-2 text-xs text-content-muted italic">
          Sketch note — this persona exists to stress: {persona.stresses}
        </p>
      </section>

      <div className="flex flex-wrap items-start gap-12">
        <section className="min-w-0">
          <h2 className="mb-1 text-sm font-semibold">Where {persona.name}&rsquo;s skills land</h2>
          <p className="mb-4 max-w-[34rem] text-xs text-content-muted">
            Share of each role&rsquo;s weighted demand already covered. The grip column is not
            decoration: it says whether the role insists on anything, and a fit against a role
            that insists on nothing is not the same result as a fit against one that does.
          </p>
          <table className="text-sm">
            <thead>
              <tr className="text-xs text-content-muted">
                <th className="pr-4 pb-2 text-left font-normal">role</th>
                <th className="pr-4 pb-2 text-right font-normal">n</th>
                <th className="pr-4 pb-2 text-left font-normal">weighted coverage</th>
                <th className="pr-3 pb-2 text-right font-normal">skills</th>
                <th className="pr-3 pb-2 text-right font-normal">top {knobs.core}</th>
                <th className="pr-3 pb-2 text-right font-normal">grip</th>
                <th className="pb-2 text-left font-normal">strongest ask</th>
              </tr>
            </thead>
            <tbody>
              {fits.map((row) => (
                <tr
                  key={row.role}
                  className={
                    row.role === target
                      ? "bg-surface-row-selected align-middle"
                      : "align-middle hover:bg-surface-row-hover"
                  }
                >
                  <td className="py-0.5 pr-4 whitespace-nowrap">
                    <a className="underline" href={link({ target: row.role })}>
                      {humanize(row.role)}
                    </a>
                    {heldRoles.has(row.role) && (
                      <span className="ml-2 text-xs text-content-muted">held</span>
                    )}
                  </td>
                  <td className="py-0.5 pr-4 text-right tabular-nums text-content-secondary">
                    {row.base}
                  </td>
                  <td className="py-0.5 pr-4">
                    <svg width={160} height={14} viewBox="0 0 160 14" role="img"
                      aria-label={`${Math.round(row.fit * 100)} percent of weighted demand covered`}>
                      <rect x={0} y={3} width={160} height={8} className="fill-surface-sunken" />
                      <rect
                        x={0}
                        y={3}
                        width={Math.max(1, row.fit * 160)}
                        height={8}
                        className={rampClass(row.fit)}
                      />
                    </svg>
                  </td>
                  <td className="py-0.5 pr-3 text-right tabular-nums text-content-secondary">
                    {Math.round(row.fit * 100)}%
                    <span className="pl-2 text-content-muted">
                      {row.matched}/{row.demanded}
                    </span>
                  </td>
                  <td className="py-0.5 pr-3 text-right tabular-nums text-content-secondary">
                    {row.core}/{Math.min(knobs.core, row.demanded)}
                  </td>
                  <td
                    className={`py-0.5 pr-3 text-right tabular-nums ${
                      row.grip < 0.45 ? "text-content-muted" : "text-content-secondary"
                    }`}
                  >
                    {row.grip.toFixed(2)}
                  </td>
                  <td className="max-w-[12rem] truncate py-0.5 text-content-muted">
                    {humanize(row.gripSkill)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="mt-3 max-w-[34rem] text-xs text-content-muted">
            The percentage is weighted by requirement rate; the pair beside it counts skills
            unweighted. Neither is comparable across targets — a role with a long thin tail
            depresses both — which is why the top-{knobs.core} column is there. Holding the two
            heaviest requirements of a role is the same achievement whether that role names ten
            more skills or thirty. See finding 1.
          </p>
        </section>

        <section className="min-w-0 flex-1">
          <h2 className="mb-1 text-sm font-semibold">
            The gap to {humanize(target)}
          </h2>
          <p className="mb-2 max-w-[38rem] text-sm text-content-secondary">
            {Math.round((targetTotal === 0 ? 0 : targetCovered / targetTotal) * 100)}% of what{" "}
            {humanize(target)} asks for, weighted — and{" "}
            {gap.slice(0, knobs.core).filter((row) => row.held).length} of its{" "}
            {Math.min(knobs.core, gap.length)}{" "}
            heaviest requirements. The second number is the
            comparable one; the first moves with how long the role&rsquo;s tail happens to be.
          </p>
          <p className="mb-2 max-w-[38rem] text-xs text-content-secondary">
            {gripReading(targetStrongest.rate, targetStrongest.skill, targetBase)}
          </p>
          <p className="mb-4 max-w-[38rem] text-xs text-content-muted">
            Role-level only. {persona.name} wants{" "}
            {titleOf(persona.target.role, persona.target.level).toLowerCase()}, and requirement
            rates move with level inside a role — but that needs P(skill | role, seniority), a
            three-way classified crossing that times out. See finding 5.
          </p>

          {gap.map((row) => (
            <div key={row.skill} className="flex items-center hover:bg-surface-row-hover">
              <span
                className={`w-[16rem] truncate pr-4 text-right text-xs ${
                  row.held ? "text-content-muted" : "font-medium text-content-primary"
                }`}
              >
                {humanize(row.skill)}
              </span>
              <svg
                width={AXIS_WIDTH}
                height={15}
                viewBox={`0 0 ${AXIS_WIDTH} 15`}
                role="img"
                aria-label={`${humanize(row.skill)}: ${Math.round(row.rate * 100)} percent of postings, ${row.held ? "held" : "missing"}`}
              >
                {/* Ink goes on the work, not the wins: a skill already held is
                    drawn as an outline, a missing one as a solid bar. The
                    conventional colouring would put the loudest marks on the
                    rows that need no action. */}
                <rect
                  x={0}
                  y={3}
                  width={Math.max(1, row.rate * AXIS_WIDTH)}
                  height={9}
                  className={row.held ? "fill-surface-sunken stroke-edge" : rampClass(row.rate)}
                  strokeWidth={row.held ? 1 : 0}
                />
              </svg>
              <span className="pl-3 text-xs tabular-nums text-content-secondary">
                {Math.round(row.rate * 100)}%
              </span>
              <span className="pl-2 text-xs text-content-muted">{row.held ? "have" : "gap"}</span>
            </div>
          ))}

          {stranded.length > 0 && (
            <div className="mt-6 max-w-[38rem]">
              <h3 className="text-sm font-semibold">What stops counting</h3>
              <p className="mt-1 mb-3 text-xs text-content-muted">
                {stranded.length} of {persona.name}&rsquo;s skills carry{" "}
                {strandedWeight.toFixed(2)} of weighted demand at {humanize(fromRole)} and fall
                below the floor at {humanize(target)}. A gap list alone never says this, and it
                is usually the sentence that changes the decision.
              </p>
              {stranded.map((entry) => (
                <div key={entry.skill} className="flex items-center text-xs">
                  <span className="w-[16rem] truncate pr-4 text-right text-content-secondary">
                    {humanize(entry.skill)}
                  </span>
                  <svg width={AXIS_WIDTH} height={15} viewBox={`0 0 ${AXIS_WIDTH} 15`} aria-hidden>
                    <rect
                      x={0}
                      y={3}
                      width={Math.max(1, entry.was * AXIS_WIDTH)}
                      height={9}
                      className="fill-stale-subtle stroke-stale-edge"
                      strokeWidth={1}
                    />
                    <rect
                      x={0}
                      y={3}
                      width={Math.max(1, entry.now * AXIS_WIDTH)}
                      height={9}
                      className="fill-stale-ink"
                    />
                  </svg>
                  <span className="pl-3 whitespace-nowrap tabular-nums text-content-secondary">
                    {Math.round(entry.was * 100)}% → {Math.round(entry.now * 100)}%
                  </span>
                </div>
              ))}
            </div>
          )}

          <ul className="mt-8 max-w-[38rem] space-y-1 border-t border-edge-hairline pt-3 text-xs text-content-muted">
            {PROFILE_CAVEATS.map((caveat) => (
              <li key={caveat}>{caveat}</li>
            ))}
          </ul>
        </section>
      </div>
    </main>
  );
}
