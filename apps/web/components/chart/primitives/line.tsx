import { line as createLine } from "d3-shape";
import type { ScaleLinear, ScaleTime } from "d3-scale";

import type { Segment } from "@/lib/chart/shaping";
import { cn } from "@/lib/utils";

export interface LineProps {
  readonly segments: readonly Segment[];
  readonly xScale: ScaleTime<number, number>;
  readonly yScale: ScaleLinear<number, number>;
  readonly className?: string;
}

/** Draws every already-gap-free segment separately so no path crosses absent data. */
export function Line({ segments, xScale, yScale, className }: LineProps) {
  const pathFor = createLine<Segment[number]>()
    .x((point) => xScale(point.x))
    .y((point) => yScale(point.y));

  return (
    <g className={cn(className)}>
      {segments.map((segment, index) => {
        const key = segmentKey(segment, index);
        const point = segment[0];

        if (segment.length === 1 && point != null) {
          return <circle key={key} cx={xScale(point.x)} cy={yScale(point.y)} r={4} className="fill-series-1" />;
        }

        return <path key={key} d={pathFor(segment) ?? undefined} className="fill-none stroke-series-1 stroke-2" />;
      })}
    </g>
  );
}

function segmentKey(segment: Segment, index: number): string {
  const firstPoint = segment[0];
  return firstPoint == null ? `empty-${index}` : `${firstPoint.x.toISOString()}-${index}`;
}
