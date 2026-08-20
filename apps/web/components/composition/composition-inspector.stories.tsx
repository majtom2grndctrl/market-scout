import type { Meta, StoryObj } from "@storybook/react";

import { SEED_COMPOSITIONS, type SeedName } from "@/lib/composition";
import { CompositionInspector } from "./composition-inspector";

// The Inspector is the grammar's verification surface: no unit test can show a
// crossing being refused mid-edit. Each story is only a starting seed — every
// slot is editable from any of them.
const meta: Meta<typeof CompositionInspector> = {
  title: "Composition/Inspector",
  component: CompositionInspector,
  parameters: { layout: "fullscreen" },
  argTypes: {
    initialSeed: {
      control: "select",
      options: Object.keys(SEED_COMPOSITIONS) as SeedName[],
    },
  },
  args: { initialSeed: "demandTrend" },
  // Keyed on the arg because `initialSeed` seeds the draft at mount and is
  // never read again — the draft belongs to whoever is editing it. An args
  // update re-renders rather than remounts, so without the key the Controls
  // select above would change nothing.
  render: (args) => <CompositionInspector key={args.initialSeed} {...args} />,
};

export default meta;
type Story = StoryObj<typeof CompositionInspector>;

// count · open · [company, week] · line. Set Measure to "lifespan" and the
// cohort slot refuses: the time measures fix their cohort.
export const DemandTrend: Story = {
  args: { initialSeed: "demandTrend" },
};

// delta · diverging_bars. Set Measure to "count" and the encoding slot refuses:
// only a signed measure renders on both sides of zero.
export const Movers: Story = {
  args: { initialSeed: "movers" },
};

// lifespan · closed · histogram, ungrouped. Check a grouping and the
// composition stays valid as small-multiples. Set Measure to "count" and the
// encoding slot refuses: a histogram renders a per-posting distribution. Set it
// to "age" — the other per-posting measure — and the *cohort* slot refuses
// instead, because age is the open cohort and this seed carries closed.
export const RoleLifecycle: Story = {
  args: { initialSeed: "roleLifecycle" },
};

// share · [company, seniority] · stacked_bar. Grouped over a classified
// dimension, so the denominator reads "Required". Uncheck either grouping and
// the groupBy slot refuses: stacked_bar needs exactly two.
export const SeniorityMix: Story = {
  args: { initialSeed: "seniorityMix" },
};

// count · [function] · ranked_bars. The simplest composition that still carries
// coverage debt.
export const DemandByFunction: Story = {
  args: { initialSeed: "demandByFunction" },
};
