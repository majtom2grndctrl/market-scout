"use client";

// Poster headlines: one size per headline, chosen so the widest line fills the
// measure. CSS cannot do this — `text-wrap: balance` balances lines at a fixed
// size, but never picks the size — so the headline is measured in the real
// font and laid out by hand.
//
// Every way to break the title into one to three lines is a candidate. Each
// gets the largest size at which its widest line fits the width and the block
// fits a share of the viewport height. The fewest lines that come within reach
// of the best size win, so "Software Engineer" stacks into two huge words while
// "Business Development Representative" takes three rather than shrinking to
// one long line. Measuring at the settled weight means the lighter weights the
// headline animates through are always narrower, never overflowing.

import { motion, useReducedMotion } from "motion/react";
import { useEffect, useRef, useState, type RefObject } from "react";

import { EASE_OUT, EASE_SWELL } from "./motion";

interface Fit {
  readonly lines: readonly string[];
  readonly px: number;
}

interface FitOptions {
  readonly weight: number;
  /** em */
  readonly tracking: number;
  readonly lineHeight: number;
  readonly maxPx: number;
  /** Share of the viewport height the whole block may take. */
  readonly heightShare: number;
  readonly maxLines: number;
  /** Prefer fewer lines when their size is at least this share of the best. */
  readonly tolerance: number;
  /** Measured onto the last line, so a trailing mark never pushes past the edge. */
  readonly suffix: string;
}

// A tall review window would otherwise license a headline taller than any real screen.
const VIEWPORT_CEILING = 1000;

function splits(words: readonly string[], k: number): string[][] {
  if (k === 1) return [[words.join(" ")]];
  const out: string[][] = [];
  for (let i = 1; i <= words.length - k + 1; i++) {
    const head = words.slice(0, i).join(" ");
    for (const rest of splits(words.slice(i), k - 1)) out.push([head, ...rest]);
  }
  return out;
}

// A line of only "+", "/" or "&" reads as a stray mark, not a word.
const BARE_MARK = /^[^\p{L}\p{N}]+$/u;

function useFit(ref: RefObject<HTMLElement | null>, text: string, o: FitOptions): Fit | null {
  const [fit, setFit] = useState<Fit | null>(null);
  const { weight, tracking, lineHeight, maxPx, heightShare, maxLines, tolerance, suffix } = o;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const words = text.trim().split(/\s+/);
    const candidates: string[][] = [];
    for (let k = 1; k <= Math.min(maxLines, words.length); k++) {
      candidates.push(...splits(words, k).filter((lines) => !lines.some((l) => BARE_MARK.test(l))));
    }

    let alive = true;
    let frame = 0;
    const measure = () => {
      const width = el.clientWidth;
      if (!alive || width === 0) return;
      const vh = Math.min(window.innerHeight, VIEWPORT_CEILING);
      const probe = document.createElement("span");
      probe.className = "font-display";
      Object.assign(probe.style, {
        position: "absolute",
        visibility: "hidden",
        whiteSpace: "pre",
        fontSize: "100px",
        fontWeight: String(weight),
        letterSpacing: `${tracking}em`,
        left: "0",
        top: "0",
      });
      el.appendChild(probe);
      const cache = new Map<string, number>();
      const widthOf = (s: string) => {
        let w = cache.get(s);
        if (w === undefined) {
          probe.textContent = s;
          w = probe.getBoundingClientRect().width;
          cache.set(s, w);
        }
        return w;
      };
      const sized = candidates.map((lines) => {
        const widest = Math.max(...lines.map((l, i) => widthOf(i === lines.length - 1 ? l + suffix : l)));
        const px = Math.min(maxPx, ((width * 0.985) / widest) * 100, (vh * heightShare) / (lines.length * lineHeight));
        return { lines, px };
      });
      probe.remove();
      const best = Math.max(...sized.map((s) => s.px));
      const pick = sized
        .filter((s) => s.px >= best * tolerance)
        .sort((a, b) => a.lines.length - b.lines.length || b.px - a.px)[0];
      if (!pick) return;
      const px = Math.floor(pick.px);
      setFit((prev) =>
        prev && prev.px === px && prev.lines.join("\n") === pick.lines.join("\n") ? prev : { lines: pick.lines, px },
      );
    };

    document.fonts.load(`${weight} 100px "Funnel Display Variable"`).then(measure, measure);
    const ro = new ResizeObserver(() => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measure);
    });
    ro.observe(el);
    return () => {
      alive = false;
      ro.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [ref, text, weight, tracking, lineHeight, maxPx, heightShare, maxLines, tolerance, suffix]);

  return fit;
}

const HEADLINE: FitOptions = {
  weight: 800,
  tracking: -0.035,
  lineHeight: 0.86,
  maxPx: 232,
  heightShare: 0.46,
  maxLines: 3,
  tolerance: 0.8,
  suffix: ".",
};

interface HeadlineProps {
  readonly text: string;
  readonly seen: boolean;
  /** Pinned roles end in a full stop: settled, kept. */
  readonly pinned: boolean;
  readonly delay?: number;
}

export function Headline({ text, seen, pinned, delay = 0 }: HeadlineProps) {
  const ref = useRef<HTMLDivElement>(null);
  const fit = useFit(ref, text, HEADLINE);
  const reduce = useReducedMotion();
  const go = seen && fit !== null;

  return (
    <div ref={ref} className="relative">
      <h2
        className="font-display text-content-primary"
        style={{
          fontSize: fit ? `${fit.px}px` : "clamp(2.5rem, 10cqi, 12rem)",
          lineHeight: HEADLINE.lineHeight,
          letterSpacing: `${HEADLINE.tracking}em`,
        }}
      >
        {fit ? (
          fit.lines.map((line, i) => (
            // The clip lets each line rise out of its own baseline. Its bottom
            // edge sits below the descenders so a settled "g" or "p" is never cut.
            <span key={i} className="block [clip-path:inset(-0.4em_-0.3em_-0.24em_-0.3em)]">
              <motion.span
                className="inline-block whitespace-nowrap"
                style={{ fontWeight: "var(--marquee-wght)" }}
                initial={{ y: "140%", "--marquee-wght": 300 }}
                animate={go ? { y: "0%", "--marquee-wght": HEADLINE.weight } : undefined}
                transition={{
                  y: { duration: reduce ? 0 : 0.95, ease: EASE_OUT, delay: delay + i * 0.09 },
                  "--marquee-wght": { duration: reduce ? 0 : 1.5, ease: EASE_SWELL, delay: delay + 0.12 + i * 0.09 },
                }}
              >
                {line}
                {i === fit.lines.length - 1 && <FullStop on={pinned} />}
              </motion.span>
            </span>
          ))
        ) : (
          <span className="block text-balance font-extrabold">{text}</span>
        )}
      </h2>
    </div>
  );
}

export function FullStop({ on }: { readonly on: boolean }) {
  return (
    <motion.span
      aria-hidden
      className="inline-block"
      style={{ originX: 0.2, originY: 0.85 }}
      initial={false}
      animate={{ opacity: on ? 1 : 0, scale: on ? 1 : 0 }}
      transition={{ type: "spring", stiffness: 520, damping: 17 }}
    >
      .
    </motion.span>
  );
}
