// Connector routing between a role and the skills it draws on. Pure: measured
// positions in, SVG path strings out, so the routing can be reasoned about
// without a DOM.
//
// Every route is orthogonal: out of the role's port, along to its own lane in
// the gutter, up or down the lane, then along to the skill. Three rules keep a
// bundle legible instead of a smear:
//
// - Each line leaves from its own port, SPACING apart, ordered like its
//   target. No two lines share a pixel. A skill level with the role gets a
//   straight line, and the other ports pack above and below it.
// - Within the lines that turn up (or down), the farthest target turns first,
//   on the lane nearest the role. That ordering is the one that never crosses:
//   a nearer line's last leg stays below the farther line's lane.
// - Ports and lanes share one spacing, so each line's first corner is
//   concentric with its neighbours': the bundle bends like a ribbon.

export const SPACING = 6;
/** Corner radius for the innermost line, and for every corner at a skill. */
export const RADIUS = 10;
/** How far off the role's level a skill can sit and still be reached in a straight line. */
const LEVEL = 12;
/** Shortest run out of the port before the first lane. */
const STUB = RADIUS + 10;

export interface Target {
  readonly slug: string;
  /** Where the line ends: the skill's marker, right edge. */
  readonly x: number;
  readonly y: number;
}

export interface Route {
  readonly slug: string;
  readonly d: string;
  /** Path length in px, so every line can draw at the same speed. */
  readonly length: number;
  readonly portY: number;
}

export interface Bundle {
  readonly routes: readonly Route[];
  /** Top and bottom of the port cluster on the role's edge. */
  readonly portTop: number;
  readonly portBottom: number;
}

export function routeBundle(origin: { x: number; y: number }, gutterLeft: number, targets: readonly Target[]): Bundle {
  const sorted = [...targets].sort((a, b) => a.y - b.y);
  const n = sorted.length;
  // A skill nearly level with the role runs straight across rather than
  // jogging a few pixels; the cluster anchors on it. Otherwise the cluster
  // centres on the role.
  let level = -1;
  sorted.forEach((t, k) => {
    if (Math.abs(t.y - origin.y) <= LEVEL && (level < 0 || Math.abs(t.y - origin.y) < Math.abs(sorted[level].y - origin.y))) level = k;
  });
  const ports =
    level >= 0
      ? sorted.map((_, k) => sorted[level].y + (k - level) * SPACING)
      : sorted.map((_, k) => origin.y + (k - (n - 1) / 2) * SPACING);
  const dir = sorted.map((t, k) => Math.sign(Math.round(t.y - ports[k])));

  // Targets are at least a row apart and ports only SPACING apart, so the
  // lines that turn up form a prefix and the lines that turn down a suffix.
  const up = dir.filter((s) => s < 0).length;
  const down = dir.filter((s) => s > 0).length;

  // The lane bundle sits centred in the gutter, never closer to the role than
  // the stub allows.
  const widest = Math.max(up, down, 1);
  const centre = (gutterLeft + origin.x) / 2;
  const rightLane = Math.min(centre + ((widest - 1) * SPACING) / 2, origin.x - STUB);

  const routes = sorted.map((t, k): Route => {
    const p = ports[k];
    const sy = dir[k];
    if (sy === 0) {
      return { slug: t.slug, d: `M${origin.x} ${p}H${t.x}`, length: origin.x - t.x, portY: p };
    }
    // Rank from the outside of the bundle: 0 is the farthest target.
    const rank = sy < 0 ? k : n - 1 - k;
    const lane = rightLane - rank * SPACING;
    const rise = Math.abs(t.y - p);

    let r1 = Math.min(RADIUS + rank * SPACING, origin.x - lane);
    let r2 = Math.min(RADIUS, lane - t.x);
    // A short rise cannot hold both corners at full size; shrink them together.
    if (r1 + r2 > rise) {
      const k2 = rise / (r1 + r2);
      r1 *= k2;
      r2 *= k2;
    }

    // In SVG's y-down space, west-then-north is a clockwise turn (sweep 1).
    const sweep1 = sy < 0 ? 1 : 0;
    const sweep2 = sy < 0 ? 0 : 1;
    const d = [
      `M${f(origin.x)} ${f(p)}`,
      `H${f(lane + r1)}`,
      `A${f(r1)} ${f(r1)} 0 0 ${sweep1} ${f(lane)} ${f(p + sy * r1)}`,
      `V${f(t.y - sy * r2)}`,
      `A${f(r2)} ${f(r2)} 0 0 ${sweep2} ${f(lane - r2)} ${f(t.y)}`,
      `H${f(t.x)}`,
    ].join("");
    const length = origin.x - lane - r1 + (rise - r1 - r2) + (lane - r2 - t.x) + (Math.PI / 2) * (r1 + r2);
    return { slug: t.slug, d, length, portY: p };
  });

  return {
    routes,
    portTop: ports[0] ?? origin.y,
    portBottom: ports[n - 1] ?? origin.y,
  };
}

const f = (v: number) => Math.round(v * 100) / 100;
