"use client";

import { AnimatePresence, motion } from "motion/react";
import { useSyncExternalStore } from "react";

import { pieceKey, type Bind, type Line, type ThreadsStore } from "../_lib/store";
import styles from "../threads.module.css";

const RUNS = ["down", "up"] as const;
const stroke = { stroke: "var(--threads-line)", strokeWidth: 1.5, strokeLinecap: "round", strokeLinejoin: "round" } as const;

/**
 * Compositor lines (see `compose.ts` for the construction). A layer that
 * sticks exactly as the skill column does holds every piece, so the skill
 * ends stay on their markers on the compositor. Script writes the pieces'
 * shapes through `bind` when a line's topology changes; scroll timelines
 * attached through `bind` move them in between.
 *
 * Shows once the draw-in path (see Connectors) has landed, and fades with it.
 */
export function Pieces({ store, bind }: { store: ThreadsStore; bind: Bind }) {
  const threads = useSyncExternalStore(store.subscribe, store.get, store.get);
  const lines = threads?.lines ?? null;

  return (
    <div aria-hidden className={styles.pieces}>
      <AnimatePresence>
        {threads && lines && lines.length > 0 && (
          <motion.div key={threads.role} className="absolute inset-0" exit={{ opacity: 0, transition: { duration: 0.16, ease: "easeOut" } }}>
            <motion.div
              className="absolute inset-0"
              initial={false}
              animate={{ opacity: threads.visible ? 1 : 0 }}
              transition={{ duration: 0.22, ease: "easeOut" }}
            >
              <div className="absolute inset-0" style={{ opacity: threads.drawn ? 1 : 0 }}>
                <Bundle role={threads.role} lines={lines} bind={bind} />
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function Bundle({ role, lines, bind }: { role: string; lines: readonly Line[]; bind: Bind }) {
  return (
    <>
      {/* Port end: rides the port's track on the page's scroll. */}
      <div ref={bind(`${role}#lift`, "page")} className="absolute top-0 left-0">
        <svg className="absolute top-0 left-0 overflow-visible" width={1} height={1} fill="none">
          <rect ref={bind(pieceKey(role, "", "port"))} width={3} rx={1.5} fill="var(--threads-line)" />
          {lines.map((l) => (
            <path key={l.slug} ref={bind(pieceKey(role, l.slug, "port"))} {...stroke} />
          ))}
        </svg>
      </div>

      {/* Skill end: rides the column's own scroll. */}
      <div ref={bind(`${role}#marks`, "unscroll")} className="absolute inset-x-0 top-0">
        <svg className="absolute top-0 left-0 overflow-visible" width={1} height={1} fill="none">
          {lines.map((l) => (
            <path key={l.slug} ref={bind(pieceKey(role, l.slug, "mark"))} {...stroke} />
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
                  <div ref={bind(pieceKey(role, l.slug, `bar-${dir}`))} className="absolute w-[1.5px] bg-(--threads-line)" />
                </div>
              </div>
            </div>
          )),
        )}
      </div>
    </>
  );
}
