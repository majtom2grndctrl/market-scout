"use client";

import { motion, useReducedMotion } from "motion/react";

import { drift, SUNRISE } from "../_lib/motion";
import styles from "../dawn.module.css";
import { cn } from "@/lib/utils";

/**
 * First light, as a sky the page scrolls across. The sky is held to the
 * viewport: blue overhead, lilac through the middle, and the horizon at the
 * bottom edge, where the pin bar sits. The roles card is opaque, so the sky
 * shows around it and through the gutter, and scrolling moves the card across
 * the sky rather than the sky with the page.
 *
 * Before first light the sky is twilight: violet overhead, no warmth low
 * down. When the first role lights and its thread reaches the skills, the
 * sun rises behind the roles card, the warm band climbs from the horizon,
 * and the blue clears overhead. It rises once and stays. With reduced motion
 * the sky starts risen and nothing moves.
 */
export function Sky({ risen }: { risen: boolean }) {
  const reduce = useReducedMotion() ?? false;
  const up = risen || reduce;
  const still = { duration: 0 };
  const rise = reduce ? still : up ? drift(SUNRISE.delay, SUNRISE.duration) : drift(0, 1.4);

  return (
    <div aria-hidden className={cn(styles.skyFrame, "pointer-events-none")}>
      <motion.div
        className={cn(styles.sky, "absolute inset-0")}
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={reduce ? still : drift(0, 1.4)}
      />
      <motion.div
        className={cn(styles.clear, "absolute inset-0")}
        initial={{ opacity: 0 }}
        animate={{ opacity: up ? 1 : 0 }}
        transition={rise}
      />
      <motion.div
        // Taller than the sky by the distance it rises, so no edge shows.
        className={cn(styles.sun, "absolute inset-x-0 bottom-0")}
        style={{ top: -SUNRISE.below }}
        initial={{ opacity: 0, y: SUNRISE.below }}
        animate={up ? { opacity: 1, y: 0 } : { opacity: SUNRISE.before, y: SUNRISE.below }}
        transition={rise}
      />
    </div>
  );
}
