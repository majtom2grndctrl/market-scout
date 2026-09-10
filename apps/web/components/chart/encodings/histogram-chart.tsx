import type { HistogramBin, HistogramPanel } from "@/lib/chart/shaping";

import { histogramBinKey, type ChartRenderContext } from "../composition-chart";
import { Axis } from "../primitives";

export interface HistogramChartProps {
  /** Composition owns the fixed-edge scales and the wrapper owns responsive panel count. */
  readonly context: ChartRenderContext;
}

/**
 * Draws the fixed bins supplied by shaping with composition-provided frames,
 * scales, and ticks; it deliberately does not construct independent scales.
 */
export function HistogramChart({ context }: HistogramChartProps) {
  if (context.shaped.encoding !== "histogram" || context.scales.kind !== "histogram") return null;

  const { histogram } = context.shaped;
  const { xScale, yScale, layout } = context.scales;
  const yAxis = context.axes.y;

  return (
    <>
      {histogram.panels.map((panel, index) => {
        const frame = layout.frames[index];
        if (frame == null) return null;

        return (
          <HistogramPanelMark
            key={panel.group || `panel-${index}`}
            frame={frame}
            panel={panel}
            xScale={xScale}
            yScale={yScale}
            yAxis={frame.column === 0 ? yAxis : undefined}
            edgeLabelOffset={Math.max(0, layout.edgeLabelGutter - 12)}
            titleOffset={layout.titleGutter / 2}
          />
        );
      })}
    </>
  );
}

function HistogramPanelMark({
  frame,
  panel,
  xScale,
  yScale,
  yAxis,
  edgeLabelOffset,
  titleOffset,
}: {
  readonly frame: Extract<ChartRenderContext["scales"], { readonly kind: "histogram" }>["layout"]["frames"][number];
  readonly panel: HistogramPanel;
  readonly xScale: Extract<ChartRenderContext["scales"], { readonly kind: "histogram" }>["xScale"];
  readonly yScale: Extract<ChartRenderContext["scales"], { readonly kind: "histogram" }>["yScale"];
  readonly yAxis: ChartRenderContext["axes"]["y"];
  readonly edgeLabelOffset: number;
  readonly titleOffset: number;
}) {
  return (
    <g transform={`translate(${frame.x}, ${frame.y})`}>
      {panel.group === "" ? null : (
        <text x={0} y={-titleOffset} className="fill-content-primary text-xs font-medium">
          {panel.group}
        </text>
      )}
      {yAxis == null ? null : (
        <g data-histogram-y-axis>
          <Axis orientation="left" {...yAxis} />
        </g>
      )}
      {panel.bins.map((bin) => (
        <HistogramBar key={histogramBinKey(bin)} bin={bin} xScale={xScale} yScale={yScale} height={frame.height} />
      ))}
      <HistogramEdgeLabels bins={panel.bins} xScale={xScale} y={frame.height + edgeLabelOffset} />
    </g>
  );
}

function HistogramBar({
  bin,
  xScale,
  yScale,
  height,
}: {
  readonly bin: HistogramBin;
  readonly xScale: Extract<ChartRenderContext["scales"], { readonly kind: "histogram" }>["xScale"];
  readonly yScale: Extract<ChartRenderContext["scales"], { readonly kind: "histogram" }>["yScale"];
  readonly height: number;
}) {
  const x = xScale(histogramBinKey(bin));
  if (x == null) return null;

  const y = yScale(bin.count);
  return (
    <rect
      x={x}
      y={y}
      width={xScale.bandwidth()}
      height={Math.max(0, height - y)}
      className={bin.isZeroDay ? "fill-series-2 stroke-surface-raised" : "fill-series-1 stroke-surface-raised"}
    />
  );
}

function HistogramEdgeLabels({
  bins,
  xScale,
  y,
}: {
  readonly bins: readonly HistogramBin[];
  readonly xScale: Extract<ChartRenderContext["scales"], { readonly kind: "histogram" }>["xScale"];
  readonly y: number;
}) {
  return (
    <g className="fill-content-muted text-xs">
      {bins.map((bin) => {
        const x = xScale(histogramBinKey(bin));
        if (x == null) return null;

        return (
          <text key={histogramBinKey(bin)} x={x + xScale.bandwidth() / 2} y={y} textAnchor="middle">
            {histogramBinLabel(bin)}
          </text>
        );
      })}
    </g>
  );
}

/** Labels the declared bucket edges rather than deriving adaptive ranges from data. */
function histogramBinLabel(bin: HistogramBin): string {
  if (bin.isZeroDay) return "0";
  return bin.x1 == null ? `${bin.x0}+` : String(bin.x1);
}
