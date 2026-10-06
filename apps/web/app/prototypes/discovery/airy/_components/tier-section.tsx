"use client";

import { motion } from "motion/react";

import type { Recommendation, Strength } from "../../_data/query";
import { capitalize, countWord, plural, type TierCopy } from "../_lib/copy";
import { drift, FIRST_CARD } from "../_lib/motion";
import { RoleCard, type Density } from "./role-card";
import { cn } from "@/lib/utils";

const DENSITY: Record<Strength, Density> = { close: "feature", adjacent: "standard", stretch: "light" };

const RINGS: Record<Strength, number> = { close: 1, adjacent: 2, stretch: 3 };

// Distance, drawn: a point for you, then one ripple per step outward. It marks
// how far a tier sits from the profile, never how good a fit it is.
function Ripples({ strength }: { strength: Strength }) {
  const rings = RINGS[strength];
  return (
    <svg aria-hidden width="44" height="20" viewBox="0 0 44 20" className="overflow-visible">
      <circle cx="4" cy="10" r="3" className="fill-(--airy-ember)" />
      {[1, 2, 3].map((i) => (
        <path
          key={i}
          d={`M ${6 + i * 10} 2 Q ${12 + i * 10} 10 ${6 + i * 10} 18`}
          fill="none"
          strokeWidth="1.5"
          strokeLinecap="round"
          className={i <= rings ? "stroke-(--airy-ember)" : "stroke-(--airy-thread)"}
          opacity={i <= rings ? 1 - (i - 1) * 0.22 : 0.45}
        />
      ))}
    </svg>
  );
}

export function TierSection({
  copy,
  recs,
  index,
  pinned,
  onToggle,
}: {
  copy: TierCopy;
  recs: Recommendation[];
  index: number;
  pinned: ReadonlySet<string>;
  onToggle: (roleSlug: string) => void;
}) {
  const density = DENSITY[copy.strength];
  const headingId = `airy-tier-${copy.strength}`;

  return (
    <section aria-labelledby={headingId} className="grid gap-8 lg:grid-cols-[13.5rem_minmax(0,1fr)] lg:gap-14">
      <motion.header
        className="lg:sticky lg:top-24 lg:self-start lg:pt-2"
        initial={{ opacity: 0, y: 18 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.5 }}
        transition={drift(index === 0 ? FIRST_CARD - 0.2 : 0, 1.1)}
      >
        <Ripples strength={copy.strength} />
        <h2 id={headingId} className="mt-4 font-display text-[1.875rem] leading-[1.1] font-normal tracking-[-0.02em] text-content-primary">
          {copy.name}
        </h2>
        <p className="mt-2 text-[0.9375rem] text-content-muted">
          {capitalize(countWord(recs.length))} {plural(recs.length, "role")}
        </p>
        {copy.note && <p className="mt-5 text-base leading-relaxed text-pretty text-content-secondary">{copy.note}</p>}
      </motion.header>

      <div
        className={cn(
          density === "feature" && "flex flex-col gap-6",
          density === "standard" && "flex flex-col gap-5",
          density === "light" && "grid gap-4 md:grid-cols-2",
        )}
      >
        {recs.map((r, i) => (
          <RoleCard
            key={r.roleSlug}
            rec={r}
            density={density}
            batchIndex={i}
            pinned={pinned.has(r.roleSlug)}
            onToggle={onToggle}
          />
        ))}
      </div>
    </section>
  );
}
