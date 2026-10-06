"use client";

import { Check, Pin } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";

import { SETTLE } from "../_lib/motion";
import { cn } from "@/lib/utils";

export function PinButton({
  roleSlug,
  headline,
  pinned,
  onToggle,
  className,
}: {
  roleSlug: string;
  headline: string;
  pinned: boolean;
  onToggle: (roleSlug: string) => void;
  className?: string;
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
        "group/pin inline-flex h-9 shrink-0 cursor-pointer items-center gap-1.5 rounded-full pr-3.5 pl-1.5 text-sm font-medium outline-none",
        "transition-[background-color,box-shadow,color] duration-300 ease-out",
        "focus-visible:ring-2 focus-visible:ring-focus focus-visible:ring-offset-2 focus-visible:ring-offset-surface-raised",
        pinned
          ? "bg-accent-solid text-on-solid hover:bg-accent-hover active:bg-accent-active"
          : "bg-surface-raised text-content-primary ring-1 ring-edge hover:bg-surface-row-hover hover:ring-edge-strong/50",
        className,
      )}
    >
      <span className="relative grid size-6 place-items-center">
        <AnimatePresence initial={false} mode="popLayout">
          <motion.span
            key={pinned ? "on" : "off"}
            className={cn(
              "absolute inset-0 grid place-items-center rounded-full",
              pinned ? "bg-on-solid/15" : "bg-surface-sunken text-content-secondary",
            )}
            initial={{ scale: 0.4, opacity: 0 }}
            animate={{ scale: 1, opacity: 1 }}
            exit={{ scale: 0.4, opacity: 0, transition: { duration: 0.12 } }}
            transition={SETTLE}
          >
            {pinned ? (
              <Check className="size-3.5" strokeWidth={2.5} />
            ) : (
              <Pin className="size-3.5 transition-transform duration-300 ease-out group-hover/pin:-rotate-12" strokeWidth={2.25} />
            )}
          </motion.span>
        </AnimatePresence>
      </span>
      <span>{pinned ? "Pinned" : "Pin"}</span>
    </motion.button>
  );
}
