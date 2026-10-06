"use client";

// Where pins collect. Sticky to the bottom of the page so the count and the
// way onward are always one glance away. A new pin lands here: the count rolls
// and the role's name rises into the line in its plate colour.

import { AnimatePresence, motion } from "motion/react";
import { useState, type CSSProperties } from "react";

import { EASE_OUT } from "./motion";
import { plateOf, type Plates } from "./palette";
import type { Role } from "./types";

interface PinTrayProps {
  readonly roles: readonly Role[];
  /** Slugs, in the order they were pinned. */
  readonly pinned: readonly string[];
  readonly plates: Plates;
}

export function PinTray({ roles, pinned, plates }: PinTrayProps) {
  // The dashboard is not built. Continuing only acknowledges, and the
  // acknowledgement lapses when the pins change.
  const [ackAt, setAckAt] = useState<string | null>(null);
  const key = pinned.join(",");
  const acknowledged = ackAt === key;
  const kept = pinned.flatMap((slug) => roles.filter((r) => r.roleSlug === slug));
  const count = kept.length;

  return (
    <div className="sticky bottom-0 z-20 border-t border-content-primary bg-surface-raised">
      <div className="mx-auto flex max-w-[96rem] items-center gap-4 px-5 py-3 sm:gap-6 sm:px-10">
        <p className="flex shrink-0 items-baseline gap-2">
          <span className="inline-grid overflow-hidden font-display text-4xl leading-none font-extrabold tabular-nums tracking-[-0.04em]">
            <AnimatePresence initial={false} mode="popLayout">
              <motion.span
                key={count}
                className="[grid-area:1/1]"
                initial={{ y: "100%" }}
                animate={{ y: "0%" }}
                exit={{ y: "-100%" }}
                transition={{ duration: 0.5, ease: EASE_OUT }}
              >
                {count}
              </motion.span>
            </AnimatePresence>
          </span>
          <span className="font-sans text-lg text-content-secondary">pinned</span>
        </p>

        <p
          className="hidden min-w-0 flex-1 overflow-hidden font-display text-xl font-semibold whitespace-nowrap tracking-[-0.02em] sm:block [mask-image:linear-gradient(to_right,black_88%,transparent)]"
          aria-live="polite"
        >
          {count === 0 ? (
            <span className="font-sans text-lg font-normal tracking-normal text-content-secondary">
              Pin the roles worth watching. They collect here.
            </span>
          ) : (
            <AnimatePresence initial={false} mode="popLayout">
              {kept.map((r) => (
                <motion.span
                  key={r.roleSlug}
                  layout
                  className="mr-[0.6em] inline-block"
                  initial={{ y: "120%", opacity: 0 }}
                  animate={{ y: "0%", opacity: 1 }}
                  exit={{ opacity: 0, transition: { duration: 0.2 } }}
                  transition={{ duration: 0.6, ease: EASE_OUT }}
                >
                  <span
                    className="marquee-band"
                    data-on
                    style={{ "--marquee-plate": plateOf(plates, r.closestPast?.titleText) } as CSSProperties}
                  >
                    {r.headline}.
                  </span>
                </motion.span>
              ))}
            </AnimatePresence>
          )}
        </p>

        <button
          type="button"
          disabled={count === 0}
          onClick={() => setAckAt(key)}
          className="ml-auto shrink-0 rounded-md bg-accent-solid px-5 py-3 font-display text-base font-semibold tracking-[-0.01em] text-on-solid transition-colors outline-offset-2 hover:bg-accent-hover focus-visible:outline-2 focus-visible:outline-focus disabled:cursor-not-allowed disabled:bg-surface-sunken disabled:text-content-disabled"
        >
          {acknowledged ? "Noted. The dashboard comes next." : "Continue to your dashboard"}
        </button>
      </div>
    </div>
  );
}
