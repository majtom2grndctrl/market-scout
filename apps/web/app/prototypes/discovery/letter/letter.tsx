"use client";

import { useCallback, useEffect, useRef, useState, type TouchEvent, type WheelEvent } from "react";
import { AnimatePresence, MotionConfig, motion } from "motion/react";

import { cn } from "@/lib/utils";

import { AllBeatView } from "./all-beat";
import type { Beat } from "./copy";
import { MarginBar } from "./margin-bar";
import { ProseBeatView } from "./prose-beat";
import { mainThread } from "./reveal";
import { RoleBeatView } from "./role-beat";
import styles from "./letter.module.css";

// A wheel flick or trackpad swipe moves one beat. Momentum keeps firing wheel
// events long after the gesture, so after a move the stage stays deaf until
// the events go quiet.
const WHEEL_THRESHOLD = 40;
const WHEEL_SETTLE_MS = 160;
const MOVE_HOLD_MS = 700;
const SWIPE_THRESHOLD = 56;

const FORWARD = new Set(["ArrowRight", "ArrowDown", "PageDown", " ", "Enter"]);
const BACKWARD = new Set(["ArrowLeft", "ArrowUp", "PageUp"]);
// Keys a scrolled beat (the full list) needs for itself.
const SCROLL_KEYS = new Set(["ArrowDown", "ArrowUp", "PageDown", "PageUp", " "]);

export function Letter({ beats, initialBeat, initialPins }: { beats: readonly Beat[]; initialBeat: number; initialPins: readonly string[] }) {
  const last = beats.length - 1;
  const [[index, direction], setPosition] = useState<[number, number]>(() => [Math.min(Math.max(initialBeat, 0), last), 1]);
  const [pins, setPins] = useState<ReadonlySet<string>>(() => new Set(initialPins));
  const [continued, setContinued] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);

  const go = useCallback(
    (to: number) => {
      const next = Math.min(Math.max(to, 0), last);
      setPosition((cur) => (next === cur[0] ? cur : [next, next > cur[0] ? 1 : -1]));
    },
    [last],
  );

  const togglePin = useCallback((slug: string) => {
    setContinued(false);
    setPins((cur) => {
      const next = new Set(cur);
      if (!next.delete(slug)) next.add(slug);
      return next;
    });
  }, []);

  // The address names the beat, so a reload or a shared link opens where the
  // reader was. Replace, not push: Back should leave the letter, not rewind it.
  useEffect(() => {
    const url = new URL(window.location.href);
    url.searchParams.set("beat", String(index + 1));
    window.history.replaceState(window.history.state, "", url);
  }, [index]);

  // Can the current beat scroll further this way? Then the gesture is the
  // beat's, not the letter's.
  const canScroll = (dy: number) => {
    const el = scroller.current;
    if (!el) return false;
    return dy > 0 ? el.scrollTop + el.clientHeight < el.scrollHeight - 2 : el.scrollTop > 0;
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      const control = target?.closest("button, a, input, textarea, select, [contenteditable]");
      if (control && (e.key === " " || e.key === "Enter")) return;
      if (target?.closest("[data-sidebar]")) return;

      const forward = FORWARD.has(e.key);
      const backward = BACKWARD.has(e.key);
      if (e.key === "Home" || e.key === "End") {
        e.preventDefault();
        go(e.key === "Home" ? 0 : last);
        return;
      }
      if (!forward && !backward) return;
      if (SCROLL_KEYS.has(e.key) && canScroll(forward ? 1 : -1)) return;
      e.preventDefault();
      go(index + (forward ? 1 : -1));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [go, index, last]);

  const wheel = useRef({ sum: 0, lastAt: 0, holdUntil: 0 });
  const onWheel = (e: WheelEvent) => {
    const w = wheel.current;
    const now = e.timeStamp;
    if (now < w.holdUntil) {
      w.holdUntil = Math.max(w.holdUntil, now + WHEEL_SETTLE_MS);
      return;
    }
    if (Math.abs(e.deltaY) < Math.abs(e.deltaX) || canScroll(e.deltaY)) {
      w.sum = 0;
      return;
    }
    if (now - w.lastAt > WHEEL_SETTLE_MS) w.sum = 0;
    w.lastAt = now;
    w.sum += e.deltaY;
    if (Math.abs(w.sum) >= WHEEL_THRESHOLD) {
      go(index + Math.sign(w.sum));
      w.sum = 0;
      w.holdUntil = now + MOVE_HOLD_MS;
    }
  };

  const touchStart = useRef<number | null>(null);
  const onTouchStart = (e: TouchEvent) => {
    touchStart.current = e.touches[0]?.clientY ?? null;
  };
  const onTouchEnd = (e: TouchEvent) => {
    const start = touchStart.current;
    const end = e.changedTouches[0]?.clientY;
    touchStart.current = null;
    if (start === null || end === undefined) return;
    const dy = start - end;
    if (Math.abs(dy) >= SWIPE_THRESHOLD && !canScroll(dy)) go(index + Math.sign(dy));
  };

  const beat = beats[index]!;

  return (
    <MotionConfig reducedMotion="user">
      <div
        className={cn(styles.root, "flex h-[calc(100svh-3.5rem)] w-full min-w-0 flex-col overflow-hidden text-content-primary")}
        onWheel={onWheel}
        onTouchStart={onTouchStart}
        onTouchEnd={onTouchEnd}
      >
        <p className="flex shrink-0 items-center gap-3 px-6 pt-6 text-xs tracking-[0.02em] text-content-muted sm:px-10 lg:px-16">
          <span aria-hidden className="h-px w-6 bg-(--letter-accent)" />A note on where you could go next
        </p>

        <p aria-live="polite" className="sr-only">
          {`${index + 1} of ${beats.length}: ${beat.label}`}
        </p>

        <div className="relative min-h-0 flex-1">
          <AnimatePresence mode="wait" custom={direction}>
            <motion.div
              key={index}
              ref={scroller}
              {...mainThread}
              custom={direction}
              variants={SHEET}
              initial="enter"
              animate="shown"
              exit="leave"
              className="absolute inset-0 overflow-y-auto overscroll-contain mask-b-from-[calc(100%-3rem)]"
            >
              <div className="mx-auto flex min-h-full w-full max-w-[76rem] flex-col px-6 sm:px-10 lg:px-16">
                {beat.kind === "prose" ? (
                  <ProseBeatView beat={beat} />
                ) : beat.kind === "role" ? (
                  <RoleBeatView beat={beat} pinned={pins.has(beat.rec.roleSlug)} onTogglePin={() => togglePin(beat.rec.roleSlug)} />
                ) : (
                  <AllBeatView beat={beat} pins={pins} onTogglePin={togglePin} />
                )}
              </div>
            </motion.div>
          </AnimatePresence>
        </div>

        <MarginBar
          labels={beats.map((b) => b.label)}
          index={index}
          go={go}
          pinned={pins.size}
          continued={continued}
          onContinue={() => setContinued(true)}
        />
      </div>
    </MotionConfig>
  );
}

// The outgoing beat lifts away in the direction of travel, quickly, so the
// eye is already free when the next beat's first words arrive.
const SHEET = {
  enter: { opacity: 1 },
  shown: { opacity: 1, y: 0, filter: "blur(0px)" },
  leave: (direction: number) => ({
    opacity: 0,
    y: direction >= 0 ? -32 : 32,
    filter: "blur(3px)",
    transition: { duration: 0.32, ease: [0.4, 0, 0.9, 0.3] as const },
  }),
};
