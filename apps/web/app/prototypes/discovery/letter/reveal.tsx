"use client";

// Reveal primitives. A beat builds one timeline and hands each phrase its cue
// in reading order, so words arrive the way a reader takes them in and the key
// fact lands last, after a held breath.

import { Fragment, type ReactNode } from "react";
import { motion } from "motion/react";

import type { Line } from "./copy";
import styles from "./letter.module.css";

export const EASE_OUT = [0.22, 1, 0.36, 1] as const;

const WORD_GAP = 0.045;
const PHRASE_GAP = 0.16;
const KEY_PAUSE = 0.38;
const WORD_DURATION = 0.75;

// Motion hands opacity and filter to the compositor (WAAPI), which headless
// Chrome's virtual clock never advances, so `--virtual-time-budget` captures
// those words half-faded at any budget. Any onUpdate keeps a value on Motion's
// own frame loop, which the virtual clock drives. The cost is main-thread
// animation for a few dozen spans.
export const mainThread = { onUpdate: () => {} };

export interface Timeline {
  /** Reserve a cue for something that arrives whole. Returns its delay. */
  cue(gap?: number): number;
  /** Reserve a cue for a run of words; the clock advances past the last one. */
  words(text: string, gap?: number): number;
}

export function timeline(start = 0.1): Timeline {
  let t = start;
  return {
    cue(gap = PHRASE_GAP) {
      t += gap;
      return t;
    },
    words(text, gap = PHRASE_GAP) {
      const at = t + gap;
      t = at + text.split(" ").length * WORD_GAP;
      return at;
    },
  };
}

// Where a line may not break: after an article, around a joining symbol
// ("Webmaster + UI"), and between a figure and its noun.
function bound(text: string): string {
  return text
    .replace(/(^|\s)(a|an|the)\s/gi, "$1$2\u00A0")
    .replace(/\s([+&/—])\s/g, "\u00A0$1\u00A0")
    .replace(/(\d[\d,.%]*)\s/g, "$1\u00A0");
}

export function Words({ text, delay }: { text: string; delay: number }) {
  const words = bound(text).split(" ");
  return words.map((word, i) => (
    <Fragment key={i}>
      <motion.span
        className="inline-block"
        {...mainThread}
        initial={{ opacity: 0, y: "0.38em", filter: "blur(6px)" }}
        animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
        transition={{ duration: WORD_DURATION, ease: EASE_OUT, delay: delay + i * WORD_GAP }}
      >
        {word}
      </motion.span>
      {i < words.length - 1 ? " " : null}
    </Fragment>
  ));
}

/**
 * A line of phrases. The key phrase waits a beat longer, takes the accent,
 * and an underline draws beneath it once its words have settled.
 *
 * A plain function, not a component: it takes its cues from the timeline when
 * called, so call order in the beat is reading order. A component would take
 * them later, at render, after the beat's own cues.
 */
export function phrases(line: Line, tl: Timeline, { first = PHRASE_GAP, note }: { first?: number; note?: ReactNode } = {}): ReactNode {
  return line.map((phrase, i) => {
    const gap = i === 0 ? first : phrase.key ? KEY_PAUSE : PHRASE_GAP;
    const delay = tl.words(phrase.text, gap);
    const words = <Words text={phrase.text} delay={delay} />;
    const trailing = phrase.note ? (
      <motion.span {...mainThread} initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.5, delay: delay + 0.4 }}>
        {note}
      </motion.span>
    ) : null;
    return (
      <Fragment key={i}>
        {i > 0 ? " " : null}
        {phrase.key ? (
          <motion.span
            className={styles.key}
            initial={{ backgroundSize: "0% 0.06em" }}
            animate={{ backgroundSize: "100% 0.06em" }}
            transition={{ duration: 0.9, ease: [0.65, 0, 0.35, 1], delay: delay + WORD_DURATION * 0.8 }}
          >
            {words}
          </motion.span>
        ) : (
          words
        )}
        {trailing}
      </Fragment>
    );
  });
}

/** Arrives whole: a fade and a short rise. For labels, controls, and asides. */
export function Arrive({ delay, children, className, as = "div" }: { delay: number; children: ReactNode; className?: string; as?: "div" | "p" | "span" | "aside" }) {
  const Tag = motion[as];
  return (
    <Tag
      className={className}
      {...mainThread}
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.7, ease: EASE_OUT, delay }}
    >
      {children}
    </Tag>
  );
}
