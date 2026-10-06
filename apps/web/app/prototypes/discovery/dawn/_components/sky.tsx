"use client";

import { motion, useReducedMotion } from "motion/react";

import { drift, ENTRANCE, SUNRISE } from "../_lib/motion";
import styles from "../dawn.module.css";
import { cn } from "@/lib/utils";

/**
 * First light, from behind the roles card. It sits in the card's grid cell,
 * under the card, and spills past its edges into the gutter, the margin, and
 * the space just above it; the skill column and everything above it stay on
 * the plain page (see `.glow` in dawn.module.css).
 *
 * Before first light only dusk shows: faint, and close to the card, arriving
 * with the card. When the first role lights and its thread reaches the
 * skills, the morning sky blooms out from behind the card, once, and stays.
 * Blue around the card plays against the warm light the lit role holds. With
 * reduced motion the morning arrives with the card, by fading alone.
 */
export function Sky({ risen, className }: { risen: boolean; className?: string }) {
  const reduce = useReducedMotion() ?? false;
  const up = risen || reduce;
  // With reduced motion the light still fades in with the card, never ahead
  // of it: the dome and the bloom reach behind the card, and would show
  // through it while it fades in.
  const withCard = drift(ENTRANCE.card, 1.1);

  return (
    <div aria-hidden className={cn(styles.glow, className)}>
      <div className={styles.glowFrame}>
        <motion.div
          className={styles.dusk}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={withCard}
        />
        <motion.div
          className={styles.morning}
          // The same first render on server and client; under reduced motion
          // MotionConfig sets the scale at once and only the fade plays.
          initial={{ opacity: 0, scale: SUNRISE.from }}
          animate={up ? { opacity: 1, scale: 1 } : { opacity: 0, scale: SUNRISE.from }}
          transition={reduce ? withCard : drift(SUNRISE.delay, SUNRISE.duration)}
        />
      </div>
    </div>
  );
}
