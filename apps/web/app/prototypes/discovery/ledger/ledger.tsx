"use client";

// Ledger: clarity as the lift. A statement, the person's record set as data,
// then the ranked table with every reason in its own column. Pins are local
// state seeded from the profile, so testing never writes to it.

import { MotionConfig } from "motion/react";
import { type CSSProperties, useEffect, useMemo, useState } from "react";

import type { DiscoveryData } from "../_data/query";
import { summarize } from "./derive";
import { LedgerTable } from "./ledger-table";
import { Masthead, SummaryBand } from "./masthead";
import { markPageStart } from "./motion";
import { Tally } from "./tally";

// The one mood colour: a Swiss signal red, spent only on pins (the marker,
// the rank of a pinned row, the tally). Fit and strength never wear it. It is
// also ink, so it sits dark enough to clear 4.5:1 on the page, a hovered row,
// and the raised chip.
const MOOD = {
  "--ledger-signal": "oklch(0.53 0.2 31)",
} as CSSProperties;

export function Ledger({ data }: { readonly data: DiscoveryData }) {
  const recs = data.recommendations;
  const summary = useMemo(() => summarize(data), [data]);
  // Effects run child-first, but a row reads this only when its observer reports, which is later still.
  useEffect(markPageStart, []);

  // Pin order is the order pins were made, so the tally reads as a running log.
  const [pinOrder, setPinOrder] = useState<readonly string[]>(() => recs.filter((r) => r.pinned).map((r) => r.roleSlug));
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const [acknowledged, setAcknowledged] = useState(false);

  const pinned = useMemo(() => new Set(pinOrder), [pinOrder]);
  const pinnedRecs = useMemo(() => {
    const bySlug = new Map(recs.map((r) => [r.roleSlug, r]));
    return pinOrder.flatMap((s) => bySlug.get(s) ?? []);
  }, [pinOrder, recs]);

  const togglePin = (slug: string) => {
    setAcknowledged(false);
    setPinOrder((prev) => (prev.includes(slug) ? prev.filter((s) => s !== slug) : [...prev, slug]));
  };

  const toggleExpand = (slug: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(slug)) next.delete(slug);
      else next.add(slug);
      return next;
    });

  return (
    <MotionConfig reducedMotion="user">
      <div className="@container" style={MOOD}>
        <div className="mx-auto max-w-[78rem] px-6 pt-8 @5xl:px-10 @5xl:pt-10">
          <Masthead data={data} summary={summary} />
          <SummaryBand data={data} summary={summary} />
          <LedgerTable recs={recs} pinned={pinned} expanded={expanded} onTogglePin={togglePin} onToggleExpand={toggleExpand} />
          <Tally pins={pinnedRecs} acknowledged={acknowledged} onContinue={() => setAcknowledged(true)} onUnpin={togglePin} />
        </div>
      </div>
    </MotionConfig>
  );
}
