"use client";

// The opening statement, set from the person's own record. Their past arrives
// light, each title marked in the plate colour its roles will wear. Then the
// page turns heavy for what comes next, and the roles follow as an index in
// rank order, each marked with the colour of the past it grows from.
//
// One orchestrated sequence, in reading order: past titles one by one, then
// the skills they named, then the turn, then the roles.

import { motion, useReducedMotion } from "motion/react";
import { Fragment, useRef, type CSSProperties } from "react";

import { article, joinList, numberWord, plural } from "./copy";
import { FullStop } from "./fit-headline";
import { EASE_OUT, EASE_SWELL, useSeen } from "./motion";
import { plateOf, type Plates } from "./palette";
import type { MarqueeData } from "./types";

const PAST_STEP = 0.32;

interface OpeningProps {
  readonly data: MarqueeData;
  readonly plates: Plates;
  readonly pinned: ReadonlySet<string>;
}

export function Opening({ data, plates, pinned }: OpeningProps) {
  const ref = useRef<HTMLElement>(null);
  const seen = useSeen(ref, "0px");
  const reduce = useReducedMotion();
  const t = (s: number) => (reduce ? 0 : s);

  const past = [...new Set(data.pastRoles.map((p) => p.titleText))];
  const skills = [...new Set(data.claimedSkills.map((c) => c.text))];
  const roles = data.recommendations;
  const { coverage } = data;

  const skillsAt = 0.25 + past.length * PAST_STEP;
  const turnAt = skillsAt + (skills.length > 0 ? 0.45 : 0);
  const indexAt = turnAt + 0.55;

  const fade = (delay: number) => ({
    initial: { opacity: 0 },
    animate: seen ? { opacity: 1 } : undefined,
    transition: { duration: t(0.9), ease: EASE_OUT, delay: t(delay) },
  });

  return (
    <section
      ref={ref}
      aria-label="Where you've been, and where it leads"
      className="mx-auto max-w-[96rem] px-5 pt-[clamp(3rem,8cqi,7rem)] pb-[clamp(4rem,9cqi,8rem)] [container-type:inline-size] sm:px-10"
    >
      {past.length > 0 && (
        <h1 className="font-display text-[clamp(2.5rem,7.4cqi,8.25rem)] leading-[0.98] font-light tracking-[-0.035em] text-balance">
          <motion.span {...fade(0)}>You&rsquo;ve been </motion.span>
          {past.map((title, i) => (
            <Fragment key={title}>
              <motion.span {...fade(0.25 + i * PAST_STEP)}>
                {article(title)}&nbsp;
                <span
                  className="marquee-band"
                  data-on={seen}
                  style={
                    {
                      "--marquee-plate": plateOf(plates, title),
                      "--marquee-delay": `${t(0.45 + i * PAST_STEP)}s`,
                    } as CSSProperties
                  }
                >
                  {title}
                </span>
              </motion.span>
              <motion.span {...fade(0.25 + i * PAST_STEP)}>
                {i < past.length - 2 ? ", " : i === past.length - 2 ? " and " : "."}
              </motion.span>
            </Fragment>
          ))}
        </h1>
      )}

      {skills.length > 0 && (
        <motion.p
          {...fade(skillsAt)}
          className="mt-[0.7em] max-w-[34ch] font-display text-[clamp(1.5rem,3.3cqi,3.5rem)] leading-[1.06] font-light tracking-[-0.025em] text-pretty text-content-secondary"
        >
          {past.length > 0 ? "You also know " : "You know "}
          {joinList(skills)}.
        </motion.p>
      )}

      <motion.p
        className="mt-[clamp(3rem,8cqi,7rem)] font-display text-[clamp(2.25rem,6.2cqi,7rem)] leading-[0.95] tracking-[-0.03em] text-balance"
        initial={{ opacity: 0, "--marquee-wght": 300 }}
        animate={seen ? { opacity: 1, "--marquee-wght": 800 } : undefined}
        transition={{
          opacity: { duration: t(0.6), delay: t(turnAt) },
          "--marquee-wght": { duration: t(1.3), ease: EASE_SWELL, delay: t(turnAt + 0.1) },
        }}
        style={{ fontWeight: "var(--marquee-wght)" }}
      >
        {roles.length === 0
          ? "No roles are ranked for this profile yet."
          : `${numberWord(roles.length, true)} ${plural(roles.length, "role", "roles")} in open postings ${plural(roles.length, "draws", "draw")} on that.`}
      </motion.p>

      {roles.length > 0 && (
        <ol className="mt-[0.55em] font-display text-[clamp(1.75rem,4.4cqi,4.75rem)] leading-[1.08] tracking-[-0.03em] text-pretty">
          {roles.map((r, i) => (
            <li key={r.roleSlug} className="mr-[0.42em] inline">
              <motion.a
                href={`#role-${r.roleSlug}`}
                className="marquee-band rounded-[0.08em] outline-offset-4 focus-visible:outline-2 focus-visible:outline-focus"
                data-on={seen}
                style={
                  {
                    fontWeight: "var(--marquee-wght)",
                    "--marquee-plate": plateOf(plates, r.closestPast?.titleText),
                    "--marquee-delay": `${t(indexAt + 0.25 + i * 0.07)}s`,
                  } as CSSProperties
                }
                initial={{ opacity: 0, "--marquee-wght": 300 }}
                animate={seen ? { opacity: 1, "--marquee-wght": 700 } : undefined}
                transition={{
                  opacity: { duration: t(0.5), delay: t(indexAt + i * 0.07) },
                  "--marquee-wght": { duration: t(1.1), ease: EASE_SWELL, delay: t(indexAt + i * 0.07) },
                }}
              >
                <sup className="mr-[0.12em] align-[0.9em] font-sans text-[0.3em] font-medium tracking-normal text-content-secondary tabular-nums">
                  {r.rank}
                </sup>
                {r.headline}
                <FullStop on={pinned.has(r.roleSlug)} />
              </motion.a>
            </li>
          ))}
        </ol>
      )}

      <motion.p {...fade(indexAt + 0.4 + roles.length * 0.07)} className="mt-[clamp(2.5rem,5cqi,4rem)] max-w-[62ch] font-sans text-sm leading-relaxed text-content-secondary">
        Ranked by how closely each role&rsquo;s skills match{" "}
        {past.length > 0 && `the ${numberWord(past.length)} ${plural(past.length, "role", "roles")} you've held`}
        {past.length > 0 && skills.length > 0 && " and "}
        {skills.length > 0 && `the ${numberWord(skills.length)} ${plural(skills.length, "skill", "skills")} you named`}
        {past.length === 0 && skills.length === 0 && "your profile"}, read from{" "}
        {coverage.classifiedPostings.toLocaleString()} classified of {coverage.openPostings.toLocaleString()} open
        postings across {coverage.rolesConsidered.toLocaleString()} roles.
        {past.length > 0 && " Each highlight matches the past role it sits closest to."} A full stop marks a role
        you&rsquo;ve pinned.
      </motion.p>
    </section>
  );
}
