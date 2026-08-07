import type { Meta, StoryObj } from "@storybook/react";

import {
  maxWidthMap,
  mbMap,
  mlMap,
  mrMap,
  mtMap,
  mxMap,
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
import { Container, type ContainerProps } from "./container";
import { Text } from "./text";

// Object keys stringify numeric union members (Spacing). A select control must
// feed the real runtime type back to Container, or the class-map lookup misses
// silently — numeric-looking keys become numbers, everything else ("auto", named
// widths) passes through unchanged.
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
  title: "Primitives/Container",
  component: Container,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "`maxWidth`, `p`, `px`, `py`, `m`, `mx`, and `my` are Responsive props. Each control here sets the bare token only — the per-breakpoint object form (`{ base, md, ... }`) is documented in Primitives/resolveResponsive, not reproduced here.",
      },
    },
  },
  argTypes: {
    as: { control: "select", options: layoutElements, table: { category: "Element" } },
    maxWidth: { control: "select", options: Object.keys(maxWidthMap.base), table: { category: "Layout" } },
    p: { control: "select", options: toOptions(Object.keys(pMap.base)), table: { category: "Padding" } },
    px: { control: "select", options: toOptions(Object.keys(pxMap.base)), table: { category: "Padding" } },
    py: { control: "select", options: toOptions(Object.keys(pyMap.base)), table: { category: "Padding" } },
    pt: { control: "select", options: toOptions(Object.keys(ptMap)), table: { category: "Padding" } },
    pr: { control: "select", options: toOptions(Object.keys(prMap)), table: { category: "Padding" } },
    pb: { control: "select", options: toOptions(Object.keys(pbMap)), table: { category: "Padding" } },
    pl: { control: "select", options: toOptions(Object.keys(plMap)), table: { category: "Padding" } },
    // Container is the one primitive whose mx type includes "auto" (it is how
    // Container centers — see container.tsx) — the only component where deriving
    // options from mxMap rather than mMap is correct.
    mx: { control: "select", options: toOptions(Object.keys(mxMap.base)), table: { category: "Margin" } },
    my: { control: "select", options: toOptions(Object.keys(myMap.base)), table: { category: "Margin" } },
    mt: { control: "select", options: toOptions(Object.keys(mtMap)), table: { category: "Margin" } },
    mr: { control: "select", options: toOptions(Object.keys(mrMap)), table: { category: "Margin" } },
    mb: { control: "select", options: toOptions(Object.keys(mbMap)), table: { category: "Margin" } },
    ml: { control: "select", options: toOptions(Object.keys(mlMap)), table: { category: "Margin" } },
  },
  args: {
    as: "section",
    maxWidth: "content",
    p: 300,
    mx: "auto",
  },
} satisfies Meta<typeof Container>;

export default meta;
type Story = StoryObj<typeof meta>;

function ContainerPlaygroundDemo(args: ContainerProps) {
  return (
    // Raw div, not a primitive: a fixed-width dashed frame is scaffolding for the
    // demo — it gives Container's own width and centering (mx) something visible
    // to sit inside, since Container itself has no width prop.
    <div style={{ width: 640, border: "1px dashed var(--border)" }}>
      <Container {...args}>
        <Box bg="accent" border="all" p={200}>
          <Text as="p">
            Container playground — adjust maxWidth and the margin props to see width and centering
            respond against the dashed frame.
          </Text>
        </Box>
      </Container>
    </div>
  );
}

export const Playground: Story = {
  render: (args) => <ContainerPlaygroundDemo {...args} />,
};
