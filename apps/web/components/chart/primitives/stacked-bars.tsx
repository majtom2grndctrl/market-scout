import { useId } from "react";
import type { ScaleBand, ScaleLinear, ScaleOrdinal } from "d3-scale";

import type { StackedCell } from "@/lib/chart/shaping";
import { cn } from "@/lib/utils";

export interface StackedBarsProps {
  readonly cells: readonly StackedCell[];
  readonly xScale: ScaleBand<string>;
  readonly yScale: ScaleLinear<number, number>;
  readonly colorScale: ScaleOrdinal<string, string>;
  readonly className?: string;
}

/** Renders precomputed cumulative stack cells without changing their values. */
export function StackedBars({ cells, xScale, yScale, colorScale, className }: StackedBarsProps) {
  const hatchId = useId().replace(/:/g, "");
  const baseline = yScale(0);

  return (
    <g className={cn(className)}>
      <defs>
        <pattern id={hatchId} width="6" height="6" patternUnits="userSpaceOnUse">
          <path d="M-1,1 L1,-1 M0,6 L6,0 M5,7 L7,5" className="stroke-unavailable-ink" />
        </pattern>
      </defs>
      {cells.map((cell, index) => {
        const x = xScale(cell.key);
        if (x === undefined) return null;

        if (cell.variant === "no-data") {
          return (
            <rect
              key={`${cell.key}-${cell.series}-${index}`}
              x={x}
              y={baseline - 8}
              width={xScale.bandwidth()}
              height={8}
              className="fill-unavailable-subtle stroke-unavailable-edge"
              fill={`url(#${hatchId})`}
            />
          );
        }

        const y0 = yScale(cell.y0);
        const y1 = yScale(cell.y1);

        return (
          <rect
            key={`${cell.key}-${cell.series}-${index}`}
            x={x}
            y={Math.min(y0, y1)}
            width={xScale.bandwidth()}
            height={Math.abs(y1 - y0)}
            fill={colorScale(cell.series)}
            className="stroke-surface-raised"
          />
        );
      })}
    </g>
  );
}
