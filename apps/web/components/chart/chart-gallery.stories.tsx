import type { Meta, StoryObj } from "@storybook/react";

import type { ChartThemeOverrides } from "@/lib/chart/chart-theme";
import type { Encoding } from "@/lib/composition";
import type { MeasureResult } from "@/lib/db/measure-engine";

import {
  CompositionChart,
  type ClassifiedResult,
  type ChartRenderContext,
  type CompositionChartCoverageProps,
  type Coverage,
  type FullCorpusResult,
} from "./composition-chart";
import { AreaChart } from "./encodings/area-chart";
import { DivergingBars } from "./encodings/diverging-bars";
import { HistogramChart } from "./encodings/histogram-chart";
import { LineChart } from "./encodings/line-chart";
import { RankedBars } from "./encodings/ranked-bars";
import { StackedBar } from "./encodings/stacked-bar";
import { Table } from "./encodings/table";

// These fixtures are representative and throwaway: they deliberately exercise
// chart honesty states without reaching the database or runComposition. The
// future shared deterministic source is demo-fixture-corpus; keep fixtures here
// until that task supplies it.

const weeklyBarWithGap = {
  measure: "count",
  cohort: "open",
  groupBy: ["week"],
  rows: [
    { keys: { week: "2026-06-01" }, value: 18 },
    { keys: { week: "2026-06-08" }, value: 0, gap: true },
    { keys: { week: "2026-06-15" }, value: 24 },
    { keys: { week: "2026-06-22" }, value: 21 },
  ],
} as const satisfies FullCorpusResult;

const fullCorpusMarketBars = {
  measure: "count",
  cohort: "open",
  groupBy: ["market"],
  rows: [
    { keys: { market: "San Francisco" }, value: 42 },
    { keys: { market: "New York" }, value: 31 },
    { keys: { market: "unmapped" }, value: 9 },
  ],
} as const satisfies FullCorpusResult;

const movers = {
  measure: "delta",
  cohort: "open",
  groupBy: ["company"],
  rows: [
    { keys: { company: "Northstar" }, value: 16 },
    { keys: { company: "Orbit" }, value: -11 },
    { keys: { company: "Fieldwork" }, value: 5 },
  ],
} as const satisfies FullCorpusResult;

const weeklyDivergingWithGap = {
  measure: "delta",
  cohort: "open",
  groupBy: ["week"],
  rows: [
    { keys: { week: "2026-06-01" }, value: 16 },
    { keys: { week: "2026-06-08" }, value: 0, gap: true },
    { keys: { week: "2026-06-15" }, value: -11 },
  ],
} as const satisfies FullCorpusResult;

const weeklyStackWithGap = {
  measure: "share",
  cohort: "open",
  groupBy: ["week", "seniority"],
  rows: [
    { keys: { week: "2026-06-01", seniority: "senior" }, value: 0.35 },
    { keys: { week: "2026-06-08", seniority: "senior" }, value: 0, gap: true },
    { keys: { week: "2026-06-15", seniority: "senior" }, value: 0.65 },
  ],
} as const satisfies FullCorpusResult;

const classifiedSeniorityMix = {
  measure: "share",
  cohort: "open",
  groupBy: ["company", "seniority"],
  rows: [
    { keys: { company: "Northstar", seniority: "senior" }, value: 0.48 },
    { keys: { company: "Northstar", seniority: "staff" }, value: 0.03 },
    { keys: { company: "Northstar", seniority: "mid" }, value: 0.31 },
    { keys: { company: "Orbit", seniority: "senior" }, value: 0.18 },
  ],
  denominator: { classified: 41, total: 67 },
} as const satisfies ClassifiedResult;

const lineWithFailedRun = {
  measure: "rate",
  cohort: "all",
  groupBy: ["week"],
  rows: [
    { keys: { week: "2026-06-01" }, value: 9 },
    { keys: { week: "2026-06-08" }, value: 14 },
    { keys: { week: "2026-06-15" }, value: 0, gap: true },
    { keys: { week: "2026-06-22" }, value: 12 },
    { keys: { week: "2026-06-29" }, value: 16 },
  ],
} as const satisfies FullCorpusResult;

const lineWithCompanyFailedRun = {
  measure: "rate",
  cohort: "all",
  groupBy: ["company", "week"],
  rows: [
    { keys: { company: "Northstar", week: "2026-06-01" }, value: 9 },
    { keys: { company: "Northstar", week: "2026-06-08" }, value: 0, gap: true },
    { keys: { company: "Northstar", week: "2026-06-15" }, value: 12 },
    { keys: { company: "Trailhead", week: "2026-06-01" }, value: 6 },
    { keys: { company: "Trailhead", week: "2026-06-08" }, value: 8 },
    { keys: { company: "Trailhead", week: "2026-06-15" }, value: 10 },
  ],
} as const satisfies FullCorpusResult;

const areaWithFailedRun = {
  measure: "count",
  cohort: "open",
  groupBy: ["week"],
  rows: [
    { keys: { week: "2026-06-01" }, value: 28 },
    { keys: { week: "2026-06-08" }, value: 35 },
    { keys: { week: "2026-06-15" }, value: 0, gap: true },
    { keys: { week: "2026-06-22" }, value: 31 },
    { keys: { week: "2026-06-29" }, value: 38 },
  ],
} as const satisfies FullCorpusResult;

const singletonSegmentsAfterFailedRun = {
  measure: "rate",
  cohort: "all",
  groupBy: ["week"],
  rows: [
    { keys: { week: "2026-06-01" }, value: 9 },
    { keys: { week: "2026-06-08" }, value: 0, gap: true },
    { keys: { week: "2026-06-15" }, value: 12 },
  ],
} as const satisfies FullCorpusResult;

const lifespanSmallMultiples = {
  measure: "lifespan",
  cohort: "closed",
  groupBy: ["role"],
  rows: [
    { keys: { role: "Engineering" }, value: 0 },
    { keys: { role: "Engineering" }, value: 0 },
    { keys: { role: "Engineering" }, value: 3 },
    { keys: { role: "Engineering" }, value: 9 },
    { keys: { role: "Engineering" }, value: 18 },
    { keys: { role: "Engineering" }, value: 77 },
    { keys: { role: "Sales" }, value: 0 },
    { keys: { role: "Sales" }, value: 0 },
    { keys: { role: "Sales" }, value: 0 },
    { keys: { role: "Sales" }, value: 12 },
    { keys: { role: "Sales" }, value: 26 },
    { keys: { role: "Sales" }, value: 46 },
  ],
} as const satisfies FullCorpusResult;

const narrowLifespanPanels = {
  measure: "lifespan",
  cohort: "closed",
  groupBy: ["role"],
  rows: Array.from({ length: 20 }, (_, index) => ({
    keys: { role: `Role ${index + 1}` },
    value: index % 71,
  })),
} satisfies FullCorpusResult;

const auditTable = {
  measure: "count",
  cohort: "open",
  groupBy: ["function"],
  rows: [
    { keys: { function: "Engineering" }, value: 52 },
    { keys: { function: "Sales" }, value: 24 },
    { keys: { function: "Operations" }, value: 8 },
  ],
} as const satisfies FullCorpusResult;

// This mimics a model/JSON value that bypassed the static ClassifiedResult arm.
// CompositionChart must refuse it before it calls the child render callback.
const runtimeClassifiedWithoutCoverage = {
  measure: "count",
  cohort: "open",
  groupBy: ["function"],
  rows: [{ keys: { function: "Engineering" }, value: 52 }],
  denominator: { classified: 12, total: 19 },
} as MeasureResult;

interface ChartPlaygroundProps {
  readonly encoding: Encoding;
  readonly result: MeasureResult;
  readonly coverage?: Coverage;
  readonly theme?: ChartThemeOverrides;
  readonly "aria-label"?: string;
}

function ChartPlayground({ encoding, result, coverage, theme, "aria-label": ariaLabel }: ChartPlaygroundProps) {
  // This Storybook-only boundary intentionally permits clearing coverage so
  // the runtime classified-result refusal remains inspectable with Controls.
  const chartProps: CompositionChartCoverageProps = coverage == null
    ? { result: result as FullCorpusResult }
    : { result: result as ClassifiedResult, coverage };

  return (
    <CompositionChart encoding={encoding} theme={theme} aria-label={ariaLabel} {...chartProps}>
      {(context) => <ChartEncoding encoding={encoding} context={context} />}
    </CompositionChart>
  );
}

function ChartEncoding({ encoding, context }: { readonly encoding: Encoding; readonly context: ChartRenderContext }) {
  switch (encoding) {
    case "ranked_bars":
      return <RankedBars context={context} />;
    case "diverging_bars":
      return <DivergingBars context={context} />;
    case "stacked_bar":
      return <StackedBar context={context} />;
    case "line":
      return <LineChart context={context} />;
    case "area":
      return <AreaChart context={context} />;
    case "histogram":
      return <HistogramChart context={context} />;
    case "table":
      return <Table context={context} />;
  }
}

const meta = {
  title: "Charts/Gallery",
  component: ChartPlayground,
  parameters: { layout: "padded" },
  argTypes: {
    encoding: {
      control: "select",
      options: ["line", "area", "diverging_bars", "ranked_bars", "histogram", "stacked_bar", "table"],
    },
    result: {
      control: "object",
      description: "Engine-shaped measure result. Edit rows, values, keys, and gap flags directly.",
    },
    coverage: {
      control: "object",
      description: "Classified-slice coverage. Clear it with a result that has a denominator to inspect the refusal.",
    },
    theme: {
      control: "object",
      description: "Optional chart-local theme overrides, such as margins or histogram edges.",
    },
    "aria-label": {
      control: "text",
    },
  },
} satisfies Meta<typeof ChartPlayground>;

export default meta;
type PlaygroundStory = StoryObj<typeof meta>;
type Story = StoryObj;

function StoryCard({ title, children }: { readonly title: string; readonly children: React.ReactNode }) {
  return (
    <section className="max-w-4xl space-y-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      {children}
    </section>
  );
}

export const Playground: PlaygroundStory = {
  args: {
    encoding: "line",
    result: lineWithFailedRun,
    theme: {},
    "aria-label": "Interactive chart playground",
  },
  render: (args) => (
    <StoryCard title="Edit CompositionChart props with Controls">
      <ChartPlayground {...args} />
    </StoryCard>
  ),
};

export const WeeklyBarGap: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Weekly bars retain and explain the failed-run week as a hatched no-data column">
      <CompositionChart encoding="ranked_bars" result={weeklyBarWithGap}>
        {(context) => <RankedBars context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const FullCorpusUnmappedBar: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Full-corpus market bars distinguish the explicit unmapped value">
      <CompositionChart encoding="ranked_bars" result={fullCorpusMarketBars}>
        {(context) => <RankedBars context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const DivergingChanges: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Diverging bars retain a visible zero baseline">
      <CompositionChart encoding="diverging_bars" result={movers}>
        {(context) => <DivergingBars context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const DivergingGap: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Diverging bars explain a failed-run week without showing it as a zero">
      <CompositionChart encoding="diverging_bars" result={weeklyDivergingWithGap}>
        {(context) => <DivergingBars context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const ClassifiedStackWithCoverage: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Thin classified slice keeps its coverage pill">
      <CompositionChart
        encoding="stacked_bar"
        result={classifiedSeniorityMix}
        coverage={classifiedSeniorityMix.denominator}
      >
        {(context) => <StackedBar context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const StackedGap: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Stacked bars explain a failed-run week without showing it as a zero">
      <CompositionChart encoding="stacked_bar" result={weeklyStackWithGap}>
        {(context) => <StackedBar context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const LineGap: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Line breaks across a failed collection week and annotates its gray band">
      <CompositionChart encoding="line" result={lineWithFailedRun}>
        {(context) => <LineChart context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const PartialSeriesLineGap: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="A company-only failed run breaks only that series and leaves the successful series unmasked">
      <CompositionChart encoding="line" result={lineWithCompanyFailedRun}>
        {(context) => <LineChart context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const AreaGap: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Area fill drops out across a failed collection week">
      <CompositionChart encoding="area" result={areaWithFailedRun}>
        {(context) => <AreaChart context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const SingletonObservations: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Line and area retain isolated observations on either side of a failed collection week">
      <div className="space-y-6">
        <CompositionChart encoding="line" result={singletonSegmentsAfterFailedRun}>
          {(context) => <LineChart context={context} />}
        </CompositionChart>
        <CompositionChart encoding="area" result={singletonSegmentsAfterFailedRun}>
          {(context) => <AreaChart context={context} />}
        </CompositionChart>
      </div>
    </StoryCard>
  ),
};

export const LifespanSmallMultiples: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Lifespan panels share fixed scales and retain the zero-day spike">
      <CompositionChart encoding="histogram" result={lifespanSmallMultiples}>
        {(context) => <HistogramChart context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const NarrowLifespanPanels: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Narrow reflow retains every grouped lifespan panel with readable shared y axes">
      <CompositionChart
        encoding="histogram"
        result={narrowLifespanPanels}
        theme={{
          reflowBreakpoints: [{ minWidth: 0, tickDensity: 4, rotateLabels: true, panelsPerRow: 1 }],
        }}
      >
        {(context) => <HistogramChart context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const AuditTable: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Table preserves the engine rows in HTML">
      <CompositionChart encoding="table" result={auditTable}>
        {(context) => <Table context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};

export const MissingClassifiedCoverageRefusal: Story = {
  parameters: { controls: { disable: true } },
  render: () => (
    <StoryCard title="Runtime classified result without coverage refuses to draw">
      <CompositionChart encoding="ranked_bars" result={runtimeClassifiedWithoutCoverage as any}>
        {(context) => <RankedBars context={context} />}
      </CompositionChart>
    </StoryCard>
  ),
};
