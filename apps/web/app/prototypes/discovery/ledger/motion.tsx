"use client";

// The page's timing table, the row reveal, and two numeric motion primitives.
// Every delay on the page is read from CUE, so the choreography can be retimed
// in one place and the reading order (masthead, summary, then rows by rank)
// cannot drift.

import { animate, AnimatePresence, motion, useInView, useMotionValue, useReducedMotion, useTransform } from "motion/react";
import { type RefObject, useEffect, useRef, useState } from "react";

import { formatCount } from "./derive";

/** Seconds. Ease curves are cubic-bezier control points. */
export const CUE = {
  rule: 0,
  headline: 0.12,
  lede: 0.24,
  band: 0.36,
  bandStagger: 0.08,
  tableHead: 0.62,
  firstRow: 0.72,
  rowStagger: 0.06,
  /** After a row resolves, its marks start: bars grow and counts tick. */
  marksAfterRow: 0.16,
} as const;

export const EASE_OUT: [number, number, number, number] = [0.16, 1, 0.3, 1];
export const EASE_RULE: [number, number, number, number] = [0.65, 0, 0.35, 1];

export function rowDelay(index: number): number {
  return CUE.firstRow + index * CUE.rowStagger;
}

// Shared across rows: when the page's choreography began.
let pageStart = 0;
export function markPageStart() {
  pageStart = performance.now();
}

/**
 * Plays a row's entrance when it first scrolls into view. Rows visible on load
 * keep their rank-order slot in the page choreography; a row reached later
 * resolves as it arrives, so the reveal is never spent off screen.
 */
export function useRowReveal<T extends Element>(index: number): { ref: RefObject<T | null>; play: boolean; delay: number } {
  const ref = useRef<T>(null);
  const inView = useInView(ref, { once: true, amount: 0.2 });
  const [delay, setDelay] = useState<number | null>(null);

  useEffect(() => {
    if (!inView || delay !== null) return;
    const elapsed = (performance.now() - pageStart) / 1000;
    // Reacting to an external signal (the observer); runs once per row.
    setDelay(Math.max(0.04, rowDelay(index) - elapsed));
  }, [inView, delay, index]);

  return { ref, play: delay !== null, delay: delay ?? 0 };
}

interface TickProps {
  readonly value: number;
  /** Holds at zero until true. */
  readonly play?: boolean;
  readonly delay?: number;
  readonly duration?: number;
  readonly className?: string;
}

/**
 * Counts up from zero once. The width never jumps because callers set
 * tabular figures. Reduced motion lands on the value immediately; MotionConfig
 * does not reach imperative animate() calls, so this checks for itself.
 */
export function Tick({ value, play = true, delay = 0, duration = 0.7, className }: TickProps) {
  const reduce = useReducedMotion();
  const mv = useMotionValue(0);
  const text = useTransform(mv, (v) => formatCount(Math.round(v)));

  useEffect(() => {
    if (reduce) {
      mv.set(value);
      return;
    }
    if (!play) return;
    const controls = animate(mv, value, { delay, duration, ease: EASE_OUT });
    return () => controls.stop();
  }, [mv, value, play, delay, duration, reduce]);

  return <motion.span className={className}>{text}</motion.span>;
}

/**
 * A count that rolls one digit-height when it changes: up when it grows, down
 * when it shrinks, so the direction of the change is legible at a glance.
 */
export function Roll({ value, className }: { readonly value: number; readonly className?: string }) {
  // Adjust-state-during-render: the direction is known in the same pass as the
  // new value, so the entering digit starts on the correct side.
  const [seen, setSeen] = useState({ value, dir: 1 });
  if (seen.value !== value) setSeen({ value, dir: value > seen.value ? 1 : -1 });

  return (
    <span className={`relative inline-flex overflow-hidden ${className ?? ""}`}>
      <AnimatePresence mode="popLayout" initial={false} custom={seen.dir}>
        <motion.span
          key={value}
          custom={seen.dir}
          variants={{
            enter: (dir: number) => ({ y: dir > 0 ? "100%" : "-100%", opacity: 0 }),
            center: { y: "0%", opacity: 1 },
            exit: (dir: number) => ({ y: dir > 0 ? "-100%" : "100%", opacity: 0 }),
          }}
          initial="enter"
          animate="center"
          exit="exit"
          transition={{ duration: 0.22, ease: EASE_OUT }}
          className="inline-block"
        >
          {formatCount(value)}
        </motion.span>
      </AnimatePresence>
    </span>
  );
}
