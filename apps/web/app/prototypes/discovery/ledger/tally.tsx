"use client";

// The running tally, pinned to the bottom of the viewport: where pins collect.
// A new pin snaps in at the end of the line and the count rolls, so the eye
// lands here after every toggle in the table.

import { ArrowRight, Check } from "lucide-react";
import { AnimatePresence, LayoutGroup, motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { plural, rankLabel } from "./derive";
import { EASE_OUT, Roll } from "./motion";

interface TallyProps {
  readonly pins: readonly Recommendation[];
  readonly acknowledged: boolean;
  readonly onContinue: () => void;
  readonly onUnpin: (slug: string) => void;
}

const SNAP = { type: "spring", stiffness: 900, damping: 42, mass: 0.6 } as const;

export function Tally({ pins, acknowledged, onContinue, onUnpin }: TallyProps) {
  const n = pins.length;
  return (
    <motion.aside
      aria-label="Pinned roles"
      initial={{ y: "100%" }}
      animate={{ y: 0 }}
      transition={{ delay: 0.5, duration: 0.45, ease: EASE_OUT }}
      className="sticky bottom-0 z-10 mt-16 -mx-6 border-t-2 border-content-primary bg-surface-page/95 px-6 backdrop-blur-sm @5xl:-mx-10 @5xl:px-10"
    >
      <div className="flex flex-col gap-3 py-3.5 @3xl:flex-row @3xl:items-center @3xl:gap-6">
        {/* The count reads into its word, "2 pinned", so the label needs no tag of its own. */}
        <div className="flex shrink-0 items-baseline gap-2">
          <Roll value={n} className={`font-display text-[28px] leading-none font-semibold tabular-nums ${n > 0 ? "text-(--ledger-signal)" : "text-content-muted"}`} />
          <span className="text-[15px] font-medium text-content-secondary">pinned</span>
        </div>

        <LayoutGroup>
          <ul className="flex min-w-0 flex-1 flex-wrap items-center gap-x-1.5 gap-y-1.5">
            <AnimatePresence mode="popLayout" initial={false}>
              {n === 0 && (
                <motion.li key="empty" layout initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0, transition: { duration: 0.1 } }} className="text-[14px] text-content-muted">
                  Pin the roles worth watching; they collect here.
                </motion.li>
              )}
              {pins.map((r) => (
                <motion.li
                  key={r.roleSlug}
                  layout
                  initial={{ opacity: 0, y: 10, scale: 0.94 }}
                  animate={{ opacity: 1, y: 0, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.94, transition: { duration: 0.12 } }}
                  transition={SNAP}
                >
                  <button
                    type="button"
                    onClick={() => onUnpin(r.roleSlug)}
                    aria-label={`Unpin ${r.headline}`}
                    className="group/chip flex items-baseline gap-2 border border-edge bg-surface-raised py-1 pr-2.5 pl-2 text-[14px] leading-5 transition-colors duration-150 hover:border-content-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus active:bg-surface-sunken"
                  >
                    <span className="font-display text-[14px] font-semibold text-(--ledger-signal) tabular-nums">{rankLabel(r.rank)}</span>
                    <span>{r.headline}</span>
                    <span aria-hidden className="text-content-muted transition-colors group-hover/chip:text-content-primary">
                      ×
                    </span>
                  </button>
                </motion.li>
              ))}
            </AnimatePresence>
          </ul>
        </LayoutGroup>

        {/* Disabled is carried by the empty outline and the cursor; the label stays at muted ink because it says what pinning unlocks. */}
        <button
          type="button"
          disabled={n === 0}
          onClick={onContinue}
          className={`flex shrink-0 items-center justify-center gap-2 px-4 py-2.5 text-[14px] font-medium transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus ${
            n === 0
              ? "cursor-not-allowed border border-edge text-content-muted"
              : acknowledged
                ? "border border-content-primary text-content-primary"
                : "bg-accent-solid text-on-solid hover:bg-accent-hover active:bg-accent-active"
          }`}
        >
          <AnimatePresence mode="wait" initial={false}>
            <motion.span
              key={acknowledged ? "ack" : "go"}
              initial={{ opacity: 0, y: 4 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -4 }}
              transition={{ duration: 0.16, ease: EASE_OUT }}
              className="flex items-center gap-2"
            >
              {acknowledged ? (
                <>
                  <Check className="size-4" strokeWidth={2} />
                  Continuing with {n} {plural(n, "role", "roles")}
                </>
              ) : (
                <>
                  Continue to your dashboard
                  <ArrowRight className="size-4" strokeWidth={2} />
                </>
              )}
            </motion.span>
          </AnimatePresence>
        </button>
      </div>
    </motion.aside>
  );
}
