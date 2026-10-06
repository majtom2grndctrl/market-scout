"use client";

import { ArrowRight, Check } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import type { Ref } from "react";

import type { PersonSkill, Recommendation } from "../../_data/query";
import { capitalize, countWord, plural, rolesHeading } from "../_lib/copy";
import { drift, ENTRANCE, SETTLE } from "../_lib/motion";
import { RoleItem } from "./role-item";
import { cn } from "@/lib/utils";

function PinBar({
  pins,
  acknowledged,
  onContinue,
  barRef,
}: {
  pins: readonly Recommendation[];
  acknowledged: boolean;
  onContinue: () => void;
  barRef: Ref<HTMLDivElement>;
}) {
  const n = pins.length;
  const ready = n > 0;

  return (
    <div ref={barRef} className="sticky bottom-4 z-20 mx-3 mt-1 mb-3">
      <div className="flex items-center gap-4 rounded-[1.25rem] bg-surface-raised/85 p-2.5 pl-5 shadow-(--threads-float) ring-1 ring-edge-hairline backdrop-blur-xl backdrop-saturate-150">
        <div className="flex min-w-0 flex-1 items-center gap-3" aria-live="polite">
          <span className="relative inline-grid h-8 min-w-[1.5ch] overflow-hidden font-display text-[1.75rem] leading-8 font-light tracking-[-0.03em] text-content-primary tabular-nums">
            <AnimatePresence mode="popLayout" initial={false}>
              <motion.span
                key={n}
                initial={{ y: "70%", opacity: 0 }}
                animate={{ y: 0, opacity: 1 }}
                exit={{ y: "-70%", opacity: 0 }}
                transition={SETTLE}
              >
                {n}
              </motion.span>
            </AnimatePresence>
          </span>
          <span className="min-w-0 text-sm leading-[1.3]">
            <span className="block font-medium text-content-primary">{plural(n, "role")} pinned</span>
            <span className="block truncate text-content-muted">
              {ready ? pins.map((p) => p.headline).join(" · ") : "Pin the roles worth watching."}
            </span>
          </span>
        </div>

        <motion.button
          type="button"
          layout
          transition={SETTLE}
          whileTap={ready ? { scale: 0.97 } : undefined}
          disabled={!ready}
          onClick={onContinue}
          className={cn(
            "group/go inline-flex h-11 shrink-0 items-center justify-center gap-2 rounded-[0.9rem] px-4.5 text-[0.875rem] font-medium outline-none",
            "transition-[background-color,color] duration-300 ease-out focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2",
            // With nothing pinned this is a genuinely disabled control, the one
            // place content-disabled belongs (build contract, Invariant 9).
            ready
              ? "cursor-pointer bg-accent-solid text-on-solid hover:bg-accent-hover active:bg-accent-active"
              : "cursor-not-allowed bg-surface-sunken text-content-disabled",
          )}
        >
          <AnimatePresence mode="popLayout" initial={false}>
            {acknowledged && ready ? (
              <motion.span
                key="ack"
                className="inline-flex items-center gap-2"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={SETTLE}
              >
                <Check className="size-4" strokeWidth={2.5} />
                {`${capitalize(countWord(n))} ${plural(n, "role")} ready to watch`}
              </motion.span>
            ) : (
              <motion.span
                key="go"
                className="inline-flex items-center gap-2"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={SETTLE}
              >
                <span>
                  Continue<span className="max-sm:hidden"> to your dashboard</span>
                </span>
                <ArrowRight className="size-4 transition-transform duration-300 ease-out group-hover/go:translate-x-0.5" />
              </motion.span>
            )}
          </AnimatePresence>
        </motion.button>
      </div>
    </div>
  );
}

export function RoleCard({
  recs,
  skills,
  active,
  pinned,
  pins,
  acknowledged,
  onLight,
  onToggle,
  onContinue,
  isScrolling,
  cardRef,
  barRef,
  registerRole,
  className,
}: {
  recs: readonly Recommendation[];
  skills: ReadonlyMap<string, PersonSkill>;
  active: string | null;
  pinned: ReadonlySet<string>;
  pins: readonly Recommendation[];
  acknowledged: boolean;
  onLight: (roleSlug: string) => void;
  onToggle: (roleSlug: string) => void;
  onContinue: () => void;
  isScrolling: () => boolean;
  cardRef: Ref<HTMLElement>;
  barRef: Ref<HTMLDivElement>;
  registerRole: (slug: string) => (el: HTMLElement | null) => void;
  className?: string;
}) {
  return (
    <motion.section
      ref={cardRef}
      aria-labelledby="threads-roles"
      className={cn("relative rounded-[1.75rem] bg-surface-raised shadow-(--threads-lift) ring-1 ring-edge-hairline/70", className)}
      initial={{ opacity: 0, y: 28 }}
      animate={{ opacity: 1, y: 0 }}
      transition={drift(ENTRANCE.card, 1.1)}
    >
      <h2 id="threads-roles" className="px-6 pt-7 pb-2 font-display sm:px-9 sm:pt-8 text-[1.25rem] font-normal tracking-[-0.015em] text-content-primary">
        {rolesHeading(recs.length)}
      </h2>
      {recs.length === 0 ? (
        <p className="px-6 pb-8 text-sm sm:px-9 text-content-muted">
          Nothing in the postings read so far sits near your profile. More appear as more postings are classified.
        </p>
      ) : (
        <ol className="pb-1">
          {recs.map((r, i) => (
            <RoleItem
              key={r.roleSlug}
              rec={r}
              index={i}
              skills={skills}
              lit={active === r.roleSlug}
              pinned={pinned.has(r.roleSlug)}
              onLight={onLight}
              onToggle={onToggle}
              isScrolling={isScrolling}
              anchorRef={registerRole(r.roleSlug)}
            />
          ))}
        </ol>
      )}
      <PinBar pins={pins} acknowledged={acknowledged} onContinue={onContinue} barRef={barRef} />
    </motion.section>
  );
}
