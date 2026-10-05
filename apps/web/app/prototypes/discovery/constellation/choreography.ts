// Entrance timing, in seconds. The eye starts where the person stands and is
// walked outward: the plaque of past titles, then the orbits that measure
// distance, then one path per role in rank order, each star igniting as its
// path arrives and its name following. Pins settle last, so the tray fills
// after the map has finished speaking.

import { createContext, useContext } from "react";

export const ENTER = {
  home: 0.15,
  orbits: 0.55,
  firstPath: 1.0,
  stagger: 0.13,
  pathDuration: 0.8,
} as const;

export const EASE_OUT = [0.22, 1, 0.36, 1] as const;

/** `order` is the 0-based position in rank order. */
export function pathDelay(order: number): number {
  return ENTER.firstPath + order * ENTER.stagger;
}

export function starDelay(order: number): number {
  return pathDelay(order) + ENTER.pathDuration * 0.8;
}

export function settleDelay(count: number): number {
  return count === 0 ? ENTER.firstPath : starDelay(count - 1) + 0.35;
}

/**
 * Multiplier on every entrance delay and duration: 1, or 0 under reduced
 * motion. MotionConfig's "user" setting only drops transforms; without this
 * the paths would still make a reduced-motion reader wait three seconds to
 * see the map.
 */
export const Tempo = createContext(1);

export function useTempo(): number {
  return useContext(Tempo);
}
