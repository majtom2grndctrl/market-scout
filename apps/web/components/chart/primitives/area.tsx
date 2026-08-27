import { area as createArea } from "d3-shape";
import type { ScaleLinear, ScaleTime } from "d3-scale";

import type { Segment } from "@/lib/chart/shaping";
import { cn } from "@/lib/utils";

export interface AreaProps {
  readonly segments: readonly Segment[];
  readonly xScale: ScaleTime<number, number>;
  readonly yScale: ScaleLinear<number, number>;
  readonly className?: string;
}

/** Fills each already-gap-free segment independently, leaving collection gaps empty. */
export function Area({ segments, xScale, yScale, className }: AreaProps) {
  const pathFor = createArea<Segment[number]>()
    .x((point) => xScale(point.x))
    .y0(yScale(0))
    .y1((point) => yScale(point.y));

  return (
    <g className={cn(className)}>
      {segments.map((segment, index) => {
        const key = segmentKey(segment, index);
        const point = segment[0];

        if (segment.length === 1 && point != null) {
          return <circle key={key} cx={xScale(point.x)} cy={yScale(point.y)} r={4} className="fill-primary" />;
        }

        return <path key={key} d={pathFor(segment) ?? undefined} className="fill-primary/25 stroke-primary stroke-2" />;
      })}
    </g>
  );
}

function segmentKey(segment: Segment, index: number): string {
  const firstPoint = segment[0];
  return firstPoint == null ? `empty-${index}` : `${firstPoint.x.toISOString()}-${index}`;
}
