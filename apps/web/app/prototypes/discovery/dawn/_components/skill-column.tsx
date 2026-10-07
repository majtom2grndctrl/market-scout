"use client";

import { motion } from "motion/react";
import type { CSSProperties, ReactNode, Ref } from "react";

import type { PersonSkill } from "../../_data/query";
import { inheritedNote, plural } from "../_lib/copy";
import { drift, ENTRANCE } from "../_lib/motion";
import type { ColumnModel, SkillRowModel } from "../_lib/skills";
import styles from "../dawn.module.css";
import { SkillMark } from "./skill-mark";
import { cn } from "@/lib/utils";

/**
 * rest: no role lit yet. lit: the active role draws on it. dim: a role is
 * lit, and not through this skill. idle: no listed role draws on it, so it
 * rests muted whatever is lit.
 */
type State = "rest" | "lit" | "dim" | "idle";

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
  idle: { claimed: "text-content-muted", inherited: "text-content-muted" },
};

const enter = (order: number) => ({
  initial: { opacity: 0, x: -8 },
  animate: { opacity: 1, x: 0 },
  transition: drift(ENTRANCE.skills + order * ENTRANCE.skillStep, 0.8),
});

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

/*
 * The reach count sits between the name and the marker, so nothing lies
 * between the marker and the gutter, where lines arrive (Invariant 12).
 */
function SkillRow({
  row,
  order,
  state,
  arrival,
  markRef,
}: {
  row: SkillRowModel;
  order: number;
  state: State;
  arrival: number;
  markRef: (el: HTMLElement | null) => void;
}) {
  const { skill, reach } = row;
  const lit = state === "lit";
  const kind = skill.claimed ? "claimed" : "inherited";
  // Light on arrival of the line, so the eye lands with it; let go at once.
  const timing: CSSProperties = { transitionDelay: lit ? `${Math.round(arrival * 1000)}ms` : "0ms" };

  return (
    <motion.li className={cn(styles.skillRow, "group/skill relative min-w-0")} {...enter(order)}>
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
        {reach > 0 && (
          <span
            style={timing}
            className={cn(
              "shrink-0 font-normal whitespace-nowrap tabular-nums transition-colors duration-500 ease-out",
              lit ? "text-content-secondary" : "text-content-muted",
            )}
          >
            <span className="sr-only">, used by </span>
            {reach} {plural(reach, "role")}
          </span>
        )}
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

/** A claimed skill no posting skill matches. It can never light, so it takes no marker a line could aim at. */
function UnmatchedRow({ text, order }: { text: string; order: number }) {
  return (
    <motion.li className={cn(styles.skillRow, "relative min-w-0")} {...enter(order)}>
      <span className={cn(styles.pill, "inline-flex max-w-full min-w-0 items-center gap-2 rounded-full border border-transparent py-[0.1875rem] pr-2 pl-2.5 text-sm leading-[1.2] font-medium text-content-muted")}>
        <span className="truncate">{text}</span>
        <SkillMark claimed className="text-content-muted" />
      </span>
    </motion.li>
  );
}

function Heading({ id, children }: { id: string; children: ReactNode }) {
  return (
    <h2 id={id} className="pl-2.5 font-display text-[1.125rem] leading-6 font-normal tracking-[-0.01em] text-content-primary">
      {children}
    </h2>
  );
}

function Note({ children, className }: { children: ReactNode; className?: string }) {
  return <p className={cn("pl-2.5 text-sm leading-[1.45] text-pretty text-content-muted", className)}>{children}</p>;
}

export function SkillColumn({
  model,
  lit,
  hasActive,
  columnRef,
  registerSkill,
}: {
  model: ColumnModel;
  /** Lit skills, each with the seconds until its thread arrives. */
  lit: ReadonlyMap<string, number>;
  /** A role holds the light, even one that lights nothing. */
  hasActive: boolean;
  columnRef: Ref<HTMLElement>;
  registerSkill: (slug: string) => (el: HTMLElement | null) => void;
}) {
  const { added, unmatched, common } = model;
  const stateOf = (r: SkillRowModel): State => (lit.has(r.skill.slug) ? "lit" : r.reach === 0 ? "idle" : hasActive ? "dim" : "rest");
  const row = (r: SkillRowModel, order: number) => (
    <SkillRow key={r.skill.slug} row={r} order={order} state={stateOf(r)} arrival={lit.get(r.skill.slug) ?? 0} markRef={registerSkill(r.skill.slug)} />
  );
  const fade = (n: number) => ({ initial: { opacity: 0 }, animate: { opacity: 1 }, transition: drift(ENTRANCE.skills + n * ENTRANCE.skillStep, 0.8) });
  const hasAdded = added.length + unmatched.length > 0;
  // Entrance order runs down the column: heading, rows, note, rows.
  const unmatchedAt = added.length + 2;
  const commonAt = hasAdded ? unmatchedAt + unmatched.length + 1 : 0;

  return (
    <aside ref={columnRef} aria-label="Your skills" className={cn(styles.column, "pb-6 @min-[60rem]/dawn:pt-8")}>
      {!hasAdded && common.length === 0 ? (
        <Note>No skills yet. Add them on your profile.</Note>
      ) : (
        <>
          {hasAdded && (
            <section aria-labelledby="dawn-added">
              <motion.div {...fade(0)}>
                <Heading id="dawn-added">Skills you added</Heading>
              </motion.div>
              {added.length > 0 && <ul className={cn(styles.skillList, "mt-2")}>{added.map((r, i) => row(r, i + 1))}</ul>}
              {unmatched.length > 0 && (
                <>
                  <motion.div {...fade(unmatchedAt - 1)}>
                    <Note className={added.length > 0 ? "mt-3" : "mt-1"}>{"These don't match a skill in postings yet."}</Note>
                  </motion.div>
                  <ul className={cn(styles.skillList, "mt-1.5")}>
                    {unmatched.map((t, i) => (
                      <UnmatchedRow key={t} text={t} order={unmatchedAt + i} />
                    ))}
                  </ul>
                </>
              )}
            </section>
          )}
          {common.length > 0 && (
            <section aria-labelledby="dawn-common" className={hasAdded ? "mt-6" : undefined}>
              <motion.div {...fade(commonAt)}>
                <Heading id="dawn-common">Common in your past roles</Heading>
                <Note className="mt-1">{"From postings for titles you've held."}</Note>
              </motion.div>
              <ul className={cn(styles.skillList, "mt-2")}>{common.map((r, i) => row(r, commonAt + 1 + i))}</ul>
            </section>
          )}
        </>
      )}
    </aside>
  );
}
