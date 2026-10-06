"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

import type { Recommendation } from "../../_data/query";
import { routeBundle, SPACING, type Bundle, type Target } from "./geometry";
import { arrivalOf } from "./motion";

/** The app layout's sticky header; a port tucked under it has nothing to draw from. */
const HEADER = 56;
/** Narrower than this between the columns, the layout has stacked and lines are off. */
const MIN_GUTTER = 48;
const SCROLL_SETTLE_MS = 140;

export interface Threads {
  /** The role this geometry was measured for. */
  readonly role: string;
  /** Lit skills, each with the seconds until its line arrives. */
  readonly lit: ReadonlyMap<string, number>;
  /** Null when the layout is stacked: skills still light, but nothing is drawn. */
  readonly bundle: Bundle | null;
  readonly originX: number;
  /** False once too little of the role shows between the header and the pin bar to hold its port. */
  readonly visible: boolean;
}

/** Half the column's 3rem soft edge: a marker past this is too faded to aim a line at. */
const FADE = 24;

/** The band of the skill column that shows, in viewport y, short of any soft edge. */
function seen(column: HTMLElement, col: DOMRect) {
  return {
    top: col.top + (column.dataset.moreAbove === "true" ? FADE : 0),
    bottom: col.bottom - (column.dataset.moreBelow === "true" ? FADE : 0),
  };
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
 * Measures the live layout and routes a bundle from the active role to the
 * skills it connects. Re-measures on scroll, resize, and any size change of
 * the columns, coalesced to one pass per frame, so lines track the sticky
 * skill column and the scrolling roles exactly.
 */
export function useThreads(active: Recommendation | null, reduceMotion: boolean) {
  const containerRef = useRef<HTMLDivElement>(null);
  const columnRef = useRef<HTMLElement>(null);
  const cardRef = useRef<HTMLElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const [skillEls, registerSkill] = useRegistry();
  const [roleEls, registerRole] = useRegistry();
  const [threads, setThreads] = useState<Threads | null>(null);
  const lastScroll = useRef(-Infinity);
  /** True while the page is mid-scroll: a role sliding under a still pointer is not a hover. */
  const isScrolling = useCallback(() => performance.now() - lastScroll.current < SCROLL_SETTLE_MS, []);

  const measure = useCallback(() => {
    const container = containerRef.current;
    const column = columnRef.current;
    const card = cardRef.current;
    if (column) {
      // Written straight to the DOM: a styling hint, not state worth a render.
      column.dataset.moreAbove = String(column.scrollTop > 1);
      column.dataset.moreBelow = String(column.scrollTop + column.clientHeight < column.scrollHeight - 1);
    }
    if (!active || !container || !column || !card) {
      setThreads(null);
      return;
    }
    const box = container.getBoundingClientRect();
    const col = column.getBoundingClientRect();
    const cardBox = card.getBoundingClientRect();
    const originX = cardBox.left - box.left;

    if (cardBox.left - col.right < MIN_GUTTER) {
      setThreads({ role: active.roleSlug, lit: new Map(active.connects.map((c) => [c.slug, 0])), bundle: null, originX, visible: false });
      return;
    }

    // A skill scrolled out of its own column, or under the column's soft edge
    // (only when the column overflows), gets no line: a line to something off
    // screen points at nothing.
    const { top: seenTop, bottom: seenBottom } = seen(column, col);
    const targets = active.connects.flatMap((c): Target[] => {
      const r = skillEls.get(c.slug)?.getBoundingClientRect();
      const y = r ? r.top + r.height / 2 : NaN;
      if (!r || !(y >= seenTop && y <= seenBottom)) return [];
      return [{ slug: c.slug, x: r.right - box.left - 0.5, y: y - box.top }];
    });

    // The port sits level with the role's headline, and slides along the
    // role's visible edge as it scrolls under the header or the pin bar, so
    // the bundle leaves from what the reader can still see of the role.
    const head = roleEls.get(active.roleSlug);
    const anchor = head?.getBoundingClientRect();
    const item = head?.closest("li")?.getBoundingClientRect();
    const floor = Math.min(window.innerHeight, barRef.current?.getBoundingClientRect().top ?? Infinity);
    const reach = ((Math.max(targets.length, 1) - 1) * SPACING) / 2 + 16;
    const lo = Math.max(item?.top ?? 0, HEADER) + reach;
    const hi = Math.min(item?.bottom ?? 0, floor) - reach;
    const visible = anchor !== undefined && lo <= hi;
    const originY = anchor ? Math.min(Math.max(anchor.top + anchor.height / 2, lo), hi) : box.top;

    const bundle = routeBundle({ x: originX, y: originY - box.top }, col.right - box.left, targets);
    const arrival = new Map(bundle.routes.map((r) => [r.slug, reduceMotion ? 0 : arrivalOf(r.length)]));
    const lit = new Map(active.connects.map((c) => [c.slug, arrival.get(c.slug) ?? 0]));

    setThreads({ role: active.roleSlug, lit, bundle, originX, visible });
  }, [active, reduceMotion, roleEls, skillEls]);

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
        const { top: seenTop, bottom: seenBottom } = seen(column, col);
        if (top < seenTop || bottom > seenBottom) {
          const mid = (top + bottom) / 2 - col.top + column.scrollTop;
          column.scrollTo({ top: mid - column.clientHeight / 2 });
        }
      }
    }
    measure();
  }, [active, measure, skillEls]);

  useEffect(() => {
    let frame = 0;
    const schedule = (e?: Event) => {
      if (e?.type === "scroll") lastScroll.current = performance.now();
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        measure();
      });
    };
    window.addEventListener("scroll", schedule, { passive: true, capture: true });
    window.addEventListener("resize", schedule);
    const ro = new ResizeObserver(() => schedule());
    for (const el of [containerRef.current, columnRef.current, cardRef.current]) if (el) ro.observe(el);
    document.fonts?.ready.then(() => schedule());
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule, { capture: true });
      window.removeEventListener("resize", schedule);
      ro.disconnect();
    };
  }, [measure]);

  return { threads, isScrolling, containerRef, columnRef, cardRef, barRef, registerSkill, registerRole };
}
