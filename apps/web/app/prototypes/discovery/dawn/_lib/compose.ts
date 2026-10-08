// Compositor lines, one layout pass at a time. Pure: the cached layout and two
// scroll offsets in, element attributes out.
//
// Each line is cut into pieces that each sit still in one frame (see
// `piecesOf`), so steady scrolling needs nothing from script:
//
// - Port end (stub, first corner, the port itself): in a layer whose
//   translateY plays the port's track on a page scroll timeline. The track
//   reproduces the port's clamp to the role's visible edge, so the port rides
//   with the card and slides at the header and the pin bar.
// - Skill end (last corner, stub): in a layer inside a box that sticks exactly
//   as the skill column does, shifted by the column's own scroll on a column
//   scroll timeline. It never leaves its marker. A line whose skill is
//   scrolled out ends on an edge marker instead (see `edges.ts`), which sits
//   still in the column's box: its skill end lives in a layer of that box
//   with no scroll timeline, and the column's scroll never moves it.
// - Run: the vertical span between the two corners, whose ends live in
//   different frames. It is a bar hung from the port end, clipped by a box
//   hung from the skill end. Bar and clip each move with their own frame, so
//   the visible span is always exactly corner to corner. One bar-and-clip pair
//   reaches down and another up, so the run shows whichever way the ends lie,
//   even in the frame before script notices a line has flipped.
//
// Script still decides the discrete things, a frame late: which way a line
// turns, its lane, and its corner radii as the run gets short. A line flips
// only as its port passes its skill, when its corners have shrunk to nothing
// and the line runs straight, so the flip and the lane change it brings are
// invisible. While the page moves, corners are sized for the next frame's
// shorter run (`slack`), so a late update leaves a little straight run rather
// than corners that overlap.

import type { Recommendation } from "../../_data/query";
import { endsOf, type Routing } from "./edges";
import { piecesOf, routeBundle, type Route, type Target } from "./geometry";
import type { Layout } from "./layout";
import { HEADER } from "./layout";
import { threadAt, type Span } from "./light";
import { frameKey, lineKey, pieceKey, portKey, type Attrs, type Line } from "./store";
import { portTrack, valueAt, type Track } from "./track";
import { offsets } from "./weight";

export interface Frame {
  readonly lines: readonly Line[];
  readonly routes: readonly Route[];
  readonly visible: boolean;
  readonly attrs: ReadonlyMap<string, Attrs>;
}

/** Half the port cluster's height, for lines of these widths, plus room to spare at the band's ends. */
export const reachOf = (widths: readonly number[]) => (offsets(widths).at(-1) ?? 0) / 2 + 16;

export function trackOf(L: Layout, reach: number): Track | null {
  if (!L.anchor) return null;
  return portTrack({
    range: L.range,
    at: L.at,
    viewport: L.viewport,
    header: HEADER,
    mid: L.boxTop + L.anchor.mid,
    top: L.boxTop + L.anchor.top,
    bottom: L.boxTop + L.anchor.bottom,
    reach,
    barStuck: L.barStuck,
    barFlow: L.barFlow,
    colStick: L.colStick,
    colFlow: L.colFlow,
    colFloor: L.colFloor,
    colHeight: L.colHeight,
  });
}

const px = (v: number) => `${Math.round(v * 100) / 100}px`;

export function composeFrame(
  active: Recommendation,
  L: Layout,
  track: Track,
  /** Which lit skills land on an edge marker. */
  routing: Routing,
  s: number,
  c: number,
  /** Each line's rise at the last pass; updated in place. */
  rises: Map<string, number>,
  withPath: boolean,
  /** The thread's gradient, so each run takes the colour of the x it stands at. */
  span: Span,
): Frame {
  const role = active.roleSlug;
  const offset = valueAt(track.offset, s);
  const visible = valueAt(track.low, s) <= valueAt(track.high, s);
  const big = 2 * L.viewport + 200;

  // Everything below is in the skill column's frame: x from its left edge, y
  // from the port's middle (port end) or from each marker (skill end). A
  // skill's marker scrolls with the column; an edge marker does not.
  const originX = L.originX - L.colLeft;
  const ends = endsOf(L, routing);
  const targets = ends.map((m): Target => {
    const rise = m.y - (m.edge ? 0 : c) - offset;
    const last = rises.get(m.slug);
    rises.set(m.slug, rise);
    // Only a shrinking run can outpace corners sized a pass ago; a growing
    // one leaves them small, which reads as a tighter bend, not a fault.
    const closing = last === undefined ? 0 : Math.abs(last) - Math.abs(rise);
    return { slug: m.slug, x: m.x - L.colLeft, y: rise, slack: closing > 0.01 ? closing * 1.5 + 1 : 0, width: m.width };
  });
  const bundle = routeBundle({ x: originX, y: 0 }, L.gutterLeft - L.colLeft, targets, { snapLevel: false, lanesForAll: true });

  const attrs = new Map<string, Attrs>();
  const port = { x: String(originX - 1.5), y: String(bundle.portTop - 7), height: String(bundle.portBottom - bundle.portTop + 14) };
  attrs.set(pieceKey(role, "", "port"), port);
  const markY = new Map(ends.map((m) => [m.slug, m.y]));
  const edgeOf = new Map(ends.map((m) => [m.slug, m.edge]));
  for (const r of bundle.routes) {
    const p = piecesOf(r, originX);
    const y = markY.get(r.slug) ?? 0;
    attrs.set(pieceKey(role, r.slug, "port"), { d: p.port });
    attrs.set(pieceKey(role, r.slug, "mark"), { d: p.mark, transform: `translate(0 ${Math.round(y * 100) / 100})` });
    const meet = y + p.runTo;
    const tone = threadAt(span, r.lane + L.colLeft);
    // Every piece of a line takes its one width, so they agree where they meet.
    const w = r.target.width ?? 1.5;
    for (const down of [true, false]) {
      const dir = down ? "down" : "up";
      const clipTop = down ? meet - big : meet;
      attrs.set(pieceKey(role, r.slug, `clip-${dir}`), { style: `top:${px(clipTop)};height:${px(big)}` });
      attrs.set(pieceKey(role, r.slug, `unclip-${dir}`), { style: `top:${px(-clipTop)}` });
      attrs.set(pieceKey(role, r.slug, `bar-${dir}`), {
        style: `left:${px(r.lane - w / 2)};top:${px(down ? p.runFrom : p.runFrom - big)};width:${px(w)};height:${px(big)};background:${tone}`,
      });
    }
  }

  // The draw-in path: the whole route, placed by script in the page's frame.
  // It shows only until its draw-in ends, then the pieces take over.
  if (withPath) {
    const column = valueAt(track.column, s);
    const boxTop = L.boxTop - (s - L.at);
    attrs.set(frameKey(role), { transform: `translate(${Math.round(L.colLeft * 100) / 100} ${Math.round((offset + column - boxTop) * 100) / 100})` });
    attrs.set(portKey(role), port);
    for (const r of bundle.routes) attrs.set(lineKey(role, r.slug), { d: r.d });
  }

  return {
    lines: bundle.routes.map((r) => ({ slug: r.slug, length: r.length, edge: edgeOf.get(r.slug) ?? null, width: r.target.width ?? 1.5 })),
    routes: bundle.routes,
    visible,
    attrs,
  };
}
