// The thread as the dawn: each line is lilac where it meets the skills and
// warms to rose where it leaves the role, at a gold port. Colour is set by x,
// not by distance along the line. x never scrolls, so the pieces of a line in
// different scroll frames (see `compose.ts`) still agree where they meet.
//
// This is light, not data: every line shares one gradient, and nothing about
// a line's colour says more or less, rising or falling. Its ends are lilac
// and rose, well clear of the trend pair's blue and orange.

import type { Layout } from "./layout";

/** Where the gradient runs, in container x. `origin` is the column's left edge, the pieces' frame. */
export interface Span {
  readonly from: number;
  readonly to: number;
  readonly origin: number;
}

/** Offsets match the CSS custom properties in dawn.module.css. */
export const STOPS = [
  { at: 0, color: "var(--dawn-thread-cool)" },
  { at: 1, color: "var(--dawn-thread-rose)" },
] as const;

/** Cool from a little into the column, so the markers sit in it; warm at the card's edge. */
export function spanOf(L: Layout): Span {
  return { from: L.colLeft + (L.gutterLeft - L.colLeft) * 0.35, to: L.originX, origin: L.colLeft };
}

export const sameSpan = (a: Span | null, b: Span | null) =>
  a === b || (a !== null && b !== null && a.from === b.from && a.to === b.to && a.origin === b.origin);

const pct = (v: number) => `${Math.round(v * 1000) / 10}%`;

/**
 * The thread's colour at container x, for pieces that hold one colour: the
 * vertical runs. Mixed in sRGB, as SVG interpolates its gradient stops.
 */
export function threadAt(span: Span, x: number): string {
  const t = Math.min(1, Math.max(0, (x - span.from) / Math.max(1, span.to - span.from)));
  const k = STOPS.findIndex((s) => s.at >= t);
  if (k <= 0) return STOPS[0].color;
  const a = STOPS[k - 1];
  const b = STOPS[k];
  const f = (t - a.at) / (b.at - a.at);
  return `color-mix(in srgb, ${a.color}, ${b.color} ${pct(f)})`;
}
