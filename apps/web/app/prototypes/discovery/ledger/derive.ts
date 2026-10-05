// Pure derivations over DiscoveryData. Every line of copy on the page is built
// from these, so a different profile reads correctly with no edits here.

import type { DiscoveryData, Recommendation, Strength } from "../_data/query";

export const TIERS: readonly Strength[] = ["close", "adjacent", "stretch"];

export const TIER_LABEL: Record<Strength, string> = {
  close: "Close",
  adjacent: "Adjacent",
  stretch: "Stretch",
};

const numberFormat = new Intl.NumberFormat("en-US");

/** Locale pinned so the server and client render the same digits. */
export function formatCount(n: number): string {
  return numberFormat.format(n);
}

export function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}

/** "A", "A and B", "A, B and C". */
export function listPhrase(items: readonly string[]): string {
  if (items.length <= 1) return items[0] ?? "";
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

/** Two-digit rank, so a ranking of nine or fewer still sets in one column width. */
export function rankLabel(rank: number): string {
  return String(rank).padStart(2, "0");
}

/** Title variants other than the headline. The data may repeat the headline in `titles`. */
export function alsoCalled(rec: Recommendation): Recommendation["titles"] {
  const head = rec.headline.toLowerCase();
  return rec.titles.filter((t) => t.title.toLowerCase() !== head);
}

export interface TierGroup {
  readonly tier: Strength;
  readonly recs: readonly Recommendation[];
  /** The past title the most of this tier builds on, and how many do; null when none does. */
  readonly commonPast: { readonly title: string; readonly count: number } | null;
}

/** Rank order is preserved; strength is monotone in rank, so groups are contiguous. */
export function groupByTier(recs: readonly Recommendation[]): TierGroup[] {
  return TIERS.flatMap((tier) => {
    const inTier = recs.filter((r) => r.strength === tier);
    if (inTier.length === 0) return [];
    return [{ tier, recs: inTier, commonPast: mostCommon(inTier.flatMap((r) => (r.closestPast ? [r.closestPast.titleText] : []))) }];
  });
}

function mostCommon(values: readonly string[]): { title: string; count: number } | null {
  const counts = new Map<string, number>();
  for (const v of values) counts.set(v, (counts.get(v) ?? 0) + 1);
  let best: { title: string; count: number } | null = null;
  for (const [title, count] of counts) {
    if (!best || count > best.count) best = { title, count };
  }
  return best;
}

/** A role builds on a record when it names a closest past title or a claimed skill it is defined by. */
export function buildsOnRecord(rec: Recommendation): boolean {
  return rec.closestPast !== null || rec.bring.length > 0;
}

export interface LedgerSummary {
  readonly roleCount: number;
  /** Distinct past titles that at least one recommendation builds on, in profile order. */
  readonly pastTitlesUsed: readonly string[];
  /** Recommendations whose top skills include at least one the person claimed. */
  readonly withBring: number;
  /** Distinct title spellings shown across all recommendations. A floor: each role carries only its most-posted few. */
  readonly titleSpellings: number;
  readonly tierCounts: Record<Strength, number>;
  readonly coverageShare: number;
}

export function summarize(data: DiscoveryData): LedgerSummary {
  const recs = data.recommendations;
  const builtOn = new Set(recs.flatMap((r) => (r.closestPast ? [r.closestPast.titleText] : [])));
  const spellings = new Set(recs.flatMap((r) => r.titles.map((t) => t.title.toLowerCase())));
  const tierCounts: Record<Strength, number> = { close: 0, adjacent: 0, stretch: 0 };
  for (const r of recs) tierCounts[r.strength] += 1;
  const { openPostings, classifiedPostings } = data.coverage;
  return {
    roleCount: recs.length,
    pastTitlesUsed: [...new Set(data.pastRoles.map((p) => p.titleText))].filter((t) => builtOn.has(t)),
    withBring: recs.filter((r) => r.bring.length > 0).length,
    titleSpellings: spellings.size,
    tierCounts,
    coverageShare: openPostings > 0 ? classifiedPostings / openPostings : 0,
  };
}

/** For each rank, whether that recommendation builds on this past title. */
export function pastPresence(recs: readonly Recommendation[], titleText: string): boolean[] {
  return recs.map((r) => r.closestPast?.titleText === titleText);
}

/** For each rank, whether that recommendation lists this claimed skill among what you bring. */
export function skillPresence(recs: readonly Recommendation[], slug: string | null): boolean[] {
  return recs.map((r) => slug !== null && r.bring.some((b) => b.slug === slug));
}
