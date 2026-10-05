"use client";

import { useEffect, useState, type RefObject } from "react";

/** Long, soft landing. Type arrives fast and settles slowly. */
export const EASE_OUT = [0.22, 1, 0.36, 1] as const;
/** For the weight axis: a swell, not a snap. */
export const EASE_SWELL = [0.65, 0, 0.35, 1] as const;

/**
 * True once the element has entered the viewport, or has already scrolled past
 * it. Content above the fold of a jump link, or below a tall screenshot window,
 * must still render settled rather than waiting for an intersection that never
 * comes.
 */
export function useSeen(ref: RefObject<Element | null>, rootMargin = "0px 0px -12% 0px"): boolean {
  const [seen, setSeen] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el || seen) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries.some((e) => e.isIntersecting || e.boundingClientRect.bottom < 0)) {
          setSeen(true);
          io.disconnect();
        }
      },
      { rootMargin },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [ref, rootMargin, seen]);
  return seen;
}
