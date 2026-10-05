"use client";

import { motion } from "motion/react";

import type { AllBeat } from "./copy";
import { figure, plural } from "./grammar";
import { PinButton } from "./pin-button";
import { Arrive, EASE_OUT, mainThread, phrases, timeline } from "./reveal";
import styles from "./letter.module.css";

const ROW_GAP = 0.06;

export function AllBeatView({ beat, pins, onTogglePin }: { beat: AllBeat; pins: ReadonlySet<string>; onTogglePin: (slug: string) => void }) {
  const tl = timeline();
  const eyebrowAt = tl.cue(0);
  const display = phrases(beat.display, tl, { first: 0.22 });
  const bodyAt = tl.cue(0.45);
  // The list follows the sentence that introduces it, one row after another
  // in rank order, quick enough that the last row is not a wait.
  let rowAt = tl.cue(0.3);
  const footAt = tl.cue(0.1);

  return (
    <div className="pt-[8vh] pb-20">
      <Arrive delay={eyebrowAt} as="p" className={styles.eyebrow}>
        {beat.eyebrow}
      </Arrive>
      <h2 className={`${styles.display} mt-6 font-display text-content-primary`}>{display}</h2>
      <Arrive delay={bodyAt} as="p" className={`${styles.prose} mt-6 max-w-[36rem] text-content-secondary`}>
        {beat.body}
      </Arrive>

      <div className="mt-14 max-w-[60rem] space-y-12">
        {beat.groups.map((group) => {
          const headAt = rowAt;
          rowAt += ROW_GAP * 1.5;
          return (
            <section key={group.strength} aria-label={group.label}>
              <motion.h3
                {...mainThread}
                className="flex items-center gap-4 text-xs font-medium tracking-[0.12em] text-content-muted uppercase"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ duration: 0.6, delay: headAt }}
              >
                {group.label}
                <motion.span
                  aria-hidden
                  className="h-px flex-1 origin-left bg-edge"
                  initial={{ scaleX: 0 }}
                  animate={{ scaleX: 1 }}
                  transition={{ duration: 0.9, ease: EASE_OUT, delay: headAt + 0.1 }}
                />
              </motion.h3>
              <ol className="mt-2">
                {group.rows.map((row) => {
                  const at = rowAt;
                  rowAt += ROW_GAP;
                  const { rec } = row;
                  return (
                    <motion.li
                      key={rec.roleSlug}
                      {...mainThread}
                      className="grid grid-cols-[2rem_minmax(0,1fr)_auto] items-start gap-x-4 border-b border-edge-hairline py-5 sm:grid-cols-[2.5rem_minmax(0,1fr)_9rem_auto] sm:items-center"
                      initial={{ opacity: 0, y: 10 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ duration: 0.6, ease: EASE_OUT, delay: at }}
                    >
                      <span className="pt-1 font-display text-lg text-content-muted tabular-nums sm:pt-0">{rec.rank}</span>
                      <div className="min-w-0">
                        <p className="font-display text-2xl leading-tight font-[460] tracking-[-0.015em] text-balance text-content-primary">
                          {rec.headline}
                        </p>
                        {row.alternates.length > 0 ? (
                          <p className="mt-1 text-sm text-pretty text-content-muted">Also {row.alternates.join(" · ")}</p>
                        ) : null}
                        {row.reason ? <p className="mt-2 text-sm text-pretty text-content-secondary">{row.reason}</p> : null}
                        <p className="mt-2 text-sm text-content-muted tabular-nums sm:hidden">{postings(rec.openPostings, rec.companies)}</p>
                      </div>
                      <p className="hidden text-right text-sm leading-snug text-content-muted tabular-nums sm:block">
                        {figure(rec.openPostings)} {plural(rec.openPostings, "posting")}
                        <br />
                        {figure(rec.companies)} {plural(rec.companies, "company", "companies")}
                      </p>
                      <PinButton quiet pinned={pins.has(rec.roleSlug)} onToggle={() => onTogglePin(rec.roleSlug)} name={rec.headline} />
                    </motion.li>
                  );
                })}
              </ol>
            </section>
          );
        })}
      </div>

      <Arrive delay={Math.max(footAt, rowAt)} as="p" className="mt-10 max-w-[40rem] text-xs leading-relaxed text-content-muted">
        {beat.coverage}
      </Arrive>
    </div>
  );
}

function postings(n: number, companies: number): string {
  return `${figure(n)} ${plural(n, "posting")} at ${figure(companies)} ${plural(companies, "company", "companies")}`;
}
