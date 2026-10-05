"use client";

// A square check box, set in the signal colour when pinned. Pressed scales the
// box, not the hit area, so the target stays put under the pointer.

import { motion } from "motion/react";

import { EASE_OUT } from "./motion";

interface PinToggleProps {
  readonly pinned: boolean;
  readonly headline: string;
  readonly onToggle: () => void;
  readonly className?: string;
}

export function PinToggle({ pinned, headline, onToggle, className = "" }: PinToggleProps) {
  return (
    <button
      type="button"
      aria-pressed={pinned}
      aria-label={pinned ? `Unpin ${headline}` : `Pin ${headline}`}
      onClick={(e) => {
        // The row toggles its detail on click; pinning must not.
        e.stopPropagation();
        onToggle();
      }}
      className={`group/pin -m-2 flex items-center gap-2 self-start p-2 focus-visible:outline-none ${className}`}
    >
      <span
        className={`flex size-[18px] items-center justify-center border-[1.5px] transition-[transform,background-color,border-color] duration-150 group-active/pin:scale-[0.86] group-focus-visible/pin:outline-2 group-focus-visible/pin:outline-offset-2 group-focus-visible/pin:outline-focus ${
          pinned
            ? "border-(--ledger-signal) bg-(--ledger-signal)"
            : "border-content-muted bg-surface-raised group-hover/pin:border-content-primary group-active/pin:bg-surface-sunken"
        }`}
      >
        <svg viewBox="0 0 12 12" className="size-3 text-on-solid" aria-hidden>
          <motion.path
            d="M2.5 6.2 L5 8.6 L9.6 3.4"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.8}
            strokeLinecap="square"
            initial={false}
            animate={{ pathLength: pinned ? 1 : 0, opacity: pinned ? 1 : 0 }}
            transition={{ duration: pinned ? 0.22 : 0.1, ease: EASE_OUT }}
          />
        </svg>
      </span>
    </button>
  );
}
