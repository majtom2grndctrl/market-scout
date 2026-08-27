import type { ScaleTime } from "d3-scale";

import type { SeriesGapRegion } from "@/lib/chart/shaping";
import { cn } from "@/lib/utils";

export interface SeriesGapMarkerProps {
  readonly regions: readonly SeriesGapRegion[];
  readonly xScale: ScaleTime<number, number>;
  readonly className?: string;
}

/** Marks a series-only collection gap without covering observations from other series. */
export function SeriesGapMarker({ regions, xScale, className }: SeriesGapMarkerProps) {
  return (
    <g className={cn(className)}>
      {regions.map((region) => {
        const start = xScale(region.start);
        const end = xScale(region.end);

        return (
          <g
            key={`${region.series}-${region.start.toISOString()}-${region.end.toISOString()}`}
            aria-label={`Gap in collected data for ${region.label}`}
          >
            <title>Gap in collected data for {region.label}</title>
            <line x1={start} x2={end} y1={3} y2={3} className="stroke-muted-foreground stroke-2" />
          </g>
        );
      })}
    </g>
  );
}
