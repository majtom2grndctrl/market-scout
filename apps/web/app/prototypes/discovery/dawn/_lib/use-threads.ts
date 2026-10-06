"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefCallback } from "react";

import type { Recommendation } from "../../_data/query";
import { composeFrame, reachOf, shownMarks, trackOf } from "./compose";
import { createDriver, type Drive } from "./drive";
import { routeBundle, type Target } from "./geometry";
import { HEADER, readLayout, seen, useRegistry, type Layout } from "./layout";
import type { LineMode } from "./modes";
import { arrivalOf, DRAW_DELAY, drawDuration } from "./motion";
import { createStore, lineKey, paint, portKey, sameThreads, type Attrs, type Bind, type Light, type Threads, type ThreadsStore } from "./store";
import { translateKeyframes, valueAt, type Track } from "./track";

const SCROLL_SETTLE_MS = 140;
/** The bundle's fade-out (see Connectors) plus a frame. A bundle hidden longer than this stops following the scroll. */
const HIDDEN_MS = 300;

const roleOf = (key: string) => key.split(/[/#]/)[0];

/**
 * Routes a bundle from the active role to the skills it connects, and keeps
 * it on them through scroll, resize, and font load. `mode` sets what happens
 * while the page scrolls (see `modes.ts`).
 *
 * A scroll frame is cheap by construction. Offsets only a layout change can
 * move are cached (see `Layout`) and re-read on resize, on any size change of
 * the columns, on font load, and once when a scroll settles. A scroll frame
 * reads a few values, all before any write, so none forces a layout; routes in
 * pure code; and writes geometry to the DOM directly. React renders the
 * connectors only when the set of lines or their visibility changes, and the
 * page only when the light moves to another role.
 */
export function useThreads(active: Recommendation | null, reduceMotion: boolean, mode: LineMode) {
  const containerRef = useRef<HTMLDivElement>(null);
  const columnRef = useRef<HTMLElement>(null);
  const cardRef = useRef<HTMLElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const [skillEls, registerSkill] = useRegistry();
  const [roleEls, registerRole] = useRegistry();
  const [light, setLight] = useState<Light | null>(null);
  const [store] = useState(createStore);
  const [driver] = useState(createDriver);
  const layout = useRef<Layout | null>(null);
  /** Current geometry by element key, for the active role only. */
  const geometry = useRef<ReadonlyMap<string, Attrs>>(new Map());
  const hiddenSince = useRef<number | null>(null);
  const lastScroll = useRef(-Infinity);
  /** True while the page is mid-scroll: a role sliding under a still pointer is not a hover. */
  const isScrolling = useCallback(() => performance.now() - lastScroll.current < SCROLL_SETTLE_MS, []);

  // Fade mode: lines hide while the page scrolls; each settle redraws them.
  const quiet = useRef(false);
  const epoch = useRef(0);
  /** Set while the hook scrolls the skill column itself, so fade mode doesn't take it for the reader's scroll. */
  const ownScroll = useRef(false);
  // Compositor mode: the port's track per layout, each line's last rise, and
  // whether the draw-in has handed over to the pieces.
  const track = useRef<{ layout: Layout; reach: number; track: Track | null } | null>(null);
  const rises = useRef(new Map<string, number>());
  const drawn = useRef(true);
  const drawTimer = useRef(0);

  // Keyed by role as well as slug, so the outgoing bundle keeps its last
  // geometry while it fades and only the active one follows the scroll.
  const [els] = useState(() => new Map<string, Element>());
  const [binds] = useState(() => new Map<string, RefCallback<Element>>());
  const bind = useCallback<Bind>(
    (key, drive?: Drive) => {
      const id = drive ? `${key}@${drive}` : key;
      let ref = binds.get(id);
      if (!ref) {
        ref = (el) => {
          if (!el) return;
          els.set(key, el);
          // A piece mounting mid-scroll takes the latest geometry before paint.
          const attrs = geometry.current.get(key);
          if (attrs) paint(el, attrs);
          const detach = drive ? driver.attach(el as HTMLElement, drive, roleOf(key)) : null;
          return () => {
            detach?.();
            if (els.get(key) === el) els.delete(key);
          };
        };
        binds.set(id, ref);
      }
      return ref;
    },
    [binds, els, driver],
  );

  /** One pass: read, route, write. `relayout` re-reads the cached offsets; `relight` re-times the skills' arrival. */
  const update = useCallback(
    (relayout: boolean, relight: boolean) => {
      const container = containerRef.current;
      const column = columnRef.current;
      const card = cardRef.current;
      if (!container || !column || !card) return;
      if (relayout || layout.current?.role !== (active?.roleSlug ?? null)) {
        layout.current = readLayout(active, container, column, card, barRef.current, skillEls, roleEls);
      }
      const L = layout.current;

      // Reads first, all of them, so no write in this pass forces a layout.
      const scrollTop = column.scrollTop;
      const live = active !== null && !L.stacked;
      const composite = mode === "compositor";
      const pageY = composite ? window.scrollY : 0;
      const box = live && !composite ? container.getBoundingClientRect() : null;
      const col = live && !composite ? column.getBoundingClientRect() : null;
      const barTop = live && !composite ? (barRef.current?.getBoundingClientRect().top ?? Infinity) : Infinity;

      const moreAbove = scrollTop > 1;
      const moreBelow = scrollTop + L.colClient < L.colScroll - 1;
      const hush = mode === "fade" && quiet.current;
      let next: Threads | null = null;
      let lit: Map<string, number> | null = null;
      let arrival: ReadonlyMap<string, number> = new Map();
      let attrs: ReadonlyMap<string, Attrs> = new Map();

      if (relight) {
        rises.current.clear();
        window.clearTimeout(drawTimer.current);
        drawn.current = true;
      }

      if (active && L.stacked) {
        next = { role: active.roleSlug, lines: null, visible: false, quiet: false, epoch: epoch.current, drawn: true };
      } else if (active && composite) {
        const marks = shownMarks(L, scrollTop, moreAbove, moreBelow);
        const reach = reachOf(marks.length);
        if (track.current?.layout !== L || track.current.reach !== reach) {
          const t = trackOf(L, reach);
          track.current = { layout: L, reach, track: t };
          if (t) driver.setPage(active.roleSlug, translateKeyframes(t.offset, L.range), valueAt(t.offset, L.at));
        }
        driver.setColumn(column, Math.max(0, L.colScroll - L.colClient));
        const t = track.current.track;
        if (t) {
          const frame = composeFrame(active, L, t, marks, pageY, scrollTop, rises.current, relight || !drawn.current);
          attrs = frame.attrs;
          if (relight) {
            arrival = new Map(frame.routes.map((r) => [r.slug, reduceMotion ? 0 : arrivalOf(r.length)]));
            // The draw-in path shows until its longest line lands; then the
            // compositor's pieces take over.
            const end = Math.max(0, ...frame.routes.map((r) => DRAW_DELAY + drawDuration(r.length)));
            drawn.current = reduceMotion || frame.routes.length === 0;
            if (!drawn.current) {
              const role = active.roleSlug;
              drawTimer.current = window.setTimeout(() => {
                drawn.current = true;
                const cur = store.get();
                if (cur?.role === role) store.set({ ...cur, drawn: true });
              }, end * 1000 + 40);
            }
          }
          next = { role: active.roleSlug, lines: frame.lines, visible: frame.visible, quiet: false, epoch: 0, drawn: drawn.current };
        } else {
          next = { role: active.roleSlug, lines: [], visible: false, quiet: false, epoch: 0, drawn: true };
        }
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
        const reach = reachOf(targets.length);
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
        next = {
          role: active.roleSlug,
          lines: bundle.routes.map((r) => ({ slug: r.slug, length: r.length })),
          visible,
          quiet: hush,
          epoch: epoch.current,
          drawn: true,
        };
        if (relight) arrival = new Map(bundle.routes.map((r) => [r.slug, reduceMotion ? 0 : arrivalOf(r.length)]));

        const a = new Map<string, Attrs>();
        a.set(portKey(active.roleSlug), {
          x: String(L.originX - 1.5),
          y: String(bundle.portTop - 7),
          height: String(bundle.portBottom - bundle.portTop + 14),
        });
        for (const r of bundle.routes) a.set(lineKey(active.roleSlug, r.slug), { d: r.d });
        attrs = a;
      }
      if (relight && active) lit = new Map(active.connects.map((c) => [c.slug, arrival.get(c.slug) ?? 0]));

      // Writes. The soft-edge hint is styling, not state worth a render.
      if (column.dataset.moreAbove !== String(moreAbove)) column.dataset.moreAbove = String(moreAbove);
      if (column.dataset.moreBelow !== String(moreBelow)) column.dataset.moreBelow = String(moreBelow);

      // Fade mode draws nothing mid-scroll: the hidden bundle keeps its last
      // geometry, and the redraw after the settle takes fresh geometry on mount.
      if (!hush) geometry.current = attrs;
      const now = performance.now();
      if (next?.visible) hiddenSince.current = null;
      else hiddenSince.current ??= now;
      // Once the bundle has faded out its lines need not follow the scroll;
      // the frame it shows again writes them all.
      if (!hush && now - (hiddenSince.current ?? now) <= HIDDEN_MS) {
        for (const [key, a] of attrs) {
          const el = els.get(key);
          if (el) paint(el, a);
        }
      }

      if (relight) setLight(active && lit ? { role: active.roleSlug, lit } : null);
      if (!sameThreads(store.get(), next)) store.set(next);
    },
    [active, reduceMotion, mode, roleEls, skillEls, store, els, driver],
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
          ownScroll.current = true;
          requestAnimationFrame(() => requestAnimationFrame(() => (ownScroll.current = false)));
          column.scrollTo({ top: mid - column.clientHeight / 2 });
        }
      }
    }
    update(true, true);
  }, [active, update, skillEls]);

  useEffect(() => () => window.clearTimeout(drawTimer.current), []);

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
    // Insurance for anything that moves the layout without resizing an
    // observed box: one full re-read per gesture, not per frame. In fade
    // mode the settle is also what brings the lines back.
    const arm = () => {
      window.clearTimeout(settle);
      settle = window.setTimeout(() => {
        if (quiet.current) {
          quiet.current = false;
          epoch.current += 1;
        }
        schedule(true);
      }, SCROLL_SETTLE_MS);
    };
    const onScroll = (e: Event) => {
      lastScroll.current = performance.now();
      if (mode === "fade" && !quiet.current) {
        if (ownScroll.current && e.target === columnRef.current) ownScroll.current = false;
        else quiet.current = true;
      }
      arm();
      schedule(false);
    };
    // This effect re-runs when the light moves, and its cleanup drops a
    // pending settle; a gesture still in flight needs one again.
    if (quiet.current || isScrolling()) arm();
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
  }, [update, mode, isScrolling]);

  return { light, threads: store as ThreadsStore, bind, isScrolling, containerRef, columnRef, cardRef, barRef, registerSkill, registerRole };
}
