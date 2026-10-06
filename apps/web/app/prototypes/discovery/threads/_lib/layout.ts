import { useCallback, useState } from "react";

import type { Recommendation } from "../../_data/query";
import type { Target } from "./geometry";

/** The app layout's sticky header; a port tucked under it has nothing to draw from. */
export const HEADER = 56;
/** Narrower than this between the columns, the layout has stacked and lines are off. */
const MIN_GUTTER = 48;
/** Half the column's 3rem soft edge: a marker past this is too faded to aim a line at. */
export const FADE = 24;

/**
 * Offsets that move only when the layout does. The roles card scrolls with
 * the container, so the role's anchor is fixed in container space; the skill
 * markers ride in the column's own scroll, so they are fixed in its content
 * space. A scroll frame then needs only where the container and the column
 * sit now.
 *
 * Compositor lines also need the sticky boxes' constraints, to predict where
 * the port and the column sit at any scroll offset (see `portTrack`).
 */
export interface Layout {
  readonly role: string | null;
  readonly viewH: number;
  readonly colClient: number;
  readonly colScroll: number;
  readonly stacked: boolean;
  /** The card's left edge, in container x. */
  readonly originX: number;
  /** The column's left and right edges, in container x. */
  readonly colLeft: number;
  readonly gutterLeft: number;
  /** The headline's middle and its row's extent, in container y. */
  readonly anchor: { readonly mid: number; readonly top: number; readonly bottom: number } | null;
  /** Each connected skill's marker: x in container space, y in the column's scroll content. */
  readonly marks: readonly Target[];
  /** The page's scroll offset when measured, its scroll range, and the viewport (the sticky boxes' scrollport). */
  readonly at: number;
  readonly range: number;
  readonly viewport: number;
  /** The container's top, in the viewport, when measured. */
  readonly boxTop: number;
  /** The column's sticky top, its top in flow and the bottom of its grid area (viewport y when measured), and its height. */
  readonly colStick: number;
  readonly colFlow: number;
  readonly colFloor: number;
  readonly colHeight: number;
  /** The pin bar's top: stuck to the viewport bottom, and in flow (viewport y when measured). */
  readonly barStuck: number;
  readonly barFlow: number;
}

export function readLayout(
  active: Recommendation | null,
  container: HTMLElement,
  column: HTMLElement,
  card: HTMLElement,
  bar: HTMLElement | null,
  skillEls: ReadonlyMap<string, HTMLElement>,
  roleEls: ReadonlyMap<string, HTMLElement>,
): Layout {
  const box = container.getBoundingClientRect();
  const col = column.getBoundingClientRect();
  const cardBox = card.getBoundingClientRect();
  const barBox = bar?.getBoundingClientRect();
  const head = active ? roleEls.get(active.roleSlug) : undefined;
  const anchor = head?.getBoundingClientRect();
  const item = head?.closest("li")?.getBoundingClientRect();
  const scrollTop = column.scrollTop;
  const marks = (active?.connects ?? []).flatMap((c): Target[] => {
    const r = skillEls.get(c.slug)?.getBoundingClientRect();
    return r ? [{ slug: c.slug, x: r.right - box.left - 0.5, y: r.top + r.height / 2 - col.top + scrollTop }] : [];
  });
  const root = document.scrollingElement ?? document.documentElement;
  const boxStyle = getComputedStyle(container);
  const barStyle = bar ? getComputedStyle(bar) : null;
  const viewport = root.clientHeight;
  const barH = barBox?.height ?? 0;
  return {
    role: active?.roleSlug ?? null,
    viewH: window.innerHeight,
    colClient: column.clientHeight,
    colScroll: column.scrollHeight,
    stacked: cardBox.left - col.right < MIN_GUTTER,
    originX: cardBox.left - box.left,
    colLeft: col.left - box.left,
    gutterLeft: col.right - box.left,
    anchor: anchor && item ? { mid: anchor.top + anchor.height / 2 - box.top, top: item.top - box.top, bottom: item.bottom - box.top } : null,
    marks,
    at: window.scrollY,
    range: Math.max(0, root.scrollHeight - viewport),
    viewport,
    boxTop: box.top,
    colStick: parseFloat(getComputedStyle(column).top) || HEADER,
    colFlow: box.top + (parseFloat(boxStyle.paddingTop) || 0),
    colFloor: box.bottom - (parseFloat(boxStyle.paddingBottom) || 0),
    colHeight: col.height,
    barStuck: barStyle ? viewport - (parseFloat(barStyle.bottom) || 0) - barH : Infinity,
    barFlow: barStyle ? cardBox.bottom - (parseFloat(barStyle.marginBottom) || 0) - barH : Infinity,
  };
}

/** The band of the skill column that shows, in viewport y, short of any soft edge. */
export function seen(col: DOMRect, moreAbove: boolean, moreBelow: boolean) {
  return { top: col.top + (moreAbove ? FADE : 0), bottom: col.bottom - (moreBelow ? FADE : 0) };
}

type Register = (key: string) => (el: HTMLElement | null) => void;

export function useRegistry(): [Map<string, HTMLElement>, Register] {
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
