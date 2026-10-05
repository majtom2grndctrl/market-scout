"use client";

import { Pin } from "lucide-react";
import { motion } from "motion/react";

import { cn } from "@/lib/utils";

/** The ring every Letter control shares: accent, offset clear of the paper. */
export const FOCUS_RING =
  "outline-none focus-visible:outline-2 focus-visible:outline-offset-3 focus-visible:outline-(--letter-accent)";

export function PinButton({
  pinned,
  onToggle,
  name,
  quiet = false,
}: {
  pinned: boolean;
  onToggle: () => void;
  /** The role's display title, for the accessible name. */
  name: string;
  /** Icon-only, for dense rows. */
  quiet?: boolean;
}) {
  return (
    <button
      type="button"
      aria-pressed={pinned}
      aria-label={`Pin ${name}`}
      onClick={onToggle}
      className={cn(
        FOCUS_RING,
        "inline-flex shrink-0 items-center justify-center gap-2 rounded-full border text-sm font-medium transition-[background-color,border-color,color] duration-200",
        quiet ? "size-10" : "h-11 px-5",
        pinned
          ? "border-transparent bg-(--letter-wash) text-(--letter-accent)"
          : "border-edge bg-transparent text-content-secondary hover:border-edge-strong hover:text-content-primary",
      )}
    >
      <motion.span
        aria-hidden
        className="inline-flex"
        initial={false}
        animate={pinned ? { rotate: [0, -24, 0], scale: [1, 1.25, 1] } : { rotate: 0, scale: 1 }}
        transition={{ duration: 0.45, ease: "easeOut" }}
      >
        <Pin className={cn("size-4", pinned && "fill-current")} strokeWidth={1.75} />
      </motion.span>
      {quiet ? null : pinned ? "Pinned" : "Pin this role"}
    </button>
  );
}
