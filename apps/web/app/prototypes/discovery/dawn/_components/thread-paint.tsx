import { STOPS, type Span } from "../_lib/light";

/**
 * The thread's gradient for one SVG, in that SVG's own x. `shift` is where
 * the SVG's x origin sits in container x: the column's left edge for the
 * compositor's pieces, zero for paths drawn over the whole layout.
 */
export function ThreadGradient({ id, span, shift }: { id: string; span: Span; shift: number }) {
  return (
    <defs>
      <linearGradient id={id} gradientUnits="userSpaceOnUse" x1={span.from - shift} x2={span.to - shift} y1={0} y2={0}>
        {STOPS.map((s) => (
          <stop key={s.at} offset={s.at} style={{ stopColor: s.color }} />
        ))}
      </linearGradient>
    </defs>
  );
}

/** The port: where the bundle leaves the role, lit like the sun on the horizon. */
export const PORT_STYLE = {
  fill: "var(--dawn-port)",
  filter: "drop-shadow(0 0 4px var(--dawn-sun))",
} as const;
