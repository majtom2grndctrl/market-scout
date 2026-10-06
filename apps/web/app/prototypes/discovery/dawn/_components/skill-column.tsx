"use client";

import { motion } from "motion/react";
import type { CSSProperties, Ref } from "react";

import type { PersonSkill } from "../../_data/query";
import { inheritedNote, outsideNote, splitSkills } from "../_lib/copy";
import { drift, ENTRANCE } from "../_lib/motion";
import styles from "../dawn.module.css";
import { SkillMark } from "./skill-mark";
import { cn } from "@/lib/utils";

/** rest: no role lit yet. lit: the active role draws on it. dim: a role is lit, and not through this skill. */
type State = "rest" | "lit" | "dim";

/*
 * Ink per state. A lit skill catches the light: its pill warms to the sun's
 * glow and its marker turns ember where the line lands, while its ink stays
 * full strength. Everything else recedes to the last readable ink step, never
 * below it (build contract, Invariant 9). Weight, the pill's edge (solid or
 * dashed), and the marker (dot or ring) keep claimed and inherited apart in
 * every state.
 */
const INK: Record<Exclude<State, "lit">, Record<"claimed" | "inherited", string>> = {
  rest: { claimed: "text-content-primary", inherited: "text-content-secondary" },
  dim: { claimed: "text-content-muted", inherited: "text-content-muted" },
};

/** Where an inherited skill comes from, on hover: the past titles it rides in on. */
function Provenance({ skill }: { skill: PersonSkill }) {
  return (
    <>
      <span className="sr-only">, {inheritedNote(skill)}</span>
      <span
        aria-hidden
        className="pointer-events-none absolute bottom-full left-1 z-20 mb-0.5 w-max max-w-[18rem] translate-y-1 rounded-lg bg-surface-overlay px-3 py-2 text-sm leading-snug text-content-secondary opacity-0 shadow-(--dawn-float) ring-1 ring-edge-hairline transition-[opacity,transform] duration-200 ease-out group-hover/skill:translate-y-0 group-hover/skill:opacity-100"
      >
        {inheritedNote(skill)}
      </span>
    </>
  );
}

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
  const kind = skill.claimed ? "claimed" : "inherited";
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
          "inline-flex max-w-full min-w-0 items-center gap-2 rounded-full border py-[0.1875rem] pr-2 pl-2.5 text-sm leading-[1.2]",
          "transition-[background-color,border-color,color,box-shadow] duration-500 ease-out",
          skill.claimed && "font-medium",
          lit
            ? skill.claimed
              ? "border-transparent bg-(--dawn-glow) text-content-primary shadow-(--dawn-halo)"
              : "border-dashed border-(--dawn-glow-edge) bg-(--dawn-horizon) text-content-primary shadow-(--dawn-halo-soft)"
            : cn("border-transparent", INK[state][kind]),
        )}
      >
        <span className="truncate">{skill.name}</span>
        <SkillMark
          ref={markRef}
          claimed={skill.claimed}
          style={timing}
          className={lit ? "text-(--dawn-ember)" : "text-content-muted"}
        />
      </span>
      {!skill.claimed && <Provenance skill={skill} />}
    </motion.li>
  );
}

function Heading({ id, title, count }: { id: string; title: string; count: number }) {
  return (
    <h2 id={id} className="flex items-baseline gap-2 pl-2.5 font-display text-[1.125rem] leading-6 font-normal tracking-[-0.01em] text-content-primary">
      {title}
      <span className="font-sans text-sm text-content-muted tabular-nums">{count}</span>
    </h2>
  );
}

function Rows({
  skills,
  offset,
  lit,
  anyLit,
  registerSkill,
}: {
  skills: readonly PersonSkill[];
  offset: number;
  lit: ReadonlyMap<string, number>;
  anyLit: boolean;
  registerSkill: (slug: string) => (el: HTMLElement | null) => void;
}) {
  if (skills.length === 0) return null;
  return (
    <ul className={cn(styles.skillList, "mt-2")}>
      {skills.map((s, i) => (
        <SkillRow
          key={s.slug}
          skill={s}
          order={offset + i}
          state={lit.has(s.slug) ? "lit" : anyLit ? "dim" : "rest"}
          arrival={lit.get(s.slug) ?? 0}
          markRef={registerSkill(s.slug)}
        />
      ))}
    </ul>
  );
}

/*
 * Inherited skills no role here draws on. No line can reach them, so they
 * leave the one-row-per-skill list and wrap below it: the rows a line can
 * land on stay few enough to fit the viewport, and these may scroll.
 */
function Outside({ skills, more, delay }: { skills: readonly PersonSkill[]; more: boolean; delay: number }) {
  if (skills.length === 0) return null;
  return (
    <motion.div
      className={cn(more ? "mt-4" : "mt-3", "pl-2.5")}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={drift(delay, 0.8)}
    >
      <p className="text-sm leading-[1.45] text-pretty text-content-muted">{outsideNote(skills.length, more)}</p>
      <ul className="mt-1.5 flex flex-wrap gap-x-3.5 gap-y-1 text-sm leading-snug text-content-muted">
        {skills.map((s) => (
          <li key={s.slug} className="group/skill relative">
            {s.name}
            {/* Inline, so a name that wraps keeps its marker on its last line. */}
            <SkillMark claimed={false} className="ml-1.5 align-middle" />
            <Provenance skill={s} />
          </li>
        ))}
      </ul>
    </motion.div>
  );
}

export function SkillColumn({
  skills,
  reached,
  lit,
  hasActive,
  columnRef,
  registerSkill,
}: {
  skills: readonly PersonSkill[];
  /** Slugs some recommendation draws on: every skill a line can land on. */
  reached: ReadonlySet<string>;
  /** Lit skills, each with the seconds until its thread arrives. */
  lit: ReadonlyMap<string, number>;
  /** A role holds the light, even one that lights nothing. */
  hasActive: boolean;
  columnRef: Ref<HTMLElement>;
  registerSkill: (slug: string) => (el: HTMLElement | null) => void;
}) {
  const { claimed, inherited } = splitSkills(skills);
  const linked = inherited.filter((s) => reached.has(s.slug));
  const outside = inherited.filter((s) => !reached.has(s.slug));
  // Only rows a line can land on share the viewport; see `.column`.
  const style = { "--dawn-rows": Math.max(claimed.length + linked.length, 1) } as CSSProperties;
  const fade = (n: number) => ({ initial: { opacity: 0 }, animate: { opacity: 1 }, transition: drift(ENTRANCE.skills + n * ENTRANCE.skillStep, 0.8) });

  return (
    <aside ref={columnRef} aria-label="Your skills" style={style} className={cn(styles.column, "pb-6 @min-[60rem]/dawn:pt-14")}>
      {skills.length === 0 ? (
        <p className="pl-2.5 text-sm text-content-muted">No skills to show yet. Name a few on your profile and they appear here.</p>
      ) : (
        <>
          {claimed.length > 0 && (
            <section aria-labelledby="dawn-named">
              <motion.div {...fade(0)}>
                <Heading id="dawn-named" title="Skills you named" count={claimed.length} />
              </motion.div>
              <Rows skills={claimed} offset={1} lit={lit} anyLit={hasActive} registerSkill={registerSkill} />
            </section>
          )}
          {inherited.length > 0 && (
            <section aria-labelledby="dawn-inherited" className={claimed.length > 0 ? "mt-6" : undefined}>
              <motion.div {...fade(claimed.length + 1)}>
                <Heading id="dawn-inherited" title="Come with roles you've held" count={inherited.length} />
                <p className="mt-1 pl-2.5 text-sm leading-[1.45] text-pretty text-content-muted">
                  Common in postings for your past titles. Read from the market, not something you said.
                </p>
              </motion.div>
              <Rows skills={linked} offset={claimed.length + 2} lit={lit} anyLit={hasActive} registerSkill={registerSkill} />
              <Outside
                skills={outside}
                more={linked.length > 0}
                delay={ENTRANCE.skills + (claimed.length + linked.length + 3) * ENTRANCE.skillStep}
              />
            </section>
          )}
        </>
      )}
    </aside>
  );
}
