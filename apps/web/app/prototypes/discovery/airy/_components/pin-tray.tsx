"use client";

import { ArrowRight, Check, X } from "lucide-react";
import { AnimatePresence, motion, useAnimate } from "motion/react";
import { useEffect, useRef } from "react";

import type { Recommendation } from "../../_data/query";
import { capitalize, countWord, plural } from "../_lib/copy";
import { drift, FLIGHT, SETTLE } from "../_lib/motion";
import { pinLayoutId } from "./pin-button";
import { cn } from "@/lib/utils";

// Chips cannot scroll: a scroll container would clip a pin mid-flight. The
// newest few stay visible and older ones fold into a count.
const VISIBLE = 2;

const TRAY_AT = 1.6;

function scrollToRole(roleSlug: string) {
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  document.getElementById(`airy-role-${roleSlug}`)?.scrollIntoView({ behavior: reduce ? "auto" : "smooth", block: "center" });
}

function Count({ n }: { n: number }) {
  return (
    <div className="flex shrink-0 items-center gap-3" aria-live="polite">
      <span className="relative inline-grid h-10 min-w-[1.75ch] overflow-hidden font-display text-[2.25rem] leading-10 font-light tracking-[-0.03em] text-content-primary tabular-nums">
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.span
            key={n}
            initial={{ y: "70%", opacity: 0 }}
            animate={{ y: 0, opacity: 1 }}
            exit={{ y: "-70%", opacity: 0 }}
            transition={SETTLE}
          >
            {n}
          </motion.span>
        </AnimatePresence>
      </span>
      <span className="text-[0.8125rem] leading-tight text-content-secondary">
        {plural(n, "role")}
        <br />
        pinned
      </span>
    </div>
  );
}

function PinChip({ rec, onUnpin }: { rec: Recommendation; onUnpin: (roleSlug: string) => void }) {
  return (
    <motion.li
      layoutId={pinLayoutId(rec.roleSlug)}
      layout
      transition={FLIGHT}
      style={{ borderRadius: 999 }}
      className="flex h-9 min-w-0 shrink items-center bg-(--airy-glow) ring-1 ring-(--airy-glow-edge) ring-inset"
    >
      <motion.span
        layout="position"
        className="flex min-w-0 items-center"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 0.22, duration: 0.35, ease: "easeOut" }}
      >
        <button
          type="button"
          onClick={() => scrollToRole(rec.roleSlug)}
          className="max-w-[13rem] min-w-0 cursor-pointer truncate rounded-full py-1 pr-1 pl-3.5 text-[0.8125rem] font-medium text-content-primary outline-none focus-visible:ring-2 focus-visible:ring-focus"
        >
          {rec.headline}
        </button>
        <button
          type="button"
          aria-label={`Unpin ${rec.headline}`}
          onClick={() => onUnpin(rec.roleSlug)}
          className="mr-1 grid size-7 shrink-0 cursor-pointer place-items-center rounded-full text-content-muted transition-colors duration-200 outline-none hover:bg-surface-raised hover:text-content-primary focus-visible:ring-2 focus-visible:ring-focus active:scale-95"
        >
          <X className="size-3.5" />
        </button>
      </motion.span>
    </motion.li>
  );
}

export function PinTray({
  pins,
  onUnpin,
  acknowledged,
  onContinue,
}: {
  pins: readonly Recommendation[];
  onUnpin: (roleSlug: string) => void;
  acknowledged: boolean;
  onContinue: () => void;
}) {
  const n = pins.length;
  const shown = pins.slice(-VISIBLE);
  const folded = n - shown.length;
  const ready = n > 0;

  // A soft lift each time a pin lands, so the tray answers the gesture.
  const [scope, animate] = useAnimate();
  const prev = useRef(n);
  useEffect(() => {
    if (n > prev.current && scope.current) {
      animate(scope.current, { y: [0, -5, 0] }, { duration: 0.7, delay: 0.32, ease: [0.33, 1, 0.68, 1] });
    }
    prev.current = n;
  }, [n, animate, scope]);

  return (
    <motion.aside
      aria-label="Pinned roles"
      className="sticky bottom-5 z-20 mx-auto mt-28 w-[calc(100%-2rem)] max-w-[60rem] md:bottom-7"
      initial={{ y: 120, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      transition={{ type: "spring", stiffness: 120, damping: 20, mass: 1, delay: TRAY_AT }}
    >
      <div
        ref={scope}
        className="flex flex-wrap items-center gap-x-4 gap-y-3 rounded-[1.75rem] bg-surface-raised/78 p-3 pl-5 shadow-(--airy-tray) ring-1 ring-edge-hairline backdrop-blur-xl backdrop-saturate-150 md:flex-nowrap md:gap-6 md:pl-6"
      >
        <Count n={n} />

        <div className="order-last flex min-h-9 w-full min-w-0 items-center md:order-none md:w-auto md:flex-1">
          <AnimatePresence initial={false} mode="popLayout">
            {n === 0 && (
              <motion.p
                key="hint"
                className="text-sm text-content-muted"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0, transition: { duration: 0.15 } }}
                transition={drift(0.2, 0.6)}
              >
                Pin the roles worth watching. They gather here.
              </motion.p>
            )}
          </AnimatePresence>
          <ul className="flex min-w-0 items-center gap-2">
            {folded > 0 && (
              <motion.li
                layout
                transition={FLIGHT}
                className="grid h-9 shrink-0 place-items-center rounded-full px-3 text-[0.8125rem] font-medium text-content-secondary ring-1 ring-edge ring-inset tabular-nums"
              >
                +{folded}
              </motion.li>
            )}
            {shown.map((r) => (
              <PinChip key={r.roleSlug} rec={r} onUnpin={onUnpin} />
            ))}
          </ul>
        </div>

        <motion.button
          type="button"
          layout
          transition={SETTLE}
          whileTap={ready ? { scale: 0.97 } : undefined}
          disabled={!ready}
          onClick={onContinue}
          className={cn(
            "group/go ml-auto inline-flex h-12 shrink-0 items-center justify-center gap-2.5 rounded-[1.25rem] px-5 text-[0.9375rem] font-medium outline-none",
            "transition-[background-color,color] duration-300 ease-out focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2",
            ready
              ? "cursor-pointer bg-accent-solid text-on-solid hover:bg-accent-hover active:bg-accent-active"
              : "cursor-not-allowed bg-surface-sunken text-content-disabled",
          )}
        >
          <AnimatePresence mode="popLayout" initial={false}>
            {acknowledged ? (
              <motion.span
                key="ack"
                className="inline-flex items-center gap-2.5"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={SETTLE}
              >
                <Check className="size-4" strokeWidth={2.5} />
                {`${capitalize(countWord(n))} ${plural(n, "role")} ready to watch`}
              </motion.span>
            ) : (
              <motion.span
                key="go"
                className="inline-flex items-center gap-2.5"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={SETTLE}
              >
                <span>
                  Continue<span className="max-sm:hidden"> to your dashboard</span>
                </span>
                <ArrowRight className="size-4 transition-transform duration-300 ease-out group-hover/go:translate-x-0.5" />
              </motion.span>
            )}
          </AnimatePresence>
        </motion.button>
      </div>
    </motion.aside>
  );
}
