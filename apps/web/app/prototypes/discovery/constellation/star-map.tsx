"use client";

import { AnimatePresence, motion } from "motion/react";
import { useMemo } from "react";

import type { Recommendation } from "../_data/query";
import { ENTER, EASE_OUT, pathDelay, settleDelay, useTempo } from "./choreography";
import { ORBIT_NAME } from "./copy";
import { type Layout, type Star, TYPE } from "./geometry";
import type { OrbitLabel, StarLabel } from "./labels";
import { type Emphasis, StarNode } from "./star-node";

export type Focus = { kind: "role"; slug: string } | { kind: "group"; key: string } | null;

interface Props {
  layout: Layout;
  labels: Map<string, StarLabel>;
  orbitLabels: Map<string, OrbitLabel>;
  recs: Map<string, Recommendation>;
  pinned: ReadonlySet<string>;
  focus: Focus;
  entered: boolean;
  onFocus: (focus: Focus) => void;
  onTogglePin: (slug: string) => void;
}

export function StarMap({ layout, labels, orbitLabels, recs, pinned, focus, entered, onFocus, onTogglePin }: Props) {
  const { width, height, cx, cy, rx, ry, plaque, stars } = layout;
  const k = useTempo();
  const ordered = useMemo(() => [...stars].sort((a, b) => a.rank - b.rank), [stars]);
  const dust = useMemo(() => starDust(width, height), [width, height]);
  const focusGroup = focus?.kind === "group" ? focus.key : focus?.kind === "role" ? stars.find((s) => s.slug === focus.slug)?.groupKey : undefined;

  const emphasisOf = (s: Star): Emphasis => {
    if (!focus) return "normal";
    if (focus.kind === "role") return s.slug === focus.slug ? "lit" : "dim";
    return s.groupKey === focus.key ? "lit" : "dim";
  };

  // Which side of each home line has a path leaving it.
  const exitsUsed = new Set(stars.map((s) => `${s.groupKey}|${s.x >= cx ? "r" : "l"}`));

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className="block"
      role="group"
      aria-label="Your past roles at the centre, recommended roles in orbit around them"
      onMouseLeave={() => focus?.kind === "group" && onFocus(null)}
    >
      <defs>
        <radialGradient id="constellation-dawn">
          <stop offset="0%" stopColor="var(--constellation-dawn)" stopOpacity={0.5} />
          <stop offset="100%" stopColor="var(--constellation-dawn)" stopOpacity={0} />
        </radialGradient>
        <radialGradient id="constellation-halo">
          <stop offset="0%" stopColor="var(--constellation-star)" stopOpacity={0.9} />
          <stop offset="35%" stopColor="var(--constellation-star)" stopOpacity={0.25} />
          <stop offset="100%" stopColor="var(--constellation-star)" stopOpacity={0} />
        </radialGradient>
      </defs>

      {/* Clicking open sky lets go of the selection. */}
      <rect width={width} height={height} fill="transparent" onClick={() => onFocus(null)} />

      {/* Dawn over the past titles. Kept to the plaque's surround so the role
          names around it always sit on night sky. */}
      <ellipse cx={cx} cy={cy} rx={width * 0.22} ry={height * 0.22} fill="url(#constellation-dawn)" pointerEvents="none" aria-hidden />

      <motion.g initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: k * 1.2 }} aria-hidden>
        {dust.map((d, i) => (
          <circle key={i} cx={d.x} cy={d.y} r={d.r} fill="var(--constellation-star)" opacity={d.o} />
        ))}
      </motion.g>

      <g aria-hidden>
        {layout.orbits.map((o, i) => {
          const at = orbitLabels.get(o.strength);
          return (
            <g key={o.strength}>
              <motion.ellipse
                cx={cx}
                cy={cy}
                rx={rx * o.rho}
                ry={ry * o.rho}
                fill="none"
                stroke="var(--constellation-orbit)"
                strokeOpacity={0.3}
                strokeWidth={0.75}
                initial={{ pathLength: 0, opacity: 0 }}
                animate={{ pathLength: 1, opacity: 1 }}
                transition={{ delay: k * (ENTER.orbits + i * 0.12), duration: k * 1.1, ease: EASE_OUT }}
              />
              {at && (
                <motion.text
                  x={at.x}
                  y={at.y}
                  textAnchor="middle"
                  fontSize={TYPE.orbit.size}
                  fontWeight={TYPE.orbit.weight}
                  letterSpacing={`${TYPE.orbit.tracking}em`}
                  fill="var(--constellation-orbit-ink)"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  transition={{ delay: k * (ENTER.orbits + 0.5 + i * 0.12), duration: k * 0.6 }}
                >
                  {ORBIT_NAME[o.strength]}
                </motion.text>
              )}
            </g>
          );
        })}
      </g>

      <g aria-hidden fill="none" strokeLinecap="round">
        {ordered.map((s, i) => {
          const e = emphasisOf(s);
          const isPinned = pinned.has(s.slug);
          return (
            <g key={s.slug} style={{ opacity: e === "dim" ? 0.14 : 1, transition: "opacity 220ms ease" }}>
              {e === "lit" && <path d={s.path} stroke="var(--constellation-pin)" strokeOpacity={0.18} strokeWidth={6} />}
              <motion.path
                d={s.path}
                stroke={e === "lit" ? "var(--constellation-pin)" : "var(--constellation-path)"}
                strokeOpacity={e === "lit" ? 1 : PATH_OPACITY[s.strength]}
                strokeWidth={e === "lit" ? 1.6 : 1}
                style={{ transition: "stroke 220ms, stroke-opacity 220ms, stroke-width 220ms" }}
                initial={{ pathLength: 0 }}
                animate={{ pathLength: 1 }}
                transition={{ delay: k * pathDelay(i), duration: k * ENTER.pathDuration, ease: EASE_OUT }}
              />
              <AnimatePresence>
                {isPinned && (
                  // A pin draws the line in: the path becomes part of the figure.
                  <motion.path
                    key="pin"
                    d={s.path}
                    stroke="var(--constellation-pin)"
                    strokeWidth={1.3}
                    strokeOpacity={0.9}
                    initial={{ pathLength: 0 }}
                    animate={{ pathLength: 1 }}
                    exit={{ opacity: 0, transition: { duration: 0.3 } }}
                    transition={{ delay: entered ? 0 : k * settleDelay(stars.length), duration: k * 0.7, ease: EASE_OUT }}
                  />
                )}
              </AnimatePresence>
            </g>
          );
        })}
      </g>

      <motion.g
        initial={{ opacity: 0, scale: 0.94 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ delay: k * ENTER.home, duration: k * 0.9, ease: EASE_OUT }}
        style={{ originX: `${cx}px`, originY: `${cy}px` }}
      >
        {plaque.lines.map((line) => {
          const quiet = line.members === 0;
          const on = focusGroup === undefined || focusGroup === line.key;
          // A title no role builds on, or one outside the focus, recedes by
          // ink; only its exit marks fade.
          const bright = on && !quiet;
          return (
            <g
              key={line.key}
              onMouseEnter={line.members > 0 ? () => onFocus({ kind: "group", key: line.key }) : undefined}
              className={line.members > 0 ? "cursor-default" : undefined}
            >
              <text
                textAnchor="middle"
                className="font-display"
                fontSize={TYPE.home.size}
                fontWeight={TYPE.home.weight}
                fontStyle={line.whole ? "italic" : undefined}
                fill={bright ? "var(--constellation-home)" : "var(--constellation-home-recede)"}
                style={{ transition: "fill 220ms ease" }}
              >
                {line.lines.map((l, i) => (
                  <tspan key={i} x={cx} y={line.y - (line.lines.length * TYPE.home.lineHeight) / 2 + i * TYPE.home.lineHeight + TYPE.home.size * 0.88}>
                    {l}
                  </tspan>
                ))}
              </text>
              {(["l", "r"] as const).map((side) =>
                exitsUsed.has(`${line.key}|${side}`) ? (
                  <circle
                    key={side}
                    cx={side === "l" ? line.exits.left.x : line.exits.right.x}
                    cy={line.y}
                    r={2.4}
                    fill="var(--constellation-pin)"
                    style={{ opacity: on ? 1 : 0.3, transition: "opacity 220ms ease" }}
                  />
                ) : null,
              )}
            </g>
          );
        })}
      </motion.g>

      {ordered.map((s, i) => {
        const rec = recs.get(s.slug);
        if (!rec) return null;
        return (
          <StarNode
            key={s.slug}
            star={s}
            label={labels.get(s.slug)}
            rec={rec}
            order={i}
            total={stars.length}
            pinned={pinned.has(s.slug)}
            emphasis={emphasisOf(s)}
            entered={entered}
            onFocus={() => onFocus({ kind: "role", slug: s.slug })}
            onTogglePin={() => onTogglePin(s.slug)}
          />
        );
      })}
    </svg>
  );
}

const PATH_OPACITY = { close: 0.62, adjacent: 0.45, stretch: 0.32 } as const;

// Fixed seed: the field is texture, and it must not reshuffle on resize.
function starDust(width: number, height: number) {
  let seed = 0x5eed;
  const rand = () => {
    seed = (seed * 1664525 + 1013904223) % 4294967296;
    return seed / 4294967296;
  };
  const count = Math.round((width * height) / 4200);
  return Array.from({ length: count }, () => ({
    x: rand() * width,
    y: rand() * height,
    r: 0.35 + rand() ** 3 * 0.9,
    o: 0.12 + rand() * 0.38,
  }));
}
