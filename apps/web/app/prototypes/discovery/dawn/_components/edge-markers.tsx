"use client";

import { ArrowDown, ArrowUp } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useSyncExternalStore, type MouseEvent } from "react";

import { EDGE_GAP, EDGE_H, landingHeight, type Edge } from "../_lib/edges";
import type { LineMode } from "../_lib/modes";
import { arrivalOf, drift } from "../_lib/motion";
import type { LineStore } from "../_lib/store";
import styles from "../dawn.module.css";
import { cn } from "@/lib/utils";

/**
 * Where lines end when the lit role draws on skills scrolled out of the
 * column: one marker pinned to each edge with skills beyond it, counting
 * them. Activating one scrolls them into view. Lines land on the small bar at
 * its right end, the twin of the port they leave from on the role.
 *
 * In a layer that sticks exactly as the column does (see `.edges`), outside
 * the column's soft-edge mask, so it reads at full strength over the faded
 * rows.
 */
export function EdgeMarkers({ store, mode, onReveal }: { store: LineStore; mode: LineMode; onReveal: (edge: Edge) => void }) {
  const set = useSyncExternalStore(store.subscribe, store.get, store.get);
  const lines = set && !set.quiet ? (set.lines ?? []) : [];
  // Arrive with the line: while it draws in, wait for its longest line to land.
  const drawing = set !== null && (mode === "fade" || !set.drawn);
  const delay = (edge: Edge) => (drawing ? Math.max(0, ...lines.filter((l) => l.edge === edge).map((l) => arrivalOf(l.length))) : 0);
  const at = (edge: Edge) => lines.filter((l) => l.edge === edge);

  return (
    <div className={styles.edges}>
      <AnimatePresence>
        {(["above", "below"] as const).map((edge) => {
          const here = at(edge);
          return here.length > 0 && set ? (
            <Marker
              key={`${set.role}:${edge}`}
              edge={edge}
              count={here.length}
              bar={landingHeight(here.map((l) => l.width))}
              role={set.role}
              delay={delay(edge)}
              onReveal={onReveal}
            />
          ) : null;
        })}
      </AnimatePresence>
    </div>
  );
}

function Marker({
  edge,
  count,
  bar,
  role,
  delay,
  onReveal,
}: {
  edge: Edge;
  count: number;
  /** The landing bar's height, so every line arriving lands on it. */
  bar: number;
  role: string;
  delay: number;
  onReveal: (edge: Edge) => void;
}) {
  const Arrow = edge === "above" ? ArrowUp : ArrowDown;
  const onClick = (e: MouseEvent<HTMLButtonElement>) => {
    // From the keyboard, focus returns to the lit role once the marker it
    // was on has gone.
    const fromKeys = e.detail === 0;
    onReveal(edge);
    if (fromKeys) document.querySelector<HTMLElement>(`[data-dawn-role="${CSS.escape(role)}"]`)?.focus({ preventScroll: true });
  };
  return (
    <motion.button
      type="button"
      onClick={onClick}
      style={{ [edge === "above" ? "top" : "bottom"]: EDGE_GAP, height: EDGE_H }}
      className={cn(
        "pointer-events-auto absolute right-0 inline-flex cursor-pointer items-center gap-1.5 rounded-full pr-2.5 pl-2.5 text-sm leading-none font-medium whitespace-nowrap outline-none",
        "bg-(--dawn-glow) text-content-primary shadow-(--dawn-halo) transition-[background-color] duration-200 ease-out hover:bg-(--dawn-horizon)",
        "focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2",
      )}
      initial={{ opacity: 0, y: edge === "above" ? -6 : 6 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, transition: { duration: 0.16, ease: "easeOut" } }}
      transition={drift(delay, 0.6)}
    >
      <Arrow aria-hidden className="size-3.5 text-(--dawn-ember)" strokeWidth={2.25} />
      <span className="tabular-nums">
        <span className="sr-only">Show </span>
        {count} more {edge}
      </span>
      <span aria-hidden style={{ height: bar }} className="ml-0.5 w-[3px] rounded-full bg-(--dawn-ember) transition-[height] duration-200 ease-out" />
    </motion.button>
  );
}
