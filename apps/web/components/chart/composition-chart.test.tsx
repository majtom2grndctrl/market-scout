import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { HistogramChart } from "./encodings/histogram-chart";
import { CompositionChart, type FullCorpusResult } from "./composition-chart";

describe("CompositionChart", () => {
  it("shapes a histogram with its resolved theme", () => {
    const result = {
      measure: "age",
      cohort: "open",
      groupBy: [],
      rows: [
        { keys: {}, value: 0 },
        { keys: {}, value: 5 },
      ],
    } satisfies FullCorpusResult;

    const markup = renderToStaticMarkup(
      <CompositionChart encoding="histogram" result={result} theme={{ histogramEdges: [5, 10] }}>
        {(context) => {
          if (context.shaped.encoding !== "histogram") return null;
          return <text>{context.shaped.histogram.xDomain.join(",")}</text>;
        }}
      </CompositionChart>,
    );

    expect(markup).toContain("0,5,10");
  });

  it("reserves a label gutter for string categories and right-aligns them before the bars", () => {
    const result = {
      measure: "count",
      cohort: "open",
      groupBy: ["week"],
      rows: [
        { keys: { week: "2026-06-01" }, value: 18 },
        { keys: { week: "2026-06-08" }, value: 24 },
      ],
    } satisfies FullCorpusResult;

    const markup = renderToStaticMarkup(
      <CompositionChart encoding="ranked_bars" result={result}>
        {(context) => <text data-categorical-geometry>{`${context.geometry.margins.left}:${context.geometry.plot.width}`}</text>}
      </CompositionChart>,
    );

    // Ten 8px characters, a 24px gutter, and an 8px viewport safety pad.
    expect(markup).toContain('data-categorical-geometry="true">112:584</text>');
    expect(markup).toContain('x="-24"');
  });

  it("keeps narrow grouped histogram panels readable and puts shared y ticks in each row", () => {
    const result = {
      measure: "age",
      cohort: "open",
      groupBy: ["role"],
      rows: Array.from({ length: 8 }, (_, index) => ({
        keys: { role: `Role ${index + 1}` },
        value: index,
      })),
    } satisfies FullCorpusResult;

    const markup = renderToStaticMarkup(
      <CompositionChart
        encoding="histogram"
        result={result}
        theme={{
          histogramPanel: { minimumPlotHeight: 160, scrollHeight: 320 },
          reflowBreakpoints: [{ minWidth: 0, tickDensity: 4, rotateLabels: true, panelsPerRow: 1 }],
        }}
      >
        {(context) => {
          if (context.scales.kind !== "histogram") return null;
          const { layout } = context.scales;
          const hasValidFrames = layout.frames.every((frame) => (
            frame.width >= 0 && frame.height >= layout.plotHeight
          ));

          return (
            <>
              <HistogramChart context={context} />
              <text data-histogram-layout>{`${layout.rowCount}:${hasValidFrames}:${layout.plotHeight}`}</text>
            </>
          );
        }}
      </CompositionChart>,
    );

    expect(markup).toContain('viewBox="0 0 720 1940"');
    expect(markup).toContain('data-chart-scrollable="true"');
    expect(markup.match(/data-histogram-y-axis/g)).toHaveLength(8);
    expect(markup).toContain('data-histogram-layout="true">8:true:160</text>');
    expect(markup).toContain('y="160"');
    expect(markup).not.toContain('height="-');
  });
});
