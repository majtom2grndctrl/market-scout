"use client";

// The one sanctioned loop. The same job, spelled the ways employers spell it,
// rolls beneath the headline. The ledger below holds every spelling with its
// posting count, and a rule slides to whichever one is showing, so the loop
// always points back at evidence. Static when there is nothing to cycle, when
// the reader prefers reduced motion, or while the stage is off screen.

import { motion, useInView, useReducedMotion } from "motion/react";
import { useEffect, useRef, useState } from "react";

import type { TitleVariant } from "../_data/query";
import { EASE_OUT } from "./motion";

const DWELL_MS = 2400;

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
      <p className="font-sans text-sm text-content-secondary">Also called</p>

      {/* Every spelling shares one grid cell, so the block is as tall as the
          longest and the page never jumps when a long title rolls in. */}
      <div
        aria-hidden
        className="mt-1 grid overflow-hidden pb-[0.12em] font-display text-[clamp(1.875rem,5.2cqi,4.5rem)] font-light leading-[1.02] tracking-[-0.025em] text-balance"
      >
        {alternates.map((t, i) => {
          const offset = (i - (active % n) + n) % n;
          const state = offset === 0 ? "in" : offset === n - 1 && n > 1 ? "out" : "next";
          return (
            <motion.span
              key={t.title}
              className="[grid-area:1/1]"
              initial={false}
              animate={state}
              variants={{
                in: { y: "0%", opacity: 1 },
                out: { y: "-55%", opacity: 0 },
                next: { y: "55%", opacity: 0 },
              }}
              transition={{ duration: 0.75, ease: EASE_OUT }}
            >
              {t.title}
            </motion.span>
          );
        })}
      </div>

      <ul className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5 font-sans text-sm text-content-secondary" aria-label="Spellings in open postings">
        {titles.map((t) => {
          const isHeadline = t.title.toLowerCase() === headline.toLowerCase();
          const showing = !isHeadline && current?.title === t.title;
          return (
            <li key={t.title} className="relative">
              <span className={isHeadline ? "font-semibold text-content-primary" : undefined}>{t.title}</span>{" "}
              <span className="tabular-nums text-content-muted">{t.postings.toLocaleString("en-US")}</span>
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
