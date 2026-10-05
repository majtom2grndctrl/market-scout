"use client";

import { motion } from "motion/react";
import type { ReactNode } from "react";

import type { DiscoveryData } from "../../_data/query";
import { coverageLine, ledeFor } from "../_lib/copy";
import { drift } from "../_lib/motion";
import { Trail } from "./trail";

// Text surfaces the way morning haze lifts: up a little, out of a blur.
function Rise({ delay, children, className }: { delay: number; children: ReactNode; className?: string }) {
  return (
    <motion.div
      className={className}
      initial={{ opacity: 0, y: 22, filter: "blur(8px)" }}
      animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
      transition={drift(delay, 1.3)}
    >
      {children}
    </motion.div>
  );
}

export function Hero({ data }: { data: DiscoveryData }) {
  const lede = ledeFor(data);

  return (
    <header className="grid gap-12 pt-14 md:pt-20 lg:grid-cols-[minmax(0,1fr)_21.5rem] lg:items-end lg:gap-16">
      <div>
        <Rise delay={0.1}>
          <p className="flex items-center gap-2.5 text-sm font-medium text-content-secondary">
            <span aria-hidden className="size-1.5 rounded-full bg-(--airy-ember)" />
            Discovery
          </p>
        </Rise>

        <h1 className="mt-7 font-display text-[clamp(2.6rem,1rem+4.2vw,5.5rem)] leading-[0.95] font-light tracking-[-0.04em] text-content-primary">
          <Rise delay={0.2}>Where your work</Rise>
          <Rise delay={0.32}>leads next.</Rise>
        </h1>

        <Rise delay={0.55}>
          <p className="mt-9 max-w-[34rem] text-xl leading-[1.55] text-pretty text-content-secondary">
            {lede.sentence}
            {lede.nearestFrom && (
              <>
                {" "}
                {lede.nearestMany ? "The nearest grow out of " : "The nearest grows out of "}
                <span className="font-medium text-content-primary">{lede.nearestFrom}</span>.
              </>
            )}
          </p>
        </Rise>

        <Rise delay={0.72}>
          <p className="mt-6 max-w-[32rem] text-[0.8125rem] leading-relaxed text-content-muted">
            {coverageLine(data.coverage)}
          </p>
        </Rise>
      </div>

      <Trail data={data} />
    </header>
  );
}
