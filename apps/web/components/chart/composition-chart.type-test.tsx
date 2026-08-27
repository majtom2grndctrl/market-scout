import type { ClassifiedResult } from "./composition-chart";
import type { CompositionChartProps } from "./composition-chart";

const classifiedResult = {
  measure: "count",
  cohort: "open",
  groupBy: ["role"],
  rows: [],
  denominator: { classified: 12, total: 20 },
} as const satisfies ClassifiedResult;

// A known classified slice cannot omit the coverage value that makes its scope legible.
// @ts-expect-error ClassifiedResult selects the CompositionChart arm requiring coverage.
const missingCoverage: CompositionChartProps = {
  encoding: "ranked_bars",
  result: classifiedResult,
};

void missingCoverage;
