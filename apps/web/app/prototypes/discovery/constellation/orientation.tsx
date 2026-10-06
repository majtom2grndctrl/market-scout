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
      <h2 className="font-display text-[1.375rem] leading-[1.2] font-semibold text-balance">{headingFor(data.recommendations)}</h2>

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
                    <span className={`font-display text-base font-medium ${row.key === WHOLE_KEY ? "italic" : ""}`}>{row.title}</span>
                    <span className="shrink-0 text-sm text-content-muted tabular-nums">
                      {leads ? `${row.recs.length} ${plural(row.recs.length, "role")}` : "—"}
                    </span>
                  </div>
                  <p className="mt-1 text-sm leading-snug text-content-secondary">
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

// "Trace back" is a claim: only a role with a closest past title or a claimed
// skill among its ten defining skills makes it.
function headingFor(recs: readonly Recommendation[]): string {
  const n = recs.length;
  if (n === 0) return "No roles are ranked for this profile yet.";
  const traced = recs.filter((r) => r.closestPast !== null || r.bring.length > 0).length;
  if (traced === n) return `${capitalise(countWord(n))} ${plural(n, "role")} ${n === 1 ? "traces" : "trace"} back to what you've done.`;
  if (traced === 0) return `${capitalise(countWord(n))} ${plural(n, "role")} ranked against your profile.`;
  return `${capitalise(countWord(traced))} of ${countWord(n)} roles ${traced === 1 ? "traces" : "trace"} back to what you've done.`;
}

function capitalise(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
