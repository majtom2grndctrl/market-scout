"use client";

import { motion } from "motion/react";

import type { DiscoveryData } from "../../_data/query";
import { countWord, namedSkills, plural, tallyByPastTitle } from "../_lib/copy";
import { drift } from "../_lib/motion";
import { cn } from "@/lib/utils";

const PANEL_AT = 0.6;
const ITEMS_AT = 0.9;
const ITEM_STEP = 0.1;

/**
 * The person's own past, read back as evidence: each past title with how many
 * of the roles below sit nearest to it, and each named skill with how many of
 * those roles count it among their ten defining skills. The thread draws down first, then titles settle on
 * it in the order the person gave them.
 */
export function Trail({ data }: { data: DiscoveryData }) {
  const tally = tallyByPastTitle(data.recommendations);
  const skills = namedSkills(data);
  const anyCarries = skills.some((s) => s.carries > 0);
  const past = data.pastRoles;
  const skillsAt = ITEMS_AT + past.length * ITEM_STEP + 0.15;

  if (past.length === 0 && skills.length === 0) return null;

  return (
    <motion.aside
      aria-label="What the recommendations draw on"
      initial={{ opacity: 0, y: 28 }}
      animate={{ opacity: 1, y: 0 }}
      transition={drift(PANEL_AT, 1.4)}
      className="rounded-[1.75rem] bg-surface-raised/72 p-6 shadow-(--airy-lift) ring-1 ring-edge-hairline backdrop-blur-md"
    >
      {past.length > 0 && (
        <section>
          <h2 className="font-display text-[1.25rem] leading-tight font-normal tracking-[-0.012em] text-content-primary">
            Where you&apos;ve been
          </h2>
          <ol className="relative mt-3">
            <motion.span
              aria-hidden
              className="absolute top-5 bottom-5 left-[5px] w-px origin-top bg-(--airy-thread)"
              initial={{ scaleY: 0 }}
              animate={{ scaleY: 1 }}
              transition={drift(ITEMS_AT - 0.1, 0.4 + past.length * ITEM_STEP * 1.6)}
            />
            {past.map((p, i) => {
              const n = tally.get(p.titleText) ?? 0;
              const note =
                n > 0
                  ? `Nearest to ${countWord(n)} of the roles below`
                  : p.openPostings !== null
                    ? `${p.openPostings.toLocaleString("en-US")} open ${plural(p.openPostings, "posting")} in this role`
                    : null;
              return (
                <motion.li
                  key={`${p.titleText}-${i}`}
                  className="relative py-1.5 pl-7"
                  initial={{ opacity: 0, x: -6 }}
                  animate={{ opacity: 1, x: 0 }}
                  transition={drift(ITEMS_AT + i * ITEM_STEP, 1)}
                >
                  <span
                    aria-hidden
                    className={cn(
                      "absolute top-[0.72rem] left-0 size-[11px] rounded-full border-2",
                      n > 0 ? "border-(--airy-ember) bg-(--airy-glow)" : "border-(--airy-thread) bg-surface-raised",
                    )}
                  />
                  <p className="text-base leading-snug font-medium text-content-primary">{p.titleText}</p>
                  {note && <p className="mt-0.5 text-sm text-content-muted">{note}</p>}
                </motion.li>
              );
            })}
          </ol>
        </section>
      )}

      {skills.length > 0 && (
        <motion.section
          className={cn(past.length > 0 && "mt-5 border-t border-edge-hairline pt-5")}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={drift(skillsAt, 1)}
        >
          <h2 className="font-display text-[1.25rem] leading-tight font-normal tracking-[-0.012em] text-content-primary">
            What you named
          </h2>
          <ul className="mt-3.5 flex flex-wrap gap-1.5">
            {skills.map((s, i) => (
              <li
                key={`${s.text}-${i}`}
                className={cn(
                  "inline-flex h-7.5 items-center rounded-full px-3 text-sm",
                  s.carries > 0
                    ? "bg-(--airy-glow) text-content-primary ring-1 ring-(--airy-glow-edge) ring-inset"
                    : "text-content-secondary ring-1 ring-edge ring-inset",
                )}
              >
                {s.text}
                {s.carries > 0 && (
                  <span className="ml-2 text-sm font-medium text-(--airy-ember) tabular-nums">{s.carries}</span>
                )}
              </li>
            ))}
          </ul>
          {anyCarries && (
            <p className="mt-4 text-sm leading-relaxed text-content-muted">
              The number is how many of the roles below count the skill among their ten defining skills.
            </p>
          )}
        </motion.section>
      )}
    </motion.aside>
  );
}
