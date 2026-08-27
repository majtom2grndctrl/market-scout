import type { ScaleTime } from "d3-scale";

import type { GapRegion } from "@/lib/chart/shaping";
import { cn } from "@/lib/utils";

export interface GapBandProps {
  readonly regions: readonly GapRegion[];
  readonly xScale: ScaleTime<number, number>;
  readonly y: number;
  readonly height: number;
  readonly className?: string;
}

/** Shows failed collection intervals in the time-series plot's coordinate space. */
export function GapBand({ regions, xScale, y, height, className }: GapBandProps) {
  return (
    <g className={cn(className)}>
      {regions.map((region) => {
        const start = xScale(region.start);
        const end = xScale(region.end);
        const width = Math.max(0, end - start);

        return (
          <g key={`${region.start.toISOString()}-${region.end.toISOString()}`}>
            <rect x={start} y={y} width={width} height={height} className="fill-muted" />
            <text
              x={start + width / 2}
              y={y + 14}
              textAnchor="middle"
              className="fill-muted-foreground text-xs"
            >
              gray marks a gap in collected data
            </text>
          </g>
        );
      })}
    </g>
  );
}
