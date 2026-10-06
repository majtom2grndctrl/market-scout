// Label placement. Greedy, in rank order: each label tries positions around
// its star and takes the cheapest, where cost is overlap with what is already
// down (labels, stars, the plaque, the frame), paths it would sit on, and
// how far it turns from "outward". Rank 1 gets the best seat; a label only
// moves off its star, with a leader line, when no adjacent seat is free.

import { ORBIT_NAME } from "./copy";
import { type Box, type Layout, type Measure, type Point, type Star, TYPE, pointOnCurve, wrapText } from "./geometry";

export type Anchor = "start" | "middle" | "end";

export interface StarLabel {
  box: Box;
  anchor: Anchor;
  lines: string[];
  meta: string;
  /** Unit direction from star to label, for the entrance drift. */
  dir: Point;
  leader: [Point, Point] | null;
}

export interface OrbitLabel {
  x: number;
  y: number;
}

const WRAP = 190;
const META_GAP = 3;

interface Candidate {
  dir: Point;
  anchor: Anchor;
  far: boolean;
  offset: number;
}

const D = Math.SQRT1_2;
const CANDIDATES: Candidate[] = [
  ...[1, -1].flatMap((sx) =>
    [0, -9, 9, -18, 18].map((offset) => ({ dir: { x: sx, y: 0 }, anchor: (sx > 0 ? "start" : "end") as Anchor, far: false, offset })),
  ),
  { dir: { x: 0, y: -1 }, anchor: "middle", far: false, offset: 0 },
  { dir: { x: 0, y: 1 }, anchor: "middle", far: false, offset: 0 },
  { dir: { x: D, y: -D }, anchor: "start", far: false, offset: 0 },
  { dir: { x: D, y: D }, anchor: "start", far: false, offset: 0 },
  { dir: { x: -D, y: -D }, anchor: "end", far: false, offset: 0 },
  { dir: { x: -D, y: D }, anchor: "end", far: false, offset: 0 },
];
const FAR = CANDIDATES.filter((c) => c.offset === 0).map((c) => ({ ...c, far: true }));

export function placeStarLabels(
  layout: Layout,
  text: (slug: string) => { headline: string; meta: string },
  measure: Measure,
): Map<string, StarLabel> {
  const placed: Box[] = [];
  const marks = layout.stars.map((s) => grow({ x: s.x - s.r, y: s.y - s.r, w: s.r * 2, h: s.r * 2 }, 5));
  const plaque = grow(layout.plaque.box, 12);
  const trail = layout.stars.flatMap((s) => Array.from({ length: 22 }, (_, i) => pointOnCurve(s.curve, (i + 1) / 23)));
  const frame = { x: 6, y: 6, w: layout.width - 12, h: layout.height - 12 };

  const out = new Map<string, StarLabel>();
  for (const star of [...layout.stars].sort((a, b) => a.rank - b.rank)) {
    const { headline, meta } = text(star.slug);
    const lines = wrapText(headline, WRAP, "headline", measure);
    const w = Math.max(...lines.map((l) => measure(l, "headline")), measure(meta, "meta"));
    const h = lines.length * TYPE.headline.lineHeight + META_GAP + TYPE.meta.lineHeight;

    let best: { cost: number; box: Box; c: Candidate } | null = null;
    for (const c of [...CANDIDATES, ...FAR]) {
      const box = boxFor(star, c, w, h);
      const cost = costOf(box, c, star, { placed, marks, plaque, trail, frame, own: marks[layout.stars.indexOf(star)] });
      if (!best || cost < best.cost) best = { cost, box, c };
    }
    if (!best) continue;
    placed.push(best.box);
    out.set(star.slug, {
      box: best.box,
      anchor: best.c.anchor,
      lines,
      meta,
      dir: best.c.dir,
      leader: best.c.far ? leaderFor(star, best.box, best.c) : null,
    });
  }
  return out;
}

/** One angle for every orbit's name, so they read as a scale up the chart. */
export function placeOrbitLabels(layout: Layout, labels: Map<string, StarLabel>, measure: Measure): Map<string, OrbitLabel> {
  const taken = [...labels.values()].map((l) => grow(l.box, 10));
  const marks = layout.stars.map((s) => grow({ x: s.x - s.r, y: s.y - s.r, w: s.r * 2, h: s.r * 2 }, 8));
  const angles = [-90, -105, -75, 90, 105, 75, -125, -55, 125, 55].map((d) => (d * Math.PI) / 180);

  let best: { cost: number; at: Map<string, OrbitLabel> } | null = null;
  for (const [i, a] of angles.entries()) {
    const at = new Map<string, OrbitLabel>();
    let cost = i * 2;
    for (const o of layout.orbits) {
      const w = measure(ORBIT_NAME[o.strength], "orbit");
      const x = layout.cx + layout.rx * o.rho * Math.cos(a);
      const below = Math.sin(a) > 0;
      const y = layout.cy + layout.ry * o.rho * Math.sin(a) + (below ? TYPE.orbit.size + 4 : -5);
      const box = { x: x - w / 2 - 4, y: y - TYPE.orbit.size, w: w + 8, h: TYPE.orbit.lineHeight };
      for (const t of [...taken, ...marks]) cost += overlap(box, t) * 4;
      at.set(o.strength, { x, y });
    }
    if (!best || cost < best.cost) best = { cost, at };
  }
  return best?.at ?? new Map();
}

function boxFor(star: Star, c: Candidate, w: number, h: number): Box {
  // Clear the pinned mark's ring (radius ~10.5) whether or not the star is
  // pinned, so pinning never crowds or moves a label.
  const gap = Math.max(star.r + 9, 16) * (c.far ? 3 : 1);
  const line = TYPE.headline.lineHeight;
  if (c.dir.y === 0) {
    // Beside the star: centre the first line of the headline on it.
    const x = c.dir.x > 0 ? star.x + gap : star.x - gap - w;
    return { x, y: star.y - line / 2 + c.offset, w, h };
  }
  if (c.dir.x === 0) {
    return { x: star.x - w / 2, y: c.dir.y < 0 ? star.y - gap - h : star.y + gap, w, h };
  }
  const g = gap * 0.8;
  const x = c.dir.x > 0 ? star.x + g : star.x - g - w;
  const y = c.dir.y < 0 ? star.y - g - h + line * 0.2 : star.y + g - line * 0.2;
  return { x, y, w, h };
}

interface Obstacles {
  placed: Box[];
  marks: Box[];
  plaque: Box;
  trail: Point[];
  frame: Box;
  own: Box | undefined;
}

function costOf(box: Box, c: Candidate, star: Star, o: Obstacles): number {
  const padded = grow(box, 4);
  let cost = 0;
  for (const p of o.placed) cost += overlap(padded, p) * 6;
  for (const m of o.marks) if (m !== o.own) cost += overlap(padded, m) * 10;
  cost += overlap(padded, o.plaque) * 10;
  cost += (box.w * box.h - overlap(box, o.frame)) * 14;
  for (const p of o.trail) if (inside(p, box)) cost += 40;
  const len = Math.hypot(c.dir.x, c.dir.y);
  cost += (1 - (c.dir.x * star.out.x + c.dir.y * star.out.y) / len) * 34;
  // Side seats read best: the name sits level with its star.
  if (c.dir.y !== 0) cost += 14;
  cost += Math.abs(c.offset) * 0.9;
  if (c.far) cost += 220;
  return cost;
}

function leaderFor(star: Star, box: Box, c: Candidate): [Point, Point] {
  const start = { x: star.x + c.dir.x * (star.r + 3), y: star.y + c.dir.y * (star.r + 3) };
  const end = {
    x: c.anchor === "start" ? box.x - 3 : c.anchor === "end" ? box.x + box.w + 3 : box.x + box.w / 2,
    y: c.dir.y === 0 ? box.y + TYPE.headline.lineHeight / 2 : c.dir.y < 0 ? box.y + box.h + 2 : box.y - 2,
  };
  return [start, end];
}

function grow(b: Box, by: number): Box {
  return { x: b.x - by, y: b.y - by, w: b.w + by * 2, h: b.h + by * 2 };
}

function overlap(a: Box, b: Box): number {
  const w = Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x);
  const h = Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y);
  return w > 0 && h > 0 ? w * h : 0;
}

function inside(p: Point, b: Box): boolean {
  return p.x > b.x && p.x < b.x + b.w && p.y > b.y && p.y < b.y + b.h;
}
