"use client";

import { motion } from "motion/react";

import type { Recommendation } from "../../_data/query";
import { shareLabel } from "../_lib/copy";
import { drift } from "../_lib/motion";
import { cn } from "@/lib/utils";

/**
 * What else the role asks for. Each share is P(skill | role) — how many of the
 * role's postings name the skill — so the bar describes the role, never the
 * person. Bars fill after their card has landed, in list order.
 */
export function GrowList({
  grow,
  limit,
  arriveAt,
  compact = false,
}: {
  grow: Recommendation["grow"];
  limit: number;
  /** Seconds from now the card lands; null until it is in view. */
  arriveAt: number | null;
  compact?: boolean;
}) {
  const rows = grow.slice(0, limit);
  if (rows.length === 0) return null;

  return (
    <div>
      <p className="text-[0.8125rem] font-medium text-content-muted">It also asks for</p>
      <ul className={cn("mt-3", compact ? "space-y-2.5" : "space-y-3.5")}>
        {rows.map((g, i) => (
          <li key={g.slug}>
            <div className="flex items-baseline justify-between gap-4">
              <span className={cn("min-w-0 text-content-primary", compact ? "text-[0.8125rem]" : "text-sm")}>{g.name}</span>
              <span className="shrink-0 text-[0.75rem] whitespace-nowrap text-content-muted tabular-nums">
                in {shareLabel(g.share)} of postings
              </span>
            </div>
            <div className={cn("mt-1.5 overflow-hidden rounded-full bg-surface-sunken", compact ? "h-[3px]" : "h-1")}>
              <motion.div
                className="h-full origin-left rounded-full bg-ramp-3"
                initial={{ scaleX: 0 }}
                animate={arriveAt === null ? undefined : { scaleX: Math.min(1, Math.max(0, g.share)) }}
                transition={drift((arriveAt ?? 0) + 0.45 + i * 0.07, 1.1)}
              />
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
