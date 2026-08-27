import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  CompositionChart,
  type ChartRenderContext,
  type CompositionChartProps,
  type FullCorpusResult,
} from "./composition-chart";
import { LineChart } from "./encodings/line-chart";

describe("CompositionChart temporal rendering", () => {
  it("keeps a negative temporal delta in the y-scale domain", () => {
    const result = {
      measure: "delta",
      cohort: "open",
      groupBy: ["week"],
      rows: [
        { keys: { week: "2026-01-05" }, value: -8 },
        { keys: { week: "2026-01-12" }, value: 3 },
      ],
    } satisfies FullCorpusResult;

    const props: CompositionChartProps = {
      encoding: "line",
      result,
      children: (context: ChartRenderContext) => {
        if (context.scales.kind !== "temporal") return null;
        return createElement("text", null, context.scales.yScale.domain().join(","));
      },
    };
    const markup = renderToStaticMarkup(createElement(CompositionChart, props));

    expect(markup).toContain("-8,3");
  });

  it("marks a series-only gap without painting a full-height no-data band", () => {
    const result = {
      measure: "rate",
      cohort: "all",
      groupBy: ["company", "week"],
      rows: [
        { keys: { company: "Northstar", week: "2026-01-05" }, value: 2 },
        { keys: { company: "Northstar", week: "2026-01-12" }, value: 0, gap: true },
        { keys: { company: "Northstar", week: "2026-01-19" }, value: 3 },
        { keys: { company: "Trailhead", week: "2026-01-05" }, value: 4 },
        { keys: { company: "Trailhead", week: "2026-01-12" }, value: 5 },
        { keys: { company: "Trailhead", week: "2026-01-19" }, value: 6 },
      ],
    } satisfies FullCorpusResult;

    const props: CompositionChartProps = {
      encoding: "line",
      result,
      children: (context: ChartRenderContext) => createElement(LineChart, { context }),
    };
    const markup = renderToStaticMarkup(createElement(CompositionChart, props));

    expect(markup).toContain("Gap in collected data for Northstar");
    expect(markup).toContain("Partial gaps in collected data: Northstar");
    expect(markup).not.toContain("gray marks a gap in collected data");
  });
});
