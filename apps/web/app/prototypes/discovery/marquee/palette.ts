// Plates: each past title gets a mood colour, and every role wears the plate
// of the past title it sits closest to. Colour therefore encodes provenance —
// "these grew from your time as X" — rather than decorating.
//
// Plates go to past titles in order of how many roles they lead to, so the
// most common chapter wears the calmest colour. Ties keep profile order.

import type { MarqueeData } from "./types";

const PLATES = [
  "var(--marquee-sky)",
  "var(--marquee-sun)",
  "var(--marquee-bloom)",
  "var(--marquee-mint)",
  "var(--marquee-lilac)",
] as const;

/** A role with no matched past title sits on the page's own sunken surface. */
export const NEUTRAL_PLATE = "var(--surface-sunken)";

export type Plates = ReadonlyMap<string, string>;

export function assignPlates(data: MarqueeData): Plates {
  const leads = new Map<string, number>();
  for (const r of data.recommendations) {
    if (r.closestPast) leads.set(r.closestPast.titleText, (leads.get(r.closestPast.titleText) ?? 0) + 1);
  }
  const titles = [...new Set([...data.pastRoles.map((p) => p.titleText), ...leads.keys()])];
  const ordered = titles
    .map((title, index) => ({ title, index, n: leads.get(title) ?? 0 }))
    .sort((a, b) => b.n - a.n || a.index - b.index);
  return new Map(ordered.map((o, k) => [o.title, PLATES[k % PLATES.length]]));
}

export function plateOf(plates: Plates, pastTitle: string | null | undefined): string {
  return (pastTitle && plates.get(pastTitle)) || NEUTRAL_PLATE;
}
