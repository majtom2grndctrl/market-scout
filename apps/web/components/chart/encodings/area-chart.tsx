import type { ChartRenderContext } from "../composition-chart";
import { Area, GapBand, SeriesGapMarker } from "../primitives";

export interface AreaChartProps {
  /** The composition-owned scales and already gap-split temporal representation. */
  readonly context: ChartRenderContext;
}

/** Renders weekly count fills one collected segment at a time. */
export function AreaChart({ context }: AreaChartProps) {
  if (context.shaped.encoding !== "area" || context.scales.kind !== "temporal") return null;

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
      <Area segments={temporal.segments} xScale={xScale} yScale={yScale} />
    </>
  );
}
