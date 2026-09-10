export interface BarGapAnnotationProps {
  /** Whether this bar-family chart contains at least one failed-run placeholder. */
  readonly hasGap: boolean;
}

/** Explains a bar-family collection gap once, without assigning it a numeric value. */
export function BarGapAnnotation({ hasGap }: BarGapAnnotationProps) {
  if (!hasGap) return null;

  return (
    <text x={0} y={-8} className="fill-unavailable-ink text-xs">
      gray marks a gap in collected data
    </text>
  );
}
