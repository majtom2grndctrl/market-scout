import type { Meta, StoryObj } from "@storybook/react";

import {
  directionMap,
  gapMap,
  gapXMap,
  gapYMap,
  heightMap,
  justifyMap,
  mMap,
  mbMap,
  minHeightMap,
  mlMap,
  mrMap,
  mtMap,
  myMap,
  pMap,
  pbMap,
  plMap,
  prMap,
  ptMap,
  pxMap,
  pyMap,
  stackAlignMap,
  stackWrapMap,
  widthMap,
} from "@/tokens/generated/class-maps";

import { Box } from "./box";
import { Stack, type StackProps } from "./stack";

// Object keys stringify numeric union members (Spacing). A select control must
// feed the real runtime type back to Stack, or the class-map lookup misses
// silently — numeric-looking keys become numbers, everything else passes through.
function toOptions(keys: string[]): (string | number)[] {
  return keys.map((key) => (Number.isNaN(Number(key)) ? key : Number(key)));
}

// LayoutElement has no generated runtime export (types.ts defines it as a bare TS
// union — no class is ever emitted for an element tag), so this one is hand-listed
// from prop-values.md §Element unions rather than derived.
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
  title: "Primitives/Stack",
  component: Stack,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "`direction`, `align`, `justify`, `gap`, `gapX`, `gapY`, `p`, `px`, `py`, `m`, `mx`, `my`, `width`, `height`, and `minHeight` are Responsive props. Each control here sets the bare token only — the per-breakpoint object form (`{ base, md, ... }`) is documented in Primitives/resolveResponsive, not reproduced here.",
      },
    },
  },
  argTypes: {
    as: { control: "select", options: layoutElements, table: { category: "Element" } },
    direction: { control: "select", options: Object.keys(directionMap.base), table: { category: "Layout" } },
    // Stack's align/wrap collide in name with Text's — these must read from
    // stackAlignMap / stackWrapMap, not textAlignMap / textWrapMap, or the control
    // would compile cleanly while offering values Stack's class map doesn't have.
    align: { control: "select", options: Object.keys(stackAlignMap.base), table: { category: "Layout" } },
    justify: { control: "select", options: Object.keys(justifyMap.base), table: { category: "Layout" } },
    wrap: { control: "select", options: Object.keys(stackWrapMap), table: { category: "Layout" } },
    gap: { control: "select", options: toOptions(Object.keys(gapMap.base)), table: { category: "Layout" } },
    gapX: { control: "select", options: toOptions(Object.keys(gapXMap.base)), table: { category: "Layout" } },
    gapY: { control: "select", options: toOptions(Object.keys(gapYMap.base)), table: { category: "Layout" } },
    p: { control: "select", options: toOptions(Object.keys(pMap.base)), table: { category: "Padding" } },
    px: { control: "select", options: toOptions(Object.keys(pxMap.base)), table: { category: "Padding" } },
    py: { control: "select", options: toOptions(Object.keys(pyMap.base)), table: { category: "Padding" } },
    pt: { control: "select", options: toOptions(Object.keys(ptMap)), table: { category: "Padding" } },
    pr: { control: "select", options: toOptions(Object.keys(prMap)), table: { category: "Padding" } },
    pb: { control: "select", options: toOptions(Object.keys(pbMap)), table: { category: "Padding" } },
    pl: { control: "select", options: toOptions(Object.keys(plMap)), table: { category: "Padding" } },
    m: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Margin" } },
    // Stack's mx has no "auto" — that value is reserved for Container. Deriving
    // options from mMap rather than mxMap keeps the control honest: mxMap carries
    // an extra "auto" key that Stack's own type rejects.
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
    direction: "row",
    align: "center",
    justify: "between",
    wrap: "wrap",
    gap: 200,
    p: 200,
    width: "full",
  },
} satisfies Meta<typeof Stack>;

export default meta;
type Story = StoryObj<typeof meta>;

const items = ["First", "Second", "Third", "Fourth", "Fifth"];
// Alternating padding gives each item a different height, so `align` has a real
// cross-axis difference to show.
const itemPadding = [0, 300, 600] as const;

function StackPlaygroundDemo(args: StackProps) {
  return (
    // Raw div, not a primitive: bounds the row so `justify` has leftover space to
    // distribute and enough items exist to make `wrap` kick in.
    <div style={{ maxWidth: 480 }}>
      <Stack {...args}>
        {items.map((item, index) => (
          <Box key={item} bg="muted" border="all" px={200} py={itemPadding[index % itemPadding.length]}>
            {item}
          </Box>
        ))}
      </Stack>
    </div>
  );
}

export const Playground: Story = {
  render: (args) => <StackPlaygroundDemo {...args} />,
};
