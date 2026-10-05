"use client";

import { motion } from "motion/react";
import type { CSSProperties, Ref } from "react";

import type { PersonSkill } from "../../_data/query";
import { inheritedNote, splitSkills } from "../_lib/copy";
import { drift, ENTRANCE } from "../_lib/motion";
import styles from "../threads.module.css";
import { SkillMark } from "./skill-mark";
import { cn } from "@/lib/utils";

/** rest: no role lit yet. lit: the active role draws on it. dim: a role is lit, and not through this skill. */
type State = "rest" | "lit" | "dim";

function SkillRow({
  skill,
  order,
  state,
  arrival,
  markRef,
}: {
  skill: PersonSkill;
  order: number;
  state: State;
  arrival: number;
  markRef: (el: HTMLElement | null) => void;
}) {
  const lit = state === "lit";
  // Light on arrival of the line, so the eye lands with it; let go at once.
  const timing: CSSProperties = { transitionDelay: lit ? `${Math.round(arrival * 1000)}ms` : "0ms" };

  return (
    <motion.li
      className={cn(styles.skillRow, "group/skill relative min-w-0")}
      initial={{ opacity: 0, x: -8 }}
      animate={{ opacity: 1, x: 0 }}
      transition={drift(ENTRANCE.skills + order * ENTRANCE.skillStep, 0.8)}
    >
      <span
        style={timing}
        className={cn(
          styles.pill,
          "inline-flex max-w-full min-w-0 items-center gap-2 rounded-full border py-[0.1875rem] pr-2 pl-2.5 leading-[1.2]",
          "transition-[background-color,border-color,color] duration-300 ease-out",
          skill.claimed ? "text-[0.8125rem] font-medium" : "text-[0.8125rem]",
          state === "rest" && (skill.claimed ? "border-transparent text-content-primary" : "border-transparent text-content-secondary"),
          state === "dim" && (skill.claimed ? "border-transparent text-content-muted" : "border-transparent text-content-disabled"),
          lit &&
            (skill.claimed
              ? "border-transparent bg-(--threads-tint) text-(--threads-ink)"
              : "border-dashed border-(--threads-edge) bg-(--threads-wash) text-(--threads-ink)"),
        )}
      >
        <span className="truncate">{skill.name}</span>
        <SkillMark
          ref={markRef}
          claimed={skill.claimed}
          style={timing}
          className={cn(lit ? "text-(--threads-line)" : state === "dim" ? "text-content-disabled" : "text-content-muted")}
        />
      </span>
      {!skill.claimed && (
        <>
          <span className="sr-only">, {inheritedNote(skill)}</span>
          {/* Where an inherited skill comes from, on hover: the past titles it rides in on. */}
          <span
            aria-hidden
            className="pointer-events-none absolute bottom-full left-1 z-20 mb-0.5 max-w-[17rem] translate-y-1 rounded-lg bg-surface-overlay px-2.5 py-1.5 text-[0.75rem] leading-snug text-content-secondary opacity-0 shadow-(--threads-float) ring-1 ring-edge-hairline transition-[opacity,transform] duration-200 ease-out group-hover/skill:translate-y-0 group-hover/skill:opacity-100"
          >
            {inheritedNote(skill)}
          </span>
        </>
      )}
    </motion.li>
  );
}

function Group({
  id,
  title,
  note,
  skills,
  offset,
  lit,
  anyLit,
  registerSkill,
  className,
}: {
  id: string;
  title: string;
  note?: string;
  skills: readonly PersonSkill[];
  offset: number;
  lit: ReadonlyMap<string, number>;
  anyLit: boolean;
  registerSkill: (slug: string) => (el: HTMLElement | null) => void;
  className?: string;
}) {
  if (skills.length === 0) return null;
  return (
    <section aria-labelledby={id} className={className}>
      <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={drift(ENTRANCE.skills + offset * ENTRANCE.skillStep, 0.8)}>
        <h2 id={id} className="flex items-baseline gap-2 pl-2.5 text-[0.9375rem] leading-6 font-normal tracking-[-0.005em] text-content-primary">
          {title}
          <span className="text-[0.8125rem] text-content-muted tabular-nums">{skills.length}</span>
        </h2>
        {note && <p className="mt-0.5 pl-2.5 text-[0.75rem] leading-[1.45] text-pretty text-content-muted">{note}</p>}
      </motion.div>
      <ul className={cn(styles.skillList, "mt-2")}>
        {skills.map((s, i) => (
          <SkillRow
            key={s.slug}
            skill={s}
            order={offset + 1 + i}
            state={lit.has(s.slug) ? "lit" : anyLit ? "dim" : "rest"}
            arrival={lit.get(s.slug) ?? 0}
            markRef={registerSkill(s.slug)}
          />
        ))}
      </ul>
    </section>
  );
}

export function SkillColumn({
  skills,
  lit,
  hasActive,
  columnRef,
  registerSkill,
}: {
  skills: readonly PersonSkill[];
  /** Lit skills, each with the seconds until its thread arrives. */
  lit: ReadonlyMap<string, number>;
  /** A role holds the light, even one that lights nothing. */
  hasActive: boolean;
  columnRef: Ref<HTMLElement>;
  registerSkill: (slug: string) => (el: HTMLElement | null) => void;
}) {
  const { claimed, inherited } = splitSkills(skills);
  const style = { "--threads-rows": Math.max(skills.length, 1) } as CSSProperties;

  return (
    <aside ref={columnRef} aria-label="Your skills" style={style} className={cn(styles.column, "pb-6 @min-[60rem]/threads:pt-14")}>
      {skills.length === 0 ? (
        <p className="pl-2.5 text-sm text-content-muted">No skills to show yet. Name a few on your profile and they appear here.</p>
      ) : (
        <>
          <Group
            id="threads-named"
            title="Skills you named"
            skills={claimed}
            offset={0}
            lit={lit}
            anyLit={hasActive}
            registerSkill={registerSkill}
          />
          <Group
            id="threads-inherited"
            title="Come with roles you've held"
            note="Common in postings for your past titles. Read from the market, not something you said."
            skills={inherited}
            offset={claimed.length + 1}
            lit={lit}
            anyLit={hasActive}
            registerSkill={registerSkill}
            className={claimed.length > 0 ? "mt-6" : undefined}
          />
        </>
      )}
    </aside>
  );
}
