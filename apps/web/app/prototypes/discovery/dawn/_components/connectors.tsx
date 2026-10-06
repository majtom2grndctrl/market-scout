"use client";

import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useId, useSyncExternalStore } from "react";

import type { Span } from "../_lib/light";
import type { LineMode } from "../_lib/modes";
import { DRAW, DRAW_DELAY, drawDuration } from "../_lib/motion";
import { frameKey, lineKey, portKey, type Bind, type Line, type LineStore } from "../_lib/store";
import { PORT_STYLE, ThreadGradient } from "./thread-paint";

/**
 * The lines as whole paths: one SVG over the whole layout. React decides
 * which lines exist and animates them in and out; their geometry (`d`, and
 * the port's position) is written by `useLines` through `bind`, because it
 * changes every scroll frame and a render per frame cost more than the
 * routing. Neither prop is set here, so a render never writes stale geometry.
 *
 * Each role's bundle mounts under its own key, so lighting a new role fades
 * the old bundle and draws the new one from the role outward: the light
 * leaves the role's gold port rose and reaches the skills lilac. Under
 * reduced motion the lines appear whole.
 *
 * Per mode (see `modes.ts`): `js` keeps these paths on screen and re-routes
 * them every frame. `fade` drops the bundle as scrolling starts and remounts
 * it, drawing in again, once scrolling settles. `compositor` shows them only
 * for the draw-in, then hands over to `Pieces`.
 */
export function Connectors({ store, bind, mode }: { store: LineStore; bind: Bind; mode: LineMode }) {
  const reduce = useReducedMotion() ?? false;
  const set = useSyncExternalStore(store.subscribe, store.get, store.get);
  const lines = set?.lines ?? null;
  const key = set ? (mode === "fade" ? `${set.role}:${set.epoch}` : set.role) : "";

  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 z-10 size-full overflow-visible" fill="none">
      <motion.g initial={false} animate={{ opacity: set?.visible ? 1 : 0 }} transition={{ duration: 0.22, ease: "easeOut" }}>
        <AnimatePresence>
          {set && lines && lines.length > 0 && !set.quiet && (
            <motion.g key={key} exit={{ opacity: 0, transition: { duration: mode === "fade" ? 0.08 : 0.16, ease: "easeOut" } }}>
              {mode === "compositor" ? (
                // Placed by script in the page's frame; hidden the moment the pieces take over.
                <g ref={bind(frameKey(set.role))} style={{ opacity: set.drawn ? 0 : 1 }}>
                  <Bundle role={set.role} lines={lines} span={set.span} shift={set.span?.origin ?? 0} bind={bind} reduce={reduce} />
                </g>
              ) : (
                <Bundle role={set.role} lines={lines} span={set.span} shift={0} bind={bind} reduce={reduce} />
              )}
            </motion.g>
          )}
        </AnimatePresence>
      </motion.g>
    </svg>
  );
}

function Bundle({
  role,
  lines,
  span,
  shift,
  bind,
  reduce,
}: {
  role: string;
  lines: readonly Line[];
  span: Span | null;
  shift: number;
  bind: Bind;
  reduce: boolean;
}) {
  const id = `dawn-thread-${useId().replace(/:/g, "")}`;
  const stroke = span ? `url(#${id})` : "var(--dawn-thread-rose)";
  return (
    <>
      {span && <ThreadGradient id={id} span={span} shift={shift} />}
      {/* The port: where the bundle leaves the role, on the card's edge. */}
      <motion.rect
        ref={bind(portKey(role))}
        width={3}
        rx={1.5}
        style={{ ...PORT_STYLE, transformBox: "fill-box", transformOrigin: "center" }}
        initial={reduce ? false : { opacity: 0, scaleY: 0.2 }}
        animate={{ opacity: 1, scaleY: 1 }}
        transition={{ duration: 0.28, ease: DRAW }}
      />
      {lines.map((l) => (
        <motion.path
          key={l.slug}
          ref={bind(lineKey(role, l.slug))}
          stroke={stroke}
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
