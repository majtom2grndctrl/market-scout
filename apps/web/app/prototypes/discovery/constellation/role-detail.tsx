"use client";

import { motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { EASE_OUT } from "./choreography";
import { ORBIT_NAME, alsoCalled, countWord, plural, postings, shareOfPostings } from "./copy";
import { PinGlyph } from "./pin-glyph";

interface Props {
  rec: Recommendation;
  pinned: boolean;
  claimedCount: number;
  onTogglePin: () => void;
}

/** What one role is, what the person brings to it, what it also asks for. Coverage language only. */
export function RoleDetail({ rec, pinned, claimedCount, onTogglePin }: Props) {
  const also = alsoCalled(rec);

  return (
    <motion.article
      key={rec.roleSlug}
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.22, ease: EASE_OUT }}
      className="flex flex-col gap-4"
    >
      <header className="space-y-1.5">
        <p className="text-[11px] font-semibold tracking-[0.14em] text-content-muted uppercase">
          No. {rec.rank} · {ORBIT_NAME[rec.strength]} orbit
        </p>
        <h2 className="font-display text-[1.7rem] leading-[1.1] font-semibold tracking-tight text-balance">{rec.headline}</h2>
        <p className="text-[13px] text-content-muted tabular-nums">
          {postings(rec.openPostings)} open at {rec.companies.toLocaleString("en-US")} {plural(rec.companies, "company", "companies")}
        </p>
        {also.length > 0 && (
          <p className="text-[13px] leading-snug text-content-secondary">
            <span className="text-content-muted">Also called </span>
            {also.map((t, i) => (
              <span key={t.title}>
                {i > 0 && <span className="text-content-disabled"> · </span>}
                {t.title}
              </span>
            ))}
          </p>
        )}
      </header>

      <dl className="space-y-3.5 text-sm">
        <Field term="Builds on">
          {rec.closestPast ? (
            <>
              your time as <span className="font-medium text-content-primary">{rec.closestPast.titleText}</span>
            </>
          ) : (
            "your profile as a whole, rather than one past title"
          )}
        </Field>

        <Field term="What you bring">
          {rec.bring.length > 0 ? (
            <ul className="flex flex-wrap gap-1.5">
              {rec.bring.map((s) => (
                <li key={s.slug} className="rounded-full bg-(--constellation-pin-wash) px-2.5 py-0.5 text-[13px] text-content-primary ring-1 ring-(--constellation-pin-ink)/25">
                  {s.name}
                </li>
              ))}
            </ul>
          ) : (
            <span className="text-content-secondary">
              {claimedCount > 0
                ? `None of your ${countWord(claimedCount)} claimed ${plural(claimedCount, "skill")} is among its top ten.`
                : "You haven’t claimed any skills to compare yet."}
              {rec.closestPast ? ` The link runs through ${rec.closestPast.titleText}.` : " The link runs through your profile as a whole."}
            </span>
          )}
        </Field>

        {rec.grow.length > 0 && (
          <Field term="It also asks for">
            <ul className="divide-y divide-edge-hairline">
              {rec.grow.map((s) => (
                <li key={s.slug} className="flex items-baseline justify-between gap-4 py-1 first:pt-0 last:pb-0">
                  <span className="text-content-primary">{s.name}</span>
                  <span className="shrink-0 text-xs text-content-muted tabular-nums">{shareOfPostings(s.share)}</span>
                </li>
              ))}
            </ul>
          </Field>
        )}

      </dl>

      <button
        type="button"
        onClick={onTogglePin}
        aria-pressed={pinned}
        className={
          pinned
            ? "group flex h-9 items-center justify-center gap-2 rounded-lg border border-(--constellation-pin-ink)/40 bg-(--constellation-pin-wash) text-sm font-medium text-content-primary transition-colors hover:bg-surface-row-hover"
            : "flex h-9 items-center justify-center gap-2 rounded-lg bg-accent-solid text-sm font-medium text-on-solid transition-colors hover:bg-accent-hover"
        }
      >
        <PinGlyph className={pinned ? "text-(--constellation-pin-ink)" : "text-on-solid"} />
        {pinned ? (
          <>
            <span className="group-hover:hidden">Pinned</span>
            <span className="hidden group-hover:inline">Unpin</span>
          </>
        ) : (
          "Pin this role"
        )}
      </button>
    </motion.article>
  );
}

function Field({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <dt className="text-[11px] font-semibold tracking-[0.12em] text-content-muted uppercase">{term}</dt>
      <dd className="leading-relaxed text-content-secondary">{children}</dd>
    </div>
  );
}
