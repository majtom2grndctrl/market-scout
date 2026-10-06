"use client";

import { AnimatePresence, motion } from "motion/react";
import { useId, useSyncExternalStore } from "react";

import type { Span } from "../_lib/light";
import { pieceKey, type Bind, type Line, type LineStore } from "../_lib/store";
import styles from "../dawn.module.css";
import { PORT_STYLE, ThreadGradient } from "./thread-paint";

const RUNS = ["down", "up"] as const;
const line = { strokeWidth: 1.5, strokeLinecap: "round", strokeLinejoin: "round" } as const;

/**
 * Compositor lines (see `compose.ts` for the construction). A layer that
 * sticks exactly as the skill column does holds every piece, so the skill
 * ends stay on their markers on the compositor. Script writes the pieces'
 * shapes through `bind` when a line's topology changes; scroll timelines
 * attached through `bind` move them in between.
 *
 * Shows once the draw-in path (see Connectors) has landed, and fades with it.
 */
export function Pieces({ store, bind }: { store: LineStore; bind: Bind }) {
  const set = useSyncExternalStore(store.subscribe, store.get, store.get);
  const lines = set?.lines ?? null;

  return (
    <div aria-hidden className={styles.pieces}>
      <AnimatePresence>
        {set && lines && lines.length > 0 && (
          <motion.div key={set.role} className="absolute inset-0" exit={{ opacity: 0, transition: { duration: 0.16, ease: "easeOut" } }}>
            <motion.div
              className="absolute inset-0"
              initial={false}
              animate={{ opacity: set.visible ? 1 : 0 }}
              transition={{ duration: 0.22, ease: "easeOut" }}
            >
              <div className="absolute inset-0" style={{ opacity: set.drawn ? 1 : 0 }}>
                <Bundle role={set.role} lines={lines} span={set.span} bind={bind} />
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

/**
 * Every piece paints from the one gradient in the column's x (see `light.ts`),
 * so a line warms continuously across pieces that scroll apart. The runs are
 * vertical and hold one colour each: compose writes the colour of their x.
 */
function Bundle({ role, lines, span, bind }: { role: string; lines: readonly Line[]; span: Span | null; bind: Bind }) {
  const id = `dawn-thread-${useId().replace(/:/g, "")}`;
  const stroke = (end: string) => ({ ...line, stroke: span ? `url(#${id}-${end})` : "var(--dawn-thread-rose)" });
  return (
    <>
      {/* Port end: rides the port's track on the page's scroll. */}
      <div ref={bind(`${role}#lift`, "page")} className="absolute top-0 left-0">
        <svg className="absolute top-0 left-0 overflow-visible" width={1} height={1} fill="none">
          {span && <ThreadGradient id={`${id}-port`} span={span} shift={span.origin} />}
          <rect ref={bind(pieceKey(role, "", "port"))} width={3} rx={1.5} style={PORT_STYLE} />
          {lines.map((l) => (
            <path key={l.slug} ref={bind(pieceKey(role, l.slug, "port"))} {...stroke("port")} />
          ))}
        </svg>
      </div>

      {/* Skill end: rides the column's own scroll. */}
      <div ref={bind(`${role}#marks`, "unscroll")} className="absolute inset-x-0 top-0">
        <svg className="absolute top-0 left-0 overflow-visible" width={1} height={1} fill="none">
          {span && <ThreadGradient id={`${id}-mark`} span={span} shift={span.origin} />}
          {lines.map((l) => (
            <path key={l.slug} ref={bind(pieceKey(role, l.slug, "mark"))} {...stroke("mark")} />
          ))}
        </svg>
        {/* Runs: a bar hung from the port end, clipped by a box hung from the
            skill end. The unclip layer undoes the column's scroll and the lift
            layer replays the port's track, so the bar moves with the port. */}
        {lines.map((l) =>
          RUNS.map((dir) => (
            <div key={`${l.slug}-${dir}`} ref={bind(pieceKey(role, l.slug, `clip-${dir}`))} className="absolute inset-x-0 overflow-clip">
              <div ref={bind(pieceKey(role, l.slug, `unclip-${dir}`), "column")} className="absolute left-0">
                <div ref={bind(pieceKey(role, l.slug, `lift-${dir}`), "page")} className="absolute top-0 left-0">
                  <div ref={bind(pieceKey(role, l.slug, `bar-${dir}`))} className="absolute w-[1.5px]" />
                </div>
              </div>
            </div>
          )),
        )}
      </div>
    </>
  );
}
