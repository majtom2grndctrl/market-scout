// Every sentence Airy shows is assembled here from DiscoveryData, so a new
// profile rewrites the page without a copy edit. Invariant 6 of the build
// contract: encouragement cites the data or it does not appear.
// See: agent-context/plans/in-progress/discovery-prototypes/index.md

import type { DiscoveryData, Recommendation, SkillRef, Strength } from "../../_data/query";

const WORDS = ["no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"];

export function countWord(n: number): string {
  return Number.isInteger(n) && n >= 0 && n < WORDS.length ? WORDS[n] : n.toLocaleString();
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function listJoin(items: readonly string[]): string {
  if (items.length <= 1) return items.join("");
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

export function postingsLine(r: Recommendation): string {
  return `${r.openPostings.toLocaleString()} open ${plural(r.openPostings, "posting")} · ${r.companies.toLocaleString()} ${plural(r.companies, "company", "companies")}`;
}

/** P(skill | role) as a share of the role's postings. Never the person's gap. */
export function shareLabel(share: number): string {
  const pct = Math.round(share * 100);
  return pct < 1 ? "<1%" : `${pct}%`;
}

// "Full Stack Engineer" and "Full-Stack Engineer" are one spelling for a reader.
const spellingKey = (title: string) => title.toLowerCase().replace(/[^a-z0-9]+/g, "");

/** Titles employers also use for this role, headline and repeat spellings removed. */
export function alsoPostedAs(r: Recommendation, limit: number): string[] {
  const seen = new Set<string>([spellingKey(r.headline)]);
  const out: string[] = [];
  for (const t of r.titles) {
    const key = spellingKey(t.title);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(t.title);
    if (out.length === limit) break;
  }
  return out;
}

function mode(values: readonly string[]): string | null {
  const counts = new Map<string, number>();
  let best: string | null = null;
  for (const v of values) {
    const n = (counts.get(v) ?? 0) + 1;
    counts.set(v, n);
    if (best === null || n > (counts.get(best) ?? 0)) best = v;
  }
  return best;
}

export interface Lede {
  readonly sentence: string;
  /** The past title most of the nearest roles grow out of, when there is one. */
  readonly nearestFrom: string | null;
  /** Whether "the nearest" names one role or several. */
  readonly nearestMany: boolean;
}

export function ledeFor(data: DiscoveryData): Lede {
  const past = data.pastRoles.length;
  const skills = data.claimedSkills.length;
  const recs = data.recommendations.length;

  const parts = [
    past > 0 ? `${countWord(past)} past ${plural(past, "title")}` : null,
    skills > 0 ? `${countWord(skills)} ${plural(skills, "skill")} you named` : null,
  ].filter((p): p is string => p !== null);
  const subject = parts.length > 0 ? capitalize(parts.join(" and ")) : "Your profile";
  const singular = past + skills <= 1;
  const corpus = `${data.coverage.classifiedPostings.toLocaleString()} classified postings`;

  const sentence =
    recs > 0
      ? `${subject}, read against ${corpus}, ${singular ? "points" : "point"} toward ${countWord(recs)} ${plural(recs, "role")}.`
      : `${subject}, read against ${corpus}, ${singular ? "doesn't" : "don't"} point toward a role yet.`;

  const close = data.recommendations.filter((r) => r.strength === "close");
  const pool = close.length > 0 ? close : data.recommendations;
  const nearestFrom = mode(pool.flatMap((r) => (r.closestPast ? [r.closestPast.titleText] : [])));

  return { sentence, nearestFrom, nearestMany: pool.length > 1 };
}

export function coverageLine(c: DiscoveryData["coverage"]): string {
  return `Ranked from ${c.classifiedPostings.toLocaleString()} classified of ${c.openPostings.toLocaleString()} open postings, across ${c.rolesConsidered.toLocaleString()} roles: what has been read so far, not the whole market.`;
}

/** How many recommendations name each past title as their closest. */
export function tallyByPastTitle(recs: readonly Recommendation[]): Map<string, number> {
  const out = new Map<string, number>();
  for (const r of recs) {
    if (r.closestPast) out.set(r.closestPast.titleText, (out.get(r.closestPast.titleText) ?? 0) + 1);
  }
  return out;
}

export interface NamedSkill {
  readonly text: string;
  /** Recommendations whose top skills include this one. */
  readonly carries: number;
}

/** Claimed skills, those that carry into recommended roles first. */
export function namedSkills(data: DiscoveryData): NamedSkill[] {
  const carried = new Map<string, number>();
  for (const r of data.recommendations) for (const s of r.bring) carried.set(s.slug, (carried.get(s.slug) ?? 0) + 1);
  return data.claimedSkills
    .map((c, i) => ({ text: c.text, carries: c.skill ? (carried.get(c.skill.slug) ?? 0) : 0, i }))
    .sort((a, b) => b.carries - a.carries || a.i - b.i)
    .map(({ text, carries }) => ({ text, carries }));
}

export interface TierCopy {
  readonly strength: Strength;
  readonly name: string;
  readonly note: string | null;
}

const TIER_NAMES: Record<Strength, string> = {
  close: "Close to home",
  adjacent: "A step across",
  stretch: "Further out",
};

/** Ranked groups in tier order; empty tiers are dropped. */
export function tiersOf(recs: readonly Recommendation[]): { copy: TierCopy; recs: Recommendation[] }[] {
  return (["close", "adjacent", "stretch"] as const)
    .map((strength) => {
      const group = recs.filter((r) => r.strength === strength);
      return { copy: { strength, name: TIER_NAMES[strength], note: tierNote(group) }, recs: group };
    })
    .filter((t) => t.recs.length > 0);
}

// The note names the evidence that binds the group: a skill every role in it
// asks for, else skills some ask for, else the past titles they sit nearest.
function tierNote(group: readonly Recommendation[]): string | null {
  const names = (skills: SkillRef[]) => listJoin(skills.map((s) => s.name));
  const shared = group.reduce<SkillRef[] | null>(
    (acc, r) => (acc === null ? [...r.bring] : acc.filter((s) => r.bring.some((b) => b.slug === s.slug))),
    null,
  );
  if (shared && shared.length > 0) {
    return group.length === 1
      ? `Asks for ${names(shared)}, which you named.`
      : `Every one asks for ${names(shared)}, which you named.`;
  }
  const union = new Map<string, SkillRef>();
  for (const r of group) for (const s of r.bring) union.set(s.slug, s);
  if (union.size > 0) return `Some ask for ${names([...union.values()].slice(0, 3))}, which you named.`;
  const pasts = [...new Set(group.flatMap((r) => (r.closestPast ? [r.closestPast.titleText] : [])))];
  if (pasts.length > 0) return `Nearest to your time as ${listJoin(pasts.slice(0, 3))}.`;
  return null;
}
