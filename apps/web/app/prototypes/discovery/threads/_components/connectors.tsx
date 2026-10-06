"use client";

import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useSyncExternalStore } from "react";

import { DRAW, DRAW_DELAY, drawDuration } from "../_lib/motion";
import { lineKey, portKey, type Bind, type ThreadsStore } from "../_lib/use-threads";

/**
 * The threads themselves: one SVG over the whole layout. React decides which
 * lines exist and animates them in and out; their geometry (`d`, and the
 * port's position) is written by `useThreads` through `bind`, because it
 * changes every scroll frame and a render per frame cost more than the
 * routing. Neither prop is set here, so a render never writes stale geometry.
 *
 * Each role's bundle mounts under its own key, so lighting a new role fades
 * the old bundle and draws the new one from the role outward. Under reduced
 * motion the lines appear whole.
 */
export function Connectors({ store, bind }: { store: ThreadsStore; bind: Bind }) {
  const reduce = useReducedMotion() ?? false;
  const threads = useSyncExternalStore(store.subscribe, store.get, store.get);
  const lines = threads?.lines ?? null;

  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 z-10 size-full overflow-visible" fill="none">
      <motion.g initial={false} animate={{ opacity: threads?.visible ? 1 : 0 }} transition={{ duration: 0.22, ease: "easeOut" }}>
        <AnimatePresence>
          {threads && lines && lines.length > 0 && (
            <motion.g key={threads.role} exit={{ opacity: 0, transition: { duration: 0.16, ease: "easeOut" } }}>
              {/* The port: where the bundle leaves the role, on the card's edge. */}
              <motion.rect
                ref={bind(portKey(threads.role))}
                width={3}
                rx={1.5}
                fill="var(--threads-line)"
                style={{ transformBox: "fill-box", transformOrigin: "center" }}
                initial={reduce ? false : { opacity: 0, scaleY: 0.2 }}
                animate={{ opacity: 1, scaleY: 1 }}
                transition={{ duration: 0.28, ease: DRAW }}
              />
              {lines.map((l) => (
                <motion.path
                  key={l.slug}
                  ref={bind(lineKey(threads.role, l.slug))}
                  stroke="var(--threads-line)"
                  strokeWidth={1.5}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  initial={reduce ? false : { pathLength: 0 }}
                  animate={{ pathLength: 1 }}
                  transition={{ duration: drawDuration(l.length), ease: DRAW, delay: DRAW_DELAY }}
                />
              ))}
            </motion.g>
          )}
        </AnimatePresence>
      </motion.g>
    </svg>
  );
}
