"use client";

import { max } from "d3-array";
import {
  scaleBand,
  scaleLinear,
  scaleOrdinal,
  scaleUtc,
  type ScaleBand,
  type ScaleLinear,
  type ScaleOrdinal,
  type ScaleTime,
} from "d3-scale";
import { utcFormat } from "d3-time-format";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import type { Encoding } from "@/lib/composition";
import { mergeChartTheme, type ChartTheme, type ChartThemeOverrides, type ReflowBreakpoint } from "@/lib/chart/chart-theme";
import {
  shapeForEncoding,
  type HistogramBin,
  type ShapedBar,
  type ShapedEncoding,
  type StackedCell,
} from "@/lib/chart/shaping";
import type { MeasureResult } from "@/lib/db/measure-engine";

import { Axis, CoveragePill, type Coverage } from "./primitives";

export type { Coverage } from "./primitives";

/** A measured result whose denominator is absent because its slice is the full corpus. */
export type FullCorpusResult = Omit<MeasureResult, "denominator"> & {
  readonly denominator?: never;
};

/** A measured classified slice. Its denominator must be shown by the caller. */
export type ClassifiedResult = Omit<MeasureResult, "denominator"> & {
  readonly denominator: Coverage;
};

export type CompositionChartCoverageProps =
  | { readonly result: FullCorpusResult; readonly coverage?: never }
  | { readonly result: ClassifiedResult; readonly coverage: Coverage };

export interface ChartGeometry {
  readonly viewBox: { readonly width: number; readonly height: number; readonly aspectRatio: number };
  readonly margins: ChartTheme["margins"];
  readonly gutters: ChartTheme["gutters"];
  readonly plot: { readonly width: number; readonly height: number };
}

export interface ChartReflow {
  readonly width: number;
  readonly tickDensity: number;
  readonly rotateLabels: boolean;
  readonly labelRotation: 0 | -45;
  readonly panelsPerRow: number;
}

export interface ChartAxis {
  readonly ticks: readonly { readonly value: string | number | Date; readonly label: string }[];
  readonly scale: (value: string | number | Date) => number | undefined;
  readonly length: number;
  readonly labelRotation: ChartReflow["labelRotation"];
}

/** A composition-calculated small-multiple frame; encodings only render within it. */
export interface HistogramPanelFrame {
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
  readonly row: number;
  readonly column: number;
}

/** Shared histogram geometry, including all space required by each panel's labels. */
export interface HistogramLayout {
  readonly frames: readonly HistogramPanelFrame[];
  readonly rowCount: number;
  readonly panelsPerRow: number;
  readonly plotHeight: number;
  readonly titleGutter: number;
  readonly edgeLabelGutter: number;
  readonly scrollable: boolean;
  readonly scrollHeight: number;
}

export interface CategoricalScales {
  readonly kind: "categorical";
  readonly xScale: ScaleLinear<number, number>;
  readonly yScale: ScaleBand<string>;
}

export interface DivergingScales {
  readonly kind: "diverging";
  readonly xScale: ScaleLinear<number, number>;
  readonly yScale: ScaleBand<string>;
}

export interface StackedScales {
  readonly kind: "stacked";
  readonly xScale: ScaleBand<string>;
  readonly yScale: ScaleLinear<number, number>;
  readonly colorScale: ScaleOrdinal<string, string>;
}

export interface TemporalScales {
  readonly kind: "temporal";
  readonly xScale: ScaleTime<number, number>;
  readonly yScale: ScaleLinear<number, number>;
}

export interface HistogramScales {
  readonly kind: "histogram";
  /** The domain contains the declared `{0}`, seven-day, and `70+` bins in fixed order. */
  readonly xScale: ScaleBand<string>;
  /** One y scale is shared by every histogram small-multiple panel. */
  readonly yScale: ScaleLinear<number, number>;
  /** Composition owns the grid and tick geometry; marks only consume it. */
  readonly layout: HistogramLayout;
}

export interface TableScales {
  readonly kind: "table";
}

export type ChartScales =
  | CategoricalScales
  | DivergingScales
  | StackedScales
  | TemporalScales
  | HistogramScales
  | TableScales;

export interface ChartAxes {
  readonly x?: ChartAxis;
  readonly y?: ChartAxis;
}

export interface ChartRenderContext {
  readonly result: MeasureResult;
  readonly encoding: Encoding;
  readonly shaped: ShapedEncoding;
  readonly geometry: ChartGeometry;
  readonly reflow: ChartReflow;
  readonly scales: ChartScales;
  /** Tick values, formatting, and label rotation are selected once with the scales. */
  readonly axes: ChartAxes;
}

interface CompositionChartBaseProps {
  /** Grammar encoding is explicit: MeasureResult intentionally carries no encoding field. */
  readonly encoding: Encoding;
  readonly children?: (context: ChartRenderContext) => ReactNode;
  readonly theme?: ChartThemeOverrides;
  readonly className?: string;
  readonly "aria-label"?: string;
}

export type CompositionChartProps = CompositionChartBaseProps & CompositionChartCoverageProps;

/**
 * The one SVG composition boundary: it server-renders deterministic geometry,
 * then progressively enhances only reflow choices after its wrapper is measured.
 */
export function CompositionChart(props: CompositionChartProps) {
  const { encoding, result, children, theme, className, coverage } = props;
  const resolvedTheme = useMemo(() => mergeChartTheme(theme), [theme]);
  const baseGeometry = useMemo(() => chartGeometry(resolvedTheme, encoding), [resolvedTheme, encoding]);
  const fallbackReflow = useMemo(
    () => reflowForWidth(resolvedTheme, baseGeometry.viewBox.width),
    [resolvedTheme, baseGeometry.viewBox.width],
  );
  const [reflow, setReflow] = useState<ChartReflow>(fallbackReflow);
  const wrapper = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const element = wrapper.current;
    if (element == null || typeof ResizeObserver === "undefined") return;

    const observer = new ResizeObserver(([entry]) => {
      const width = entry?.contentRect.width;
      if (width == null || width <= 0) return;

      const next = reflowForWidth(resolvedTheme, width);
      setReflow((current) => (sameReflow(current, next) ? current : next));
    });

    observer.observe(element);
    return () => observer.disconnect();
  }, [resolvedTheme]);

  // Statically typed classified results cannot reach this branch without a
  // coverage prop. It protects dynamic model/JSON values that bypass the union.
  if (hasDenominator(result) && coverage == null) {
    return (
      <div className={className} role="alert" data-chart-refusal="missing-coverage">
        Coverage is required before a classified chart can be rendered.
      </div>
    );
  }

  const context = createRenderContext(result, encoding, baseGeometry, reflow, resolvedTheme);
  const { geometry } = context;
  const viewBox = geometry.viewBox;
  const accessibilityDescription = temporalGapDescription(context.shaped);
  const histogramLayout = context.scales.kind === "histogram" ? context.scales.layout : undefined;

  // Tables are deliberately HTML, not SVG. The small SVG header keeps the
  // composition-owned coverage annotation available without giving a table a
  // meaningless coordinate system or a scale.
  if (encoding === "table") {
    return (
      <div
        ref={wrapper}
        className={className}
        data-chart-encoding={encoding}
      >
        {coverage == null ? null : (
          <svg
            viewBox={`0 0 ${viewBox.width} 32`}
            preserveAspectRatio="xMinYMid meet"
            role="img"
            aria-label="Classification coverage"
            className="h-8 w-full"
          >
            <CoveragePill coverage={coverage} x={geometry.margins.left} y={0} />
          </svg>
        )}
        {children?.(context)}
      </div>
    );
  }

  return (
    <div
      ref={wrapper}
      className={className}
      style={histogramLayout?.scrollable
        ? { maxHeight: histogramLayout.scrollHeight, overflowY: "auto" }
        : { aspectRatio: `${viewBox.width} / ${viewBox.height}` }}
      data-chart-encoding={encoding}
      data-chart-scrollable={histogramLayout?.scrollable || undefined}
    >
      <svg
        viewBox={`0 0 ${viewBox.width} ${viewBox.height}`}
        preserveAspectRatio="xMidYMid meet"
        role="img"
        aria-label={props["aria-label"] ?? `${encoding} chart`}
        className={histogramLayout?.scrollable ? "block h-auto w-full overflow-visible" : "h-full w-full overflow-visible"}
      >
        {accessibilityDescription == null ? null : <desc>{accessibilityDescription}</desc>}
        {coverage == null ? null : <CoveragePill coverage={coverage} x={geometry.margins.left} y={0} />}
        <g transform={`translate(${geometry.margins.left}, ${geometry.margins.top})`}>
          {children?.(context)}
          <CompositionAxes axes={context.axes} geometry={geometry} suppressY={encoding === "histogram"} />
        </g>
      </svg>
    </div>
  );
}

/** Converts a fixed histogram bin into the immutable declared-edge scale key. */
export function histogramBinKey(bin: HistogramBin): string {
  if (bin.isZeroDay) return "{0}";
  if (bin.x1 == null) return `${bin.x0}+`;
  return `[${bin.x0},${bin.x1})`;
}

function createRenderContext(
  result: MeasureResult,
  encoding: Encoding,
  baseGeometry: ChartGeometry,
  reflow: ChartReflow,
  theme: ChartTheme,
): ChartRenderContext {
  const shaped = shapeForEncoding(result, encoding, theme);
  const geometry = geometryForShaped(baseGeometry, shaped, reflow, theme);
  const { scales, axes } = createScales(shaped, geometry, reflow, theme);

  return { result, encoding, shaped, geometry, reflow, scales, axes };
}

function createScales(
  shaped: ShapedEncoding,
  geometry: ChartGeometry,
  reflow: ChartReflow,
  theme: ChartTheme,
): { readonly scales: ChartScales; readonly axes: ChartAxes } {
  switch (shaped.encoding) {
    case "ranked_bars":
      return categoricalScales(shaped.bars, geometry, reflow, theme);
    case "diverging_bars":
      return divergingScales(shaped.bars, geometry, reflow, theme);
    case "stacked_bar":
      return stackedScales(shaped.cells, geometry, reflow, theme);
    case "line":
    case "area":
      return temporalScales(shaped.temporal, geometry, reflow);
    case "histogram":
      return histogramScales(shaped.histogram, geometry, reflow, theme);
    case "table":
      return { scales: { kind: "table" }, axes: {} };
  }
}

function categoricalScales(
  bars: readonly ShapedBar[],
  geometry: ChartGeometry,
  reflow: ChartReflow,
  theme: ChartTheme,
) {
  const yScale = scaleBand<string>()
    .domain(orderedKeys(bars.map((bar) => bar.key)))
    .range([0, geometry.plot.height])
    .paddingInner(theme.bandPadding.inner)
    .paddingOuter(theme.bandPadding.outer);
  const xScale = scaleLinear()
    .domain([0, positiveDomain(bars.map((bar) => bar.value))])
    .range([0, geometry.plot.width]);

  return {
    scales: { kind: "categorical" as const, xScale, yScale },
    axes: {
      x: linearAxis(xScale, geometry.plot.width, reflow),
      y: bandAxis(yScale, geometry.plot.height, reflow),
    },
  };
}

function divergingScales(
  bars: readonly ShapedBar[],
  geometry: ChartGeometry,
  reflow: ChartReflow,
  theme: ChartTheme,
) {
  const extent = positiveDomain(bars.map((bar) => Math.abs(bar.value)));
  const yScale = scaleBand<string>()
    .domain(orderedKeys(bars.map((bar) => bar.key)))
    .range([0, geometry.plot.height])
    .paddingInner(theme.bandPadding.inner)
    .paddingOuter(theme.bandPadding.outer);
  const xScale = scaleLinear().domain([-extent, extent]).range([0, geometry.plot.width]);

  return {
    scales: { kind: "diverging" as const, xScale, yScale },
    axes: {
      x: linearAxis(xScale, geometry.plot.width, reflow),
      y: bandAxis(yScale, geometry.plot.height, reflow),
    },
  };
}

function stackedScales(
  cells: readonly StackedCell[],
  geometry: ChartGeometry,
  reflow: ChartReflow,
  theme: ChartTheme,
) {
  const xScale = scaleBand<string>()
    .domain(orderedKeys(cells.map((cell) => cell.key)))
    .range([0, geometry.plot.width])
    .paddingInner(theme.bandPadding.inner)
    .paddingOuter(theme.bandPadding.outer);
  const yScale = scaleLinear()
    .domain([0, positiveDomain(cells.map((cell) => cell.y1))])
    .range([geometry.plot.height, 0]);
  const colorScale = scaleOrdinal<string, string>()
    .domain(orderedKeys(cells.map((cell) => cell.series)))
    .range(["var(--chart-1)", "var(--chart-2)", "var(--chart-3)", "var(--chart-4)", "var(--chart-5)"]);

  return {
    scales: { kind: "stacked" as const, xScale, yScale, colorScale },
    axes: {
      x: bandAxis(xScale, geometry.plot.width, reflow),
      y: linearAxis(yScale, geometry.plot.height, reflow),
    },
  };
}

function temporalScales(
  temporal: Extract<ShapedEncoding, { readonly encoding: "line" | "area" }>["temporal"],
  geometry: ChartGeometry,
  reflow: ChartReflow,
) {
  const dates = [
    ...temporal.segments.flatMap((segment) => segment.map((point) => point.x)),
    ...temporal.gapRegions.flatMap((region) => [region.start, region.end]),
    ...temporal.seriesGapRegions.flatMap((region) => [region.start, region.end]),
  ];
  const fallbackDate = new Date(0);
  const start = minDate(dates) ?? fallbackDate;
  const end = maxDate(dates) ?? new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000);
  const xScale = scaleUtc().domain(start.getTime() === end.getTime() ? [start, new Date(end.getTime() + 7 * 24 * 60 * 60 * 1000)] : [start, end]).range([0, geometry.plot.width]);
  const yScale = scaleLinear()
    .domain(zeroInclusiveDomain(temporal.segments.flatMap((segment) => segment.map((point) => point.y))))
    .range([geometry.plot.height, 0]);

  return {
    scales: { kind: "temporal" as const, xScale, yScale },
    axes: {
      x: temporalAxis(xScale, geometry.plot.width, reflow),
      y: linearAxis(yScale, geometry.plot.height, reflow),
    },
  };
}

function histogramScales(
  histogram: Extract<ShapedEncoding, { readonly encoding: "histogram" }>["histogram"],
  geometry: ChartGeometry,
  reflow: ChartReflow,
  theme: ChartTheme,
) {
  const layout = histogramLayout(geometry, histogram.panels.length, reflow, theme);
  const referenceBins = histogram.panels[0]?.bins ?? declaredHistogramBins(theme);
  const xScale = scaleBand<string>()
    .domain(referenceBins.map(histogramBinKey))
    .range([0, layout.frames[0]?.width ?? geometry.plot.width])
    .paddingInner(theme.bandPadding.inner)
    .paddingOuter(theme.bandPadding.outer);
  const yScale = scaleLinear()
    .domain([histogram.yDomain[0], positiveDomain([histogram.yDomain[1]])])
    .range([layout.plotHeight, 0]);

  return {
    scales: { kind: "histogram" as const, xScale, yScale, layout },
    axes: {
      y: linearAxis(yScale, layout.plotHeight, reflow),
    },
  };
}

function geometryForShaped(
  baseGeometry: ChartGeometry,
  shaped: ShapedEncoding,
  reflow: ChartReflow,
  theme: ChartTheme,
): ChartGeometry {
  if (shaped.encoding !== "histogram") {
    return categoricalLabelGeometry(baseGeometry, shaped, theme);
  }

  const layout = histogramLayout(baseGeometry, shaped.histogram.panels.length, reflow, theme);
  const viewBoxHeight = baseGeometry.margins.top + histogramContentHeight(layout) + baseGeometry.margins.bottom;

  return {
    ...baseGeometry,
    viewBox: {
      ...baseGeometry.viewBox,
      height: viewBoxHeight,
      aspectRatio: baseGeometry.viewBox.width / viewBoxHeight,
    },
    plot: { ...baseGeometry.plot, height: histogramContentHeight(layout) },
  };
}

/**
 * String category axes need more than the theme's baseline margin: reserve a
 * conservative text width, a full chart gutter, and a small edge safety pad.
 * The labels themselves are right-aligned at that gutter in CompositionAxes.
 */
function categoricalLabelGeometry(
  baseGeometry: ChartGeometry,
  shaped: Exclude<ShapedEncoding, { readonly encoding: "histogram" }>,
  theme: ChartTheme,
): ChartGeometry {
  if (shaped.encoding !== "ranked_bars" && shaped.encoding !== "diverging_bars") {
    return baseGeometry;
  }

  const longestLabel = shaped.bars.reduce((longest, bar) => (
    Math.max(longest, bar.label.length)
  ), 0);
  const labelWidth = longestLabel * 8;
  const left = Math.max(baseGeometry.margins.left, labelWidth + theme.gutters.y + 8);

  if (left === baseGeometry.margins.left) return baseGeometry;

  return {
    ...baseGeometry,
    margins: { ...baseGeometry.margins, left },
    plot: {
      ...baseGeometry.plot,
      width: Math.max(0, baseGeometry.viewBox.width - left - baseGeometry.margins.right),
    },
  };
}

function histogramLayout(
  geometry: ChartGeometry,
  panelCount: number,
  reflow: ChartReflow,
  theme: ChartTheme,
): HistogramLayout {
  const panelsPerRow = Math.max(1, Math.floor(Math.min(panelCount || 1, reflow.panelsPerRow)) || 1);
  const rowCount = Math.max(1, Math.ceil(panelCount / panelsPerRow));
  const panel = theme.histogramPanel;
  // A one-row chart preserves the static theme's default viewBox. Extra rows
  // reserve title and declared-edge-label gutters before the next plot begins.
  const plotHeight = rowCount === 1
    ? geometry.plot.height
    : Math.max(0, panel.minimumPlotHeight);
  const width = Math.max(0, (geometry.plot.width - geometry.gutters.panel * (panelsPerRow - 1)) / panelsPerRow);
  const rowAdvance = plotHeight + panel.edgeLabelGutter + panel.titleGutter + geometry.gutters.panel;
  const frames = Array.from({ length: panelCount }, (_, index) => ({
    x: (index % panelsPerRow) * (width + geometry.gutters.panel),
    y: Math.floor(index / panelsPerRow) * rowAdvance,
    width,
    height: plotHeight,
    row: Math.floor(index / panelsPerRow),
    column: index % panelsPerRow,
  }));
  const contentHeight = rowCount === 1 ? plotHeight : plotHeight + (rowCount - 1) * rowAdvance;

  return {
    frames,
    rowCount,
    panelsPerRow,
    plotHeight,
    titleGutter: panel.titleGutter,
    edgeLabelGutter: panel.edgeLabelGutter,
    // `reflow.width` is the rendered SVG width after ResizeObserver; compare
    // CSS pixels so a narrow chart is not made scrollable unnecessarily.
    scrollable: geometry.viewBox.width > 0
      && ((contentHeight + geometry.margins.top + geometry.margins.bottom) / geometry.viewBox.width) * reflow.width > panel.scrollHeight,
    scrollHeight: panel.scrollHeight,
  };
}

function histogramContentHeight(layout: HistogramLayout): number {
  const last = layout.frames.at(-1);
  return last == null ? layout.plotHeight : last.y + last.height;
}

function chartGeometry(theme: ChartTheme, encoding: Encoding): ChartGeometry {
  const viewBox = theme.viewBox[encoding];
  const width = Math.max(0, viewBox.width - theme.margins.left - theme.margins.right);
  const height = Math.max(0, viewBox.height - theme.margins.top - theme.margins.bottom);

  return {
    viewBox,
    margins: theme.margins,
    gutters: theme.gutters,
    plot: { width, height },
  };
}

function reflowForWidth(theme: ChartTheme, width: number): ChartReflow {
  const breakpoint = matchingBreakpoint(theme.reflowBreakpoints, width);
  return {
    width,
    tickDensity: breakpoint.tickDensity,
    rotateLabels: breakpoint.rotateLabels,
    labelRotation: breakpoint.rotateLabels ? -45 : 0,
    panelsPerRow: breakpoint.panelsPerRow,
  };
}

function matchingBreakpoint(breakpoints: readonly ReflowBreakpoint[], width: number): ReflowBreakpoint {
  const [first, ...rest] = breakpoints;
  if (first == null) throw new Error("Chart theme requires at least one reflow breakpoint");

  return rest.reduce<ReflowBreakpoint>((match, breakpoint) => (
    breakpoint.minWidth <= width ? breakpoint : match
  ), first);
}

function linearAxis(scale: ScaleLinear<number, number>, length: number, reflow: ChartReflow): ChartAxis {
  const ticks = scale.ticks(reflow.tickDensity);
  return {
    ticks: ticks.map((value) => ({ value, label: formatNumber(value) })),
    scale: (value) => (typeof value === "number" ? scale(value) : undefined),
    length,
    labelRotation: reflow.labelRotation,
  };
}

function temporalAxis(scale: ScaleTime<number, number>, length: number, reflow: ChartReflow): ChartAxis {
  const format = utcFormat("%b %-d");
  const ticks = scale.ticks(reflow.tickDensity);
  return {
    ticks: ticks.map((value) => ({ value, label: format(value) })),
    scale: (value) => (value instanceof Date ? scale(value) : undefined),
    length,
    labelRotation: reflow.labelRotation,
  };
}

function bandAxis(scale: ScaleBand<string>, length: number, reflow: ChartReflow): ChartAxis {
  const domain = sampled(scale.domain(), reflow.tickDensity);
  return {
    ticks: domain.map((value) => ({ value, label: value })),
    scale: (value) => {
      if (typeof value !== "string") return undefined;
      const position = scale(value);
      return position == null ? undefined : position + scale.bandwidth() / 2;
    },
    length,
    labelRotation: reflow.labelRotation,
  };
}

function declaredHistogramBins(theme: ChartTheme): readonly HistogramBin[] {
  const edges = theme.histogramEdges;
  const first = edges[0] ?? theme.histogramZeroDay;
  const bounded = [theme.histogramZeroDay, ...edges].slice(0, -1).map((x0, index) => ({
    x0,
    x1: edges[index] ?? null,
    count: 0,
    isZeroDay: false,
  }));

  return [
    { x0: theme.histogramZeroDay, x1: theme.histogramZeroDay, count: 0, isZeroDay: true },
    ...bounded,
    { x0: edges.at(-1) ?? first, x1: null, count: 0, isZeroDay: false },
  ];
}

function orderedKeys(values: readonly string[]): string[] {
  return [...new Set(values)];
}

function positiveDomain(values: readonly number[]): number {
  return Math.max(1, max(values.filter(Number.isFinite)) ?? 0);
}

/** Keeps temporal zero baselines visible while retaining legitimate negative deltas. */
function zeroInclusiveDomain(values: readonly number[]): [number, number] {
  const finite = values.filter(Number.isFinite);
  const minimum = Math.min(0, ...finite);
  const maximum = Math.max(0, ...finite);
  return minimum === maximum ? [0, 1] : [minimum, maximum];
}

function sampled(values: readonly string[], count: number): readonly string[] {
  if (values.length <= count) return values;
  const step = (values.length - 1) / Math.max(1, count - 1);
  return Array.from({ length: count }, (_, index) => values[Math.round(index * step)] ?? "");
}

function minDate(dates: readonly Date[]): Date | undefined {
  return dates.reduce<Date | undefined>((minimum, date) => (
    minimum == null || date < minimum ? date : minimum
  ), undefined);
}

function maxDate(dates: readonly Date[]): Date | undefined {
  return dates.reduce<Date | undefined>((maximum, date) => (
    maximum == null || date > maximum ? date : maximum
  ), undefined);
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 2 }).format(value);
}

function temporalGapDescription(shaped: ShapedEncoding): string | undefined {
  if (shaped.encoding !== "line" && shaped.encoding !== "area") return undefined;
  if (shaped.temporal.seriesGapRegions.length === 0) return undefined;

  const gaps = shaped.temporal.seriesGapRegions.map((region) => (
    `${region.label} from ${region.start.toISOString().slice(0, 10)} to ${region.end.toISOString().slice(0, 10)}`
  ));
  return `Partial gaps in collected data: ${gaps.join("; ")}.`;
}

function hasDenominator(result: MeasureResult): result is ClassifiedResult {
  return Object.hasOwn(result, "denominator") && result.denominator !== undefined;
}

function sameReflow(left: ChartReflow, right: ChartReflow): boolean {
  return left.tickDensity === right.tickDensity
    && left.rotateLabels === right.rotateLabels
    && left.panelsPerRow === right.panelsPerRow;
}

function CompositionAxes({
  axes,
  geometry,
  suppressY = false,
}: {
  readonly axes: ChartAxes;
  readonly geometry: ChartGeometry;
  readonly suppressY?: boolean;
}) {
  return (
    <>
      {axes.x == null ? null : (
        <g transform={`translate(0, ${geometry.plot.height})`}>
          <ReflowAxis orientation="bottom" axis={axes.x} />
        </g>
      )}
      {suppressY || axes.y == null ? null : (
        <ReflowAxis orientation="left" axis={axes.y} labelOffset={geometry.gutters.y} />
      )}
    </>
  );
}

function ReflowAxis({
  orientation,
  axis,
  labelOffset,
}: {
  readonly orientation: "bottom" | "left";
  readonly axis: ChartAxis;
  readonly labelOffset?: number;
}) {
  if (orientation !== "bottom" || axis.labelRotation === 0) {
    return <Axis orientation={orientation} labelOffset={labelOffset} {...axis} />;
  }

  return (
    <g className="stroke-muted-foreground fill-muted-foreground text-xs">
      <line x1={0} x2={axis.length} y1={0} y2={0} />
      {axis.ticks.map((tick) => {
        const position = axis.scale(tick.value);
        if (position === undefined) return null;

        return (
          <g key={tick.label} transform={`translate(${position}, 0)`}>
            <line y2={6} />
            <text
              y={18}
              textAnchor="end"
              transform={`rotate(${axis.labelRotation})`}
              className="stroke-none"
            >
              {tick.label}
            </text>
          </g>
        );
      })}
    </g>
  );
}
