"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefCallback } from "react";

import type { Recommendation } from "../../_data/query";
import { routeBundle, SPACING, type Target } from "./geometry";
import { arrivalOf } from "./motion";

/** The app layout's sticky header; a port tucked under it has nothing to draw from. */
const HEADER = 56;
/** Narrower than this between the columns, the layout has stacked and lines are off. */
const MIN_GUTTER = 48;
const SCROLL_SETTLE_MS = 140;
/** Half the column's 3rem soft edge: a marker past this is too faded to aim a line at. */
const FADE = 24;
/** The bundle's fade-out (see Connectors) plus a frame. A bundle hidden longer than this stops following the scroll. */
const HIDDEN_MS = 300;

/** The role holding the light, and the skills it lights. Changes only when the role does. */
export interface Light {
  readonly role: string;
  /** Lit skills, each with the seconds until its line arrives. */
  readonly lit: ReadonlyMap<string, number>;
}

export interface Line {
  readonly slug: string;
  /** Length when the line appeared, so it draws at the shared speed. */
  readonly length: number;
}

/**
 * Which lines exist and whether they show. Changes when a role lights, a line
 * appears or drops, or the bundle fades. Line geometry is not here: it
 * changes every scroll frame, so the hook writes it straight to the SVG
 * (see `bind`).
 */
export interface Threads {
  readonly role: string;
  /** Lines in port order. Null when the layout is stacked: skills still light, but nothing is drawn. */
  readonly lines: readonly Line[] | null;
  /** False once too little of the role shows between the header and the pin bar to hold its port. */
  readonly visible: boolean;
}

/**
 * Holds `Threads` outside React state, so a change mid-scroll re-renders the
 * connectors alone (through useSyncExternalStore), not the whole page.
 */
export interface ThreadsStore {
  readonly get: () => Threads | null;
  readonly subscribe: (onChange: () => void) => () => void;
}

/** Ref for an SVG element whose geometry the hook writes. */
export type Bind = (key: string) => RefCallback<SVGElement>;

export const lineKey = (role: string, slug: string) => `${role}/${slug}`;
export const portKey = (role: string) => `${role}#port`;

type Attrs = Readonly<Record<string, string>>;

function paint(el: Element, attrs: Attrs) {
  for (const name in attrs) if (el.getAttribute(name) !== attrs[name]) el.setAttribute(name, attrs[name]);
}

function createStore() {
  let value: Threads | null = null;
  const subs = new Set<() => void>();
  return {
    get: () => value,
    set: (next: Threads | null) => {
      value = next;
      subs.forEach((f) => f());
    },
    subscribe: (f: () => void) => {
      subs.add(f);
      return () => void subs.delete(f);
    },
  };
}

const sameThreads = (a: Threads | null, b: Threads | null) =>
  a === b ||
  (a !== null &&
    b !== null &&
    a.role === b.role &&
    a.visible === b.visible &&
    (a.lines === b.lines ||
      (a.lines !== null && b.lines !== null && a.lines.length === b.lines.length && a.lines.every((l, i) => l.slug === b.lines?.[i].slug))));

/**
 * Offsets that move only when the layout does. The roles card scrolls with
 * the container, so the role's anchor is fixed in container space; the skill
 * markers ride in the column's own scroll, so they are fixed in its content
 * space. A scroll frame then needs only where the container and the column
 * sit now.
 */
interface Layout {
  readonly role: string | null;
  readonly viewH: number;
  readonly colClient: number;
  readonly colScroll: number;
  readonly stacked: boolean;
  /** The card's left edge, in container x. */
  readonly originX: number;
  /** The column's right edge, in container x. */
  readonly gutterLeft: number;
  /** The headline's middle and its row's extent, in container y. */
  readonly anchor: { readonly mid: number; readonly top: number; readonly bottom: number } | null;
  /** Each connected skill's marker: x in container space, y in the column's scroll content. */
  readonly marks: readonly Target[];
}

function readLayout(
  active: Recommendation | null,
  container: HTMLElement,
  column: HTMLElement,
  card: HTMLElement,
  skillEls: ReadonlyMap<string, HTMLElement>,
  roleEls: ReadonlyMap<string, HTMLElement>,
): Layout {
  const box = container.getBoundingClientRect();
  const col = column.getBoundingClientRect();
  const cardBox = card.getBoundingClientRect();
  const head = active ? roleEls.get(active.roleSlug) : undefined;
  const anchor = head?.getBoundingClientRect();
  const item = head?.closest("li")?.getBoundingClientRect();
  const scrollTop = column.scrollTop;
  const marks = (active?.connects ?? []).flatMap((c): Target[] => {
    const r = skillEls.get(c.slug)?.getBoundingClientRect();
    return r ? [{ slug: c.slug, x: r.right - box.left - 0.5, y: r.top + r.height / 2 - col.top + scrollTop }] : [];
  });
  return {
    role: active?.roleSlug ?? null,
    viewH: window.innerHeight,
    colClient: column.clientHeight,
    colScroll: column.scrollHeight,
    stacked: cardBox.left - col.right < MIN_GUTTER,
    originX: cardBox.left - box.left,
    gutterLeft: col.right - box.left,
    anchor: anchor && item ? { mid: anchor.top + anchor.height / 2 - box.top, top: item.top - box.top, bottom: item.bottom - box.top } : null,
    marks,
  };
}

/** The band of the skill column that shows, in viewport y, short of any soft edge. */
function seen(col: DOMRect, moreAbove: boolean, moreBelow: boolean) {
  return { top: col.top + (moreAbove ? FADE : 0), bottom: col.bottom - (moreBelow ? FADE : 0) };
}

type Register = (key: string) => (el: HTMLElement | null) => void;

function useRegistry(): [Map<string, HTMLElement>, Register] {
  const [els] = useState(() => new Map<string, HTMLElement>());
  const [refs] = useState(() => new Map<string, (el: HTMLElement | null) => void>());
  const register = useCallback<Register>(
    (key) => {
      let ref = refs.get(key);
      if (!ref) {
        ref = (el) => {
          if (el) els.set(key, el);
          else els.delete(key);
        };
        refs.set(key, ref);
      }
      return ref;
    },
    [els, refs],
  );
  return [els, register];
}

/**
 * Routes a bundle from the active role to the skills it connects, and keeps
 * it on them through scroll, resize, and font load.
 *
 * A scroll frame is cheap by construction. Offsets only a layout change can
 * move are cached (see `Layout`) and re-read on resize, on any size change of
 * the columns, on font load, and once when a scroll settles. A scroll frame
 * reads four values, all before any write, so none forces a layout; routes in
 * pure code; and writes path geometry to the SVG directly. React renders the
 * connectors only when the set of lines or their visibility changes, and the
 * page only when the light moves to another role.
 */
export function useThreads(active: Recommendation | null, reduceMotion: boolean) {
  const containerRef = useRef<HTMLDivElement>(null);
  const columnRef = useRef<HTMLElement>(null);
  const cardRef = useRef<HTMLElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const [skillEls, registerSkill] = useRegistry();
  const [roleEls, registerRole] = useRegistry();
  const [light, setLight] = useState<Light | null>(null);
  const [store] = useState(createStore);
  const layout = useRef<Layout | null>(null);
  /** Current geometry by element key, for the active role only. */
  const geometry = useRef<ReadonlyMap<string, Attrs>>(new Map());
  const hiddenSince = useRef<number | null>(null);
  const lastScroll = useRef(-Infinity);
  /** True while the page is mid-scroll: a role sliding under a still pointer is not a hover. */
  const isScrolling = useCallback(() => performance.now() - lastScroll.current < SCROLL_SETTLE_MS, []);

  // Keyed by role as well as slug, so the outgoing bundle keeps its last
  // geometry while it fades and only the active one follows the scroll.
  const [svgEls] = useState(() => new Map<string, SVGElement>());
  const [binds] = useState(() => new Map<string, RefCallback<SVGElement>>());
  const bind = useCallback<Bind>(
    (key) => {
      let ref = binds.get(key);
      if (!ref) {
        ref = (el) => {
          if (!el) return;
          svgEls.set(key, el);
          // A line mounting mid-scroll takes the latest geometry before paint.
          const attrs = geometry.current.get(key);
          if (attrs) paint(el, attrs);
          return () => {
            if (svgEls.get(key) === el) svgEls.delete(key);
          };
        };
        binds.set(key, ref);
      }
      return ref;
    },
    [binds, svgEls],
  );

  /** One pass: read, route, write. `relayout` re-reads the cached offsets; `relight` re-times the skills' arrival. */
  const update = useCallback(
    (relayout: boolean, relight: boolean) => {
      const container = containerRef.current;
      const column = columnRef.current;
      const card = cardRef.current;
      if (!container || !column || !card) return;
      if (relayout || layout.current?.role !== (active?.roleSlug ?? null)) {
        layout.current = readLayout(active, container, column, card, skillEls, roleEls);
      }
      const L = layout.current;

      // Reads first, all of them, so no write in this pass forces a layout.
      const scrollTop = column.scrollTop;
      const live = active !== null && !L.stacked;
      const box = live ? container.getBoundingClientRect() : null;
      const col = live ? column.getBoundingClientRect() : null;
      const barTop = live ? (barRef.current?.getBoundingClientRect().top ?? Infinity) : Infinity;

      const moreAbove = scrollTop > 1;
      const moreBelow = scrollTop + L.colClient < L.colScroll - 1;
      let next: Threads | null = null;
      let lit: Map<string, number> | null = null;
      const attrs = new Map<string, Attrs>();

      if (active && L.stacked) {
        next = { role: active.roleSlug, lines: null, visible: false };
        if (relight) lit = new Map(active.connects.map((c) => [c.slug, 0]));
      } else if (active && box && col) {
        // A skill scrolled out of its own column, or under the column's soft
        // edge (only when the column overflows), gets no line: a line to
        // something off screen points at nothing.
        const band = seen(col, moreAbove, moreBelow);
        const targets = L.marks.flatMap((m): Target[] => {
          const y = col.top + m.y - scrollTop;
          return y >= band.top && y <= band.bottom ? [{ slug: m.slug, x: m.x, y: y - box.top }] : [];
        });

        // The port sits level with the role's headline, and slides along the
        // role's visible edge as it scrolls under the header or the pin bar,
        // so the bundle leaves from what the reader can still see of the role.
        const reach = ((Math.max(targets.length, 1) - 1) * SPACING) / 2 + 16;
        const floor = Math.min(L.viewH, barTop);
        let visible = false;
        let originY = box.top;
        if (L.anchor) {
          const lo = Math.max(box.top + L.anchor.top, HEADER) + reach;
          const hi = Math.min(box.top + L.anchor.bottom, floor) - reach;
          visible = lo <= hi;
          originY = Math.min(Math.max(box.top + L.anchor.mid, lo), hi);
        }

        const bundle = routeBundle({ x: L.originX, y: originY - box.top }, L.gutterLeft, targets);
        next = { role: active.roleSlug, lines: bundle.routes.map((r) => ({ slug: r.slug, length: r.length })), visible };
        if (relight) {
          const arrival = new Map(bundle.routes.map((r) => [r.slug, reduceMotion ? 0 : arrivalOf(r.length)]));
          lit = new Map(active.connects.map((c) => [c.slug, arrival.get(c.slug) ?? 0]));
        }

        attrs.set(portKey(active.roleSlug), {
          x: String(L.originX - 1.5),
          y: String(bundle.portTop - 7),
          height: String(bundle.portBottom - bundle.portTop + 14),
        });
        for (const r of bundle.routes) attrs.set(lineKey(active.roleSlug, r.slug), { d: r.d });
      }

      // Writes. The soft-edge hint is styling, not state worth a render.
      if (column.dataset.moreAbove !== String(moreAbove)) column.dataset.moreAbove = String(moreAbove);
      if (column.dataset.moreBelow !== String(moreBelow)) column.dataset.moreBelow = String(moreBelow);

      geometry.current = attrs;
      const now = performance.now();
      if (next?.visible) hiddenSince.current = null;
      else hiddenSince.current ??= now;
      // Once the bundle has faded out its lines need not follow the scroll;
      // the frame it shows again writes them all.
      if (now - (hiddenSince.current ?? now) <= HIDDEN_MS) {
        for (const [key, a] of attrs) {
          const el = svgEls.get(key);
          if (el) paint(el, a);
        }
      }

      if (relight) setLight(active && lit ? { role: active.roleSlug, lit } : null);
      if (!sameThreads(store.get(), next)) store.set(next);
    },
    [active, reduceMotion, roleEls, skillEls, store, svgEls],
  );

  // Before paint, so a newly lit role never shows a frame of stale lines.
  useLayoutEffect(() => {
    // When the skill column overflows a short viewport and a lit skill sits
    // outside the part of it that shows, centre the lit skills before
    // measuring. If they already show, leave the reader's scroll alone.
    const column = columnRef.current;
    if (active && column && column.scrollHeight > column.clientHeight + 1) {
      const col = column.getBoundingClientRect();
      const marks = active.connects.flatMap((c) => skillEls.get(c.slug)?.getBoundingClientRect() ?? []);
      if (marks.length > 0) {
        const top = Math.min(...marks.map((r) => r.top));
        const bottom = Math.max(...marks.map((r) => r.bottom));
        const band = seen(col, column.dataset.moreAbove === "true", column.dataset.moreBelow === "true");
        if (top < band.top || bottom > band.bottom) {
          const mid = (top + bottom) / 2 - col.top + column.scrollTop;
          column.scrollTo({ top: mid - column.clientHeight / 2 });
        }
      }
    }
    update(true, true);
  }, [active, update, skillEls]);

  useEffect(() => {
    let frame = 0;
    let relayout = false;
    let settle = 0;
    const run = () => {
      frame = 0;
      const r = relayout;
      relayout = false;
      update(r, false);
    };
    const schedule = (stale: boolean) => {
      relayout ||= stale;
      if (!frame) frame = requestAnimationFrame(run);
    };
    const onScroll = () => {
      lastScroll.current = performance.now();
      // Insurance for anything that moves the layout without resizing an
      // observed box: one full re-read per gesture, not per frame.
      window.clearTimeout(settle);
      settle = window.setTimeout(() => schedule(true), SCROLL_SETTLE_MS);
      schedule(false);
    };
    const onLayout = () => schedule(true);
    window.addEventListener("scroll", onScroll, { passive: true, capture: true });
    window.addEventListener("resize", onLayout);
    const ro = new ResizeObserver(onLayout);
    const column = columnRef.current;
    // The column's own box is viewport-sized; its sections are what grow
    // when its content reflows.
    for (const el of [containerRef.current, column, cardRef.current, ...(column ? Array.from(column.children) : [])]) if (el) ro.observe(el);
    document.fonts?.ready.then(onLayout);
    return () => {
      cancelAnimationFrame(frame);
      window.clearTimeout(settle);
      window.removeEventListener("scroll", onScroll, { capture: true });
      window.removeEventListener("resize", onLayout);
      ro.disconnect();
    };
  }, [update]);

  return { light, threads: store as ThreadsStore, bind, isScrolling, containerRef, columnRef, cardRef, barRef, registerSkill, registerRole };
}
