// Edge markers: where a line ends when its skill is scrolled out of the
// column. Pure: the cached layout and the column's scroll in, which end each
// line lands on out.
//
// A lit skill inside the column's visible band takes its line on its own
// marker. One scrolled past an edge takes it on the edge marker pinned there,
// which carries the count beyond that edge. The edge markers sit still in the
// column's box, so a line ending there rides the same sticky frame as the
// column, never its scroll.
//
// Which end a line lands on is decided per layout pass, not per scroll frame
// (see `useLines`): mid-scroll a line keeps the end it had, and stays joined
// to it, and the settle re-routes.

import type { Layout } from "./layout";
import { offsets } from "./weight";

export type Edge = "above" | "below";

/** The marker's gap from the column's edge, and its height. Shared with EdgeMarkers' inline styles. */
export const EDGE_GAP = 6;
export const EDGE_H = 30;
/** From the marker's right edge to where a line lands: its padding and glyph, as on a skill's pill. */
export const EDGE_LAND = 10.5;
/** How far into the column each edge marker reaches. A skill marker under it is beyond that edge. */
const EDGE_BAND = EDGE_GAP + EDGE_H + 4;
/**
 * The most a group of lines may spread over the marker, centre to centre, so
 * they land on it rather than around it: the pill's height less room for the
 * heaviest line's edges.
 */
const EDGE_SPREAD = EDGE_H - 8;

/**
 * Centre offsets of lines landing on one edge marker, in order, around its
 * middle. Neighbours keep the bundle's clear gap (see `weight.ts`) until that
 * would overrun the marker; then the group closes up evenly.
 */
export function landing(widths: readonly number[]): number[] {
  const off = offsets(widths);
  const span = off.at(-1) ?? 0;
  const k = span > EDGE_SPREAD ? EDGE_SPREAD / span : 1;
  return off.map((o) => (o - span / 2) * k);
}

/** How tall an edge marker's landing bar stands for lines of these widths: their spread, plus the outer lines' edges. */
export function landingHeight(widths: readonly number[]): number {
  const at = landing(widths);
  if (at.length === 0) return 10;
  return Math.max(10, at[at.length - 1] - at[0] + (widths[0] + widths[widths.length - 1]) / 2 + 4);
}

/** Which lit skills sit beyond each edge. Everything else lands on its own marker. */
export interface Routing {
  readonly above: readonly string[];
  readonly below: readonly string[];
}

export const NO_ROUTING: Routing = { above: [], below: [] };

/** The column's visible band at scroll `c`, in its scroll content. An edge with nothing beyond it gives up no band. */
export function bandOf(clientH: number, c: number, moreAbove: boolean, moreBelow: boolean) {
  return { top: c + (moreAbove ? EDGE_BAND : 0), bottom: c + clientH - (moreBelow ? EDGE_BAND : 0) };
}

export function routingOf(L: Layout, c: number, moreAbove: boolean, moreBelow: boolean): Routing {
  const band = bandOf(L.colClient, c, moreAbove, moreBelow);
  return {
    above: L.marks.filter((m) => m.y < band.top).map((m) => m.slug),
    below: L.marks.filter((m) => m.y > band.bottom).map((m) => m.slug),
  };
}

/** Where an edge marker's lines land, in the column's box. */
export function edgeY(L: Layout, edge: Edge): number {
  return edge === "above" ? EDGE_GAP + EDGE_H / 2 : L.colClient - EDGE_GAP - EDGE_H / 2;
}

export interface End {
  readonly slug: string;
  /** Container x. */
  readonly x: number;
  /** In the column's scroll content when `edge` is null; in the column's box when it lands on an edge marker. */
  readonly y: number;
  readonly edge: Edge | null;
  readonly width?: number;
}

/**
 * Every lit skill's line end. Lines meeting at one edge marker land a pitch
 * apart (see `landing`), in the order of their skills, so no two share a
 * pixel and none crosses another (see `routeBundle`).
 */
export function endsOf(L: Layout, routing: Routing): End[] {
  const at = new Map<string, Edge>([...routing.above.map((s) => [s, "above"] as const), ...routing.below.map((s) => [s, "below"] as const)]);
  const groups: Record<Edge, string[]> = { above: [], below: [] };
  for (const m of [...L.marks].sort((a, b) => a.y - b.y)) {
    const e = at.get(m.slug);
    if (e) groups[e].push(m.slug);
  }
  const width = new Map(L.marks.map((m) => [m.slug, m.width ?? 1.5]));
  const lands = {
    above: landing(groups.above.map((s) => width.get(s) ?? 1.5)),
    below: landing(groups.below.map((s) => width.get(s) ?? 1.5)),
  };
  const x = L.gutterLeft - EDGE_LAND;
  return L.marks.map((m): End => {
    const edge = at.get(m.slug) ?? null;
    if (!edge) return { slug: m.slug, x: m.x, y: m.y, edge, width: m.width };
    return { slug: m.slug, x, y: edgeY(L, edge) + lands[edge][groups[edge].indexOf(m.slug)], edge, width: m.width };
  });
}

const clampTo = (clientH: number, scrollH: number) => (v: number) => Math.min(Math.max(0, scrollH - clientH), Math.max(0, Math.round(v)));

/**
 * The scroll that centres every lit skill in the column's band, given their
 * markers' y in its scroll content. Null when they don't fit it.
 */
export function centreScroll(clientH: number, scrollH: number, lit: readonly number[]): number | null {
  if (lit.length === 0) return null;
  const lo = Math.min(...lit);
  const hi = Math.max(...lit);
  if (hi - lo > clientH - 2 * EDGE_BAND - 16) return null;
  return clampTo(clientH, scrollH)((lo + hi) / 2 - clientH / 2);
}

/**
 * Where an edge marker scrolls the column: every lit skill centred when they
 * fit, otherwise just far enough that `group`, the ones beyond `edge`, show.
 */
export function revealScroll(clientH: number, scrollH: number, lit: readonly number[], group: readonly number[], edge: Edge): number {
  const centred = centreScroll(clientH, scrollH, lit);
  if (centred !== null || group.length === 0) return centred ?? clampTo(clientH, scrollH)(0);
  const clamp = clampTo(clientH, scrollH);
  return edge === "above" ? clamp(Math.min(...group) - EDGE_BAND - 12) : clamp(Math.max(...group) + EDGE_BAND + 12 - clientH);
}
