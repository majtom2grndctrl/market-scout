// Dawn's motion vocabulary. The entrance reads in story order: the cool sky
// and the headline, then the skills (what you have), then the roles (where it
// leads), then the first thread drawn between them, and with it the sunrise.
// After that, light only answers the reader: a role they light, a pin they
// make.

import type { Transition } from "motion/react";

/** Long, soft deceleration for things arriving. */
export const DRIFT = [0.16, 1, 0.3, 1] as const;
/** A line being drawn: quick off the port, easing into the skill. */
export const DRAW = [0.32, 0.72, 0, 1] as const;

export const drift = (delay = 0, duration = 1): Transition => ({ duration, delay, ease: DRIFT });

/** Small physical responses: a press, a pin settling. */
export const SETTLE: Transition = { type: "spring", stiffness: 420, damping: 32 };

export const ENTRANCE = {
  headline: 0.05,
  lede: 0.2,
  /** First skill row; each later row SKILL_STEP after. */
  skills: 0.3,
  skillStep: 0.022,
  card: 0.85,
  /** First role row; each later row ROLE_STEP after. */
  roles: 1.0,
  roleStep: 0.06,
  /** The top recommendation lights once the first rows have landed. */
  light: 1.75,
} as const;

/** Lines draw at one speed, so nearer skills light first and the eye follows the wave outward. */
export const DRAW_SPEED = 1100;
export const DRAW_DELAY = 0.06;
export const drawDuration = (length: number) => Math.min(0.85, Math.max(0.32, length / DRAW_SPEED));
/** A skill starts to light just before its line lands, so the colour peaks on contact. */
export const arrivalOf = (length: number) => DRAW_DELAY + drawDuration(length) * 0.85;

/** Hover dwell before a role takes the light, so sweeping across the list doesn't strobe. */
export const HOVER_DWELL_MS = 70;

/**
 * The sun behind the roles card. It waits below the horizon, faint, then
 * rises `below` px as the first thread lands while the blue clears overhead:
 * a beat after the light, slow enough to stay in the corner of the eye while
 * the line draws.
 */
export const SUNRISE = { below: 120, before: 0.16, delay: 0.25, duration: 2.6 } as const;

/** How far the sun's centre sits below the pin bar's bottom edge, per pin: set, then a step up per pin, to three. */
export const HORIZON = [80, 14, 4, -6] as const;
