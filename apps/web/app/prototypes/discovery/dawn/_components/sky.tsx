"use client";

import { motion } from "motion/react";

import { drift, SUNRISE } from "../_lib/motion";
import styles from "../dawn.module.css";
import { cn } from "@/lib/utils";

/**
 * First light. The cool sky is there from the start, high over the skills.
 * The sun waits below the horizon until the first role lights and its thread
 * reaches the skills, then rises over the roles: the morning arrives with the
 * first connection, not on a timer of its own. It rises once and stays.
 */
export function Sky({ risen }: { risen: boolean }) {
  return (
    <div aria-hidden className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-[46rem] overflow-hidden">
      <motion.div
        className={cn(styles.sky, "absolute inset-0")}
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={drift(0, 1.4)}
      />
      <motion.div
        // Taller than the sky by the distance it rises, so no edge shows.
        className={cn(styles.sun, "absolute inset-x-0 bottom-0")}
        style={{ top: -SUNRISE.below }}
        initial={{ opacity: 0, y: SUNRISE.below }}
        animate={risen ? { opacity: 1, y: 0 } : { opacity: SUNRISE.before, y: SUNRISE.below }}
        transition={risen ? drift(SUNRISE.delay, SUNRISE.duration) : drift(0, 1.4)}
      />
    </div>
  );
}
