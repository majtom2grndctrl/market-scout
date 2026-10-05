"use client";

// The ranked table: a sticky column header, then rows grouped by strength.
// Groups are contiguous in rank, so the tier headings read as section breaks
// in one list rather than three separate lists.

import { motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { formatCount, groupByTier, plural, TIER_LABEL, type TierGroup } from "./derive";
import { COLS } from "./grid";
import { LedgerRow } from "./ledger-row";
import { Rule } from "./marks";
import { CUE, EASE_OUT, useRowReveal } from "./motion";

interface LedgerTableProps {
  readonly recs: readonly Recommendation[];
  readonly pinned: ReadonlySet<string>;
  readonly expanded: ReadonlySet<string>;
  readonly onTogglePin: (slug: string) => void;
  readonly onToggleExpand: (slug: string) => void;
}

export function LedgerTable({ recs, pinned, expanded, onTogglePin, onToggleExpand }: LedgerTableProps) {
  if (recs.length === 0) return null;
  const groups = groupByTier(recs);

  return (
    <section aria-label="Ranked roles" className="mt-16">
      <ColumnHeader />
      {groups.map((g) => (
        <div key={g.tier}>
          <TierHeading group={g} />
          <ol>
            {g.recs.map((rec) => (
              <LedgerRow
                key={rec.roleSlug}
                rec={rec}
                index={rec.rank - 1}
                pinned={pinned.has(rec.roleSlug)}
                expanded={expanded.has(rec.roleSlug)}
                onTogglePin={() => onTogglePin(rec.roleSlug)}
                onToggleExpand={() => onToggleExpand(rec.roleSlug)}
              />
            ))}
          </ol>
        </div>
      ))}
    </section>
  );
}

const HEAD = "text-[10.5px] font-medium tracking-[0.12em] text-content-muted uppercase";

function ColumnHeader() {
  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ delay: CUE.tableHead, duration: 0.3 }}
      className="sticky top-14 z-[5] hidden bg-surface-page @5xl:block"
    >
      <Rule delay={CUE.tableHead} />
      <div className={`grid items-end gap-x-5 px-3 pt-2.5 pb-2 ${HEAD} ${COLS}`}>
        <span>Rank</span>
        <span>Role</span>
        <span>Builds on</span>
        <span>You bring</span>
        <span className="flex items-end justify-between gap-3">
          <span>Also asks for</span>
          <span>% of postings</span>
        </span>
        <span className="text-right">Postings</span>
        <span className="text-right">Companies</span>
        <span className="text-right">Pin</span>
      </div>
      <div className="h-px bg-edge" />
    </motion.div>
  );
}

function TierHeading({ group }: { readonly group: TierGroup }) {
  // Keyed to its first row's slot, a beat early, so the heading lands just before the row it introduces.
  const first = group.recs[0];
  const { ref, play, delay } = useRowReveal<HTMLDivElement>(Math.max(0, (first?.rank ?? 1) - 1.7));
  const n = group.recs.length;
  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0 }}
      animate={{ opacity: play ? 1 : 0 }}
      transition={{ delay, duration: 0.3, ease: EASE_OUT }}
      className="flex items-baseline gap-4 border-b border-content-primary px-3 pt-9 pb-2"
    >
      <h3 className="font-display text-[15px] font-semibold tracking-[-0.005em]">{TIER_LABEL[group.tier]}</h3>
      <span className="text-[12px] text-content-muted tabular-nums">
        {formatCount(n)} {plural(n, "role", "roles")}
        {group.commonPast && (
          <>
            <span className="mx-2 text-content-disabled">/</span>
            {n === 1 ? "builds on" : "most build on"} <span className="text-content-secondary">{group.commonPast}</span>
          </>
        )}
      </span>
    </motion.div>
  );
}
