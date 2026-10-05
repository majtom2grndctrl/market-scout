"use client";

import { motion } from "motion/react";
import type { ReactNode } from "react";

import type { DiscoveryData } from "../../_data/query";
import { coverageLine, ledeFor } from "../_lib/copy";
import { drift, ENTRANCE } from "../_lib/motion";

function Rise({ delay, children, className }: { delay: number; children: ReactNode; className?: string }) {
  return (
    <motion.div
      className={className}
      initial={{ opacity: 0, y: 18, filter: "blur(6px)" }}
      animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
      transition={drift(delay, 1.1)}
    >
      {children}
    </motion.div>
  );
}

export function Intro({ data, className }: { data: DiscoveryData; className?: string }) {
  return (
    <header className={className}>
      <h1 className="font-display text-[clamp(2.5rem,1.4rem+2.4vw,3.75rem)] leading-[0.98] font-light tracking-[-0.035em] text-content-primary">
        <Rise delay={ENTRANCE.headline}>Where your skills</Rise>
        <Rise delay={ENTRANCE.headline + 0.1}>come together.</Rise>
      </h1>
      <Rise delay={ENTRANCE.lede}>
        <p className="mt-6 max-w-[35rem] text-[1.0625rem] leading-[1.6] text-pretty text-content-secondary">{ledeFor(data)}</p>
      </Rise>
      <Rise delay={ENTRANCE.lede + 0.12}>
        <p className="mt-4 max-w-[35rem] text-[0.75rem] leading-relaxed text-content-muted">{coverageLine(data.coverage)}</p>
      </Rise>
    </header>
  );
}
