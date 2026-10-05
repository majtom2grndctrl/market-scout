"use client";

import { motion } from "motion/react";

import type { DiscoveryData, Recommendation } from "../_data/query";
import { EASE_OUT } from "./choreography";
import { WHOLE_PROFILE, countWord, plural } from "./copy";
import { WHOLE_KEY, groupKeyOf } from "./geometry";

interface Props {
  data: DiscoveryData;
  activeGroup: string | null;
  onGroup: (key: string | null) => void;
}

/**
 * The rail before any star is chosen: each past title and the roles that
 * build on it. Evidence-first encouragement — a title that leads somewhere is
 * shown leading there, by name.
 */
export function Orientation({ data, activeGroup, onGroup }: Props) {
  const byKey = new Map<string, Recommendation[]>();
  for (const r of data.recommendations) byKey.set(groupKeyOf(r), [...(byKey.get(groupKeyOf(r)) ?? []), r]);

  const rows = [
    ...[...new Set(data.pastRoles.map((p) => p.titleText))].map((t) => ({ key: t, title: t, recs: byKey.get(t) ?? [] })),
    ...(byKey.has(WHOLE_KEY) ? [{ key: WHOLE_KEY, title: WHOLE_PROFILE, recs: byKey.get(WHOLE_KEY) ?? [] }] : []),
  ];

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, ease: EASE_OUT }}
      className="flex flex-col gap-4"
    >
      <header className="space-y-1.5">
        <p className="text-[11px] font-semibold tracking-[0.14em] text-content-muted uppercase">Paths from your past</p>
        <h2 className="font-display text-xl leading-snug font-semibold text-balance">
          {data.recommendations.length > 0
            ? `${capitalise(countWord(data.recommendations.length))} ${plural(data.recommendations.length, "role")} trace back to what you've done.`
            : "No roles are ranked for this profile yet."}
        </h2>
      </header>

      {rows.length > 0 && (
        <ul className="-mx-2 flex flex-col" onMouseLeave={() => onGroup(null)}>
          {rows.map((row) => {
            const leads = row.recs.length > 0;
            const on = activeGroup === row.key;
            return (
              <li key={row.key}>
                <div
                  onMouseEnter={leads ? () => onGroup(row.key) : undefined}
                  className={`rounded-lg px-2 py-2.5 transition-colors ${on ? "bg-surface-row-hover" : ""}`}
                >
                  <div className="flex items-baseline justify-between gap-3">
                    <span className={`font-display text-[15px] font-medium ${row.key === WHOLE_KEY ? "italic" : ""}`}>{row.title}</span>
                    <span className="shrink-0 text-xs text-content-muted tabular-nums">
                      {leads ? `${row.recs.length} ${plural(row.recs.length, "role")}` : "—"}
                    </span>
                  </div>
                  <p className="mt-0.5 text-[13px] leading-snug text-content-secondary">
                    {leads ? row.recs.map((r) => r.headline).join(" · ") : "No role in this ranking builds on it most closely."}
                  </p>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </motion.div>
  );
}

function capitalise(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
