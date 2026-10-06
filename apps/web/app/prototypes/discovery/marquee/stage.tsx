"use client";

// One role, one poster. Reading order is the reveal order: headline, the
// other names it goes by, why it is here, what the person brings, what its
// postings also ask for, and how much of it there is.

import { motion, useReducedMotion } from "motion/react";
import { Fragment, useRef, type CSSProperties, type ReactNode } from "react";

import type { Strength } from "../_data/query";
import { AlsoCalled } from "./also-called";
import { article, percentOfPostings, plural } from "./copy";
import { Headline } from "./fit-headline";
import { EASE_OUT, useSeen } from "./motion";
import { PinToggle } from "./pin-toggle";
import { FOLIO, LIST, STATEMENT, SUPPORT } from "./scale";
import type { Role } from "./types";

const STRENGTH_COPY: Record<Strength, string> = {
  close: "Close to what you've done",
  adjacent: "A step over from what you've done",
  stretch: "A longer step from what you've done",
};

interface StageProps {
  readonly role: Role;
  readonly total: number;
  readonly plate: string;
  readonly pinned: boolean;
  readonly onToggle: (slug: string) => void;
}

export function Stage({ role, total, plate, pinned, onToggle }: StageProps) {
  const ref = useRef<HTMLElement>(null);
  const seen = useSeen(ref);
  const hasWhy = role.closestPast !== null || role.bring.length > 0;

  return (
    <section
      ref={ref}
      id={`role-${role.roleSlug}`}
      aria-label={role.headline}
      className="scroll-mt-14 border-t border-content-primary bg-(--marquee-plate) text-content-primary"
      style={{ "--marquee-plate": plate } as CSSProperties}
    >
      <div className="mx-auto max-w-[96rem] px-5 pt-6 pb-16 [container-type:inline-size] sm:px-10 md:pb-24">
        {/* The folio runs at one size: rank, how far the step is, and the pin.
            Strength reads as a running line, not a label over the headline. */}
        <Reveal seen={seen} delay={0} className="flex items-start justify-between gap-6">
          <p className={`${FOLIO} text-pretty`}>
            <span className="font-semibold tabular-nums">
              {role.rank}
              <span className="font-light text-content-secondary">/{total}</span>
            </span>
            <span className="ml-[0.6em] font-light text-content-secondary">{STRENGTH_COPY[role.strength]}</span>
          </p>
          <PinToggle label={role.headline} pinned={pinned} onToggle={() => onToggle(role.roleSlug)} />
        </Reveal>

        <div className="mt-[clamp(1.5rem,4cqi,3.5rem)]">
          <Headline text={role.headline} seen={seen} pinned={pinned} delay={0.05} />
        </div>

        <Reveal seen={seen} delay={0.55}>
          <AlsoCalled slug={role.roleSlug} headline={role.headline} titles={role.titles} seen={seen} />
        </Reveal>

        <div className="mt-[clamp(2.5rem,6cqi,5rem)] grid gap-x-10 gap-y-10 @[48rem]:grid-cols-12">
          {hasWhy && (
            <div className="@[48rem]:col-span-5">
              {role.closestPast && (
                <Reveal seen={seen} delay={0.75}>
                  <p className={`${STATEMENT} font-medium text-balance`}>
                    Because you were {article(role.closestPast.titleText)}&nbsp;{role.closestPast.titleText}.
                  </p>
                </Reveal>
              )}
              {role.bring.length > 0 && (
                <Reveal seen={seen} delay={0.85} className={role.closestPast ? "mt-[0.9em]" : undefined}>
                  <p className={`${STATEMENT} font-medium text-balance`}>
                    You bring{" "}
                    {role.bring.map((s, i) => (
                      <Fragment key={s.slug}>
                        <span className="font-bold tracking-[-0.025em]">{s.name}</span>
                        {i < role.bring.length - 2 ? ", " : i === role.bring.length - 2 ? " and " : "."}
                      </Fragment>
                    ))}
                  </p>
                </Reveal>
              )}
            </div>
          )}

          {role.grow.length > 0 && (
            <Reveal seen={seen} delay={0.95} className={hasWhy ? "@[48rem]:col-span-4" : "@[48rem]:col-span-9"}>
              {/* The list's own first line, set at its size in the receding ink:
                  it names what the percentages are a share of (P(skill | role)). */}
              <p className={`${LIST} text-pretty text-content-secondary`}>
                Also asked for, as a share of its {role.openPostings.toLocaleString("en-US")}{" "}
                {plural(role.openPostings, "posting", "postings")}
              </p>
              <ul className="mt-[0.4em]">
                {role.grow.map((s) => (
                  <li
                    key={s.slug}
                    className={`${LIST} flex items-baseline justify-between gap-4 border-t border-content-primary/20 py-[0.4em]`}
                  >
                    <span className="text-balance">{s.name}</span>
                    <span className="shrink-0 font-light tabular-nums">{percentOfPostings(s.share)}</span>
                  </li>
                ))}
              </ul>
            </Reveal>
          )}

          <Reveal seen={seen} delay={1.05} className="flex gap-10 @[48rem]:col-span-3 @[48rem]:flex-col @[48rem]:gap-5">
            <Count n={role.openPostings} label={plural(role.openPostings, "open posting", "open postings")} />
            <Count n={role.companies} label={plural(role.companies, "company hiring", "companies hiring")} />
          </Reveal>
        </div>
      </div>
    </section>
  );
}

function Count({ n, label }: { readonly n: number; readonly label: string }) {
  return (
    <p>
      <span className="block font-display text-[clamp(2.75rem,5.5cqi,4.5rem)] leading-[0.9] font-extrabold tabular-nums tracking-[-0.04em]">
        {n.toLocaleString("en-US")}
      </span>
      <span className={`${SUPPORT} mt-[0.3em] block text-content-secondary`}>{label}</span>
    </p>
  );
}

interface RevealProps {
  readonly seen: boolean;
  readonly delay: number;
  readonly className?: string;
  readonly children: ReactNode;
}

function Reveal({ seen, delay, className, children }: RevealProps) {
  const reduce = useReducedMotion();
  return (
    <motion.div
      className={className}
      initial={{ opacity: 0, y: 18 }}
      animate={seen ? { opacity: 1, y: 0 } : undefined}
      transition={{ duration: reduce ? 0 : 0.8, ease: EASE_OUT, delay }}
    >
      {children}
    </motion.div>
  );
}
