"use client";

import { motion } from "motion/react";
import { useRef } from "react";

import type { PersonSkill, Recommendation } from "../../_data/query";
import { alsoAsksFor, alsoPostedAs, connectsOf, postingsLine, shareLabel } from "../_lib/copy";
import { drift, ENTRANCE, HOVER_DWELL_MS } from "../_lib/motion";
import styles from "../dawn.module.css";
import { PinButton } from "./pin-button";
import { SkillMark } from "./skill-mark";
import { cn } from "@/lib/utils";

function Chip({ skill, lit }: { skill: PersonSkill; lit: boolean }) {
  return (
    <li
      className={cn(
        "inline-flex h-7 items-center gap-1.5 rounded-full border pr-3 pl-2.5 text-sm leading-none whitespace-nowrap transition-[background-color,border-color,color] duration-300 ease-out",
        skill.claimed
          ? lit
            ? "border-transparent bg-(--dawn-glow) font-medium text-content-primary"
            : "border-transparent bg-surface-sunken font-medium text-content-primary"
          : lit
            ? "border-dashed border-(--dawn-glow-edge) bg-(--dawn-horizon) text-content-primary"
            : "border-dashed border-edge text-content-secondary",
      )}
    >
      <SkillMark claimed={skill.claimed} className={lit ? "text-(--dawn-ember)" : "text-content-muted"} />
      {skill.name}
    </li>
  );
}

function DrawsOn({ rec, skills, lit }: { rec: Recommendation; skills: ReadonlyMap<string, PersonSkill>; lit: boolean }) {
  const { claimed, inherited } = connectsOf(rec, skills);
  if (claimed.length + inherited.length === 0) {
    return (
      <p className="mt-4 text-sm leading-relaxed text-pretty text-content-muted">
        {rec.closestPast && `Closest to your ${rec.closestPast.titleText} role. `}
        {"Its most-asked skills aren't on your list."}
      </p>
    );
  }
  const rows = [
    { label: "You added", list: claimed },
    { label: "From past roles", list: inherited },
  ].filter((r) => r.list.length > 0);

  return (
    <dl className="mt-5 grid grid-cols-1 gap-y-1.5 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-start sm:gap-x-4 sm:gap-y-2">
      {rows.map((r) => (
        <div key={r.label} className="contents">
          <dt className="text-sm leading-7 whitespace-nowrap text-content-muted">{r.label}</dt>
          <dd className="max-sm:mb-1.5">
            <ul className="flex flex-wrap gap-1.5">
              {r.list.map((s) => (
                <Chip key={s.slug} skill={s} lit={lit} />
              ))}
            </ul>
          </dd>
        </div>
      ))}
    </dl>
  );
}

function AlsoAsks({ rec }: { rec: Recommendation }) {
  const grow = alsoAsksFor(rec).slice(0, 3);
  if (grow.length === 0) return null;
  return (
    <p className="mt-4 text-sm leading-relaxed text-pretty text-content-muted">
      Also asks for{" "}
      {grow.map((g, i) => (
        <span key={g.slug}>
          <span className="text-content-secondary">{g.name}</span>{" "}
          <span className="tabular-nums">({i === 0 ? `${shareLabel(g.share)} of postings` : shareLabel(g.share)})</span>
          {i < grow.length - 1 ? ", " : ""}
        </span>
      ))}
    </p>
  );
}

export function RoleItem({
  rec,
  index,
  skills,
  lit,
  pinned,
  onLight,
  onToggle,
  isScrolling,
  anchorRef,
}: {
  rec: Recommendation;
  index: number;
  skills: ReadonlyMap<string, PersonSkill>;
  lit: boolean;
  pinned: boolean;
  onLight: (roleSlug: string) => void;
  onToggle: (roleSlug: string) => void;
  isScrolling: () => boolean;
  anchorRef: (el: HTMLElement | null) => void;
}) {
  const also = alsoPostedAs(rec, 3);
  const dwell = useRef<number | null>(null);
  const clear = () => {
    if (dwell.current !== null) window.clearTimeout(dwell.current);
    dwell.current = null;
  };
  // Light after a short dwell, and only once scrolling has settled: rows
  // sliding under a resting pointer would otherwise strobe the lines.
  const arm = () => {
    dwell.current = window.setTimeout(() => (isScrolling() ? arm() : onLight(rec.roleSlug)), HOVER_DWELL_MS);
  };

  return (
    <motion.li
      className="group/role relative"
      initial={{ opacity: 0, y: 14 }}
      animate={{ opacity: 1, y: 0 }}
      transition={drift(ENTRANCE.roles + index * ENTRANCE.roleStep, 0.9)}
      onPointerEnter={(e) => {
        if (e.pointerType !== "mouse") return;
        clear();
        arm();
      }}
      onPointerLeave={clear}
      onFocus={() => onLight(rec.roleSlug)}
    >
      {/* The lit tile: light falls on it from the port, at the card's left
          edge. Hover previews the tile faintly; the lit role holds the light. */}
      <span
        aria-hidden
        className={cn(
          "pointer-events-none absolute inset-x-2 inset-y-1 rounded-[1.25rem] transition-colors duration-300 ease-out",
          !lit && "group-hover/role:bg-surface-row-hover/60",
        )}
      />
      <span
        aria-hidden
        className={cn(
          styles.litTile,
          "pointer-events-none absolute inset-x-2 inset-y-1 rounded-[1.25rem] transition-opacity duration-500 ease-out",
          lit ? "opacity-100" : "opacity-0",
        )}
      />

      <div className="relative grid grid-cols-[1.75rem_minmax(0,1fr)_auto] gap-x-2 px-5 py-6 sm:grid-cols-[2rem_minmax(0,1fr)_auto] sm:gap-x-3 sm:px-7">
        <span
          aria-hidden
          className={cn(
            "pt-[0.5rem] font-display text-sm tabular-nums transition-colors duration-300",
            lit ? "text-(--dawn-ember)" : "text-content-muted",
          )}
        >
          {String(rec.rank).padStart(2, "0")}
        </span>

        <div className="min-w-0">
          <h3 ref={anchorRef} className="font-display text-[1.625rem] leading-[1.15] font-normal tracking-[-0.02em] text-balance text-content-primary">
            {/* Stretched over the row, so a tap anywhere lights the role. */}
            <button
              type="button"
              data-dawn-role={rec.roleSlug}
              aria-pressed={lit}
              onClick={() => onLight(rec.roleSlug)}
              className="cursor-pointer text-left outline-none after:absolute after:inset-x-2 after:inset-y-1 after:rounded-[1.25rem] after:transition-shadow focus-visible:after:ring-2 focus-visible:after:ring-focus"
            >
              {rec.headline}
            </button>
          </h3>
          {also.length > 0 && (
            <p className="mt-2 text-[0.9375rem] leading-snug text-pretty text-content-secondary">
              <span className="text-content-muted">Also posted as </span>
              {also.join(" · ")}
            </p>
          )}

          <DrawsOn rec={rec} skills={skills} lit={lit} />
          <AlsoAsks rec={rec} />

          <p className="mt-4 text-sm text-content-muted tabular-nums">
            {postingsLine(rec)}
            {rec.closestPast && rec.connects.length > 0 && <> · closest to your {rec.closestPast.titleText} role</>}
          </p>
        </div>

        <PinButton
          roleSlug={rec.roleSlug}
          headline={rec.headline}
          pinned={pinned}
          onToggle={onToggle}
          className="relative z-10 mt-0.5 self-start"
        />
      </div>
    </motion.li>
  );
}
