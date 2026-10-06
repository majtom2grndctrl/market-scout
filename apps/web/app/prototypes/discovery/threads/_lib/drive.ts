// Scroll-driven animations for compositor lines. Each animated element plays
// a translateY on a scroll timeline, so the compositor moves it in the same
// frame as the scroll that moves its frame-mates. Script only swaps keyframes
// when the layout changes.

/**
 * What an element's translateY follows.
 * - page: the port's offset from the skill column, as the page scrolls (per role).
 * - column: the column's own scroll, for things anchored to the port.
 * - unscroll: the opposite, for the layer that holds the skills' ends.
 */
export type Drive = "page" | "column" | "unscroll";

interface Entry {
  readonly el: HTMLElement;
  readonly drive: Drive;
  readonly role: string;
  anim: Animation | null;
}

interface PageFrames {
  readonly keyframes: Keyframe[];
  /** The value to hold when the page cannot scroll, where a scroll timeline is inactive. */
  readonly still: string;
  readonly sig: string;
}

export interface Driver {
  readonly attach: (el: HTMLElement, drive: Drive, role: string) => () => void;
  readonly setPage: (role: string, keyframes: Keyframe[], still: number) => void;
  readonly setColumn: (source: Element, range: number) => void;
}

export function createDriver(): Driver {
  const entries = new Set<Entry>();
  const pages = new Map<string, PageFrames>();
  let pageTimeline: ScrollTimeline | null = null;
  let column: { source: Element; timeline: ScrollTimeline; range: number } | null = null;

  const columnFrames = (drive: Drive): Keyframe[] => {
    const shift = (column?.range ?? 0) * (drive === "unscroll" ? -1 : 1);
    return [{ transform: "translateY(0px)" }, { transform: `translateY(${shift}px)` }];
  };

  const play = (e: Entry) => {
    if (e.drive === "page") {
      const frames = pages.get(e.role);
      if (!frames) return;
      e.el.style.transform = frames.still;
      pageTimeline ??= new ScrollTimeline({ source: document.scrollingElement ?? document.documentElement, axis: "block" });
      if (e.anim) (e.anim.effect as KeyframeEffect).setKeyframes(frames.keyframes);
      else e.anim = e.el.animate(frames.keyframes, { timeline: pageTimeline, fill: "both", easing: "linear" });
      return;
    }
    if (!column) return;
    if (e.anim && e.anim.timeline === column.timeline) (e.anim.effect as KeyframeEffect).setKeyframes(columnFrames(e.drive));
    else {
      e.anim?.cancel();
      e.anim = e.el.animate(columnFrames(e.drive), { timeline: column.timeline, fill: "both", easing: "linear" });
    }
  };

  return {
    attach(el, drive, role) {
      const e: Entry = { el, drive, role, anim: null };
      entries.add(e);
      play(e);
      return () => {
        e.anim?.cancel();
        entries.delete(e);
      };
    },
    setPage(role, keyframes, still) {
      const sig = JSON.stringify(keyframes);
      if (pages.get(role)?.sig === sig) return;
      pages.set(role, { keyframes, still: `translateY(${Math.round(still * 100) / 100}px)`, sig });
      // Keep only the roles still on screen (the active one and any fading out).
      const live = new Set([...entries].map((e) => e.role));
      for (const r of pages.keys()) if (r !== role && !live.has(r)) pages.delete(r);
      for (const e of entries) if (e.drive === "page" && e.role === role) play(e);
    },
    setColumn(source, range) {
      if (column?.source === source && column.range === range) return;
      const timeline = column?.source === source ? column.timeline : new ScrollTimeline({ source, axis: "block" });
      column = { source, timeline, range };
      for (const e of entries) if (e.drive !== "page") play(e);
    },
  };
}
