"use client";

import { motion } from "motion/react";

import type { Recommendation } from "../../_data/query";
import { useArrival } from "../_lib/arrival";
import { alsoPostedAs, postingsLine } from "../_lib/copy";
import { drift, SETTLE } from "../_lib/motion";
import { GrowList } from "./grow-list";
import { PinButton } from "./pin-button";
import { cn } from "@/lib/utils";

/**
 * Room shrinks with distance. The nearest roles are wide rows with the most
 * air; the next tier, slimmer rows; the furthest, small tiles two abreast.
 */
export type Density = "feature" | "standard" | "light";

const ALSO_LIMIT: Record<Density, number> = { feature: 4, standard: 3, light: 2 };
const GROW_LIMIT: Record<Density, number> = { feature: 4, standard: 3, light: 3 };

function Evidence({ rec, light }: { rec: Recommendation; light: boolean }) {
  if (!rec.closestPast && rec.bring.length === 0) return null;
  return (
    <dl className={cn("flex flex-wrap gap-x-10 gap-y-4", light && "gap-x-8 gap-y-3")}>
      {rec.closestPast && (
        <div>
          <dt className="text-[0.8125rem] font-medium text-content-muted">Nearest past title</dt>
          <dd className={cn("mt-1.5 font-medium text-content-primary", light ? "text-sm" : "text-[0.9375rem]")}>
            {rec.closestPast.titleText}
          </dd>
        </div>
      )}
      {rec.bring.length > 0 && (
        <div>
          <dt className="text-[0.8125rem] font-medium text-content-muted">You bring</dt>
          <dd className="mt-1 flex flex-wrap gap-1.5">
            {rec.bring.map((s) => (
              <span
                key={s.slug}
                className="inline-flex h-7 items-center rounded-full bg-(--airy-glow) px-2.5 text-[0.8125rem] text-content-primary ring-1 ring-(--airy-glow-edge) ring-inset"
              >
                {s.name}
              </span>
            ))}
          </dd>
        </div>
      )}
    </dl>
  );
}

export function RoleCard({
  rec,
  density,
  batchIndex,
  pinned,
  onToggle,
}: {
  rec: Recommendation;
  density: Density;
  batchIndex: number;
  pinned: boolean;
  onToggle: (roleSlug: string) => void;
}) {
  const { ref, delay } = useArrival<HTMLElement>(rec.rank - 1, batchIndex);
  const also = alsoPostedAs(rec, ALSO_LIMIT[density]);
  const feature = density === "feature";
  const light = density === "light";
  const titleId = `airy-role-${rec.roleSlug}-title`;

  const heading = (
    <div className="min-w-0">
      <p className="font-display text-[0.8125rem] text-content-muted tabular-nums">{String(rec.rank).padStart(2, "0")}</p>
      <h3
        id={titleId}
        className={cn(
          "mt-2 font-display font-normal text-balance text-content-primary",
          feature && "text-[clamp(2rem,1.3rem+1.4vw,2.625rem)] leading-[1.02] tracking-[-0.03em]",
          density === "standard" && "text-[1.75rem] leading-[1.08] tracking-[-0.022em]",
          light && "text-[1.25rem] leading-[1.15] tracking-[-0.012em]",
        )}
      >
        {rec.headline}
      </h3>
      {also.length > 0 && (
        <p className={cn("mt-2.5 text-pretty text-content-secondary", light ? "text-[0.8125rem]" : "text-[0.9375rem]")}>
          <span className="text-content-muted">Also posted as </span>
          {also.join(" · ")}
        </p>
      )}
    </div>
  );

  const pin = <PinButton roleSlug={rec.roleSlug} headline={rec.headline} pinned={pinned} onToggle={onToggle} />;
  const postings = <p className="text-[0.8125rem] text-content-muted tabular-nums">{postingsLine(rec)}</p>;

  return (
    <motion.article
      ref={ref}
      id={`airy-role-${rec.roleSlug}`}
      aria-labelledby={titleId}
      className={cn("h-full scroll-mt-28", light && "md:odd:last:col-span-2")}
      initial={{ opacity: 0, y: 40 }}
      animate={delay === null ? undefined : { opacity: 1, y: 0 }}
      transition={drift(delay ?? 0, 1.25)}
    >
      <motion.div
        whileHover={{ y: light ? -2 : -3 }}
        transition={SETTLE}
        className={cn(
          "relative h-full transition-[box-shadow,background-color] duration-500 ease-out",
          feature && "rounded-[1.75rem] bg-surface-raised p-8 shadow-(--airy-lift) hover:shadow-(--airy-lift-hover) md:p-10",
          density === "standard" && "rounded-3xl bg-surface-raised p-7 shadow-(--airy-lift) hover:shadow-(--airy-lift-hover) md:p-8",
          light && "rounded-2xl bg-surface-raised/55 p-6 ring-1 ring-edge-hairline hover:bg-surface-raised hover:shadow-(--airy-lift)",
        )}
      >
        {/* Pinned wash: the card keeps a little of the light it sent to the tray. */}
        <motion.span
          aria-hidden
          className="pointer-events-none absolute inset-0 rounded-[inherit] bg-linear-to-b from-(--airy-glow) to-transparent to-70% ring-1 ring-(--airy-glow-edge) ring-inset"
          initial={false}
          animate={{ opacity: pinned ? 1 : 0 }}
          transition={{ duration: 0.6, ease: "easeOut" }}
        />

        {light ? (
          <div className="relative flex h-full flex-col">
            <div className="flex items-start justify-between gap-5">
              {heading}
              {pin}
            </div>
            <div className="mt-5 space-y-5">
              <Evidence rec={rec} light />
              <GrowList grow={rec.grow} limit={GROW_LIMIT[density]} arriveAt={delay} compact />
            </div>
            <div className="mt-auto pt-6">{postings}</div>
          </div>
        ) : (
          <div
            className={cn(
              "relative grid gap-8",
              feature ? "md:grid-cols-[minmax(0,1fr)_minmax(0,19rem)] md:gap-14" : "md:grid-cols-[minmax(0,1fr)_minmax(0,16.5rem)] md:gap-12",
            )}
          >
            <div className="flex min-w-0 flex-col">
              {heading}
              <div className={feature ? "mt-8" : "mt-6"}>
                <Evidence rec={rec} light={false} />
              </div>
              <div className={cn("mt-auto", feature ? "pt-8" : "pt-6")}>{postings}</div>
            </div>
            <div className="flex min-w-0 flex-col gap-7">
              <div className="flex justify-end max-md:order-last max-md:justify-start">{pin}</div>
              <GrowList grow={rec.grow} limit={GROW_LIMIT[density]} arriveAt={delay} compact={!feature} />
            </div>
          </div>
        )}
      </motion.div>
    </motion.article>
  );
}
