"use client";

// Client root. Owns pin state, which is local by contract: seeded from the
// profile, never written back, reset by a reload.

import { MotionConfig } from "motion/react";
import { useCallback, useMemo, useState } from "react";

import { Opening } from "./opening";
import { assignPlates, plateOf } from "./palette";
import { PinTray } from "./pin-tray";
import { Stage } from "./stage";
import type { MarqueeData } from "./types";

export function Marquee({ data }: { readonly data: MarqueeData }) {
  const roles = data.recommendations;
  const plates = useMemo(() => assignPlates(data), [data]);
  const [pinned, setPinned] = useState<readonly string[]>(() => roles.filter((r) => r.pinned).map((r) => r.roleSlug));
  const pinnedSet = useMemo(() => new Set(pinned), [pinned]);
  const stagePlates = useMemo(() => rhythm(roles.map((r) => plateOf(plates, r.closestPast?.titleText))), [roles, plates]);

  const toggle = useCallback((slug: string) => {
    setPinned((prev) => (prev.includes(slug) ? prev.filter((s) => s !== slug) : [...prev, slug]));
  }, []);

  return (
    <MotionConfig reducedMotion="user">
      <main className="marquee-root bg-surface-page text-content-primary">
        <Opening data={data} plates={plates} pinned={pinnedSet} />

        {roles.map((r, i) => (
          <Stage
            key={r.roleSlug}
            role={r}
            total={roles.length}
            plate={stagePlates[i] ?? plateOf(plates, r.closestPast?.titleText)}
            pinned={pinnedSet.has(r.roleSlug)}
            onToggle={toggle}
          />
        ))}

        <footer className="border-t border-content-primary px-5 py-10 font-sans text-sm text-content-secondary sm:px-10">
          <p className="mx-auto max-w-[96rem]">
            Ranked from {data.coverage.classifiedPostings.toLocaleString()} classified of{" "}
            {data.coverage.openPostings.toLocaleString()} open postings. Titles are spelled as employers wrote them;
            counts are open postings.
          </p>
        </footer>

        <PinTray roles={roles} pinned={pinned} plates={plates} />
      </main>
    </MotionConfig>
  );
}

// Neighbouring roles that share a past share a hue. Alternate posters in a run
// are set on a paler pull of the same plate so each still reads as its own
// sheet, while hue keeps carrying provenance.
function rhythm(plates: readonly string[]): string[] {
  let run = 0;
  return plates.map((plate, i) => {
    run = i > 0 && plates[i - 1] === plate ? run + 1 : 0;
    return run % 2 === 1 ? `color-mix(in oklch, ${plate} 48%, var(--surface-page))` : plate;
  });
}
