"use client";

// The statement and the summary band above the table. The band sets the
// person's own record as data: each past title and claimed skill carries a
// strip of ticks, one per ranked role, so "this counts in four of twelve" is
// read off the page rather than asserted.

import { motion } from "motion/react";
import type { ReactNode } from "react";

import type { DiscoveryData } from "../_data/query";
import { buildsOnRecord, formatCount, listPhrase, plural, type LedgerSummary, pastPresence, skillPresence, TIER_LABEL, TIERS } from "./derive";
import { PresenceStrip, Rule } from "./marks";
import { CUE, EASE_OUT, Tick } from "./motion";

const rise = (delay: number) => ({
  initial: { opacity: 0, y: 10 },
  animate: { opacity: 1, y: 0 },
  transition: { delay, duration: 0.5, ease: EASE_OUT },
});

export function Masthead({ data, summary }: { readonly data: DiscoveryData; readonly summary: LedgerSummary }) {
  const n = summary.roleCount;
  return (
    <header>
      <Rule delay={CUE.rule} />
      <motion.div {...rise(CUE.rule + 0.1)} className="flex items-baseline justify-between pt-3 text-[11px] font-medium tracking-[0.14em] text-content-muted uppercase">
        <span>Discovery</span>
        <span className="tabular-nums">
          {n} of {formatCount(data.coverage.rolesConsidered)} roles shown
        </span>
      </motion.div>

      <div className="mt-10 grid grid-cols-1 gap-x-6 gap-y-6 @5xl:grid-cols-12">
        <motion.h1 {...rise(CUE.headline)} className="font-display text-[44px] leading-[0.98] font-semibold tracking-[-0.035em] text-balance @3xl:text-[64px] @5xl:col-span-8">
          {n === 0 ? (
            "No roles ranked yet."
          ) : (
            <>
              <span className="tabular-nums">{n}</span>
              {data.recommendations.every(buildsOnRecord)
                ? ` ${plural(n, "role builds", "roles build")} on what you’ve already done.`
                : ` ${plural(n, "role", "roles")} ranked against what you’ve already done.`}
            </>
          )}
        </motion.h1>
        <motion.p {...rise(CUE.lede)} className="max-w-[34rem] self-end text-[15px] leading-[1.55] text-content-secondary @5xl:col-span-4">
          <Lede summary={summary} />
        </motion.p>
      </div>
    </header>
  );
}

function Lede({ summary }: { readonly summary: LedgerSummary }) {
  const { roleCount, pastTitlesUsed, withBring, titleSpellings } = summary;
  if (roleCount === 0) return null;
  const parts: ReactNode[] = [];
  if (pastTitlesUsed.length > 0) {
    parts.push(
      <span key="past">
        Ranked against your record as <strong className="font-medium text-content-primary">{listPhrase(pastTitlesUsed)}</strong>.{" "}
      </span>,
    );
  }
  if (withBring > 0) {
    parts.push(
      <span key="bring">
        In {withBring} of them, a skill you claimed is already among the ten that define the role.{" "}
      </span>,
    );
  }
  parts.push(
    <span key="titles">
      Employers post {plural(roleCount, "it", "them")} under at least {titleSpellings} different{" "}
      {plural(titleSpellings, "title", "titles")}.
    </span>,
  );
  return <>{parts}</>;
}

export function SummaryBand({ data, summary }: { readonly data: DiscoveryData; readonly summary: LedgerSummary }) {
  const recs = data.recommendations;
  const rankSpan = recs.length > 0 ? `Ranks 1–${recs.length}` : "";
  const col = (i: number) => CUE.band + i * CUE.bandStagger;

  return (
    <section aria-label="Summary" className="mt-12">
      <div className="grid grid-cols-1 gap-x-10 gap-y-10 @3xl:grid-cols-2 @5xl:grid-cols-3">
        <BandColumn title="Your record" note={rankSpan} delay={col(0)}>
          {data.pastRoles.map((p, i) => {
            const cells = pastPresence(recs, p.titleText);
            const count = cells.filter(Boolean).length;
            return (
              <BandRow
                key={`${p.titleText}-${i}`}
                label={p.titleText}
                sub={p.seniority}
                cells={cells}
                count={count}
                delay={col(0) + 0.1 + i * 0.04}
                aria={`${p.titleText}: ${count} of ${recs.length} ranked roles build on it`}
              />
            );
          })}
        </BandColumn>

        <BandColumn title="What you claimed" note={rankSpan} delay={col(1)}>
          {data.claimedSkills.map((c, i) => {
            const cells = skillPresence(recs, c.skill?.slug ?? null);
            const count = cells.filter(Boolean).length;
            return (
              <BandRow
                key={`${c.text}-${i}`}
                label={c.skill?.name ?? c.text}
                sub={c.skill ? null : "not matched"}
                cells={cells}
                count={count}
                delay={col(1) + 0.1 + i * 0.04}
                aria={`${c.skill?.name ?? c.text}: brought to ${count} of ${recs.length} ranked roles`}
              />
            );
          })}
        </BandColumn>

        <BandColumn title="The ranking" note={rankSpan} delay={col(2)}>
          {TIERS.map((tier, i) => {
            const cells = recs.map((r) => r.strength === tier);
            const count = summary.tierCounts[tier];
            return (
              <BandRow
                key={tier}
                label={TIER_LABEL[tier]}
                sub={null}
                cells={cells}
                count={count}
                delay={col(2) + 0.1 + i * 0.04}
                aria={`${TIER_LABEL[tier]}: ${count} roles`}
              />
            );
          })}
          <Coverage data={data} share={summary.coverageShare} delay={col(2) + 0.3} />
        </BandColumn>
      </div>
    </section>
  );
}

function BandColumn({ title, note, delay, children }: { readonly title: string; readonly note: string; readonly delay: number; readonly children: ReactNode }) {
  return (
    <motion.div {...rise(delay)}>
      <div className="flex items-baseline justify-between border-b border-content-primary pb-2 text-[11px] font-medium tracking-[0.12em] text-content-muted uppercase">
        <h2 className="font-sans text-content-primary">{title}</h2>
        <span className="tabular-nums">{note}</span>
      </div>
      <ul>{children}</ul>
    </motion.div>
  );
}

interface BandRowProps {
  readonly label: string;
  readonly sub: string | null;
  readonly cells: readonly boolean[];
  readonly count: number;
  readonly delay: number;
  readonly aria: string;
}

function BandRow({ label, sub, cells, count, delay, aria }: BandRowProps) {
  return (
    <li className="grid grid-cols-[minmax(0,1fr)_auto_2ch] items-center gap-x-4 border-b border-edge-hairline py-[7px] text-[14px] leading-5">
      <span className="min-w-0 truncate">
        {label}
        {sub && <span className="ml-2 text-[12px] text-content-muted">{sub}</span>}
      </span>
      <PresenceStrip cells={cells} delay={delay} label={aria} />
      <span className={`text-right tabular-nums ${count === 0 ? "text-content-disabled" : "text-content-primary"}`}>
        <Tick value={count} delay={delay} duration={0.5} />
      </span>
    </li>
  );
}

function Coverage({ data, share, delay }: { readonly data: DiscoveryData; readonly share: number; readonly delay: number }) {
  const { classifiedPostings, openPostings, rolesConsidered } = data.coverage;
  return (
    <li className="pt-4 text-[12px] leading-[1.5] text-content-muted">
      <div aria-hidden className="mb-2.5 h-[3px] w-full bg-edge-hairline">
        <motion.div
          className="h-full origin-left bg-content-secondary"
          style={{ width: `${Math.min(1, share) * 100}%` }}
          initial={{ scaleX: 0 }}
          animate={{ scaleX: 1 }}
          transition={{ delay, duration: 0.7, ease: EASE_OUT }}
        />
      </div>
      Ranked from <span className="text-content-secondary">{formatCount(classifiedPostings)}</span> classified of{" "}
      <span className="text-content-secondary">{formatCount(openPostings)}</span> open postings, across{" "}
      {formatCount(rolesConsidered)} roles with enough postings to compare.
    </li>
  );
}
