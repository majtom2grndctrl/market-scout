// Every sentence Threads shows is assembled here from DiscoveryData, so a new
// profile rewrites the page without a copy edit. Encouragement cites the data
// or it does not appear; an inherited skill is never phrased as one the person
// claimed (build contract, Invariants 6 and 7).
// See: agent-context/plans/in-progress/discovery-prototypes/index.md

import type { DiscoveryData, PersonSkill, Recommendation } from "../../_data/query";

const ONES = ["no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"];
const TENS = ["", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"];

/** Counts in words below a hundred, so "eight" and "twenty-five" read alike in one sentence. */
export function countWord(n: number): string {
  if (!Number.isInteger(n) || n < 0 || n >= 100) return n.toLocaleString("en-US");
  if (n < ONES.length) return ONES[n];
  const t = TENS[Math.floor(n / 10)];
  return n % 10 === 0 ? t : `${t}-${ONES[n % 10]}`;
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
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

export interface SkillSplit {
  readonly claimed: PersonSkill[];
  readonly inherited: PersonSkill[];
}

export function splitSkills(skills: readonly PersonSkill[]): SkillSplit {
  return { claimed: skills.filter((s) => s.claimed), inherited: skills.filter((s) => !s.claimed) };
}

/** A role's connects, resolved against personSkills so claimed and inherited read apart. */
export function connectsOf(r: Recommendation, bySlug: ReadonlyMap<string, PersonSkill>): SkillSplit {
  const resolved = r.connects.flatMap((c) => {
    const s = bySlug.get(c.slug);
    return s ? [s] : [];
  });
  return splitSkills(resolved);
}

/**
 * What else the role asks for, beyond the skills already lit. grow excludes
 * only claimed skills, so an inherited skill can sit in both lists; showing it
 * twice would read as a gap the person has already been shown to cover.
 */
export function alsoAsksFor(r: Recommendation) {
  const lit = new Set(r.connects.map((c) => c.slug));
  return r.grow.filter((g) => !lit.has(g.slug));
}

export function ledeFor(data: DiscoveryData): string {
  const { claimed, inherited } = splitSkills(data.personSkills);
  const unmatched = data.claimedSkills.filter((c) => c.skill === null).length;
  const titles = new Set(inherited.flatMap((s) => s.fromPast)).size;
  const recs = data.recommendations;
  const drawing = recs.filter((r) => r.connects.length > 0).length;

  const named =
    claimed.length > 0
      ? `You named ${countWord(claimed.length)} ${plural(claimed.length, "skill")}${unmatched > 0 ? `, plus ${countWord(unmatched)} we don't recognise yet` : ""}.`
      : unmatched > 0
        ? `You named ${countWord(unmatched)} ${plural(unmatched, "skill")}, none of which we recognise yet.`
        : "You haven't named a skill yet.";

  const came =
    inherited.length > 0
      ? ` ${capitalize(countWord(inherited.length))} ${claimed.length > 0 ? "more " : ""}${inherited.length === 1 ? "is" : "are"} common in postings for the ${titles > 1 ? `${countWord(titles)} titles` : "title"} you've held.`
      : "";

  let roles = "";
  if (recs.length > 0 && drawing > 0) {
    const share = drawing === recs.length ? (recs.length === 1 ? "The role below" : `All ${countWord(recs.length)} roles below`) : `${capitalize(countWord(drawing))} of the ${countWord(recs.length)} roles below`;
    roles = ` ${share} ${drawing === 1 ? "draws" : "draw"} on them. Hover or tap one to see which.`;
  }

  return `${named}${came}${roles}`;
}

export function coverageLine(c: DiscoveryData["coverage"]): string {
  return `Ranked from ${n(c.classifiedPostings)} classified of ${n(c.openPostings)} open postings, across ${n(c.rolesConsidered)} roles: what has been read so far, not the whole market.`;
}

/** Every slug some recommendation draws on: the only skills a line can reach. */
export function reachedSlugs(recs: readonly Recommendation[]): ReadonlySet<string> {
  return new Set(recs.flatMap((r) => r.connects.map((c) => c.slug)));
}

/**
 * Labels the inherited skills no role here draws on, in the card's own term
 * for a role's top ten ("defining skills"). `more` when reached ones are
 * listed above them.
 */
export function outsideNote(count: number, more: boolean): string {
  if (!more) return `None ${count === 1 ? "is" : "are"} among the defining skills of these roles.`;
  return `${capitalize(countWord(count))} more, outside every role's defining skills`;
}

export function inheritedNote(skill: PersonSkill): string {
  return skill.fromPast.length > 0 ? `Comes with ${listJoin(skill.fromPast)}` : "Comes with roles you've held";
}

export function rolesHeading(count: number): string {
  if (count === 0) return "No roles stand out yet";
  return count === 1 ? "One role" : `${capitalize(countWord(count))} roles, nearest first`;
}
