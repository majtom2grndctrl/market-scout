import type { Meta, StoryObj } from "@storybook/react";

import { Box } from "@/components/primitives/box";
import { Grid } from "@/components/primitives/grid";
import { Stack } from "@/components/primitives/stack";
import { Text } from "@/components/primitives/text";
import {
  leadingMap,
  pMap,
  sizeMap,
  trackingMap,
  weightMap,
} from "@/tokens/generated/class-maps";
import type { Leading, Spacing, TextSize, Tracking, Weight } from "@/tokens/generated/types";

import { namedScale, numericScale } from "./scale-keys";
import { SpecRow } from "./spec-row";

const spacingSteps = numericScale<Spacing>(pMap.base);
const textSizes = numericScale<TextSize>(sizeMap.base);
const weights = numericScale<Weight>(weightMap);
const leadings = namedScale<Leading>(leadingMap);
const trackings = namedScale<Tracking>(trackingMap);

const sample = "Market Scout tracks hiring signals across company job boards.";
const wrappingSample =
  "Market Scout tracks hiring signals across company job boards, snapshotting every posting on each fetch, so an appearance and a disappearance both register as data rather than as an error.";

const meta = {
  title: "Style guide/Foundations",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Reference: Story = {
  render: () => (
    <Stack as="section" gap={700} p={400} aria-labelledby="foundations-title">
      <Stack gap={200}>
        <Text as="h1" id="foundations-title">
          Foundations
        </Text>
        <Text as="p" color="muted">
          Spacing, type size, weight, leading, and tracking, each ordered along its scale. Every list
          here is read back from the generated class maps, not hand-listed.
        </Text>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="foundations-spacing">
        <Text as="h2" id="foundations-spacing">
          Spacing scale
        </Text>
        <Text as="p" color="muted">
          One step per row, smallest first. The outer block carries the step as padding and the inner
          block is the same size in every row, so the ring around it is the step.
        </Text>
        <Grid columns={12} gap={300}>
          {spacingSteps.map((step) => (
            <SpecRow key={step} token={`p={${step}}`} emitted={pMap.base[step]}>
              <Box bg="accent" p={step} width="fit">
                <Box bg="primary" p={200} radius="sm" />
              </Box>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="foundations-size">
        <Text as="h2" id="foundations-size">
          Type size ramp
        </Text>
        <Text as="p" color="muted">
          One step per row, smallest first. Each size also carries a paired line height.
        </Text>
        <Grid columns={12} gap={300}>
          {textSizes.map((size) => (
            <SpecRow key={size} token={`size={${size}}`} emitted={sizeMap.base[size]}>
              <Text as="p" size={size}>
                {sample}
              </Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="foundations-weight">
        <Text as="h2" id="foundations-weight">
          Font weights
        </Text>
        <Grid columns={12} gap={300}>
          {weights.map((weight) => (
            <SpecRow key={weight} token={`weight={${weight}}`} emitted={weightMap[weight]}>
              <Text as="p" weight={weight}>
                {sample}
              </Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="foundations-leading">
        <Text as="h2" id="foundations-leading">
          Leading
        </Text>
        <Text as="p" color="muted">
          Leading overrides the line height paired with the size. Each sample runs long enough to
          wrap.
        </Text>
        <Grid columns={12} gap={300}>
          {leadings.map((leading) => (
            <SpecRow key={leading} token={`leading="${leading}"`} emitted={leadingMap[leading]}>
              <Text as="p" leading={leading}>
                {wrappingSample}
              </Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="foundations-tracking">
        <Text as="h2" id="foundations-tracking">
          Tracking
        </Text>
        <Grid columns={12} gap={300}>
          {trackings.map((tracking) => (
            <SpecRow key={tracking} token={`tracking="${tracking}"`} emitted={trackingMap[tracking]}>
              <Text as="p" tracking={tracking}>
                {sample}
              </Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>
    </Stack>
  ),
};
