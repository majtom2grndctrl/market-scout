import type { Encoding } from "@/lib/composition";

export interface ChartMargin {
  readonly top: number;
  readonly right: number;
  readonly bottom: number;
  readonly left: number;
}

export interface ChartGutter {
  readonly x: number;
  readonly y: number;
  readonly panel: number;
}

export interface BandPadding {
  readonly inner: number;
  readonly outer: number;
}

export interface ChartViewBox {
  readonly width: number;
  readonly height: number;
  readonly aspectRatio: number;
}

export interface ReflowBreakpoint {
  readonly minWidth: number;
  readonly tickDensity: number;
  readonly rotateLabels: boolean;
  readonly panelsPerRow: number;
}

/** Geometry reserved for an honest, readable histogram small multiple. */
export interface HistogramPanelTheme {
  /** Bar drawing area; labels and a group title sit outside this region. */
  readonly minimumPlotHeight: number;
  /** Space between a row's edge labels and the following row's title. */
  readonly titleGutter: number;
  /** Space needed below a panel's bars for its declared-edge labels. */
  readonly edgeLabelGutter: number;
  /** A very tall collection of panels scrolls within this visual height. */
  readonly scrollHeight: number;
}

export interface ChartTheme {
  readonly margins: ChartMargin;
  readonly gutters: ChartGutter;
  readonly bandPadding: BandPadding;
  readonly viewBox: Readonly<Record<Encoding, ChartViewBox>>;
  readonly histogramZeroDay: number;
  readonly histogramEdges: readonly number[];
  readonly histogramPanel: HistogramPanelTheme;
  readonly reflowBreakpoints: readonly ReflowBreakpoint[];
}

export type ChartThemeOverrides = DeepPartial<ChartTheme>;

type DeepPartial<Value> = Value extends readonly unknown[]
  ? Value
  : Value extends object
    ? { readonly [Key in keyof Value]?: DeepPartial<Value[Key]> }
    : Value;

const defaultChartTheme = {
  margins: {
    top: 24,
    right: 24,
    bottom: 48,
    left: 56,
  },
  gutters: {
    x: 24,
    y: 24,
    panel: 32,
  },
  bandPadding: {
    inner: 0.2,
    outer: 0.1,
  },
  viewBox: {
    ranked_bars: { width: 720, height: 420, aspectRatio: 720 / 420 },
    line: { width: 720, height: 400, aspectRatio: 720 / 400 },
    area: { width: 720, height: 400, aspectRatio: 720 / 400 },
    diverging_bars: { width: 720, height: 420, aspectRatio: 720 / 420 },
    histogram: { width: 720, height: 440, aspectRatio: 720 / 440 },
    stacked_bar: { width: 720, height: 420, aspectRatio: 720 / 420 },
    table: { width: 720, height: 320, aspectRatio: 720 / 320 },
  },
  histogramZeroDay: 0,
  histogramEdges: [7, 14, 21, 28, 35, 42, 49, 56, 63, 70],
  histogramPanel: {
    minimumPlotHeight: 180,
    titleGutter: 18,
    edgeLabelGutter: 34,
    scrollHeight: 720,
  },
  reflowBreakpoints: [
    { minWidth: 0, tickDensity: 4, rotateLabels: true, panelsPerRow: 1 },
    { minWidth: 640, tickDensity: 6, rotateLabels: false, panelsPerRow: 2 },
    { minWidth: 1024, tickDensity: 8, rotateLabels: false, panelsPerRow: 3 },
  ],
} as const satisfies ChartTheme;

// Geometry is shared by shaping and rendering. Freezing makes accidental
// mutation fail close to its source while mergeChartTheme returns fresh values.
export const chartTheme: ChartTheme = freeze(defaultChartTheme);

/**
 * Returns chart geometry with a chart-local override, without changing the
 * shared theme or retaining mutable nested objects from either input.
 */
export function mergeChartTheme(overrides: ChartThemeOverrides = {}): ChartTheme {
  return mergeThemeValue<ChartTheme>(chartTheme, overrides);
}

function mergeThemeValue<Value>(base: Value, override: DeepPartial<Value> | undefined): Value {
  if (override === undefined) {
    return cloneThemeValue(base);
  }

  if (Array.isArray(override)) {
    return override.map((value) => cloneThemeValue(value)) as Value;
  }

  if (isRecord(base) && isRecord(override)) {
    const merged: Record<string, unknown> = {};

    for (const [key, value] of Object.entries(base)) {
      merged[key] = mergeThemeValue(value, override[key]);
    }

    return merged as Value;
  }

  return override as Value;
}

function cloneThemeValue<Value>(value: Value): Value {
  if (Array.isArray(value)) {
    return value.map((item) => cloneThemeValue(item)) as Value;
  }

  if (isRecord(value)) {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, cloneThemeValue(item)]),
    ) as Value;
  }

  return value;
}

function freeze<Value>(value: Value): Value {
  if (Array.isArray(value)) {
    value.forEach(freeze);
  } else if (isRecord(value)) {
    Object.values(value).forEach(freeze);
  }

  return Object.freeze(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
