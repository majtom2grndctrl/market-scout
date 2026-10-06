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
    <header className="grid gap-12 pt-14 md:pt-20 lg:grid-cols-[minmax(0,1fr)_23rem] lg:items-end lg:gap-16">
      <div>
        <h1 className="font-display text-[clamp(2.6rem,1rem+4.2vw,5.5rem)] leading-[0.95] font-light tracking-[-0.04em] text-content-primary">
          <Rise delay={0.1}>Where your work</Rise>
          <Rise delay={0.22}>leads next.</Rise>
        </h1>

        <Rise delay={0.45}>
          <p className="mt-10 max-w-[36rem] text-[1.3125rem] leading-[1.55] text-pretty text-content-secondary">
            {lede.sentence}
            {lede.nearestLead && lede.nearestFrom && (
              <>
                {" "}
                {lede.nearestLead}{" "}
                <span className="font-medium text-content-primary">{lede.nearestFrom}</span>.
              </>
            )}
          </p>
        </Rise>

        <Rise delay={0.62}>
          <p className="mt-7 max-w-[34rem] text-[0.9375rem] leading-relaxed text-content-muted">
            {coverageLine(data.coverage)}
          </p>
        </Rise>
      </div>

      <Trail data={data} />
    </header>
  );
}
