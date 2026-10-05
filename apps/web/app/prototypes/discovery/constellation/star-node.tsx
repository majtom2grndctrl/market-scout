"use client";

import { AnimatePresence, motion } from "motion/react";

import type { Recommendation } from "../_data/query";
import { EASE_OUT, settleDelay, starDelay, useTempo } from "./choreography";
import { ORBIT_NAME, postings } from "./copy";
import { type Star, TYPE } from "./geometry";
import type { StarLabel } from "./labels";

export type Emphasis = "lit" | "normal" | "dim";

interface Props {
  star: Star;
  label: StarLabel | undefined;
  rec: Recommendation;
  order: number;
  total: number;
  pinned: boolean;
  emphasis: Emphasis;
  entered: boolean;
  onFocus: () => void;
  onTogglePin: () => void;
}

/** One role: its mark and its name, as a single pin-able control. */
export function StarNode({ star, label, rec, order, total, pinned, emphasis, entered, onFocus, onTogglePin }: Props) {
  const k = useTempo();
  const ignite = k * starDelay(order);
  const settle = entered ? 0 : k * settleDelay(total);
  const lit = emphasis === "lit";

  return (
    <g
      role="button"
      tabIndex={0}
      aria-pressed={pinned}
      aria-label={`${rec.headline}, ${ORBIT_NAME[rec.strength].toLowerCase()} orbit, rank ${rec.rank}. ${pinned ? "Pinned. Press Enter to unpin." : "Press Enter to pin."}`}
      className="cursor-pointer outline-none"
      onMouseEnter={onFocus}
      onFocus={onFocus}
      onClick={onTogglePin}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onTogglePin();
        }
      }}
      style={{ opacity: emphasis === "dim" ? 0.36 : 1, transition: "opacity 220ms ease" }}
    >
      <g transform={`translate(${star.x},${star.y})`}>
        {/* Generous hit area: a 3px star is not a target. */}
        <circle r={16} fill="transparent" />
        <motion.g
          initial={{ scale: 0, opacity: 0 }}
          animate={{ scale: 1, opacity: 1 }}
          transition={{ delay: ignite, type: "spring", stiffness: 380, damping: 16 }}
        >
          <circle r={star.r * 3.4} fill="url(#constellation-halo)" opacity={pinned ? 0.75 : HALO[rec.strength]} />
          <AnimatePresence initial={false}>
            {lit && (
              <motion.circle
                key="ring"
                r={star.r + 6}
                fill="none"
                stroke="var(--constellation-star)"
                strokeWidth={1}
                initial={{ scale: 0.4, opacity: 0 }}
                animate={{ scale: 1, opacity: 0.9 }}
                exit={{ scale: 1.3, opacity: 0 }}
                transition={{ duration: 0.28, ease: EASE_OUT }}
              />
            )}
          </AnimatePresence>
          {pinned ? (
            <PinnedMark r={star.r} delay={settle} />
          ) : (
            <circle r={star.r} fill="var(--constellation-star)" />
          )}
        </motion.g>
      </g>

      {label && (
        <motion.g
          initial={{ opacity: 0, x: label.dir.x * -6, y: label.dir.y * -6 }}
          animate={{ opacity: 1, x: 0, y: 0 }}
          transition={{ delay: ignite + 0.12, duration: 0.5, ease: EASE_OUT }}
        >
          {label.leader && (
            <line
              x1={label.leader[0].x}
              y1={label.leader[0].y}
              x2={label.leader[1].x}
              y2={label.leader[1].y}
              stroke="var(--constellation-orbit)"
              strokeOpacity={0.5}
              strokeWidth={0.75}
            />
          )}
          <LabelText label={label} meta={`${pinned ? "Pinned · " : ""}${postings(rec.openPostings)}`} pinned={pinned} lit={lit} />
        </motion.g>
      )}
    </g>
  );
}

const HALO = { close: 0.55, adjacent: 0.38, stretch: 0.26 } as const;

function LabelText({ label, meta, pinned, lit }: { label: StarLabel; meta: string; pinned: boolean; lit: boolean }) {
  const x = label.anchor === "start" ? label.box.x : label.anchor === "end" ? label.box.x + label.box.w : label.box.x + label.box.w / 2;
  const lh = TYPE.headline.lineHeight;
  const metaY = label.box.y + label.lines.length * lh + 3 + TYPE.meta.size;
  return (
    <text textAnchor={label.anchor} className="select-none">
      {label.lines.map((line, i) => (
        <tspan
          key={i}
          x={x}
          y={label.box.y + i * lh + TYPE.headline.size * 0.86}
          className="font-display fill-content-primary"
          fontSize={TYPE.headline.size}
          fontWeight={lit ? 600 : TYPE.headline.weight}
          style={{ transition: "font-weight 200ms" }}
        >
          {line}
        </tspan>
      ))}
      <tspan
        x={x}
        y={metaY}
        fontSize={TYPE.meta.size}
        fontWeight={TYPE.meta.weight}
        letterSpacing={`${TYPE.meta.tracking}em`}
        className={pinned ? "" : "fill-content-muted"}
        fill={pinned ? "var(--constellation-pin)" : undefined}
        style={{ fontVariantNumeric: "tabular-nums" }}
      >
        {meta}
      </tspan>
    </text>
  );
}

// A four-point star: the settled state. Distinct from every unpinned dot by
// shape as well as colour, so it survives greyscale.
function PinnedMark({ r, delay }: { r: number; delay: number }) {
  const R = Math.max(7, r * 1.9);
  const k = R * 0.2;
  const d = `M0,${-R} Q${k},${-k} ${R},0 Q${k},${k} 0,${R} Q${-k},${k} ${-R},0 Q${-k},${-k} 0,${-R} Z`;
  return (
    <motion.g
      initial={{ scale: 2.4, opacity: 0, rotate: -45 }}
      animate={{ scale: 1, opacity: 1, rotate: 0 }}
      exit={{ scale: 0.4, opacity: 0 }}
      transition={{ delay, type: "spring", stiffness: 260, damping: 18 }}
    >
      <circle r={R + 3.5} fill="none" stroke="var(--constellation-pin)" strokeOpacity={0.55} strokeWidth={0.8} />
      <path d={d} fill="var(--constellation-pin)" />
    </motion.g>
  );
}
