// Map geometry: where the past sits, where each role orbits, and the path
// between them. Pure functions of the data and the pixel size of the map, so
// the layout is recomputed for the real width rather than scaled — scaling a
// viewBox would scale the type with it.
//
// Placement rules (contract: place by rank and strength, never by score):
//   - Orbit band comes from `strength`; position inside the band from `rank`.
//   - Angle comes from the past title the role builds on. Each title owns a
//     sector sized by how many roles build on it, and its roles fan through
//     that sector in rank order, so a path visibly splays from one title.

import type { DiscoveryData, Recommendation, Strength } from "../_data/query";

export interface Point {
  x: number;
  y: number;
}

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type FontRole = "headline" | "meta" | "home" | "orbit";
export type Measure = (text: string, role: FontRole) => number;

/**
 * Canvas font strings mirror the SVG text styles, so measured widths match
 * rendered ones. The scale steps as a set: the past titles at the centre are
 * the largest type on the map, role names next, and the posting counts and
 * orbit names sit at the 14px floor, set apart by face and ink rather than
 * by shrinking further.
 */
export const TYPE: Record<FontRole, { size: number; lineHeight: number; font: string; tracking: number; weight: number }> = {
  home: { size: 18, lineHeight: 22, weight: 500, tracking: 0, font: '500 18px "Funnel Display Variable", sans-serif' },
  headline: { size: 16, lineHeight: 19, weight: 500, tracking: 0, font: '500 16px "Funnel Display Variable", sans-serif' },
  meta: { size: 14, lineHeight: 17, weight: 400, tracking: 0, font: '400 14px "Schibsted Grotesk Variable", sans-serif' },
  orbit: { size: 14, lineHeight: 17, weight: 500, tracking: 0.04, font: '500 14px "Schibsted Grotesk Variable", sans-serif' },
};

/** Group key for roles whose closestPast is null: anchored to the person, not a title. */
export const WHOLE_KEY = "\u0000whole";

export function groupKeyOf(rec: Recommendation): string {
  return rec.closestPast?.titleText ?? WHOLE_KEY;
}

export interface HomeLine {
  key: string;
  lines: string[];
  whole: boolean;
  members: number;
  /** Middle of the text block; paths leave from its ends. */
  y: number;
  width: number;
  exits: { left: Point; right: Point };
}

export interface Plaque {
  box: Box;
  lines: HomeLine[];
}

export interface Star {
  slug: string;
  rank: number;
  strength: Strength;
  groupKey: string;
  angle: number;
  x: number;
  y: number;
  /** Mark radius: discrete by strength, like stellar magnitude. */
  r: number;
  /** Unit vector from the centre through the star: "outward". */
  out: Point;
  /** Cubic from the home line's exit to the star. */
  curve: [Point, Point, Point, Point];
  path: string;
}

export interface Orbit {
  strength: Strength;
  rho: number;
}

export interface Layout {
  width: number;
  height: number;
  cx: number;
  cy: number;
  rx: number;
  ry: number;
  plaque: Plaque;
  stars: Star[];
  orbits: Orbit[];
}

const MARK_RADIUS: Record<Strength, number> = { close: 5.5, adjacent: 4.25, stretch: 3.25 };
const TIERS: readonly Strength[] = ["close", "adjacent", "stretch"];
// Space a sector keeps either side of its roles, in role-widths. Keeps fans
// from different titles visibly apart.
const SECTOR_PAD = 0.9;
const HOME_WRAP = 240;
const HOME_GAP = 7;

export function layoutConstellation(
  data: Pick<DiscoveryData, "pastRoles" | "recommendations">,
  width: number,
  height: number,
  measure: Measure,
): Layout {
  const cx = width / 2;
  const cy = height / 2;
  const ry = Math.max(150, height / 2 - 58);
  // Labels sit beside stars, so the map needs more room across than down.
  const rx = Math.min(Math.max(150, width / 2 - 150), ry * 1.18);

  const groups = buildGroups(data);
  const sectors = allocateSectors(groups.filter((g) => g.members.length > 0));

  const plaque = layoutPlaque(groups, sectors, cx, cy, measure);

  // The inner orbit has to clear the plaque, or the closest role sits on its text.
  const margin = 30;
  const a = plaque.box.w / 2 + margin;
  const b = plaque.box.h / 2 + margin;
  const rho0 = Math.min(0.56, Math.max(0.3, Math.sqrt((a / rx) ** 2 + (b / ry) ** 2)));

  const present = TIERS.filter((t) => data.recommendations.some((r) => r.strength === t));
  const bandGap = 0.07;
  const bandWidth = present.length > 0 ? (1 - rho0 - bandGap * (present.length - 1)) / present.length : 0;

  const rhoOf = new Map<string, number>();
  const orbits: Orbit[] = present.map((tier, i) => {
    const start = rho0 + i * (bandWidth + bandGap);
    const members = data.recommendations.filter((r) => r.strength === tier);
    members.forEach((r, j) => {
      const t = members.length === 1 ? 0.5 : j / (members.length - 1);
      rhoOf.set(r.roleSlug, start + t * bandWidth);
    });
    return { strength: tier, rho: start + bandWidth / 2 };
  });

  const angleOf = new Map<string, number>();
  for (const s of sectors) {
    const unit = s.span / s.weight;
    const slots = middleOutSlots(s.members.length);
    s.members.forEach((r, i) => angleOf.set(r.roleSlug, s.start + unit * (SECTOR_PAD / 2 + (slots[i] ?? i) + 0.5)));
  }

  const lineByKey = new Map(plaque.lines.map((l) => [l.key, l]));
  const stars: Star[] = data.recommendations.map((rec) => {
    const angle = angleOf.get(rec.roleSlug) ?? 0;
    const rho = rhoOf.get(rec.roleSlug) ?? 1;
    const x = cx + rx * rho * Math.cos(angle);
    const y = cy + ry * rho * Math.sin(angle);
    const len = Math.hypot(x - cx, y - cy) || 1;
    const out = { x: (x - cx) / len, y: (y - cy) / len };
    const key = groupKeyOf(rec);
    const home = lineByKey.get(key);
    const side = x >= cx ? 1 : -1;
    const exit = home ? (side > 0 ? home.exits.right : home.exits.left) : { x: cx, y: cy };
    const edge = side > 0 ? plaque.box.x + plaque.box.w : plaque.box.x;
    const curve = curveFrom(exit, side, { x, y }, out, home ? Math.abs(edge - exit.x) : 0);
    return {
      slug: rec.roleSlug,
      rank: rec.rank,
      strength: rec.strength,
      groupKey: key,
      angle,
      x,
      y,
      r: MARK_RADIUS[rec.strength],
      out,
      curve,
      path: `M${f(curve[0].x)},${f(curve[0].y)} C${f(curve[1].x)},${f(curve[1].y)} ${f(curve[2].x)},${f(curve[2].y)} ${f(curve[3].x)},${f(curve[3].y)}`,
    };
  });

  return { width, height, cx, cy, rx, ry, plaque, stars, orbits };
}

interface Group {
  key: string;
  label: string;
  order: number;
  members: Recommendation[];
}

interface Sector {
  key: string;
  members: Recommendation[];
  weight: number;
  start: number;
  span: number;
  centre: number;
}

function buildGroups(data: Pick<DiscoveryData, "pastRoles" | "recommendations">): Group[] {
  const groups = new Map<string, Group>();
  for (const p of data.pastRoles) {
    if (!groups.has(p.titleText)) groups.set(p.titleText, { key: p.titleText, label: p.titleText, order: groups.size, members: [] });
  }
  for (const rec of data.recommendations) {
    const key = groupKeyOf(rec);
    const g = groups.get(key) ?? { key, label: rec.closestPast?.titleText ?? "", order: groups.size, members: [] };
    g.members.push(rec);
    groups.set(key, g);
  }
  return [...groups.values()];
}

// Balance the circle: deal groups into a right and a left half by weight, the
// biggest in the middle of each half, then centre the right half on 3 o'clock.
// Side-placed labels stack best down the left and right, so that is where the
// largest fans go.
function allocateSectors(groups: Group[]): Sector[] {
  const weighted = groups
    .map((g) => ({ g, w: g.members.length + SECTOR_PAD }))
    .sort((p, q) => q.w - p.w || p.g.order - q.g.order);
  const right: typeof weighted = [];
  const left: typeof weighted = [];
  let wr = 0;
  let wl = 0;
  for (const item of weighted) {
    if (wr <= wl) {
      right.push(item);
      wr += item.w;
    } else {
      left.push(item);
      wl += item.w;
    }
  }
  const middleOut = (xs: typeof weighted) => xs.reduce<typeof weighted>((acc, x, i) => (i % 2 === 0 ? [...acc, x] : [x, ...acc]), []);
  const ring = [...middleOut(right), ...middleOut(left)];
  const total = wr + wl;
  if (total === 0) return [];

  let angle = -Math.PI * (wr / total);
  return ring.map(({ g, w }) => {
    const span = (w / total) * Math.PI * 2;
    const s = { key: g.key, members: g.members, weight: w, start: angle, span, centre: angle + span / 2 };
    angle += span;
    return s;
  });
}

// Rank 1 takes the middle of its sector, straight out from the title it
// builds on; later ranks alternate either side and, sitting on farther
// orbits, open the fan outward.
function middleOutSlots(n: number): number[] {
  const mid = Math.floor((n - 1) / 2);
  return Array.from({ length: n }, (_, i) => mid + (i % 2 === 0 ? i / 2 : -(i + 1) / 2) * (n % 2 === 0 ? -1 : 1));
}

function layoutPlaque(
  groups: Group[],
  sectors: Sector[],
  cx: number,
  cy: number,
  measure: Measure,
): Plaque {
  // Lines run top to bottom in the order their fans leave the circle, so paths
  // exiting the same side never cross each other at the plaque.
  const centreOf = new Map(sectors.map((s) => [s.key, s.centre]));
  const ordered = [...groups].sort((p, q) => {
    const a = centreOf.get(p.key);
    const b = centreOf.get(q.key);
    if (a === undefined || b === undefined) return a === undefined ? (b === undefined ? p.order - q.order : 1) : -1;
    return Math.sin(a) - Math.sin(b);
  });

  const lh = TYPE.home.lineHeight;

  const blocks = ordered.map((g) => {
    const whole = g.key === WHOLE_KEY;
    const text = whole ? "Your profile as a whole" : g.label;
    const lines = wrapText(text, HOME_WRAP, "home", measure);
    const width = Math.max(...lines.map((l) => measure(l, "home")));
    return { g, whole, lines, width, h: lines.length * lh };
  });

  const h = blocks.reduce((sum, b) => sum + b.h, 0) + HOME_GAP * Math.max(0, blocks.length - 1);
  const w = Math.max(...blocks.map((b) => b.width), 60);
  const top = cy - h / 2;

  let y = top;
  const lines: HomeLine[] = blocks.map((b) => {
    const mid = y + b.h / 2;
    y += b.h + HOME_GAP;
    const reach = b.width / 2 + 10;
    return {
      key: b.g.key,
      lines: b.lines,
      whole: b.whole,
      members: b.g.members.length,
      y: mid,
      width: b.width,
      exits: { left: { x: cx - reach, y: mid }, right: { x: cx + reach, y: mid } },
    };
  });

  return { box: { x: cx - w / 2, y: top, w, h }, lines };
}

// Leaves level and runs past the plaque's edge before turning, so a path
// never strikes through another title; then arrives along the star's own
// radial, so a fan reads as rays from one point rather than a tangle of
// chords. The inward pull on arrival is capped, or a star just above the
// plaque would hook back over it.
function curveFrom(exit: Point, side: number, star: Point, out: Point, clear: number): [Point, Point, Point, Point] {
  const d = Math.hypot(star.x - exit.x, star.y - exit.y);
  const dx = Math.abs(star.x - exit.x);
  const c1 = { x: exit.x + side * Math.max(clear + 40, dx * 0.55), y: exit.y };
  const pull = Math.min(d * 0.36, 70);
  const c2 = { x: star.x - out.x * pull, y: star.y - out.y * pull };
  return [exit, c1, c2, star];
}

export function pointOnCurve([p0, p1, p2, p3]: [Point, Point, Point, Point], t: number): Point {
  const u = 1 - t;
  const a = u * u * u;
  const b = 3 * u * u * t;
  const c = 3 * u * t * t;
  const d = t * t * t;
  return { x: a * p0.x + b * p1.x + c * p2.x + d * p3.x, y: a * p0.y + b * p1.y + c * p2.y + d * p3.y };
}

export function wrapText(text: string, max: number, role: FontRole, measure: Measure): string[] {
  const words = text.split(/\s+/).filter(Boolean);
  const lines: string[] = [];
  let line = "";
  for (const word of words) {
    const next = line ? `${line} ${word}` : word;
    if (line && measure(next, role) > max) {
      lines.push(line);
      line = word;
    } else {
      line = next;
    }
  }
  if (line) lines.push(line);
  return lines.length > 0 ? lines : [""];
}

const f = (n: number) => Math.round(n * 10) / 10;
