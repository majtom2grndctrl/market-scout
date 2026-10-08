// Every sentence Dawn shows is assembled here from DiscoveryData, so a new
// profile rewrites the page without a copy edit. Encouragement cites the data
// or it does not appear; an inherited skill is never phrased as one the person
// claimed (build contract, Invariants 6 and 7).
// See: agent-context/plans/in-progress/discovery-prototypes/index.md

import type { DiscoveryData, PersonSkill, Recommendation } from "../../_data/query";

export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

export function listJoin(items: readonly string[]): string {
  if (items.length <= 1) return items.join("");
  if (items.length === 2) return `${items[0]} and ${items[1]}`;
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

const n = (v: number) => v.toLocaleString("en-US");

export function postingsLine(r: Recommendation): string {
  return `${n(r.openPostings)} open ${plural(r.openPostings, "posting")} · ${n(r.companies)} ${plural(r.companies, "company", "companies")}`;
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

/** A person's skill as one role uses it: share is P(skill | role), never the person's fit. */
export type UsedSkill = PersonSkill & { readonly share: number };

/**
 * The skills a role's lines connect, resolved against personSkills so claimed
 * and inherited read apart, each in share order as the read gives them.
 */
export function usesOf(r: Recommendation, bySlug: ReadonlyMap<string, PersonSkill>): { claimed: UsedSkill[]; inherited: UsedSkill[] } {
  const resolved = r.uses.flatMap((u): UsedSkill[] => {
    const s = bySlug.get(u.slug);
    return s ? [{ ...s, share: u.share }] : [];
  });
  return { claimed: resolved.filter((s) => s.claimed), inherited: resolved.filter((s) => !s.claimed) };
}

/** A used skill's share, as the card's screen-reader text and hover title read it. */
export function askedFor(share: number): string {
  return `asked for in ${shareLabel(share)} of postings`;
}

/**
 * What else the role asks for, beyond the skills already lit. grow excludes
 * only claimed skills, so an inherited skill can sit in both lists; showing it
 * twice would read as a gap the person has already been shown to cover.
 */
export function alsoAsksFor(r: Recommendation) {
  const lit = new Set(r.uses.map((u) => u.slug));
  return r.grow.filter((g) => !lit.has(g.slug));
}

/** Where the ranking comes from (build contract, Invariant 4). */
export function coverageLine(c: DiscoveryData["coverage"]): string {
  return `Ranked from ${n(c.classifiedPostings)} classified of ${n(c.openPostings)} open postings, across ${n(c.rolesConsidered)} roles.`;
}

/** Where an inherited skill comes from: postings for the person's past titles, never their own word. */
export function inheritedNote(skill: PersonSkill): string {
  return skill.fromPast.length > 0 ? `Common in ${listJoin(skill.fromPast)} postings` : "Common in postings for titles you've held";
}

export function rolesHeading(count: number): string {
  if (count === 0) return "No roles yet";
  return count === 1 ? "1 role" : `${n(count)} roles, closest first`;
}
