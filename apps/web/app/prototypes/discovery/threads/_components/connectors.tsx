"use client";

import { AnimatePresence, motion, useReducedMotion } from "motion/react";

import { DRAW, DRAW_DELAY, drawDuration } from "../_lib/motion";
import type { Threads } from "../_lib/use-threads";

/**
 * The threads themselves: one SVG over the whole layout, routed from measured
 * positions. Each role's bundle mounts under its own key, so lighting a new
 * role fades the old bundle and draws the new one from the role outward.
 * Under reduced motion the lines appear whole.
 */
export function Connectors({ threads }: { threads: Threads | null }) {
  const reduce = useReducedMotion() ?? false;
  const bundle = threads?.bundle ?? null;

  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 z-10 size-full overflow-visible" fill="none">
      <motion.g initial={false} animate={{ opacity: threads?.visible ? 1 : 0 }} transition={{ duration: 0.22, ease: "easeOut" }}>
        <AnimatePresence>
          {threads && bundle && bundle.routes.length > 0 && (
            <motion.g key={threads.role} exit={{ opacity: 0, transition: { duration: 0.16, ease: "easeOut" } }}>
              {/* The port: where the bundle leaves the role, on the card's edge. */}
              <motion.rect
                x={threads.originX - 1.5}
                y={bundle.portTop - 7}
                width={3}
                height={bundle.portBottom - bundle.portTop + 14}
                rx={1.5}
                fill="var(--threads-line)"
                style={{ transformBox: "fill-box", transformOrigin: "center" }}
                initial={reduce ? false : { opacity: 0, scaleY: 0.2 }}
                animate={{ opacity: 1, scaleY: 1 }}
                transition={{ duration: 0.28, ease: DRAW }}
              />
              {bundle.routes.map((r) => (
                <motion.path
                  key={r.slug}
                  d={r.d}
                  stroke="var(--threads-line)"
                  strokeWidth={1.5}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  initial={reduce ? false : { pathLength: 0 }}
                  animate={{ pathLength: 1 }}
                  transition={{ duration: drawDuration(r.length), ease: DRAW, delay: DRAW_DELAY }}
                />
              ))}
            </motion.g>
          )}
        </AnimatePresence>
      </motion.g>
    </svg>
  );
}
