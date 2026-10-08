"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefCallback } from "react";

import type { Recommendation } from "../../_data/query";
import { composeFrame, reachOf, trackOf } from "./compose";
import { createDriver, type Drive } from "./drive";
import { bandOf, centreScroll, NO_ROUTING, revealScroll, routingOf, type Edge, type Routing } from "./edges";
import { readLayout, useRegistry, type Layout } from "./layout";
import { spanOf } from "./light";
import type { LineMode } from "./modes";
import { arrivalOf, DRAW_DELAY, drawDuration } from "./motion";
import { routeOnPage } from "./page-route";
import { createStore, paint, sameLineSet, type Attrs, type Bind, type Light, type LineSet, type LineStore } from "./store";
import { translateKeyframes, valueAt, type Track } from "./track";

const SCROLL_SETTLE_MS = 140;
/** The bundle's fade-out (see Connectors) plus a frame. A bundle hidden longer than this stops following the scroll. */
const HIDDEN_MS = 300;

const roleOf = (key: string) => key.split(/[/#]/)[0];

/**
 * Routes a bundle from the active role to the skills it uses, and keeps
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
export function useLines(active: Recommendation | null, reduceMotion: boolean, mode: LineMode) {
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

  // Which lit skills land on an edge marker (see `edges.ts`). Compositor
  // lines keep it from one layout pass to the next, so mid-scroll a line
  // keeps its end; `released` routes every line to its own skill while an
  // edge marker scrolls them into view.
  const routing = useRef<Routing>(NO_ROUTING);
  const released = useRef(false);
  /** Arms the scroll settle from outside the effect that owns it. */
  const settleSoon = useRef<() => void>(() => {});

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
      const span = spanOf(L);

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
      let next: LineSet | null = null;
      let lit: Map<string, number> | null = null;
      let arrival: ReadonlyMap<string, number> = new Map();
      let attrs: ReadonlyMap<string, Attrs> = new Map();

      if (relight) {
        rises.current.clear();
        window.clearTimeout(drawTimer.current);
        drawn.current = true;
      }

      // A layout pass ends a reveal: by then the skills it brought in show.
      if (relayout || relight) released.current = false;
      if (!live || released.current) routing.current = NO_ROUTING;
      else if (relayout || relight || !composite) routing.current = routingOf(L, scrollTop, moreAbove, moreBelow);
      const edges = { above: routing.current.above.length, below: routing.current.below.length };

      if (active && L.stacked) {
        next = { role: active.roleSlug, lines: null, visible: false, quiet: false, epoch: epoch.current, drawn: true, span: null, edges };
      } else if (active && composite) {
        const reach = reachOf(L.marks.map((m) => m.width ?? 1.5));
        if (track.current?.layout !== L || track.current.reach !== reach) {
          const t = trackOf(L, reach);
          track.current = { layout: L, reach, track: t };
          if (t) driver.setPage(active.roleSlug, translateKeyframes(t.offset, L.range), valueAt(t.offset, L.at));
        }
        driver.setColumn(column, Math.max(0, L.colScroll - L.colClient));
        const t = track.current.track;
        if (t) {
          const frame = composeFrame(active, L, t, routing.current, pageY, scrollTop, rises.current, relight || !drawn.current, span);
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
          next = { role: active.roleSlug, lines: frame.lines, visible: frame.visible, quiet: false, epoch: 0, drawn: drawn.current, span, edges };
        } else {
          next = { role: active.roleSlug, lines: [], visible: false, quiet: false, epoch: 0, drawn: true, span, edges };
        }
      } else if (active && box && col) {
        const page = routeOnPage(active, L, routing.current, box, col, barTop, scrollTop, { hush, epoch: epoch.current }, span);
        next = page.next;
        attrs = page.attrs;
        if (relight) arrival = new Map(page.routes.map((r) => [r.slug, reduceMotion ? 0 : arrivalOf(r.length)]));
      }
      if (relight && active) lit = new Map(active.uses.map((u) => [u.slug, arrival.get(u.slug) ?? 0]));

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
      if (!sameLineSet(store.get(), next)) store.set(next);
    },
    [active, reduceMotion, mode, roleEls, skillEls, store, els, driver],
  );

  // Before paint, so a newly lit role never shows a frame of stale lines.
  useLayoutEffect(() => {
    // When a lit skill sits outside the column's band, centre the lit skills
    // if they fit it; if not, the edge markers point the way. Never while
    // the pointer is in the column (Invariant 13): the reader may be reading
    // it. The browser's own hover state knows, with no event since load.
    const column = columnRef.current;
    if (active && column && column.scrollHeight > column.clientHeight + 1 && !column.matches(":hover")) {
      const col = column.getBoundingClientRect();
      const c = column.scrollTop;
      const ys = active.uses.flatMap((s) => {
        const r = skillEls.get(s.slug)?.getBoundingClientRect();
        return r ? [r.top + r.height / 2 - col.top + c] : [];
      });
      const band = bandOf(column.clientHeight, c, column.dataset.moreAbove === "true", column.dataset.moreBelow === "true");
      const top = ys.some((y) => y < band.top || y > band.bottom) ? centreScroll(column.clientHeight, column.scrollHeight, ys) : null;
      if (top !== null && Math.abs(top - c) > 1) {
        ownScroll.current = true;
        requestAnimationFrame(() => requestAnimationFrame(() => (ownScroll.current = false)));
        column.scrollTo({ top });
      }
    }
    update(true, true);
  }, [active, update, skillEls]);

  /**
   * An edge marker, activated: scroll the skills beyond it into view. The
   * reader asked, so this one may move the column under the pointer.
   * Compositor lines move to the skills first and ride in with them; fade
   * lines hide for the scroll and draw in on the skills once it settles.
   */
  const reveal = useCallback(
    (edge: Edge) => {
      const column = columnRef.current;
      const L = layout.current;
      if (!column || !L || !active) return;
      const y = new Map(L.marks.map((m) => [m.slug, m.y]));
      const group = routing.current[edge].flatMap((s) => y.get(s) ?? []);
      const top = revealScroll(L.colClient, L.colScroll, L.marks.map((m) => m.y), group, edge);
      if (mode === "compositor") released.current = true;
      else if (mode === "fade") quiet.current = true;
      update(false, false);
      column.scrollTo({ top, behavior: reduceMotion ? "auto" : "smooth" });
      settleSoon.current();
    },
    [active, mode, reduceMotion, update],
  );

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
    // mode the settle is also what brings the lines back, and for every mode
    // it is when a line may move between a skill and an edge marker.
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
    settleSoon.current = arm;
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
    if (quiet.current || isScrolling() || released.current) arm();
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
      settleSoon.current = () => {};
      window.removeEventListener("scroll", onScroll, { capture: true });
      window.removeEventListener("resize", onLayout);
      ro.disconnect();
    };
  }, [update, mode, isScrolling]);

  return { light, store: store as LineStore, bind, reveal, isScrolling, containerRef, columnRef, cardRef, barRef, registerSkill, registerRole };
}
