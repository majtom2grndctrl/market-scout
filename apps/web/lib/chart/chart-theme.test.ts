import { describe, expect, it } from "vitest";

import { chartTheme, mergeChartTheme } from "./chart-theme";

describe("chartTheme", () => {
  it("declares the fixed histogram edges for the zero-day, seven-day, and open buckets", () => {
    expect(chartTheme.histogramZeroDay).toBe(0);
    expect(chartTheme.histogramEdges).toEqual([7, 14, 21, 28, 35, 42, 49, 56, 63, 70]);

    const ranges = [0, ...chartTheme.histogramEdges].map((edge, index, edges) => ({
      start: edge,
      end: edges[index + 1] ?? null,
    }));

    expect(ranges).toEqual([
      { start: 0, end: 7 },
      { start: 7, end: 14 },
      { start: 14, end: 21 },
      { start: 21, end: 28 },
      { start: 28, end: 35 },
      { start: 35, end: 42 },
      { start: 42, end: 49 },
      { start: 49, end: 56 },
      { start: 56, end: 63 },
      { start: 63, end: 70 },
      { start: 70, end: null },
    ]);
    expect(chartTheme.histogramPanel.minimumPlotHeight).toBeGreaterThan(0);
  });

  it("deep-merges local overrides without mutating or sharing nested default values", () => {
    const override = {
      margins: { left: 72 },
      viewBox: { line: { height: 360 } },
      reflowBreakpoints: [
        { minWidth: 0, tickDensity: 3, rotateLabels: true, panelsPerRow: 1 },
      ],
      histogramPanel: { minimumPlotHeight: 144 },
    };

    const themed = mergeChartTheme(override);

    expect(themed.margins).toEqual({ top: 24, right: 24, bottom: 48, left: 72 });
    expect(themed.viewBox.line).toEqual({ width: 720, height: 360, aspectRatio: 720 / 400 });
    expect(themed.viewBox.area).toEqual(chartTheme.viewBox.area);
    expect(themed.reflowBreakpoints).toEqual(override.reflowBreakpoints);
    expect(themed.histogramPanel).toEqual({
      minimumPlotHeight: 144,
      titleGutter: 18,
      edgeLabelGutter: 34,
      scrollHeight: 720,
    });
    expect(themed.margins).not.toBe(chartTheme.margins);
    expect(themed.viewBox.line).not.toBe(chartTheme.viewBox.line);
    expect(themed.reflowBreakpoints).not.toBe(override.reflowBreakpoints);
    expect(chartTheme.margins.left).toBe(56);
    expect(chartTheme.viewBox.line.height).toBe(400);
  });
});
