import type { ChartRenderContext } from "../composition-chart";
import { StackedBars } from "../primitives";

import { BarGapAnnotation } from "./bar-gap-annotation";

export interface StackedBarProps {
  /** The globally accumulated cells and scale set created by CompositionChart. */
  readonly context: ChartRenderContext;
}

/** Renders the engine's global-share stack without constructing or renormalizing cells. */
export function StackedBar({ context }: StackedBarProps) {
  if (context.shaped.encoding !== "stacked_bar" || context.scales.kind !== "stacked") {
    return null;
  }

  return (
    <>
      <BarGapAnnotation hasGap={context.shaped.cells.some((cell) => cell.variant === "no-data")} />
      <StackedBars
        cells={context.shaped.cells}
        xScale={context.scales.xScale}
        yScale={context.scales.yScale}
        colorScale={context.scales.colorScale}
      />
    </>
  );
}
