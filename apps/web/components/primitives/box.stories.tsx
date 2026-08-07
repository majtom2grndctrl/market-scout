import type { Meta, StoryObj } from "@storybook/react";

import {
  bgMap,
  borderMap,
  heightMap,
  mMap,
  mbMap,
  minHeightMap,
  mlMap,
  mrMap,
  mtMap,
  myMap,
  overflowMap,
  pMap,
  pbMap,
  plMap,
  prMap,
  ptMap,
  pxMap,
  pyMap,
  radiusMap,
  shadowMap,
  widthMap,
} from "@/tokens/generated/class-maps";

import { Box, type BoxProps } from "./box";
import { Text } from "./text";

// Object keys stringify numeric union members (Spacing, TextSize, ...). A select
// control must feed the real runtime type back to the component, or the class-map
// lookup misses silently — so non-numeric-looking keys (e.g. "full", "auto") pass
// through untouched and numeric-looking keys become numbers.
function toOptions(keys: string[]): (string | number)[] {
  return keys.map((key) => (Number.isNaN(Number(key)) ? key : Number(key)));
}

// LayoutElement has no generated runtime export — types.ts defines it as a bare
// TS union (no class is ever emitted for an element tag), so unlike every other
// control below this one is hand-listed from prop-values.md §Element unions.
const layoutElements = [
  "div",
  "section",
  "article",
  "header",
  "footer",
  "main",
  "aside",
  "nav",
  "figure",
  "ul",
  "li",
] as const;

const meta = {
  title: "Primitives/Box",
  component: Box,
  tags: ["autodocs"],
  argTypes: {
    as: { control: "select", options: layoutElements, table: { category: "Element" } },
    bg: { control: "select", options: Object.keys(bgMap), table: { category: "Appearance" } },
    radius: { control: "select", options: Object.keys(radiusMap), table: { category: "Appearance" } },
    border: { control: "select", options: Object.keys(borderMap), table: { category: "Appearance" } },
    shadow: { control: "select", options: Object.keys(shadowMap), table: { category: "Appearance" } },
    overflow: { control: "select", options: Object.keys(overflowMap), table: { category: "Appearance" } },
    p: {
      control: "select",
      options: toOptions(Object.keys(pMap.base)),
      description: "Responsive prop — this control sets the bare token only. Object form documented in Primitives/resolveResponsive.",
      table: { category: "Padding" },
    },
    px: { control: "select", options: toOptions(Object.keys(pxMap.base)), table: { category: "Padding" } },
    py: { control: "select", options: toOptions(Object.keys(pyMap.base)), table: { category: "Padding" } },
    pt: { control: "select", options: toOptions(Object.keys(ptMap)), table: { category: "Padding" } },
    pr: { control: "select", options: toOptions(Object.keys(prMap)), table: { category: "Padding" } },
    pb: { control: "select", options: toOptions(Object.keys(pbMap)), table: { category: "Padding" } },
    pl: { control: "select", options: toOptions(Object.keys(plMap)), table: { category: "Padding" } },
    m: {
      control: "select",
      options: toOptions(Object.keys(mMap.base)),
      description: "Responsive prop — bare token only. Object form documented in Primitives/resolveResponsive.",
      table: { category: "Margin" },
    },
    // Box's mx has no "auto" — that value is reserved for Container (see
    // box.type-test.tsx). Deriving options from mMap rather than mxMap keeps the
    // control honest: mxMap carries an extra "auto" key that Box's own type rejects.
    mx: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Margin" } },
    my: { control: "select", options: toOptions(Object.keys(myMap.base)), table: { category: "Margin" } },
    mt: { control: "select", options: toOptions(Object.keys(mtMap)), table: { category: "Margin" } },
    mr: { control: "select", options: toOptions(Object.keys(mrMap)), table: { category: "Margin" } },
    mb: { control: "select", options: toOptions(Object.keys(mbMap)), table: { category: "Margin" } },
    ml: { control: "select", options: toOptions(Object.keys(mlMap)), table: { category: "Margin" } },
    width: { control: "select", options: toOptions(Object.keys(widthMap.base)), table: { category: "Sizing" } },
    height: { control: "select", options: toOptions(Object.keys(heightMap.base)), table: { category: "Sizing" } },
    minHeight: { control: "select", options: toOptions(Object.keys(minHeightMap.base)), table: { category: "Sizing" } },
  },
  args: {
    as: "div",
    bg: "card",
    radius: "lg",
    border: "all",
    shadow: "md",
    overflow: "auto",
    p: 400,
    m: 200,
    width: "full",
    height: "auto",
    minHeight: "none",
  },
} satisfies Meta<typeof Box>;

export default meta;
type Story = StoryObj<typeof meta>;

function BoxPlaygroundDemo(args: BoxProps) {
  const onDark = args.bg === "primary" || args.bg === "destructive";
  // A scrolling overflow value makes the Box a scroll container, and a scroll container
  // that no one can focus is unreachable by keyboard — axe flags it as
  // scrollable-region-focusable. Box emits no tabIndex of its own, so the consumer owns
  // this. Passing it through here keeps the default args clean and shows the fix.
  const scrolls = args.overflow === "auto" || args.overflow === "x-auto" || args.overflow === "y-auto";
  return (
    // Raw div, not a primitive: a fixed width plus a dashed outline is scaffolding
    // for the demo (visible margin frame, a bound for the overflow example), not
    // styling of Box or Text — style is never passed to a primitive.
    <div style={{ maxWidth: 360, border: "1px dashed var(--border)", padding: 8 }}>
      <Box
        {...args}
        {...(scrolls ? { tabIndex: 0, role: "region", "aria-label": "Box overflow example" } : {})}
      >
        <Text as="p" weight={600} color={onDark ? "inverse" : "default"}>
          Box playground
        </Text>
        <Text as="p" color={onDark ? "inverse" : "default"}>
          Padding sits between this text and the Box edge. Margin is the gap between the Box edge
          and the dashed frame around it.
        </Text>
        <div style={{ whiteSpace: "nowrap" }}>
          <Text as="span" color={onDark ? "inverse" : "default"}>
            A deliberately long unbroken line shows what the overflow control does once it is
            wider than the Box.
          </Text>
        </div>
      </Box>
    </div>
  );
}

export const Playground: Story = {
  render: (args) => <BoxPlaygroundDemo {...args} />,
};
