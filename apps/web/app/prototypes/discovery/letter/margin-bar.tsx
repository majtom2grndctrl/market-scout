"use client";

import { ArrowLeft, ArrowRight, Check, Pin } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";

import { cn } from "@/lib/utils";

import { count } from "./grammar";
import { FOCUS_RING } from "./pin-button";
import { EASE_OUT, mainThread } from "./reveal";

/**
 * The letter's foot: where you are, where pins collect, and the way on. It
 * stays put while beats change above it, so the eye only travels here when
 * asked to: a pin lands in the tally, the last beat offers Continue.
 */
export function MarginBar({
  labels,
  index,
  go,
  pinned,
  continued,
  onContinue,
}: {
  labels: readonly string[];
  index: number;
  go: (to: number) => void;
  pinned: number;
  continued: boolean;
  onContinue: () => void;
}) {
  const last = labels.length - 1;
  const atEnd = index === last;

  return (
    <div className="flex h-18 shrink-0 items-center gap-4 border-t border-edge-hairline px-6 sm:gap-6 sm:px-10 lg:px-16">
      <nav aria-label="Where you are in the letter" className="flex items-center gap-5">
        <p className="text-sm text-content-muted tabular-nums sm:hidden">
          {index + 1} / {labels.length}
        </p>
        <ol className="hidden items-center sm:flex">
          {labels.map((label, i) => (
            <li key={i}>
              <button
                type="button"
                onClick={() => go(i)}
                aria-label={`${i + 1} of ${labels.length}: ${label}`}
                aria-current={i === index ? "step" : undefined}
                className={cn(FOCUS_RING, "group flex h-8 items-center rounded-full px-1")}
              >
                <span
                  className={cn(
                    "block h-[3px] rounded-full transition-[width,background-color] duration-500 ease-out",
                    i === index ? "w-9 bg-(--letter-accent)" : "w-3.5 group-hover:bg-edge-strong",
                    i < index ? "bg-content-disabled" : i > index ? "bg-edge" : "",
                  )}
                />
              </button>
            </li>
          ))}
        </ol>
        <AnimatePresence>
          {index === 0 ? (
            <motion.p
              {...mainThread}
              className="hidden text-sm text-content-muted md:block"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1, transition: { delay: 2.6, duration: 0.8 } }}
              exit={{ opacity: 0, transition: { duration: 0.2 } }}
            >
              Arrow keys, space, or scroll to read on
            </motion.p>
          ) : null}
        </AnimatePresence>
      </nav>

      <div className="ml-auto flex items-center gap-3 sm:gap-5">
        {!atEnd ? (
          <button
            type="button"
            onClick={() => go(last)}
            className={cn(FOCUS_RING, "hidden rounded-sm text-sm text-content-muted underline-offset-4 hover:text-content-primary hover:underline lg:block")}
          >
            Skip to {labels[last]?.toLowerCase()}
          </button>
        ) : null}

        <PinTally pinned={pinned} />

        <button
          type="button"
          onClick={() => go(index - 1)}
          disabled={index === 0}
          aria-label={index > 0 ? `Back: ${labels[index - 1]}` : "Back"}
          className={cn(
            FOCUS_RING,
            "flex size-11 items-center justify-center rounded-full border border-edge text-content-secondary transition-colors hover:border-edge-strong hover:text-content-primary disabled:pointer-events-none disabled:opacity-0",
          )}
        >
          <ArrowLeft className="size-4" strokeWidth={1.75} />
        </button>

        {atEnd ? (
          <ContinueButton pinned={pinned} continued={continued} onContinue={onContinue} />
        ) : (
          <button
            type="button"
            onClick={() => go(index + 1)}
            className={cn(FOCUS_RING, "group flex items-center gap-3 rounded-full py-1 pl-1 text-sm")}
          >
            <span className="hidden max-w-[18rem] truncate text-right sm:block">
              <span className="text-content-muted">Next&ensp;·&ensp;</span>
              <span className="text-content-primary">{labels[index + 1]}</span>
            </span>
            <span className="sr-only sm:hidden">Next: {labels[index + 1]}</span>
            <span className="flex size-11 items-center justify-center rounded-full bg-accent-solid text-on-solid transition-colors group-hover:bg-accent-hover">
              <motion.span
                className="inline-flex"
                whileHover={{ x: 2 }}
                transition={{ type: "spring", stiffness: 400, damping: 25 }}
              >
                <ArrowRight className="size-4" strokeWidth={2} />
              </motion.span>
            </span>
          </button>
        )}
      </div>
    </div>
  );
}

function PinTally({ pinned }: { pinned: number }) {
  return (
    <p aria-live="polite" className="flex items-center text-sm text-content-secondary">
      <Pin className={cn("mr-2 size-3.5", pinned > 0 && "fill-current text-(--letter-accent)")} strokeWidth={1.75} aria-hidden />
      <span className="relative inline-flex overflow-hidden tabular-nums">
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.span
            key={pinned}
            initial={{ y: "-100%", opacity: 0 }}
            animate={{ y: 0, opacity: 1 }}
            exit={{ y: "100%", opacity: 0 }}
            transition={{ duration: 0.35, ease: EASE_OUT }}
          >
            {pinned > 0 ? count(pinned) : "none"}
          </motion.span>
        </AnimatePresence>
      </span>
      <span className="hidden sm:inline">&nbsp;pinned</span>
    </p>
  );
}

// Before a pin, Continue is a genuinely disabled control in content-disabled
// ink: the one exemption the readable-type rule allows. Its reason sits
// outside it, so the reason is read in ordinary ink.
function ContinueButton({ pinned, continued, onContinue }: { pinned: number; continued: boolean; onContinue: () => void }) {
  const ready = pinned > 0;
  return (
    <>
      {ready ? null : (
        <span id="letter-continue-why" className="sr-only">
          Pin at least one role first.
        </span>
      )}
      <button
        type="button"
        onClick={onContinue}
        disabled={!ready}
        aria-describedby={ready ? undefined : "letter-continue-why"}
        className={cn(
          FOCUS_RING,
          "flex h-11 items-center gap-2 rounded-full px-5 text-sm font-medium transition-colors duration-300",
          ready ? "bg-accent-solid text-on-solid hover:bg-accent-hover" : "cursor-not-allowed bg-surface-sunken text-content-disabled",
        )}
      >
        <AnimatePresence mode="wait" initial={false}>
          <motion.span
            key={continued ? "done" : "go"}
            className="flex items-center gap-2"
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.25 }}
          >
            {continued ? (
            <>
                <Check className="size-4" strokeWidth={2} />
                <span>
                  Noted.<span className="hidden sm:inline"> Your dashboard is next.</span>
                </span>
            </>
          ) : (
            <>
                <span>
                  Continue<span className="hidden sm:inline"> to your dashboard</span>
                </span>
                <ArrowRight className="size-4" strokeWidth={2} />
            </>
          )}
        </motion.span>
      </AnimatePresence>
    </button>
    </>
  );
}
