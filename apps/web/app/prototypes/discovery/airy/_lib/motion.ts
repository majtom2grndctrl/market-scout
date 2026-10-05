// Airy's motion vocabulary. Eased tweens carry information arriving (text,
// cards, bars); springs carry things the person moves (a pin, the tray).

import type { Transition } from "motion/react";

/** Long, soft deceleration: arrives quickly, settles slowly. */
export const DRIFT = [0.16, 1, 0.3, 1] as const;

export const drift = (delay = 0, duration = 1.1): Transition => ({ duration, delay, ease: DRIFT });

/** A pin travelling from its card to the tray. */
export const FLIGHT: Transition = { type: "spring", stiffness: 210, damping: 28, mass: 0.9 };

/** Small physical responses: press, hover lift, tray settling. */
export const SETTLE: Transition = { type: "spring", stiffness: 380, damping: 30 };

/**
 * Reading-order schedule for recommendations. The first card in reading order
 * arrives at FIRST_CARD seconds and each later one STEP after it. A card that
 * scrolls into view after its slot has passed arrives at once, staggered only
 * against the others in its own batch.
 */
export const FIRST_CARD = 1.05;
export const STEP = 0.11;
export const BATCH_STEP = 0.09;
