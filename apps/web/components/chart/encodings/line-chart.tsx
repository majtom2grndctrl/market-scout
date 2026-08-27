import type { ChartRenderContext } from "../composition-chart";
import { GapBand, Line, SeriesGapMarker } from "../primitives";

export interface LineChartProps {
  /** The composition-owned scales and already gap-split temporal representation. */
  readonly context: ChartRenderContext;
}

/** Renders rate observations without inventing a line through an uncollected week. */
export function LineChart({ context }: LineChartProps) {
  if (context.shaped.encoding !== "line" || context.scales.kind !== "temporal") return null;

  const { temporal } = context.shaped;
  const { xScale, yScale } = context.scales;

  return (
    <>
      <SeriesGapMarker regions={temporal.seriesGapRegions} xScale={xScale} />
      <GapBand
        regions={temporal.gapRegions}
        xScale={xScale}
        y={0}
        height={context.geometry.plot.height}
      />
      <Line segments={temporal.segments} xScale={xScale} yScale={yScale} />
    </>
  );
}
