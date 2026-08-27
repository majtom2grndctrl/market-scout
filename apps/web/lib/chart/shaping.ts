import type { MeasureResult, MeasureRow } from "@/lib/db/measure-engine";
import type { Encoding, Grouping, Measure } from "@/lib/composition";

import { chartTheme, type ChartTheme } from "./chart-theme";

/** A distribution measure the histogram encoding can represent without aggregating it. */
export type HistogramMeasure = Extract<Measure, "age" | "lifespan">;

export interface HistogramBin {
  readonly x0: number;
  readonly x1: number | null;
  readonly count: number;
  readonly isZeroDay: boolean;
}

export interface HistogramPanel {
  readonly group: string;
  readonly bins: readonly HistogramBin[];
}

export interface HistogramPlot {
  readonly panels: readonly HistogramPanel[];
  readonly xDomain: readonly number[];
  readonly yDomain: readonly [number, number];
}

export type BarVariant = "normal" | "unmapped" | "no-data";

export interface ShapedBar {
  readonly key: string;
  readonly value: number;
  readonly label: string;
  readonly variant: BarVariant;
}

export interface StackedCell {
  readonly key: string;
  readonly series: string;
  readonly y0: number;
  readonly y1: number;
  readonly variant: BarVariant;
}

export interface PlotPoint {
  readonly x: Date;
  readonly y: number;
}

export type Segment = readonly PlotPoint[];

export interface GapRegion {
  readonly start: Date;
  readonly end: Date;
}

/** A failed interval belonging to one series, not every series in the chart. */
export interface SeriesGapRegion extends GapRegion {
  /** Opaque series identity, stable only within this shaped result. */
  readonly series: string;
  /** Human-readable values of the non-week groupings for chart narration. */
  readonly label: string;
}

export interface PlottedSeries {
  readonly segments: readonly Segment[];
  /** Failed intervals shared by every displayed series, safe to show as a full-height band. */
  readonly gapRegions: readonly GapRegion[];
  /** Failed intervals limited to one series, retained without obscuring other series' data. */
  readonly seriesGapRegions: readonly SeriesGapRegion[];
}

export type ShapedEncoding =
  | { readonly encoding: "histogram"; readonly histogram: HistogramPlot }
  | { readonly encoding: "line" | "area"; readonly temporal: PlottedSeries }
  | {
      readonly encoding: "ranked_bars" | "diverging_bars";
      readonly bars: readonly ShapedBar[];
    }
  | { readonly encoding: "stacked_bar"; readonly cells: readonly StackedCell[] }
  | { readonly encoding: "table"; readonly rows: readonly MeasureRow[] };

/**
 * Bins posting-level age or lifespan values against the declared, fixed edges.
 * The x and y domains intentionally live above the panels so every small
 * multiple compares against the same scales.
 */
export function shapeHistogram(result: MeasureResult, theme: ChartTheme = chartTheme): HistogramPlot {
  const grouping = result.groupBy[0];
  const panels = new Map<string, number[]>();

  if (grouping == null) panels.set("", emptyHistogramCounts(theme));

  for (const row of result.rows) {
    const group = grouping == null ? "" : groupKey(row, grouping);
    const counts = panels.get(group) ?? emptyHistogramCounts(theme);
    counts[histogramBinIndex(row.value, theme)] += 1;
    panels.set(group, counts);
  }

  const shapedPanels = [...panels.entries()].map(([group, counts]) => ({
    group,
    bins: histogramBins(counts, theme),
  }));
  const maximumCount = shapedPanels.reduce(
    (maximum, panel) => Math.max(maximum, ...panel.bins.map((bin) => bin.count)),
    0,
  );

  return {
    panels: shapedPanels,
    xDomain: [theme.histogramZeroDay, ...theme.histogramEdges],
    yDomain: [0, maximumCount],
  };
}

/** Shapes result rows for either ordinary categorical bar encoding. */
export function shapeBars(result: MeasureResult): readonly ShapedBar[] {
  const grouping = result.groupBy[0];

  return result.rows.map((row) => {
    const key = grouping == null ? "" : groupKey(row, grouping);
    const variant = barVariant(row);

    return {
      key,
      label: key,
      // A gap's engine value is deliberately not a drawable zero. The mark
      // consumes its no-data variant and treats this placeholder as no value.
      value: variant === "no-data" ? 0 : row.value,
      variant,
    };
  });
}

/**
 * Converts globally-normalized share rows into cumulative stack offsets.
 * It deliberately accumulates the engine values as received: there is no
 * panel- or key-level normalization in the chart layer.
 */
export function shapeStackedBars(result: MeasureResult): readonly StackedCell[] {
  const [keyGrouping, seriesGrouping] = result.groupBy;
  let offset = 0;

  return result.rows.map((row) => {
    const variant = barVariant(row);
    const value = variant === "no-data" ? 0 : row.value;
    const cell = {
      key: keyGrouping == null ? "" : groupKey(row, keyGrouping),
      series: seriesGrouping == null ? "" : groupKey(row, seriesGrouping),
      y0: offset,
      y1: offset + value,
      variant,
    };
    offset = cell.y1;
    return cell;
  });
}

/**
 * Splits a weekly series before any mark sees it. Every failed collection week
 * is absent from its line/area path. Only an interval failed for every displayed
 * series becomes a full-height band; the others retain their series identity.
 */
export function shapeTemporal(result: MeasureResult): PlottedSeries {
  const segments: PlotPoint[][] = [];
  const seriesGroupings = result.groupBy.filter((grouping) => grouping !== "week");
  const seriesLabels = new Map<string, string>();
  const gapsByInterval = new Map<number, Map<string, SeriesGapRegion>>();
  let current: PlotPoint[] = [];
  let currentSeries: string | undefined;

  for (const row of result.rows) {
    const week = utcWeek(row);
    const series = seriesKey(row, seriesGroupings);
    seriesLabels.set(series, seriesLabel(row, seriesGroupings));

    if (row.gap === true) {
      if (current.length > 0) segments.push(current);
      current = [];
      currentSeries = undefined;
      const region = {
        start: week,
        end: new Date(week.getTime() + WEEK_MS),
        series,
        label: seriesLabels.get(series) ?? "this series",
      };
      const gapsForWeek = gapsByInterval.get(week.getTime()) ?? new Map<string, SeriesGapRegion>();
      gapsForWeek.set(series, region);
      gapsByInterval.set(week.getTime(), gapsForWeek);
      continue;
    }

    if (current.length > 0 && currentSeries !== series) {
      segments.push(current);
      current = [];
    }

    currentSeries = series;
    current.push({ x: week, y: row.value });
  }

  if (current.length > 0) segments.push(current);

  const gapRegions: GapRegion[] = [];
  const seriesGapRegions: SeriesGapRegion[] = [];

  for (const gapsForWeek of gapsByInterval.values()) {
    if (gapsForWeek.size === seriesLabels.size) {
      const [region] = gapsForWeek.values();
      if (region != null) gapRegions.push({ start: region.start, end: region.end });
      continue;
    }

    seriesGapRegions.push(...gapsForWeek.values());
  }

  return { segments, gapRegions, seriesGapRegions };
}

/** Selects the declared representation for a validated composition result. */
export function shapeForEncoding(
  result: MeasureResult,
  encoding: Encoding,
  theme: ChartTheme = chartTheme,
): ShapedEncoding {
  switch (encoding) {
    case "histogram":
      return { encoding: "histogram", histogram: shapeHistogram(result, theme) };
    case "line":
    case "area":
      return { encoding, temporal: shapeTemporal(result) };
    case "ranked_bars":
    case "diverging_bars":
      return { encoding, bars: shapeBars(result) };
    case "stacked_bar":
      return { encoding: "stacked_bar", cells: shapeStackedBars(result) };
    case "table":
      return { encoding: "table", rows: result.rows };
  }
}

function emptyHistogramCounts(theme: ChartTheme): number[] {
  return Array.from({ length: theme.histogramEdges.length + 2 }, () => 0);
}

function histogramBins(counts: readonly number[], theme: ChartTheme): readonly HistogramBin[] {
  const [firstEdge, ...remainingEdges] = theme.histogramEdges;
  if (firstEdge == null) return [];

  const zeroDay: HistogramBin = {
    x0: theme.histogramZeroDay,
    x1: theme.histogramZeroDay,
    count: counts[0] ?? 0,
    isZeroDay: true,
  };
  const bounded = [theme.histogramZeroDay, firstEdge, ...remainingEdges].slice(0, -1).map(
    (x0, index) => ({
      x0,
      x1: theme.histogramEdges[index] ?? null,
      count: counts[index + 1] ?? 0,
      isZeroDay: false,
    }),
  );
  const openTop: HistogramBin = {
    x0: theme.histogramEdges.at(-1) ?? theme.histogramZeroDay,
    x1: null,
    count: counts.at(-1) ?? 0,
    isZeroDay: false,
  };

  return [zeroDay, ...bounded, openTop];
}

function histogramBinIndex(value: number, theme: ChartTheme): number {
  if (!Number.isFinite(value) || value < theme.histogramZeroDay) {
    throw new RangeError("Histogram values must be finite, non-negative day counts");
  }

  if (value === theme.histogramZeroDay) return 0;

  for (let index = 0; index < theme.histogramEdges.length; index += 1) {
    const edge = theme.histogramEdges[index];
    if (edge != null && value > theme.histogramZeroDay && value < edge) return index + 1;
  }

  return theme.histogramEdges.length + 1;
}

function barVariant(row: MeasureRow): BarVariant {
  if (row.gap === true) return "no-data";
  return Object.values(row.keys).includes("unmapped") ? "unmapped" : "normal";
}

function groupKey(row: MeasureRow, grouping: Grouping): string {
  return row.keys[grouping] ?? "";
}

function seriesKey(row: MeasureRow, groupings: readonly Grouping[]): string {
  return groupings.map((grouping) => groupKey(row, grouping)).join("\u0000");
}

function seriesLabel(row: MeasureRow, groupings: readonly Grouping[]): string {
  if (groupings.length === 0) return "all series";
  return groupings.map((grouping) => groupKey(row, grouping)).join(", ");
}

function utcWeek(row: MeasureRow): Date {
  const week = row.keys.week;
  if (week == null) throw new Error("A temporal result row requires a UTC week key");

  const date = new Date(`${week}T00:00:00.000Z`);
  if (Number.isNaN(date.getTime())) throw new Error(`Invalid UTC week key "${week}"`);
  return date;
}

const WEEK_MS = 7 * 24 * 60 * 60 * 1000;
