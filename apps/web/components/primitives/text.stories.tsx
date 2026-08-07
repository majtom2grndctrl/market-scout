import type { Meta, StoryObj } from "@storybook/react";

import {
  colorMap,
  elementDefaults,
  familyMap,
  leadingMap,
  lineClampMap,
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
  sizeMap,
  textAlignMap,
  textWrapMap,
  trackingMap,
  transformMap,
  weightMap,
} from "@/tokens/generated/class-maps";

import { Box } from "./box";
import { Text } from "./text";

// Object keys stringify numeric union members (TextSize, Weight, LineClamp). A
// select control must feed the real runtime type back to Text, or the class-map
// lookup misses silently — numeric-looking keys become numbers, everything else
// (e.g. "start", "tight") passes through unchanged.
function toOptions(keys: string[]): (string | number)[] {
  return keys.map((key) => (Number.isNaN(Number(key)) ? key : Number(key)));
}

// TextProps is not exported from text.tsx, so its shape is recovered from the
// component's own signature rather than a hand-maintained duplicate.
type TextArgs = Parameters<typeof Text>[0];

const meta = {
  title: "Primitives/Text",
  component: Text,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "`size` is the only Responsive prop on Text. This control sets its bare token only — the per-breakpoint object form (`{ base, md, ... }`) is documented in Primitives/resolveResponsive, not reproduced here.",
      },
    },
  },
  argTypes: {
    // elementDefaults is a generated Record<TextElement, ElementDefault> — its keys
    // are the full TextElement union, so this is a real derived export (unlike
    // LayoutElement on the other primitives, which has no runtime array).
    as: { control: "select", options: Object.keys(elementDefaults), table: { category: "Element" } },
    size: {
      control: "select",
      options: toOptions(Object.keys(sizeMap.base)),
      description: "Responsive prop — bare token only. Leave unset to see the element default from `as`.",
      table: { category: "Type" },
    },
    weight: { control: "select", options: toOptions(Object.keys(weightMap)), table: { category: "Type" } },
    leading: { control: "select", options: Object.keys(leadingMap), table: { category: "Type" } },
    tracking: { control: "select", options: Object.keys(trackingMap), table: { category: "Type" } },
    // Text's align/wrap collide in name with Stack's — these must read from
    // textAlignMap / textWrapMap, not stackAlignMap / stackWrapMap, or the control
    // would compile cleanly while offering values Text's class map doesn't have.
    align: { control: "select", options: Object.keys(textAlignMap), table: { category: "Type" } },
    wrap: { control: "select", options: Object.keys(textWrapMap), table: { category: "Type" } },
    color: { control: "select", options: Object.keys(colorMap), table: { category: "Appearance" } },
    family: { control: "select", options: Object.keys(familyMap), table: { category: "Appearance" } },
    transform: { control: "select", options: Object.keys(transformMap), table: { category: "Appearance" } },
    truncate: { control: "boolean", table: { category: "Overflow" } },
    lineClamp: { control: "select", options: toOptions(Object.keys(lineClampMap)), table: { category: "Overflow" } },
    htmlFor: { control: "text", table: { category: "Element attributes" } },
    dateTime: { control: "text", table: { category: "Element attributes" } },
    cite: { control: "text", table: { category: "Element attributes" } },
    // prop-values.md's spacing summary names Box, Stack, Grid, GridItem, and
    // Container as the props that "take" padding and margin, but TextProps (see
    // text.tsx) composes the full SpacingProps and Text actually renders these
    // classes — the doc's summary sentence appears to predate that composition.
    // Exposed here to match the real, compiled prop surface; flagged for the doc.
    p: { control: "select", options: toOptions(Object.keys(pMap.base)), table: { category: "Spacing" } },
    px: { control: "select", options: toOptions(Object.keys(pxMap.base)), table: { category: "Spacing" } },
    py: { control: "select", options: toOptions(Object.keys(pyMap.base)), table: { category: "Spacing" } },
    pt: { control: "select", options: toOptions(Object.keys(ptMap)), table: { category: "Spacing" } },
    pr: { control: "select", options: toOptions(Object.keys(prMap)), table: { category: "Spacing" } },
    pb: { control: "select", options: toOptions(Object.keys(pbMap)), table: { category: "Spacing" } },
    pl: { control: "select", options: toOptions(Object.keys(plMap)), table: { category: "Spacing" } },
    // Same reasoning as Box: Text's mx (via the shared SpacingProps) has no "auto"
    // — that's Container-only — so options come from mMap, not mxMap.
    m: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Spacing" } },
    mx: { control: "select", options: toOptions(Object.keys(mMap.base)), table: { category: "Spacing" } },
    my: { control: "select", options: toOptions(Object.keys(myMap.base)), table: { category: "Spacing" } },
    mt: { control: "select", options: toOptions(Object.keys(mtMap)), table: { category: "Spacing" } },
    mr: { control: "select", options: toOptions(Object.keys(mrMap)), table: { category: "Spacing" } },
    mb: { control: "select", options: toOptions(Object.keys(mbMap)), table: { category: "Spacing" } },
    ml: { control: "select", options: toOptions(Object.keys(mlMap)), table: { category: "Spacing" } },
  },
  args: {
    as: "h2",
    color: "default",
    family: "sans",
    align: "start",
    wrap: "normal",
    transform: "none",
    truncate: false,
    m: 200,
  },
} satisfies Meta<typeof Text>;

export default meta;
type Story = StoryObj<typeof meta>;

function TextPlaygroundDemo(args: TextArgs) {
  const onDark = args.color === "inverse";
  return (
    <Box as="section" bg={onDark ? "primary" : "background"} p={400}>
      {/* Raw div, not a primitive: bounds the line so wrap, align, and lineClamp
          have a real width to act against. */}
      <div style={{ maxWidth: 480 }}>
        <Text {...args}>
          Text playground: the quick brown fox jumps over the lazy dog. The quick brown fox jumps
          over the lazy dog again, so wrap, align, and line-clamp controls have enough lines to
          visibly act on.
        </Text>
      </div>
    </Box>
  );
}

export const Playground: Story = {
  render: (args) => <TextPlaygroundDemo {...args} />,
};
