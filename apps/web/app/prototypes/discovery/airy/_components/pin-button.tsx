"use client";

import { Check, Pin } from "lucide-react";
import { motion } from "motion/react";

import { FLIGHT, SETTLE } from "../_lib/motion";
import { cn } from "@/lib/utils";

export const pinLayoutId = (roleSlug: string) => `airy-pin-${roleSlug}`;

/**
 * The seed — the warm disc around the pin glyph — shares a layoutId with the
 * role's chip in the tray. Pinning unmounts it here and mounts the chip there,
 * so the disc flies to where pins collect and unfurls into the chip.
 */
export function PinButton({
  roleSlug,
  headline,
  pinned,
  onToggle,
}: {
  roleSlug: string;
  headline: string;
  pinned: boolean;
  onToggle: (roleSlug: string) => void;
}) {
  return (
    <motion.button
      type="button"
      aria-pressed={pinned}
      aria-label={pinned ? `Unpin ${headline}` : `Pin ${headline}`}
      onClick={() => onToggle(roleSlug)}
      whileTap={{ scale: 0.94 }}
      transition={SETTLE}
      className={cn(
        "group/pin relative inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 rounded-full pr-4 pl-1.5 text-sm font-medium outline-none",
        "transition-[background-color,box-shadow,color] duration-300 ease-out",
        "focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-surface-raised",
        pinned
          ? "bg-accent-solid text-on-solid hover:bg-accent-hover active:bg-accent-active"
          : "bg-surface-raised text-content-primary ring-1 ring-edge hover:bg-(--airy-dawn) hover:ring-(--airy-glow-edge)",
      )}
    >
      <span className="relative size-6">
        {pinned ? (
          <motion.span
            key="check"
            className="absolute inset-0 grid place-items-center rounded-full bg-on-solid/15"
            initial={{ scale: 0.3, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            transition={{ ...SETTLE, delay: 0.08 }}
          >
            <Check className="size-3.5" strokeWidth={2.5} />
          </motion.span>
        ) : (
          <motion.span
            key="seed"
            layoutId={pinLayoutId(roleSlug)}
            transition={FLIGHT}
            style={{ borderRadius: 999 }}
            className="absolute inset-0 grid place-items-center bg-(--airy-glow) text-(--airy-ember) ring-1 ring-(--airy-glow-edge) ring-inset"
          >
            <Pin className="size-3.5 transition-transform duration-300 ease-out group-hover/pin:-rotate-12" strokeWidth={2.25} />
          </motion.span>
        )}
      </span>
      <span>{pinned ? "Pinned" : "Pin"}</span>
    </motion.button>
  );
}
