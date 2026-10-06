"use client";

import { MotionConfig, useReducedMotion } from "motion/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { DiscoveryData, Recommendation } from "../../_data/query";
import { reachedSlugs } from "../_lib/copy";
import { effectiveLineMode, type LineMode } from "../_lib/modes";
import { ENTRANCE } from "../_lib/motion";
import { useThreads } from "../_lib/use-threads";
import styles from "../threads.module.css";
import { Connectors } from "./connectors";
import { Intro } from "./intro";
import { Pieces } from "./pieces";
import { RoleCard } from "./role-card";
import { SkillColumn } from "./skill-column";
import { cn } from "@/lib/utils";

const NO_LIGHT: ReadonlyMap<string, number> = new Map();

export function ThreadsDiscovery({ data, lines }: { data: DiscoveryData; lines: LineMode }) {
  const recs = data.recommendations;
  const reduce = useReducedMotion() ?? false;

  // The server renders the requested mode; the browser falls back to fade
  // where it lacks scroll-driven animations. No line draws before the first
  // light, so the switch is never seen.
  const [mode, setMode] = useState<LineMode>(lines);
  useEffect(() => setMode(effectiveLineMode(lines)), [lines]);

  // One role holds the light at a time. It stays with the last role hovered,
  // focused, or tapped, so the reader can move to the skills without losing it.
  const [activeSlug, setActiveSlug] = useState<string | null>(null);
  const chosen = useRef(false);
  const light = useCallback((roleSlug: string) => {
    chosen.current = true;
    setActiveSlug(roleSlug);
  }, []);

  // After the entrance, the top recommendation lights on its own, so the page
  // tells its story before anyone touches it.
  const first = recs[0]?.roleSlug ?? null;
  useEffect(() => {
    if (!first) return;
    const t = window.setTimeout(() => {
      if (!chosen.current) setActiveSlug(first);
    }, (reduce ? 0.4 : ENTRANCE.light) * 1000);
    return () => window.clearTimeout(t);
  }, [first, reduce]);

  const active = useMemo(() => recs.find((r) => r.roleSlug === activeSlug) ?? null, [recs, activeSlug]);
  const { light: lighting, threads, bind, isScrolling, containerRef, columnRef, cardRef, barRef, registerSkill, registerRole } = useThreads(active, reduce, mode);

  // Pins are local by contract: seeded from the profile, never written back.
  const [order, setOrder] = useState<string[]>(() => recs.filter((r) => r.pinned).map((r) => r.roleSlug));
  const [acknowledged, setAcknowledged] = useState(false);
  const toggle = useCallback((roleSlug: string) => {
    setAcknowledged(false);
    setOrder((o) => (o.includes(roleSlug) ? o.filter((s) => s !== roleSlug) : [...o, roleSlug]));
  }, []);
  const pinned = useMemo(() => new Set(order), [order]);
  const pins = useMemo(
    () => order.flatMap((s): Recommendation[] => recs.filter((r) => r.roleSlug === s)),
    [order, recs],
  );

  const reached = useMemo(() => reachedSlugs(recs), [recs]);
  const skillsBySlug = useMemo(() => new Map(data.personSkills.map((s) => [s.slug, s])), [data.personSkills]);

  return (
    <MotionConfig reducedMotion="user">
      <div className={cn(styles.root, "relative isolate min-h-[calc(100svh-3.5rem)]")}>
        <div aria-hidden className={cn(styles.sky, "pointer-events-none absolute inset-x-0 top-0 -z-10 h-[36rem]")} />

        <div ref={containerRef} className={cn(styles.grid, "relative mx-auto max-w-[76rem] px-6 pb-16 md:px-10")}>
          <Intro data={data} className={cn(styles.intro, "pt-12 @min-[60rem]/threads:pt-14 @min-[60rem]/threads:pb-10")} />
          <SkillColumn
            skills={data.personSkills}
            reached={reached}
            lit={lighting?.lit ?? NO_LIGHT}
            hasActive={lighting !== null}
            columnRef={columnRef}
            registerSkill={registerSkill}
          />
          <div aria-hidden className={styles.gutter} />
          <RoleCard
            className={styles.card}
            recs={recs}
            skills={skillsBySlug}
            active={activeSlug}
            pinned={pinned}
            pins={pins}
            acknowledged={acknowledged}
            onLight={light}
            onToggle={toggle}
            onContinue={() => setAcknowledged(true)}
            isScrolling={isScrolling}
            cardRef={cardRef}
            barRef={barRef}
            registerRole={registerRole}
          />
          <Connectors store={threads} bind={bind} mode={mode} />
          {mode === "compositor" && <Pieces store={threads} bind={bind} />}
        </div>
      </div>
    </MotionConfig>
  );
}
