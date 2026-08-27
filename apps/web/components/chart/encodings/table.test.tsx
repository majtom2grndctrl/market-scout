import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { CompositionChart, type FullCorpusResult } from "../composition-chart";

import { Table } from "./table";

describe("Table", () => {
  it("renders a failed collection gap as no data rather than its zero placeholder", () => {
    const result = {
      measure: "count",
      cohort: "open",
      groupBy: ["week"],
      rows: [
        { keys: { week: "2026-06-08" }, value: 0 },
        { keys: { week: "2026-06-15" }, value: 0, gap: true },
      ],
    } satisfies FullCorpusResult;

    const markup = renderToStaticMarkup(
      <CompositionChart encoding="table" result={result}>
        {(context) => <Table context={context} />}
      </CompositionChart>,
    );

    expect(markup).toContain("No data (collection failed)");
    expect(markup.match(/>0<\/td>/g)).toHaveLength(1);
  });
});
