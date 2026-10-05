"use client";

// The pin is set as type, not as a button. Its disc sits where a full stop
// would, and fills when the role is kept: the same full stop that lands on a
// pinned headline, its index entry, and the tray. One mark means "kept"
// everywhere on the page.

import { motion } from "motion/react";

interface PinToggleProps {
  readonly label: string;
  readonly pinned: boolean;
  readonly onToggle: () => void;
}

export function PinToggle({ label, pinned, onToggle }: PinToggleProps) {
  return (
    <button
      type="button"
      aria-pressed={pinned}
      aria-label={`Pin ${label}`}
      onClick={onToggle}
      className="group -m-2 inline-flex shrink-0 items-baseline gap-[0.12em] rounded-md p-2 font-display text-[clamp(1.375rem,2.4cqi,2rem)] leading-none font-semibold tracking-[-0.025em] outline-offset-2 focus-visible:outline-2 focus-visible:outline-focus"
    >
      {/* Both labels hold the cell, right-aligned, so the disc never moves. */}
      <span className="grid justify-items-end">
        <span className={`[grid-area:1/1] transition-opacity duration-300 ${pinned ? "opacity-0" : "opacity-100"}`}>
          Pin
        </span>
        <span className={`[grid-area:1/1] transition-opacity duration-300 ${pinned ? "opacity-100" : "opacity-0"}`}>
          Pinned
        </span>
      </span>
      <span className="relative grid size-[0.4em] place-items-center rounded-full border-[0.075em] border-current transition-transform duration-300 group-hover:scale-125">
        <motion.span
          className="size-full rounded-full bg-current"
          initial={false}
          animate={{ scale: pinned ? 1 : 0 }}
          transition={{ type: "spring", stiffness: 520, damping: 20 }}
        />
      </span>
    </button>
  );
}
