"use client";

// Data marks and rules. Bars are magnitude, so they wear the sequential ramp;
// presence ticks are a yes/no, so they stay achromatic ink.

import { motion } from "motion/react";

import { EASE_OUT, EASE_RULE } from "./motion";

/** A horizontal rule that draws from the left edge. */
export function Rule({ delay = 0, weight = "strong", className = "" }: { readonly delay?: number; readonly weight?: "strong" | "fine"; readonly className?: string }) {
  return (
    <motion.div
      aria-hidden
      className={`origin-left ${weight === "strong" ? "h-0.5 bg-content-primary" : "h-px bg-edge"} ${className}`}
      initial={{ scaleX: 0 }}
      animate={{ scaleX: 1 }}
      transition={{ delay, duration: 0.6, ease: EASE_RULE }}
    />
  );
}

/**
 * P(skill | role) as a bar on a fixed 0–100% track. The track width is the
 * same in every row, so bars compare down the column as well as across it.
 */
export function ShareBar({ share, delay, play = true }: { readonly share: number; readonly delay: number; readonly play?: boolean }) {
  return (
    <div aria-hidden className="relative h-[3px] w-full bg-edge-hairline">
      <motion.div
        className="absolute inset-y-0 left-0 origin-left bg-ramp-4"
        style={{ width: `${Math.min(1, Math.max(0, share)) * 100}%` }}
        initial={{ scaleX: 0 }}
        animate={{ scaleX: play ? 1 : 0 }}
        transition={{ delay, duration: 0.55, ease: EASE_OUT }}
      />
    </div>
  );
}

/**
 * One tick per recommendation, in rank order; filled where the condition
 * holds. Reads as "which of the ranked roles does this touch".
 */
export function PresenceStrip({ cells, delay, label }: { readonly cells: readonly boolean[]; readonly delay: number; readonly label: string }) {
  if (cells.length === 0) return null;
  return (
    <div role="img" aria-label={label} className="flex h-3 items-end gap-[2px]">
      {cells.map((on, i) => (
        <motion.span
          key={i}
          className={`block h-full w-[5px] origin-bottom ${on ? "bg-content-primary" : "bg-edge"}`}
          initial={{ scaleY: 0 }}
          animate={{ scaleY: on ? 1 : 0.34 }}
          transition={{ delay: delay + i * 0.025, duration: 0.28, ease: EASE_OUT }}
        />
      ))}
    </div>
  );
}
