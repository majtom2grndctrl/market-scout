"use client";

import { AnimatePresence, motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { EASE_OUT } from "./choreography";
import { PinGlyph } from "./pin-glyph";

interface Props {
  className?: string;
  pinned: Recommendation[];
  /** Holds the tray back until the map's entrance has finished. */
  delay: number;
  continued: boolean;
  onUnpin: (slug: string) => void;
  onFocus: (slug: string) => void;
  onContinue: () => void;
}

/** Where pins collect. Local state only; nothing here writes the profile. */
export function PinTray({ className, pinned, delay, continued, onUnpin, onFocus, onContinue }: Props) {
  const ready = pinned.length > 0;

  return (
    <motion.section
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay, duration: 0.5, ease: EASE_OUT }}
      className={`rounded-xl border border-edge-hairline bg-surface-raised p-4 shadow-sm ${className ?? ""}`}
      aria-label="Pinned roles"
    >
      <header className="flex items-baseline justify-between">
        <h2 className="font-display text-base font-semibold">Pinned</h2>
        <motion.span
          key={pinned.length}
          initial={{ scale: 1.35, opacity: 0.4 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ type: "spring", stiffness: 420, damping: 20 }}
          className="text-xs font-medium text-content-muted tabular-nums"
        >
          {pinned.length} {pinned.length === 1 ? "role" : "roles"}
        </motion.span>
      </header>

      <ul className="mt-2 max-h-44 overflow-y-auto">
        <AnimatePresence initial={false}>
          {pinned.map((r) => (
            <motion.li
              key={r.roleSlug}
              layout
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: "auto" }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ duration: 0.3, ease: EASE_OUT }}
              className="overflow-hidden"
            >
              <div className="group flex items-center gap-2.5 border-t border-edge-hairline py-2" onMouseEnter={() => onFocus(r.roleSlug)}>
                <motion.span
                  initial={{ rotate: -90, scale: 0.2 }}
                  animate={{ rotate: 0, scale: 1 }}
                  transition={{ type: "spring", stiffness: 300, damping: 16, delay: 0.1 }}
                  className="text-(--constellation-pin-ink)"
                >
                  <PinGlyph />
                </motion.span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{r.headline}</span>
                  <span className="block truncate text-xs text-content-muted">From {r.closestPast?.titleText ?? "your profile as a whole"}</span>
                </span>
                <button
                  type="button"
                  onClick={() => onUnpin(r.roleSlug)}
                  className="rounded-md px-1.5 py-0.5 text-xs text-content-muted opacity-0 transition-opacity group-hover:opacity-100 hover:text-content-primary focus-visible:opacity-100"
                  aria-label={`Unpin ${r.headline}`}
                >
                  Remove
                </button>
              </div>
            </motion.li>
          ))}
        </AnimatePresence>
      </ul>

      {!ready && <p className="mt-1 text-[13px] leading-snug text-content-muted">Click a star to pin it. Pinned roles collect here.</p>}

      <button
        type="button"
        disabled={!ready}
        onClick={onContinue}
        className="mt-3 flex h-10 w-full items-center justify-center rounded-lg bg-accent-solid text-sm font-medium text-on-solid transition-colors hover:bg-accent-hover disabled:cursor-not-allowed disabled:bg-surface-sunken disabled:text-content-disabled"
      >
        <AnimatePresence mode="wait" initial={false}>
          <motion.span
            key={continued && ready ? "done" : "go"}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.18 }}
          >
            {continued && ready ? "Noted. The dashboard is next." : "Continue to your dashboard"}
          </motion.span>
        </AnimatePresence>
      </button>
    </motion.section>
  );
}
