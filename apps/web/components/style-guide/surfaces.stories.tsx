import type { Meta, StoryObj } from "@storybook/react";

import { Box } from "@/components/primitives/box";
import { Grid } from "@/components/primitives/grid";
import { Stack } from "@/components/primitives/stack";
import { Text } from "@/components/primitives/text";
import {
  bgMap,
  borderMap,
  radiusMap,
  shadowMap,
} from "@/tokens/generated/class-maps";
import type { Background, Border, Radius, Shadow } from "@/tokens/generated/types";

import { namedScale } from "./scale-keys";

const backgrounds = namedScale<Background>(bgMap);
const radii = namedScale<Radius>(radiusMap);
const borders = namedScale<Border>(borderMap);
const shadows = namedScale<Shadow>(shadowMap);

// Labels sitting ON a swatch never use color="muted". shadcn's --muted-foreground
// (#737373) measures 4.34:1 against the light-grey surfaces and 4.48:1 against white,
// both under WCAG AA's 4.5:1 — axe flags the grey case. Section prose below sits on the
// page background and keeps muted; only in-swatch labels are pinned to default/inverse.
//
// The two dark surfaces. Their labels need color="inverse"; the ambient
// foreground color on either of them measures about 1.1:1 against a required
// 4.5:1.
const darkBackgrounds: readonly Background[] = ["primary", "destructive"];

function emittedLabel(className: string) {
  return className === "" ? "(no class)" : className;
}

const meta = {
  title: "Style guide/Surfaces",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Reference: Story = {
  render: () => (
    <Stack as="section" gap={700} p={400} aria-labelledby="surfaces-title">
      <Stack gap={200}>
        <Text as="h1" id="surfaces-title">
          Surfaces
        </Text>
        <Text as="p" color="muted">
          Background, radius, border, and shadow, side by side. Every list here is read back from the
          generated class maps, not hand-listed. Backgrounds come from the shadcn color variables, so
          they follow the theme toggle in the toolbar.
        </Text>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="surfaces-background">
        <Text as="h2" id="surfaces-background">
          Backgrounds
        </Text>
        <Text as="p" color="muted">
          Each swatch carries a border, so a value that paints no background still reads as a swatch.
          Labels on the dark surfaces are set to the inverse foreground color.
        </Text>
        <Grid columns={{ base: 1, sm: 2, lg: 3 }} gap={400}>
          {backgrounds.map((bg) => (
            <Box key={bg} bg={bg} border="all" radius="md" p={400}>
              <Text
                as="p"
                family="mono"
                size={200}
                color={darkBackgrounds.includes(bg) ? "inverse" : "default"}
              >
                {`bg="${bg}"`}
              </Text>
              <Text
                as="p"
                family="mono"
                size={100}
                color={darkBackgrounds.includes(bg) ? "inverse" : "default"}
              >
                {emittedLabel(bgMap[bg])}
              </Text>
            </Box>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="surfaces-radius">
        <Text as="h2" id="surfaces-radius">
          Radius
        </Text>
        <Text as="p" color="muted">
          Smallest corner first. Same swatch each time, one radius value apart.
        </Text>
        <Grid columns={{ base: 1, sm: 2, lg: 3 }} gap={400}>
          {radii.map((radius) => (
            <Box key={radius} bg="muted" border="all" radius={radius} p={500}>
              <Text as="p" family="mono" size={200}>
                {`radius="${radius}"`}
              </Text>
              <Text as="p" family="mono" size={100} color="default">
                {emittedLabel(radiusMap[radius])}
              </Text>
            </Box>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="surfaces-border">
        <Text as="h2" id="surfaces-border">
          Border
        </Text>
        <Text as="p" color="muted">
          These props set width only. The color comes from the base rule in globals.css.
        </Text>
        <Grid columns={{ base: 1, sm: 2, lg: 4 }} gap={400}>
          {borders.map((border) => (
            <Box key={border} border={border} p={400}>
              <Text as="p" family="mono" size={200}>
                {`border="${border}"`}
              </Text>
              <Text as="p" family="mono" size={100} color="default">
                {emittedLabel(borderMap[border])}
              </Text>
            </Box>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="surfaces-shadow">
        <Text as="h2" id="surfaces-shadow">
          Shadow
        </Text>
        <Text as="p" color="muted">
          Shallowest first. These swatches carry no border, so the shadow is the only edge.
        </Text>
        <Grid columns={{ base: 1, sm: 2, lg: 4 }} gap={600} p={300}>
          {shadows.map((shadow) => (
            <Box key={shadow} bg="card" radius="lg" shadow={shadow} p={400}>
              <Text as="p" family="mono" size={200}>
                {`shadow="${shadow}"`}
              </Text>
              <Text as="p" family="mono" size={100} color="default">
                {emittedLabel(shadowMap[shadow])}
              </Text>
            </Box>
          ))}
        </Grid>
      </Stack>
    </Stack>
  ),
};
