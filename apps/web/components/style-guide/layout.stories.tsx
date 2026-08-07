import type { Meta, StoryObj } from "@storybook/react";

import { Box } from "@/components/primitives/box";
import { Container } from "@/components/primitives/container";
import { Grid } from "@/components/primitives/grid";
import { Stack } from "@/components/primitives/stack";
import { Text } from "@/components/primitives/text";
import {
  columnsMap,
  directionMap,
  justifyMap,
  minItemWidthMap,
  stackAlignMap,
} from "@/tokens/generated/class-maps";
import type {
  Columns,
  Direction,
  Justify,
  MinItemWidth,
  Spacing,
  StackAlign,
} from "@/tokens/generated/types";

import { namedScale, numericScale } from "./scale-keys";
import { SpecRow } from "./spec-row";

const directions = namedScale<Direction>(directionMap.base);
const aligns = namedScale<StackAlign>(stackAlignMap.base);
const justifications = namedScale<Justify>(justifyMap.base);
const columnCounts = numericScale<Columns>(columnsMap.base);
const trackMinimums = namedScale<MinItemWidth>(minItemWidthMap);

const items = ["First", "Second", "Third"];

// Align is only observable when the children have different cross-axis sizes.
// With equal-height children every value renders pixel-identical, and a broken
// map would look correct. The height enum has no fixed step, so the variance
// comes from padding.
const alignItemPadding: readonly Spacing[] = [100, 300, 600];

function Chip({ label, py }: { label: string; py?: Spacing }) {
  return (
    <Box bg="accent" border="all" radius="sm" px={300} py={py ?? 200}>
      <Text as="span" size={200}>
        {label}
      </Text>
    </Box>
  );
}

const meta = {
  title: "Style guide/Layout",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Reference: Story = {
  render: () => (
    <Stack as="section" gap={700} p={400} aria-labelledby="layout-title">
      <Stack gap={200}>
        <Text as="h1" id="layout-title">
          Layout
        </Text>
        <Text as="p" color="muted">
          Direction, align, justify, column counts, and intrinsic tracks, each value beside the rest
          of its set. Every list here is read back from the generated class maps, not hand-listed.
        </Text>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="layout-direction">
        <Text as="h2" id="layout-direction">
          Direction
        </Text>
        <Text as="p" color="muted">
          The same three children in source order, once per direction.
        </Text>
        <Grid columns={12} gap={300}>
          {directions.map((direction) => (
            <SpecRow
              key={direction}
              token={`direction="${direction}"`}
              emitted={directionMap.base[direction]}
            >
              <Stack direction={direction} gap={200}>
                {items.map((item) => (
                  <Chip key={item} label={item} />
                ))}
              </Stack>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="layout-align">
        <Text as="h2" id="layout-align">
          Align
        </Text>
        <Text as="p" color="muted">
          The three children carry different vertical padding, so each row has three different
          heights for align to act on. With equal-height children every value below would render
          identically.
        </Text>
        <Grid columns={12} gap={300}>
          {aligns.map((align) => (
            <SpecRow key={align} token={`align="${align}"`} emitted={stackAlignMap.base[align]}>
              <Box border="all" radius="md" p={200}>
                <Stack direction="row" align={align} gap={200}>
                  {items.map((item, index) => (
                    <Chip key={item} label={item} py={alignItemPadding[index]} />
                  ))}
                </Stack>
              </Box>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="layout-justify">
        <Text as="h2" id="layout-justify">
          Justify
        </Text>
        <Text as="p" color="muted">
          Each row is full width, so there is free space on the main axis to distribute.
        </Text>
        <Grid columns={12} gap={300}>
          {justifications.map((justify) => (
            <SpecRow key={justify} token={`justify="${justify}"`} emitted={justifyMap.base[justify]}>
              <Box border="all" radius="md" p={200}>
                <Stack direction="row" justify={justify} gap={200} width="full">
                  {items.map((item) => (
                    <Chip key={item} label={item} />
                  ))}
                </Stack>
              </Box>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="layout-columns">
        <Text as="h2" id="layout-columns">
          Grid columns
        </Text>
        <Text as="p" color="muted">
          One row per column count, each filled with that many cells. Columns are even, and the cells
          narrow as the count climbs.
        </Text>
        <Grid columns={12} gap={300}>
          {columnCounts.map((count) => (
            <SpecRow key={count} token={`columns={${count}}`} emitted={columnsMap.base[count]}>
              <Grid columns={count} gap={100}>
                {Array.from({ length: count }, (_, index) => (
                  <Box key={index} bg="accent" border="all" radius="sm" py={200}>
                    <Text as="p" size={100} align="center">
                      {index + 1}
                    </Text>
                  </Box>
                ))}
              </Grid>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="layout-intrinsic">
        <Text as="h2" id="layout-intrinsic">
          Intrinsic tracks
        </Text>
        <Text as="p" color="muted">
          columns=&quot;auto&quot; wraps at a track minimum instead of a fixed count. Every grid below
          is capped at the same width and holds the same six cells, so a difference in track count
          comes from the minimum alone. Two adjacent minimums can still fit the same number of tracks
          at a given width and look identical — narrow the viewport to separate them.
        </Text>
        <Container maxWidth="content">
          <Stack gap={500}>
            {trackMinimums.map((minItemWidth) => (
              <Stack key={minItemWidth} gap={200}>
                <Text as="p" family="mono" size={200}>
                  {`columns="auto" minItemWidth="${minItemWidth}"`}
                </Text>
                <Text as="p" family="mono" size={100} color="muted">
                  {minItemWidthMap[minItemWidth]}
                </Text>
                <Grid columns="auto" minItemWidth={minItemWidth} gap={200}>
                  {Array.from({ length: 6 }, (_, index) => (
                    <Box key={index} bg="accent" border="all" radius="sm" p={300}>
                      <Text as="p" size={200} align="center">
                        {index + 1}
                      </Text>
                    </Box>
                  ))}
                </Grid>
              </Stack>
            ))}
          </Stack>
        </Container>
      </Stack>
    </Stack>
  ),
};
