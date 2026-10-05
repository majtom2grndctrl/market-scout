"use client";

import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useInView } from "motion/react";

import { BATCH_STEP, FIRST_CARD, STEP } from "./motion";

/** performance.now() when the page mounted; the zero of the arrival schedule. */
export const ArrivalClock = createContext(0);

/**
 * When a recommendation should drift in, in seconds from now; null until it
 * scrolls into view. On load the cards keep reading order across tiers. A card
 * reached later by scrolling arrives promptly, staggered only within its batch,
 * so nothing waits on a schedule the reader has already passed.
 */
export function useArrival<E extends Element>(order: number, batchIndex: number) {
  const ref = useRef<E>(null);
  const inView = useInView(ref, { once: true, amount: 0.12 });
  const startedAt = useContext(ArrivalClock);
  const [delay, setDelay] = useState<number | null>(null);

  useEffect(() => {
    if (!inView) return;
    const elapsed = (performance.now() - startedAt) / 1000;
    setDelay((d) => d ?? Math.max(batchIndex * BATCH_STEP, FIRST_CARD + order * STEP - elapsed));
  }, [inView, startedAt, order, batchIndex]);

  return { ref, delay };
}
