// Where the port sits, as a function of how far the page has scrolled. Pure.
//
// Every position that moves with scroll here is piecewise linear in the scroll
// offset: content moves one pixel per pixel scrolled, and sticky boxes clamp
// that motion with min and max. A piecewise-linear function is exactly what a
// scroll-driven animation plays when its keyframes sit on the breakpoints, so
// the compositor can move the port in step with the scroll, with no frame of
// main-thread lag.

/** Breakpoints of a piecewise-linear function over scroll offsets [0, range]. */
export interface Pl {
  readonly s: readonly number[];
  readonly v: readonly number[];
}

/** Something that moves with the page: `y` measured at scroll offset `at`. */
const moving = (range: number, y: number, at: number): Pl => ({ s: [0, range], v: [y + at, y + at - range] });
const fixed = (range: number, y: number): Pl => ({ s: [0, range], v: [y, y] });

export function valueAt(f: Pl, s: number): number {
  const { s: xs, v } = f;
  if (s <= xs[0]) return v[0];
  for (let i = 1; i < xs.length; i++) {
    if (s <= xs[i]) {
      const span = xs[i] - xs[i - 1];
      return span > 0 ? v[i - 1] + ((v[i] - v[i - 1]) * (s - xs[i - 1])) / span : v[i];
    }
  }
  return v[v.length - 1];
}

/** Combine two functions point by point, adding a breakpoint wherever they cross. */
function combine(a: Pl, b: Pl, op: (x: number, y: number) => number): Pl {
  const xs = [...new Set([...a.s, ...b.s])].sort((p, q) => p - q);
  const all: number[] = [];
  for (let i = 0; i < xs.length; i++) {
    all.push(xs[i]);
    if (i === xs.length - 1) break;
    const d0 = valueAt(a, xs[i]) - valueAt(b, xs[i]);
    const d1 = valueAt(a, xs[i + 1]) - valueAt(b, xs[i + 1]);
    if (d0 * d1 < 0) all.push(xs[i] + ((xs[i + 1] - xs[i]) * d0) / (d0 - d1));
  }
  return { s: all, v: all.map((x) => op(valueAt(a, x), valueAt(b, x))) };
}

const min = (a: Pl, b: Pl) => combine(a, b, Math.min);
const max = (a: Pl, b: Pl) => combine(a, b, Math.max);
const minus = (a: Pl, b: Pl) => combine(a, b, (x, y) => x - y);
const plus = (a: Pl, k: number): Pl => ({ s: a.s, v: a.v.map((x) => x + k) });

/** Drop breakpoints that sit on the line through their neighbours. */
function simplify(f: Pl): Pl {
  const s = [f.s[0]];
  const v = [f.v[0]];
  for (let i = 1; i < f.s.length - 1; i++) {
    const [x0, y0] = [s[s.length - 1], v[v.length - 1]];
    const [x2, y2] = [f.s[i + 1], f.v[i + 1]];
    const onLine = x2 > x0 && Math.abs(y0 + ((y2 - y0) * (f.s[i] - x0)) / (x2 - x0) - f.v[i]) < 0.01;
    if (!onLine && f.s[i] - x0 > 1e-6) {
      s.push(f.s[i]);
      v.push(f.v[i]);
    }
  }
  s.push(f.s[f.s.length - 1]);
  v.push(f.v[f.v.length - 1]);
  return { s, v };
}

/** Viewport positions measured in one layout pass, at scroll offset `at`. */
export interface TrackInput {
  /** The page's scroll range, and the offset when these were measured. */
  readonly range: number;
  readonly at: number;
  readonly viewport: number;
  /** The app header's height: nothing under it can hold a port. */
  readonly header: number;
  /** The role's headline middle, and its row's extent. */
  readonly mid: number;
  readonly top: number;
  readonly bottom: number;
  /** Half the port cluster's height plus its margin. */
  readonly reach: number;
  /** The pin bar's top: where it sticks, and where it sits in flow. */
  readonly barStuck: number;
  readonly barFlow: number;
  /** The skill column's sticky top, its top in flow, and the bottom of the area it sticks within. */
  readonly colStick: number;
  readonly colFlow: number;
  readonly colFloor: number;
  readonly colHeight: number;
}

export interface Track {
  /** The port's middle, measured from the skill column's top. */
  readonly offset: Pl;
  /** The skill column's top, in the viewport. */
  readonly column: Pl;
  /** The band the port may slide along. Empty (low above high) when too little of the role shows. */
  readonly low: Pl;
  readonly high: Pl;
}

/**
 * The port slides along the role's visible edge, between the header and the
 * pin bar (the same clamp the main-thread route uses). The skill column is
 * sticky, so its top is clamped too. The port's offset from the column is the
 * difference: in that frame, a scroll-driven animation of the offset keeps the
 * port on the role card while the column moves on its own.
 */
export function portTrack(t: TrackInput): Track {
  const R = Math.max(t.range, 0);
  const page = (y: number) => moving(R, y, t.at);
  const bar = min(fixed(R, t.barStuck), page(t.barFlow));
  const floor = min(fixed(R, t.viewport), bar);
  const low = plus(max(page(t.top), fixed(R, t.header)), t.reach);
  const high = plus(min(page(t.bottom), floor), -t.reach);
  const port = min(max(page(t.mid), low), high);
  const column = min(max(page(t.colFlow), fixed(R, t.colStick)), plus(page(t.colFloor), -t.colHeight));
  return { offset: simplify(minus(port, column)), column: simplify(column), low, high };
}

/** Keyframes for a scroll-driven translateY that plays `f` over the whole scroll range. */
export function translateKeyframes(f: Pl, range: number): Keyframe[] {
  return f.s.map((s, i) => ({
    offset: range > 0 ? Math.min(1, Math.max(0, s / range)) : 0,
    transform: `translateY(${Math.round(f.v[i] * 100) / 100}px)`,
  }));
}
