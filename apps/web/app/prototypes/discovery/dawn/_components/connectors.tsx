"use client";

import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useSyncExternalStore } from "react";

import type { LineMode } from "../_lib/modes";
import { DRAW, DRAW_DELAY, drawDuration } from "../_lib/motion";
import { frameKey, lineKey, portKey, type Bind, type Line, type ThreadsStore } from "../_lib/store";

/**
 * The threads as whole paths: one SVG over the whole layout. React decides
 * which lines exist and animates them in and out; their geometry (`d`, and
 * the port's position) is written by `useThreads` through `bind`, because it
 * changes every scroll frame and a render per frame cost more than the
 * routing. Neither prop is set here, so a render never writes stale geometry.
 *
 * Each role's bundle mounts under its own key, so lighting a new role fades
 * the old bundle and draws the new one from the role outward. Under reduced
 * motion the lines appear whole.
 *
 * Per mode (see `modes.ts`): `js` keeps these paths on screen and re-routes
 * them every frame. `fade` drops the bundle as scrolling starts and remounts
 * it, drawing in again, once scrolling settles. `compositor` shows them only
 * for the draw-in, then hands over to `Pieces`.
 */
export function Connectors({ store, bind, mode }: { store: ThreadsStore; bind: Bind; mode: LineMode }) {
  const reduce = useReducedMotion() ?? false;
  const threads = useSyncExternalStore(store.subscribe, store.get, store.get);
  const lines = threads?.lines ?? null;
  const key = threads ? (mode === "fade" ? `${threads.role}:${threads.epoch}` : threads.role) : "";

  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 z-10 size-full overflow-visible" fill="none">
      <motion.g initial={false} animate={{ opacity: threads?.visible ? 1 : 0 }} transition={{ duration: 0.22, ease: "easeOut" }}>
        <AnimatePresence>
          {threads && lines && lines.length > 0 && !threads.quiet && (
            <motion.g key={key} exit={{ opacity: 0, transition: { duration: mode === "fade" ? 0.08 : 0.16, ease: "easeOut" } }}>
              {mode === "compositor" ? (
                // Placed by script in the page's frame; hidden the moment the pieces take over.
                <g ref={bind(frameKey(threads.role))} style={{ opacity: threads.drawn ? 0 : 1 }}>
                  <Bundle role={threads.role} lines={lines} bind={bind} reduce={reduce} />
                </g>
              ) : (
                <Bundle role={threads.role} lines={lines} bind={bind} reduce={reduce} />
              )}
            </motion.g>
          )}
        </AnimatePresence>
      </motion.g>
    </svg>
  );
}

function Bundle({ role, lines, bind, reduce }: { role: string; lines: readonly Line[]; bind: Bind; reduce: boolean }) {
  return (
    <>
      {/* The port: where the bundle leaves the role, on the card's edge. */}
      <motion.rect
        ref={bind(portKey(role))}
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
          ref={bind(lineKey(role, l.slug))}
          stroke="var(--threads-line)"
          strokeWidth={1.5}
          strokeLinecap="round"
          strokeLinejoin="round"
          initial={reduce ? false : { pathLength: 0 }}
          animate={{ pathLength: 1 }}
          transition={{ duration: drawDuration(l.length), ease: DRAW, delay: DRAW_DELAY }}
        />
      ))}
    </>
  );
}
