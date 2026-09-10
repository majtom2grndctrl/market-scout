import { useId } from "react";
import type { ScaleBand, ScaleLinear } from "d3-scale";

import type { ShapedBar } from "@/lib/chart/shaping";
import { cn } from "@/lib/utils";

export interface BarsProps {
  readonly bars: readonly ShapedBar[];
  readonly xScale: ScaleLinear<number, number>;
  readonly yScale: ScaleBand<string>;
  readonly className?: string;
}

/** Renders horizontal categorical bars; labels belong to the encoding layer. */
export function Bars({ bars, xScale, yScale, className }: BarsProps) {
  const hatchId = useId().replace(/:/g, "");
  const baseline = xScale(0);

  return (
    <g className={cn(className)}>
      <defs>
        <pattern id={hatchId} width="6" height="6" patternUnits="userSpaceOnUse">
          <path d="M-1,1 L1,-1 M0,6 L6,0 M5,7 L7,5" className="stroke-unavailable-ink" />
        </pattern>
      </defs>
      {bars.map((bar) => {
        const y = yScale(bar.key);
        if (y === undefined) return null;

        const scaledValue = xScale(bar.value);
        const x = Math.min(baseline, scaledValue);
        const width = Math.abs(scaledValue - baseline);

        if (bar.variant === "no-data") {
          return (
            <rect
              key={bar.key}
              x={baseline}
              y={y}
              width={8}
              height={yScale.bandwidth()}
              className="fill-unavailable-subtle stroke-unavailable-edge"
              fill={`url(#${hatchId})`}
            />
          );
        }

        return (
          <rect
            key={bar.key}
            x={x}
            y={y}
            width={width}
            height={yScale.bandwidth()}
            className={cn(
              "stroke-surface-raised",
              bar.variant === "unmapped" ? "fill-series-other" : "fill-series-1",
            )}
          />
        );
      })}
    </g>
  );
}
