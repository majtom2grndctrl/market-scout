"use client";

// The one sanctioned loop. The same job, spelled the ways employers spell it,
// rolls beneath the headline. The ledger below holds every spelling with its
// posting count, and a rule slides to whichever one is showing, so the loop
// always points back at evidence. Static when there is nothing to cycle, when
// the reader prefers reduced motion, or while the stage is off screen.

import { AnimatePresence, motion, useInView, useReducedMotion } from "motion/react";
import { useEffect, useRef, useState } from "react";

import type { TitleVariant } from "../_data/query";
import { EASE_OUT } from "./motion";
import { ROLL, SUPPORT } from "./scale";

const DWELL_MS = 2400;
const PREFIX = "Also called ";

interface AlsoCalledProps {
  readonly slug: string;
  readonly headline: string;
  readonly titles: readonly TitleVariant[];
  readonly seen: boolean;
}

export function AlsoCalled({ slug, headline, titles, seen }: AlsoCalledProps) {
  const ref = useRef<HTMLDivElement>(null);
  const inView = useInView(ref);
  const reduce = useReducedMotion();
  const [active, setActive] = useState(0);

  const alternates = titles.filter((t) => t.title.toLowerCase() !== headline.toLowerCase());
  const n = alternates.length;
  const cycling = seen && inView && !reduce && n > 1;

  useEffect(() => {
    if (!cycling) return;
    const id = window.setInterval(() => setActive((a) => (a + 1) % n), DWELL_MS);
    return () => window.clearInterval(id);
  }, [cycling, n]);

  if (n === 0) return null;
  const current = alternates[active % n];

  return (
    <div ref={ref} className="mt-[clamp(1rem,2.4cqi,2rem)]">
      {/* "Also called" leads the line at the roll's own size, so no label sits
          above it, and a long spelling wraps after it as a sentence would. All
          layers share one grid cell: the still prefix, hidden sizers that hold
          the cell at the tallest phrase so the page never jumps, and the rolling
          title, which reserves the prefix's width with an invisible copy. The
          roll is a clip, not a fade: a title is either fully inked or outside
          the window, never half-read. The ledger carries it all for assistive tech. */}
      <p aria-hidden className={`${ROLL} grid overflow-hidden pb-[0.12em] text-pretty`}>
        <span className="text-content-secondary [grid-area:1/1]">{PREFIX}</span>
        {alternates.map((t) => (
          <span key={t.title} className="invisible [grid-area:1/1]">
            {PREFIX}
            {t.title}
          </span>
        ))}
        <AnimatePresence initial={false}>
          <motion.span
            key={current?.title}
            className="[grid-area:1/1]"
            initial={{ y: "115%" }}
            animate={{ y: "0%" }}
            exit={{ y: "-115%" }}
            transition={{ duration: 0.75, ease: EASE_OUT }}
          >
            <span className="invisible">{PREFIX}</span>
            {current?.title}
          </motion.span>
        </AnimatePresence>
      </p>

      <ul className={`${SUPPORT} mt-[0.9em] flex flex-wrap gap-x-[1.1em] gap-y-[0.35em]`} aria-label="Also called, in open postings">
        {titles.map((t) => {
          const isHeadline = t.title.toLowerCase() === headline.toLowerCase();
          const showing = !isHeadline && current?.title === t.title;
          return (
            <li key={t.title} className="relative">
              <span className={isHeadline ? "font-semibold" : undefined}>{t.title}</span>{" "}
              <span className="tabular-nums text-content-secondary">{t.postings.toLocaleString("en-US")}</span>
              {showing && n > 1 && (
                <motion.span
                  layoutId={`marquee-spelling-${slug}`}
                  className="absolute inset-x-0 -bottom-0.5 h-0.5 bg-content-primary"
                  transition={{ duration: 0.6, ease: EASE_OUT }}
                />
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
