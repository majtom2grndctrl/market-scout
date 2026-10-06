"use client";

// The depth behind one row, set on the same columns as the row above it so
// each paragraph sits under the cell it explains.

import { motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { formatCount, listPhrase, plural } from "./derive";
import { COLS, NARROW } from "./grid";
import { EASE_OUT } from "./motion";

export function RowDetail({ rec }: { readonly rec: Recommendation }) {
  const maxTitle = Math.max(1, ...rec.titles.map((t) => t.postings));
  const perCompany = rec.companies > 0 ? rec.openPostings / rec.companies : 0;

  return (
    <div className={`grid ${NARROW} gap-x-5 gap-y-6 px-3 pt-1 pb-7 ${COLS}`}>
      <section className="col-start-2 min-w-0 @5xl:col-start-2">
        <Label>Posted as</Label>
        <ul className="space-y-1.5">
          {rec.titles.map((t, i) => (
            <li key={`${t.title}-${i}`} className="grid grid-cols-[minmax(0,1fr)_2rem_3ch] items-center gap-x-3 text-[14px] leading-[1.35] @2xl:grid-cols-[minmax(0,1fr)_3.5rem_3ch]">
              <span className="truncate" title={t.title}>
                {t.title}
              </span>
              <span aria-hidden className="h-[3px] bg-edge-hairline">
                <motion.span
                  className="block h-full origin-left bg-ramp-3"
                  style={{ width: `${(t.postings / maxTitle) * 100}%` }}
                  initial={{ scaleX: 0 }}
                  animate={{ scaleX: 1 }}
                  transition={{ delay: 0.1 + i * 0.04, duration: 0.4, ease: EASE_OUT }}
                />
              </span>
              <span className="text-right text-content-secondary tabular-nums">{formatCount(t.postings)}</span>
            </li>
          ))}
        </ul>
        <p className="mt-2.5 text-[14px] leading-[1.4] text-content-muted">Open postings per title, as written.</p>
      </section>

      <section className="col-start-2 min-w-0 @2xl:col-start-3 @5xl:col-span-2 @5xl:col-start-3">
        <Label>Why it&rsquo;s here</Label>
        <div className="space-y-2 text-[14px] leading-[1.5] text-content-secondary">
          {rec.closestPast && (
            <p>
              Of your past titles, its skills sit nearest <Em>{rec.closestPast.titleText}</Em>.
            </p>
          )}
          {rec.bring.length > 0 ? (
            <p>
              You claimed <Em>{listPhrase(rec.bring.map((b) => b.name))}</Em>, {plural(rec.bring.length, "one of", "each among")} the ten skills that
              most distinguish this role.
            </p>
          ) : (
            <p>None of your claimed skills is among its ten most distinctive; it ranks here on how its skills overlap your record as a whole.</p>
          )}
        </div>
      </section>

      <section className="col-start-2 min-w-0 @5xl:col-start-5">
        <Label>Reading the bars</Label>
        <p className="text-[14px] leading-[1.5] text-content-secondary">
          Each bar is the share of this role&rsquo;s <Em>{formatCount(rec.openPostings)}</Em> open postings that ask for the skill. They are
          listed by how particular the skill is to this role, not by share.
        </p>
      </section>

      <section className="col-start-2 min-w-0 @2xl:col-start-3 @5xl:col-span-2 @5xl:col-start-6 @5xl:text-right">
        <Label>Per company</Label>
        <p className="text-[16px] font-medium tabular-nums">{perCompany.toFixed(1)}</p>
        <p className="mt-1 text-[14px] leading-[1.4] text-content-muted">open postings each, on average</p>
      </section>
    </div>
  );
}

/** A subheading, not a tag: sentence case in primary ink, one step above the 14px text it heads. */
function Label({ children }: { readonly children: React.ReactNode }) {
  return <h3 className="mb-2 font-sans text-[15px] leading-5 font-semibold text-content-primary">{children}</h3>;
}

function Em({ children }: { readonly children: React.ReactNode }) {
  return <span className="text-content-primary">{children}</span>;
}
