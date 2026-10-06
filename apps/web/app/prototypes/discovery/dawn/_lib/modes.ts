// How the lines behave while the page scrolls, chosen by `?lines=`.
//
// Chrome scrolls on the compositor thread. The role card and the skill column
// move at once; anything the main thread draws arrives a frame later. So a
// line drawn by script drifts off its marker for a frame and snaps back.
//
// - compositor: every piece of a line rides in the frame its end belongs to,
//   and the run between them stretches on a scroll-driven animation. Steady
//   scrolling never waits on script.
// - fade: lines fade out as scrolling starts and draw back in once it settles.
// - js: the main thread re-routes every frame. The baseline for comparison.

export type LineMode = "compositor" | "fade" | "js";

const MODES: readonly LineMode[] = ["compositor", "fade", "js"];

export const DEFAULT_LINE_MODE: LineMode = "compositor";

/** Reads a search param. Anything unknown falls back to the default. */
export function parseLineMode(value: string | string[] | undefined): LineMode {
  const v = Array.isArray(value) ? value[0] : value;
  return MODES.find((m) => m === v) ?? DEFAULT_LINE_MODE;
}

/** Compositor lines need scroll-driven animations; without them, fade. */
export function effectiveLineMode(requested: LineMode): LineMode {
  if (requested !== "compositor") return requested;
  const ok =
    typeof window !== "undefined" &&
    typeof window.ScrollTimeline === "function" &&
    typeof CSS !== "undefined" &&
    CSS.supports("animation-timeline: view()");
  return ok ? "compositor" : "fade";
}
