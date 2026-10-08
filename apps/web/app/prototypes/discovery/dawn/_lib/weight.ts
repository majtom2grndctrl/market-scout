// Line weight: how much a role leans on a skill, read from the share of its
// postings that ask for it, P(skill | role). Never the person's gap or fit
// (build contract, Invariants 3 and 15).
//
// Three steps, the same for every role, so equal weights mean the same thing
// across the page and heavier always means a larger share (Invariant 16).
// Widths step by a little more each time, so the top step reads as a step and
// not as a slightly thicker middle one. The lightest is round 1's single
// width, whose colour was checked at 3:1 against everything it crosses; the
// colours do not change with weight.

/** The lower bound of the middle and top steps. */
export const STEP_FLOORS = [0.2, 0.4] as const;
/** Stroke width per step, px. */
export const WIDTHS = [1.5, 2.5, 4] as const;

export type Step = 0 | 1 | 2;

export const stepOf = (share: number): Step => (share >= STEP_FLOORS[1] ? 2 : share >= STEP_FLOORS[0] ? 1 : 0);
export const widthOf = (share: number): number => WIDTHS[stepOf(share)];

/**
 * Clear space between two neighbouring lines' edges, in a lane bundle and at
 * the ports. Two light lines sit 6px apart centre to centre, as in round 1;
 * two heavy ones 8.5px, so their edges keep the same gap.
 */
export const CLEAR = 4.5;

/** Centre-to-centre distance between neighbouring lines of widths `a` and `b`. */
export const pitch = (a: number, b: number) => (a + b) / 2 + CLEAR;

/** Each line's offset from the first, centre to centre, for lines in order. */
export function offsets(widths: readonly number[]): number[] {
  const out: number[] = [];
  widths.forEach((w, k) => out.push(k === 0 ? 0 : out[k - 1] + pitch(widths[k - 1], w)));
  return out;
}
