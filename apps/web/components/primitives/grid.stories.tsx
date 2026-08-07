import type { Meta, StoryObj } from "@storybook/react";

import {
  columnsMap,
  gapMap,
  gapXMap,
  gapYMap,
  mMap,
  mbMap,
  minItemWidthMap,
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
} from "@/tokens/generated/class-maps";

import { Box } from "./box";
import { Grid, type GridProps } from "./grid";

// Object keys stringify numeric union members (Spacing, Columns). A select control
// must feed the real runtime type back to Grid, or the class-map lookup misses
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

// GridProps is a discriminated union: `columns="auto"` requires `minItemWidth`,
// and a responsive `columns` object excludes "auto" entirely (see grid.tsx and
// grid.type-test.tsx). A single independent pair of controls could produce a
// combination the type forbids (e.g. "auto" with no minItemWidth).
//
// This Playground drives both from one `columns` control instead: its options are
// the generated 1-12 scale from columnsMap plus the literal "auto" appended by
// hand (there is no generated array for that literal — it is a structural
// discriminant in grid.tsx, not a token value that could drift). `minItemWidth`
// always has a default value in args and is never cleared by the other control, so
// the resulting args object is a valid FixedGridProps when columns is a number and
// a valid AutoGridProps when columns is "auto" — never the forbidden combination.
const columnsOptions = [...toOptions(Object.keys(columnsMap.base)), "auto"];

const meta = {
  title: "Primitives/Grid",
  component: Grid,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "`columns`, `gap`, `gapX`, `gapY`, `p`, `px`, `py`, `m`, `mx`, and `my` are Responsive props. Each control here sets the bare token only — the per-breakpoint object form (`{ base, md, ... }`) is documented in Primitives/resolveResponsive, not reproduced here. Note also that a responsive `columns` object excludes \"auto\" by type — this control's \"auto\" option always applies as a bare value.",
      },
    },
  },
  argTypes: {
    as: { control: "select", options: layoutElements, table: { category: "Element" } },
    columns: { control: "select", options: columnsOptions, table: { category: "Layout" } },
    minItemWidth: {
      control: "select",
      options: Object.keys(minItemWidthMap),
      description: 'Only meaningful when columns is "auto".',
      table: { category: "Layout" },
    },
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
    // Grid's mx has no "auto" — that value is reserved for Container. Deriving
    // options from mMap rather than mxMap keeps the control honest: mxMap carries
    // an extra "auto" key that Grid's own type rejects.
    mx: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Margin" } },
    my: { control: "select", options: toOptions(Object.keys(myMap.base)), table: { category: "Margin" } },
    mt: { control: "select", options: toOptions(Object.keys(mtMap)), table: { category: "Margin" } },
    mr: { control: "select", options: toOptions(Object.keys(mrMap)), table: { category: "Margin" } },
    mb: { control: "select", options: toOptions(Object.keys(mbMap)), table: { category: "Margin" } },
    ml: { control: "select", options: toOptions(Object.keys(mlMap)), table: { category: "Margin" } },
  },
  args: {
    as: "div",
    columns: 4,
    minItemWidth: "md",
    gap: 200,
    p: 200,
  },
} satisfies Meta<typeof Grid>;

export default meta;
type Story = StoryObj<typeof meta>;

function GridPlaygroundDemo(args: GridProps) {
  return (
    // Raw div, not a primitive: bounds the track so fixed column counts and the
    // auto-fit "auto" mode both have a real width to reflow against.
    <div style={{ maxWidth: 640 }}>
      <Grid {...args}>
        {Array.from({ length: 8 }, (_, index) => (
          <Box key={index} bg="muted" border="all" p={200}>
            Item {index + 1}
          </Box>
        ))}
      </Grid>
    </div>
  );
}

export const Playground: Story = {
  render: (args) => <GridPlaygroundDemo {...(args as GridProps)} />,
};
