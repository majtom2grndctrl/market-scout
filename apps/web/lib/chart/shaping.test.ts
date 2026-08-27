import { describe, expect, it } from "vitest";

import type { MeasureResult } from "@/lib/db/measure-engine";

import { mergeChartTheme } from "./chart-theme";
import { shapeBars, shapeForEncoding, shapeHistogram, shapeStackedBars, shapeTemporal } from "./shaping";

function result(overrides: Partial<MeasureResult>): MeasureResult {
  return {
    measure: "count",
    cohort: "open",
    groupBy: [],
    rows: [],
    ...overrides,
  };
}

describe("shapeHistogram", () => {
  it("keeps the exact zero-day spike separate and uses fixed half-open seven-day bins", () => {
    const plot = shapeHistogram(
      result({
        measure: "age",
        rows: [0, 3.5, 7, 50, 69.999, 70, 90].map((value) => ({ keys: {}, value })),
      }),
    );

    expect(plot.panels).toHaveLength(1);
    expect(plot.panels[0]?.bins).toEqual([
      { x0: 0, x1: 0, count: 1, isZeroDay: true },
      { x0: 0, x1: 7, count: 1, isZeroDay: false },
      { x0: 7, x1: 14, count: 1, isZeroDay: false },
      { x0: 14, x1: 21, count: 0, isZeroDay: false },
      { x0: 21, x1: 28, count: 0, isZeroDay: false },
      { x0: 28, x1: 35, count: 0, isZeroDay: false },
      { x0: 35, x1: 42, count: 0, isZeroDay: false },
      { x0: 42, x1: 49, count: 0, isZeroDay: false },
      { x0: 49, x1: 56, count: 1, isZeroDay: false },
      { x0: 56, x1: 63, count: 0, isZeroDay: false },
      { x0: 63, x1: 70, count: 1, isZeroDay: false },
      { x0: 70, x1: null, count: 2, isZeroDay: false },
    ]);
    expect(plot.xDomain).toEqual([0, 7, 14, 21, 28, 35, 42, 49, 56, 63, 70]);
  });

  it("emits every grouped panel with the same x and y domains", () => {
    const plot = shapeHistogram(
      result({
        measure: "lifespan",
        groupBy: ["company"],
        rows: [
          { keys: { company: "Northstar" }, value: 0 },
          { keys: { company: "Northstar" }, value: 3 },
          { keys: { company: "Trailhead" }, value: 10 },
        ],
      }),
    );

    expect(plot.panels.map((panel) => panel.group)).toEqual(["Northstar", "Trailhead"]);
    expect(plot.yDomain).toEqual([0, 1]);
    expect(plot.xDomain).toEqual([0, 7, 14, 21, 28, 35, 42, 49, 56, 63, 70]);
  });

  it("uses custom declared edges consistently for bins and domains", () => {
    const plot = shapeHistogram(
      result({
        measure: "age",
        rows: [0, 2, 5, 9.9, 10].map((value) => ({ keys: {}, value })),
      }),
      mergeChartTheme({ histogramEdges: [5, 10] }),
    );

    expect(plot.panels[0]?.bins).toEqual([
      { x0: 0, x1: 0, count: 1, isZeroDay: true },
      { x0: 0, x1: 5, count: 1, isZeroDay: false },
      { x0: 5, x1: 10, count: 2, isZeroDay: false },
      { x0: 10, x1: null, count: 1, isZeroDay: false },
    ]);
    expect(plot.xDomain).toEqual([0, 5, 10]);
  });
});

describe("shapeTemporal", () => {
  it("breaks paths at every gap and represents each missing week in UTC domain units", () => {
    const temporal = shapeTemporal(
      result({
        measure: "rate",
        groupBy: ["week"],
        rows: [
          { keys: { week: "2026-01-05" }, value: 2 },
          { keys: { week: "2026-01-12" }, value: 99, gap: true },
          { keys: { week: "2026-01-19" }, value: 3 },
        ],
      }),
    );

    expect(temporal.segments).toEqual([
      [{ x: new Date("2026-01-05T00:00:00.000Z"), y: 2 }],
      [{ x: new Date("2026-01-19T00:00:00.000Z"), y: 3 }],
    ]);
    expect(temporal.gapRegions).toEqual([
      {
        start: new Date("2026-01-12T00:00:00.000Z"),
        end: new Date("2026-01-19T00:00:00.000Z"),
      },
    ]);
    expect(temporal.seriesGapRegions).toEqual([]);
  });

  it("keeps adjacent ordered series separate without re-sorting them", () => {
    const temporal = shapeTemporal(
      result({
        measure: "rate",
        groupBy: ["company", "week"],
        rows: [
          { keys: { company: "Northstar", week: "2026-01-05" }, value: 2 },
          { keys: { company: "Northstar", week: "2026-01-12" }, value: 3 },
          { keys: { company: "Trailhead", week: "2026-01-05" }, value: 4 },
        ],
      }),
    );

    expect(temporal.segments).toEqual([
      [
        { x: new Date("2026-01-05T00:00:00.000Z"), y: 2 },
        { x: new Date("2026-01-12T00:00:00.000Z"), y: 3 },
      ],
      [{ x: new Date("2026-01-05T00:00:00.000Z"), y: 4 }],
    ]);
    expect(temporal.gapRegions).toEqual([]);
    expect(temporal.seriesGapRegions).toEqual([]);
  });

  it("keeps a company-only failed week out of the full-height gap bands", () => {
    const temporal = shapeTemporal(
      result({
        measure: "rate",
        groupBy: ["company", "week"],
        rows: [
          { keys: { company: "Northstar", week: "2026-01-05" }, value: 2 },
          { keys: { company: "Northstar", week: "2026-01-12" }, value: 0, gap: true },
          { keys: { company: "Northstar", week: "2026-01-19" }, value: 3 },
          { keys: { company: "Trailhead", week: "2026-01-05" }, value: 4 },
          { keys: { company: "Trailhead", week: "2026-01-12" }, value: 5 },
          { keys: { company: "Trailhead", week: "2026-01-19" }, value: 6 },
        ],
      }),
    );

    expect(temporal.segments).toEqual([
      [{ x: new Date("2026-01-05T00:00:00.000Z"), y: 2 }],
      [{ x: new Date("2026-01-19T00:00:00.000Z"), y: 3 }],
      [
        { x: new Date("2026-01-05T00:00:00.000Z"), y: 4 },
        { x: new Date("2026-01-12T00:00:00.000Z"), y: 5 },
        { x: new Date("2026-01-19T00:00:00.000Z"), y: 6 },
      ],
    ]);
    expect(temporal.gapRegions).toEqual([]);
    expect(temporal.seriesGapRegions).toEqual([
      {
        start: new Date("2026-01-12T00:00:00.000Z"),
        end: new Date("2026-01-19T00:00:00.000Z"),
        series: "Northstar",
        label: "Northstar",
      },
    ]);
  });

  it("deduplicates a scope-wide failed week emitted once for every series", () => {
    const temporal = shapeTemporal(
      result({
        measure: "share",
        groupBy: ["market", "week"],
        rows: [
          { keys: { market: "New York", week: "2026-01-05" }, value: 0.5 },
          { keys: { market: "New York", week: "2026-01-12" }, value: 0, gap: true },
          { keys: { market: "New York", week: "2026-01-19" }, value: 0.5 },
          { keys: { market: "Seattle", week: "2026-01-05" }, value: 0.5 },
          { keys: { market: "Seattle", week: "2026-01-12" }, value: 0, gap: true },
          { keys: { market: "Seattle", week: "2026-01-19" }, value: 0.5 },
        ],
      }),
    );

    expect(temporal.gapRegions).toEqual([
      {
        start: new Date("2026-01-12T00:00:00.000Z"),
        end: new Date("2026-01-19T00:00:00.000Z"),
      },
    ]);
    expect(temporal.seriesGapRegions).toEqual([]);
  });
});

describe("bar shaping", () => {
  it("preserves row order and distinguishes unmapped categories from no-data weeks", () => {
    const bars = shapeBars(
      result({
        groupBy: ["market"],
        rows: [
          { keys: { market: "seattle" }, value: 4 },
          { keys: { market: "unmapped" }, value: 3 },
          { keys: { market: "portland" }, value: 88, gap: true },
        ],
      }),
    );

    expect(bars).toEqual([
      { key: "seattle", label: "seattle", value: 4, variant: "normal" },
      { key: "unmapped", label: "unmapped", value: 3, variant: "unmapped" },
      { key: "portland", label: "portland", value: 0, variant: "no-data" },
    ]);
  });

  it("accumulates globally normalized shares in input order without re-normalizing", () => {
    const cells = shapeStackedBars(
      result({
        measure: "share",
        groupBy: ["company", "seniority"],
        rows: [
          { keys: { company: "Northstar", seniority: "senior" }, value: 0.2 },
          { keys: { company: "Northstar", seniority: "junior" }, value: 0.3 },
          { keys: { company: "Trailhead", seniority: "senior" }, value: 0.5 },
        ],
      }),
    );

    expect(cells).toEqual([
      { key: "Northstar", series: "senior", y0: 0, y1: 0.2, variant: "normal" },
      { key: "Northstar", series: "junior", y0: 0.2, y1: 0.5, variant: "normal" },
      { key: "Trailhead", series: "senior", y0: 0.5, y1: 1, variant: "normal" },
    ]);
    expect(cells.at(-1)?.y1).toBeCloseTo(1);
  });
});

describe("shapeForEncoding", () => {
  it("takes the grammar encoding explicitly because MeasureResult has no encoding field", () => {
    const shaped = shapeForEncoding(
      result({ groupBy: ["company"], rows: [{ keys: { company: "Northstar" }, value: 4 }] }),
      "ranked_bars",
    );

    expect(shaped).toEqual({
      encoding: "ranked_bars",
      bars: [{ key: "Northstar", label: "Northstar", value: 4, variant: "normal" }],
    });
  });
});
