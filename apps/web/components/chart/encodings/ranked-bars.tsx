import type { ChartRenderContext } from "../composition-chart";
import { Bars } from "../primitives";

import { BarGapAnnotation } from "./bar-gap-annotation";

export interface RankedBarsProps {
  /** The completed shaping, scales, and geometry owned by CompositionChart. */
  readonly context: ChartRenderContext;
}

/** Composes ordered categorical marks and their end labels; marks own no labels. */
export function RankedBars({ context }: RankedBarsProps) {
  if (context.shaped.encoding !== "ranked_bars" || context.scales.kind !== "categorical") {
    return null;
  }

  const { bars } = context.shaped;
  const { xScale, yScale } = context.scales;

  return (
    <>
      <BarGapAnnotation hasGap={bars.some((bar) => bar.variant === "no-data")} />
      <Bars bars={bars} xScale={xScale} yScale={yScale} />
      <BarEndLabels context={context} />
    </>
  );
}

function BarEndLabels({ context }: RankedBarsProps) {
  if (context.shaped.encoding !== "ranked_bars" || context.scales.kind !== "categorical") {
    return null;
  }

  const { xScale, yScale } = context.scales;

  return (
    <g className="fill-foreground text-xs">
      {context.shaped.bars.map((bar) => {
        const y = yScale(bar.key);
        if (y == null) return null;

        const valueEnd = xScale(bar.value);
        return (
          <text
            key={bar.key}
            x={valueEnd + (bar.variant === "no-data" ? 12 : 4)}
            y={y + yScale.bandwidth() / 2}
            dominantBaseline="middle"
          >
            {bar.variant === "no-data" ? "No data" : formatValue(bar.value)}
          </text>
        );
      })}
    </g>
  );
}

function formatValue(value: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }).format(value);
}
