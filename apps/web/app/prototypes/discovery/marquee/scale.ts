// Marquee's type ladder below the fitted headline, kept in one place so every
// section steps the same way. Sizes ride the section's container width (cqi),
// so a poster keeps its proportions from phone to wall.
//
// At 1440 wide: headline ≈ 200 → roll 57 → statement 35 → folio 27 →
// list 24 → support 19. On a phone: 44 → 30 → 26 → 22 → 20 → 16. The drop
// from display to support stays steep on purpose; that contrast is the voice.
//
// Recession is carried by ink (`content-primary` → `-secondary`) and weight.
// `content-muted` falls under 4.5:1 on the plates, so nothing on a poster uses it.

/** Rank, strength, and the pin: the poster's running line. */
export const FOLIO = "font-display text-[clamp(1.375rem,2.4cqi,2rem)] leading-[1.1] tracking-[-0.025em]";

/** The alternate titles, and the "Also called" they follow. */
export const ROLL = "font-display text-[clamp(1.875rem,5.2cqi,4.5rem)] leading-[1.02] font-light tracking-[-0.025em]";

/** Why the role is here, and what the person brings to it. */
export const STATEMENT = "font-display text-[clamp(1.625rem,3.2cqi,2.625rem)] leading-[1.06] tracking-[-0.02em]";

/** What a role's postings also ask for, and its rows. */
export const LIST = "font-display text-[clamp(1.25rem,2.2cqi,1.875rem)] leading-tight tracking-[-0.015em]";

/** Reading text that supports the display type: spellings, count labels, method notes. */
export const SUPPORT = "font-sans text-[clamp(1rem,1.7cqi,1.25rem)] leading-snug";
