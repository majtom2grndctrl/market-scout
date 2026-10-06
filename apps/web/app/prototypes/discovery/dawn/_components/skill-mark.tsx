import { forwardRef, type CSSProperties } from "react";

import { cn } from "@/lib/utils";

/**
 * The one glyph that tells a claimed skill from an inherited one, used on both
 * sides of the page: a filled dot for a skill the person named, a ring for one
 * that comes with a role they've held. On the skill column it is also where a
 * thread lands.
 */
interface Props {
  claimed: boolean;
  className?: string;
  style?: CSSProperties;
}

export const SkillMark = forwardRef<HTMLSpanElement, Props>(function SkillMark({ claimed, className, style }, ref) {
  return (
    <span
      ref={ref}
      aria-hidden
      style={style}
      className={cn(
        "inline-block size-[7px] shrink-0 rounded-full border-[1.5px] border-current transition-colors duration-300",
        claimed ? "bg-current" : "bg-transparent",
        className,
      )}
    />
  );
});
