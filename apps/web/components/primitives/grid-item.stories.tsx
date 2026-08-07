import type { Meta, StoryObj } from "@storybook/react";

import {
  colSpanMap,
  mMap,
  mbMap,
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
  rowSpanMap,
} from "@/tokens/generated/class-maps";

import { Box } from "./box";
import { Grid, GridItem, type GridItemProps } from "./grid";

// Object keys stringify numeric union members (Spacing, ColSpan, RowSpan). A
// select control must feed the real runtime type back to GridItem, or the
// class-map lookup misses silently — numeric-looking keys become numbers,
// non-numeric ones ("full") pass through unchanged.
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
  title: "Primitives/GridItem",
  component: GridItem,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "`colSpan`, `rowSpan`, `p`, `px`, `py`, `m`, `mx`, and `my` are Responsive props. Each control here sets the bare token only — the per-breakpoint object form (`{ base, md, ... }`) is documented in Primitives/resolveResponsive, not reproduced here.",
      },
    },
  },
  argTypes: {
    as: { control: "select", options: layoutElements, table: { category: "Element" } },
    colSpan: { control: "select", options: toOptions(Object.keys(colSpanMap.base)), table: { category: "Layout" } },
    rowSpan: { control: "select", options: toOptions(Object.keys(rowSpanMap.base)), table: { category: "Layout" } },
    p: { control: "select", options: toOptions(Object.keys(pMap.base)), table: { category: "Padding" } },
    px: { control: "select", options: toOptions(Object.keys(pxMap.base)), table: { category: "Padding" } },
    py: { control: "select", options: toOptions(Object.keys(pyMap.base)), table: { category: "Padding" } },
    pt: { control: "select", options: toOptions(Object.keys(ptMap)), table: { category: "Padding" } },
    pr: { control: "select", options: toOptions(Object.keys(prMap)), table: { category: "Padding" } },
    pb: { control: "select", options: toOptions(Object.keys(pbMap)), table: { category: "Padding" } },
    pl: { control: "select", options: toOptions(Object.keys(plMap)), table: { category: "Padding" } },
    m: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Margin" } },
    // GridItem's mx has no "auto" — that value is reserved for Container. Deriving
    // options from mMap rather than mxMap keeps the control honest: mxMap carries
    // an extra "auto" key that GridItem's own type rejects.
    mx: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Margin" } },
    my: { control: "select", options: toOptions(Object.keys(myMap.base)), table: { category: "Margin" } },
    mt: { control: "select", options: toOptions(Object.keys(mtMap)), table: { category: "Margin" } },
    mr: { control: "select", options: toOptions(Object.keys(mrMap)), table: { category: "Margin" } },
    mb: { control: "select", options: toOptions(Object.keys(mbMap)), table: { category: "Margin" } },
    ml: { control: "select", options: toOptions(Object.keys(mlMap)), table: { category: "Margin" } },
  },
  args: {
    as: "div",
    colSpan: 3,
    rowSpan: 1,
    p: 200,
  },
} satisfies Meta<typeof GridItem>;

export default meta;
type Story = StoryObj<typeof meta>;

function GridItemPlaygroundDemo(args: GridItemProps) {
  return (
    // A fixed 12-column parent Grid, matching ColSpan's full 1-12 range, so every
    // colSpan value has room to actually span without immediately wrapping.
    <div style={{ maxWidth: 640 }}>
      <Grid columns={12} gap={200} p={200}>
        <GridItem {...args}>
          <Box bg="accent" border="all" p={200}>
            Controlled item
          </Box>
        </GridItem>
        {Array.from({ length: 5 }, (_, index) => (
          <GridItem key={index}>
            <Box bg="muted" border="all" p={200}>
              Sibling {index + 1}
            </Box>
          </GridItem>
        ))}
      </Grid>
    </div>
  );
}

export const Playground: Story = {
  render: (args) => <GridItemPlaygroundDemo {...args} />,
};
