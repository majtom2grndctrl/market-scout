"use client";

// One ranked role. Cells share the table's column template and align on their
// first baseline, so a 28px rank, a 20px headline, and 14px reasons all sit on
// one line however many lines each cell runs to.

import { Plus } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { alsoCalled, rankLabel } from "./derive";
import { COLS, NARROW } from "./grid";
import { ShareBar } from "./marks";
import { CUE, EASE_OUT, Tick, useRowReveal } from "./motion";
import { PinToggle } from "./pin-toggle";
import { RowDetail } from "./row-detail";

const HOVER_LINE = "underline-offset-4 group-hover/head:underline group-hover/head:decoration-edge-strong group-hover/head:decoration-1";

interface LedgerRowProps {
  readonly rec: Recommendation;
  readonly index: number;
  readonly pinned: boolean;
  readonly expanded: boolean;
  readonly onTogglePin: () => void;
  readonly onToggleExpand: () => void;
}

export function LedgerRow({ rec, index, pinned, expanded, onTogglePin, onToggleExpand }: LedgerRowProps) {
  const { ref, play, delay } = useRowReveal<HTMLLIElement>(index);
  const marks = delay + CUE.marksAfterRow;
  const also = alsoCalled(rec);
  const words = rec.headline.split(" ");
  const last = words.pop() ?? "";
  const lead = words.join(" ");
  const detailId = `ledger-detail-${rec.roleSlug}`;

  return (
    <motion.li
      ref={ref}
      initial={{ opacity: 0, y: 8 }}
      animate={play ? { opacity: 1, y: 0 } : { opacity: 0, y: 8 }}
      transition={{ delay, duration: 0.36, ease: EASE_OUT }}
      className={`group/row relative border-b border-edge transition-colors duration-150 ${expanded ? "bg-surface-raised" : "hover:bg-surface-row-hover"}`}
    >
      {/* Pinned rows carry a signal rule on the left edge. It grows from the top so it reads as being set, not toggled. */}
      <AnimatePresence initial={false}>
        {pinned && (
          <motion.span
            aria-hidden
            className="absolute inset-y-0 -left-px w-[3px] origin-top bg-(--ledger-signal)"
            initial={{ scaleY: 0 }}
            animate={{ scaleY: 1 }}
            exit={{ scaleY: 0, transition: { duration: 0.14 } }}
            transition={{ duration: 0.24, ease: EASE_OUT }}
          />
        )}
      </AnimatePresence>

      <div
        onClick={onToggleExpand}
        className={`grid cursor-pointer items-baseline gap-x-5 gap-y-3 px-3 py-5 ${NARROW} ${COLS}`}
      >
        <span
          className={`font-display text-[22px] leading-none font-medium tracking-[-0.02em] tabular-nums transition-colors duration-200 @5xl:text-[28px] ${pinned ? "text-(--ledger-signal)" : "text-content-primary"}`}
        >
          {rankLabel(rec.rank)}
        </span>

        <div className="min-w-0 @2xl:col-span-2 @5xl:col-span-1">
          <button
            type="button"
            aria-expanded={expanded}
            aria-controls={detailId}
            className="group/head block w-fit max-w-full rounded-xs text-left font-display text-[20px] leading-[1.15] font-semibold tracking-[-0.015em] focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
          >
            {lead && <span className={HOVER_LINE}>{lead} </span>}
            {/* Chrome breaks around an inline-block even after a no-break space, so the last word and the glyph wrap as one unit. */}
            <span className="whitespace-nowrap">
              <span className={HOVER_LINE}>{last}</span>
              <motion.span
                aria-hidden
                animate={{ rotate: expanded ? 45 : 0 }}
                transition={{ duration: 0.2, ease: EASE_OUT }}
                className="relative top-[1px] ml-2 inline-block text-content-muted group-hover/row:text-content-primary"
              >
                <Plus className="size-3.5" strokeWidth={2} />
              </motion.span>
            </span>
          </button>
          {also.length > 0 && (
            <p className="mt-1.5 line-clamp-2 text-[13px] leading-[1.45] text-content-muted">
              <span className="text-content-secondary">Also called</span> {also.map((t) => t.title).join(" · ")}
            </p>
          )}
        </div>

        <PinToggle
          pinned={pinned}
          headline={rec.headline}
          onToggle={onTogglePin}
          className="col-start-3 row-start-1 justify-self-end @2xl:col-start-4 @5xl:col-start-8"
        />

        <Cell label="Builds on" className="col-start-2 @5xl:col-start-3 @5xl:row-start-1">
          {rec.closestPast ? (
            <>
              <span className="block">{rec.closestPast.titleText}</span>
              {rec.closestPast.roleName.toLowerCase() !== rec.closestPast.titleText.toLowerCase() && (
                <span className="mt-0.5 block text-[12px] text-content-muted">as {rec.closestPast.roleName}</span>
              )}
            </>
          ) : (
            <Dash />
          )}
        </Cell>

        <Cell label="You bring" className="col-start-2 @2xl:col-start-3 @5xl:col-start-4 @5xl:row-start-1">
          {rec.bring.length > 0 ? (
            <ul className="space-y-1">
              {rec.bring.map((s) => (
                <li key={s.slug} className="flex items-baseline gap-2">
                  <span aria-hidden className="relative -top-px size-[5px] shrink-0 bg-content-primary" />
                  {s.name}
                </li>
              ))}
            </ul>
          ) : (
            <Dash />
          )}
        </Cell>

        <Cell label="Also asks for" className="col-start-2 @2xl:col-end-4 @5xl:col-start-5 @5xl:col-end-auto @5xl:row-start-1">
          {rec.grow.length > 0 ? (
            <ul className="space-y-2">
              {rec.grow.map((g, i) => (
                <li key={g.slug} className="grid grid-cols-[minmax(0,1fr)_3ch] items-baseline gap-x-3 gap-y-[5px]">
                  <span className="truncate text-[13px] leading-[1.35]" title={g.name}>
                    {g.name}
                  </span>
                  <span className="text-right text-[13px] leading-[1.35] text-content-secondary tabular-nums" aria-label={`asked for in ${Math.round(g.share * 100)}% of postings`}>
                    <Tick value={Math.round(g.share * 100)} play={play} delay={marks + i * 0.05} duration={0.55} />
                  </span>
                  <span className="col-span-2">
                    <ShareBar share={g.share} play={play} delay={marks + i * 0.05} />
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <Dash />
          )}
        </Cell>

        <Cell label="Postings" className="col-start-2 @5xl:col-start-6 @5xl:row-start-1 @5xl:text-right">
          <span className="text-[15px] tabular-nums">
            <Tick value={rec.openPostings} play={play} delay={marks} />
          </span>
        </Cell>

        <Cell label="Companies" className="col-start-2 @2xl:col-start-3 @5xl:col-start-7 @5xl:row-start-1 @5xl:text-right">
          <span className="text-[15px] text-content-secondary tabular-nums">
            <Tick value={rec.companies} play={play} delay={marks + 0.04} />
          </span>
        </Cell>
      </div>

      <AnimatePresence initial={false}>
        {expanded && (
          <motion.div
            id={detailId}
            key="detail"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0, transition: { duration: 0.18, ease: EASE_OUT } }}
            transition={{ height: { duration: 0.28, ease: EASE_OUT }, opacity: { duration: 0.2, delay: 0.06 } }}
            className="overflow-hidden"
          >
            <RowDetail rec={rec} />
          </motion.div>
        )}
      </AnimatePresence>
    </motion.li>
  );
}

/** On wide containers the column header names the cell; stacked, it carries its own label. */
function Cell({ label, className, children }: { readonly label: string; readonly className: string; readonly children: React.ReactNode }) {
  return (
    <div className={`min-w-0 text-[14px] leading-[1.4] ${className}`}>
      <span className="mb-1 block text-[10.5px] font-medium tracking-[0.12em] text-content-muted uppercase @5xl:hidden">{label}</span>
      {children}
    </div>
  );
}

function Dash() {
  return <span className="text-content-disabled">—</span>;
}
