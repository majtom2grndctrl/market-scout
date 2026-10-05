// Copy derived from the data. Nothing here names this person's titles, skills,
// or counts: the profile changes, and every encouraging line has to cite what
// is actually in it (contract invariant 6).

import type { Recommendation, Strength } from "../_data/query";

const WORDS = ["no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"];

/** Small counts read better as words in a sentence; larger ones stay numerals. */
export function countWord(n: number): string {
  return WORDS[n] ?? n.toLocaleString();
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return n === 1 ? one : many;
}

export function listJoin(items: readonly string[]): string {
  if (items.length <= 1) return items[0] ?? "";
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

export function postings(n: number): string {
  return `${n.toLocaleString()} ${plural(n, "posting")}`;
}

export const ORBIT_NAME: Record<Strength, string> = {
  close: "Close",
  adjacent: "Adjacent",
  stretch: "Stretch",
};

/** "Also called" spellings: every title variant except the headline itself. */
export function alsoCalled(rec: Recommendation): Recommendation["titles"] {
  const head = rec.headline.toLowerCase();
  return rec.titles.filter((t) => t.title.toLowerCase() !== head);
}

/** Share of a role's postings asking for a skill. Describes the role, never the person. */
export function shareOfPostings(share: number): string {
  const pct = Math.round(share * 100);
  return pct < 1 ? "under 1% of postings" : `${pct}% of postings`;
}

export const WHOLE_PROFILE = "Your profile as a whole";

/** Label for the place a recommendation's path starts. */
export function anchorOf(rec: Recommendation): string {
  return rec.closestPast?.titleText ?? WHOLE_PROFILE;
}
