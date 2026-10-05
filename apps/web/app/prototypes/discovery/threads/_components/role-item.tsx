"use client";

import { motion } from "motion/react";
import { useRef } from "react";

import type { PersonSkill, Recommendation } from "../../_data/query";
import { alsoAsksFor, alsoPostedAs, connectsOf, postingsLine, shareLabel } from "../_lib/copy";
import { drift, ENTRANCE, HOVER_DWELL_MS } from "../_lib/motion";
import { PinButton } from "./pin-button";
import { SkillMark } from "./skill-mark";
import { cn } from "@/lib/utils";

function Chip({ skill, lit }: { skill: PersonSkill; lit: boolean }) {
  return (
    <li
      className={cn(
        "inline-flex h-6 items-center gap-1.5 rounded-full border pr-2.5 pl-2 text-[0.8125rem] leading-none whitespace-nowrap transition-[background-color,border-color,color] duration-300 ease-out",
        skill.claimed
          ? lit
            ? "border-transparent bg-(--threads-tint) font-medium text-(--threads-ink)"
            : "border-transparent bg-surface-sunken font-medium text-content-primary"
          : lit
            ? "border-dashed border-(--threads-edge) bg-(--threads-wash) text-(--threads-ink)"
            : "border-dashed border-edge text-content-secondary",
      )}
    >
      <SkillMark claimed={skill.claimed} className={lit ? "text-(--threads-line)" : "text-content-muted"} />
      {skill.name}
    </li>
  );
}

function DrawsOn({ rec, skills, lit }: { rec: Recommendation; skills: ReadonlyMap<string, PersonSkill>; lit: boolean }) {
  const { claimed, inherited } = connectsOf(rec, skills);
  if (claimed.length + inherited.length === 0) {
    return (
      <p className="mt-4 text-[0.8125rem] leading-relaxed text-content-muted">
        None of its ten defining skills are on your list
        {rec.closestPast ? `. It sits nearest your time as ${rec.closestPast.titleText}.` : "."}
      </p>
    );
  }
  const rows = [
    { label: "You named", list: claimed },
    { label: "From roles you've held", list: inherited },
  ].filter((r) => r.list.length > 0);

  return (
    <dl className="mt-4 grid grid-cols-1 gap-y-1.5 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-start sm:gap-x-4 sm:gap-y-2">
      {rows.map((r) => (
        <div key={r.label} className="contents">
          <dt className="text-[0.75rem] leading-none whitespace-nowrap text-content-muted sm:pt-[0.3125rem]">{r.label}</dt>
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
    <p className="mt-3 text-[0.8125rem] leading-relaxed text-pretty text-content-muted">
      Also asks for{" "}
      {grow.map((g, i) => (
        <span key={g.slug}>
          <span className="text-content-secondary">{g.name}</span>{" "}
          <span className="tabular-nums">({i === 0 ? `in ${shareLabel(g.share)} of postings` : shareLabel(g.share)})</span>
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
  // sliding under a resting pointer would otherwise strobe the threads.
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
      {/* The lit tile. Hover previews it faintly; the lit role holds it. */}
      <span
        aria-hidden
        className={cn(
          "pointer-events-none absolute inset-x-2 inset-y-1 rounded-[1.25rem] transition-[background-color,box-shadow] duration-300 ease-out",
          lit ? "bg-(--threads-wash) shadow-[inset_0_0_0_1px_var(--threads-tint)]" : "group-hover/role:bg-surface-row-hover/60",
        )}
      />

      <div className="relative grid grid-cols-[1.75rem_minmax(0,1fr)_auto] gap-x-2 px-5 py-6 sm:grid-cols-[2rem_minmax(0,1fr)_auto] sm:gap-x-3 sm:px-7">
        <span
          aria-hidden
          className={cn(
            "pt-[0.4375rem] font-display text-[0.8125rem] tabular-nums transition-colors duration-300",
            lit ? "text-(--threads-ink)" : "text-content-muted",
          )}
        >
          {String(rec.rank).padStart(2, "0")}
        </span>

        <div className="min-w-0">
          <h3 ref={anchorRef} className="font-display text-[1.5rem] leading-[1.15] font-normal tracking-[-0.02em] text-balance text-content-primary">
            {/* Stretched over the row, so a tap anywhere lights the role. */}
            <button
              type="button"
              aria-pressed={lit}
              onClick={() => onLight(rec.roleSlug)}
              className="cursor-pointer text-left outline-none after:absolute after:inset-x-2 after:inset-y-1 after:rounded-[1.25rem] after:transition-shadow focus-visible:after:ring-2 focus-visible:after:ring-focus"
            >
              {rec.headline}
            </button>
          </h3>
          {also.length > 0 && (
            <p className="mt-1.5 text-[0.875rem] leading-snug text-pretty text-content-secondary">
              <span className="text-content-muted">Also posted as </span>
              {also.join(" · ")}
            </p>
          )}

          <DrawsOn rec={rec} skills={skills} lit={lit} />
          <AlsoAsks rec={rec} />

          <p className="mt-3 text-[0.75rem] text-content-muted tabular-nums">
            {postingsLine(rec)}
            {rec.closestPast && rec.connects.length > 0 && <> · nearest your time as {rec.closestPast.titleText}</>}
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
