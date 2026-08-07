import type { Meta, StoryObj } from "@storybook/react";

import { Box } from "@/components/primitives/box";
import { Grid } from "@/components/primitives/grid";
import { Stack } from "@/components/primitives/stack";
import { Text } from "@/components/primitives/text";
import {
  colorMap,
  elementDefaults,
  familyMap,
  sizeMap,
  transformMap,
  weightMap,
} from "@/tokens/generated/class-maps";
import type { Color, Family, TextElement, Transform } from "@/tokens/generated/types";

import { namedScale } from "./scale-keys";
import { SpecRow } from "./spec-row";

const elements = Object.keys(elementDefaults) as TextElement[];
const headings = elements.filter((element) => /^h[1-6]$/.test(element));
const colors = namedScale<Color>(colorMap);
const families = namedScale<Family>(familyMap);
const transforms = namedScale<Transform>(transformMap);

const sample = "Market Scout tracks hiring signals across company job boards.";

// The classes an element renders with when neither size nor weight is passed.
// An axis whose default is absent contributes no class, which is the case both
// the table and the strong/em paragraph below are there to show.
function defaultClasses(element: TextElement) {
  const defaults = elementDefaults[element];
  return [
    defaults.size === undefined ? "" : sizeMap.base[defaults.size],
    defaults.weight === undefined ? "" : weightMap[defaults.weight],
  ]
    .filter(Boolean)
    .join(" ");
}

const meta = {
  title: "Style guide/Typography",
  tags: ["autodocs"],
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Reference: Story = {
  render: () => (
    <Stack as="section" gap={700} p={400} aria-labelledby="typography-title">
      <Stack gap={200}>
        <Text as="h1" id="typography-title">
          Typography
        </Text>
        <Text as="p" color="muted">
          What an element renders as on its own, what an override changes, and the remaining text
          axes. Every list here is read back from the generated class maps, not hand-listed.
        </Text>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-defaults">
        <Text as="h2" id="typography-defaults">
          Element defaults
        </Text>
        <Text as="p" color="muted">
          Every element in the union, with the size and weight it renders at when neither prop is
          passed. A cell marked (no class) emits nothing for that axis, so the element inherits it
          from its parent.
        </Text>
        <table>
          <thead>
            <tr>
              <th scope="col">
                <Text as="div" size={200} weight={600} align="start" pr={500}>
                  Element
                </Text>
              </th>
              <th scope="col">
                <Text as="div" size={200} weight={600} align="start" pr={500}>
                  Size
                </Text>
              </th>
              <th scope="col">
                <Text as="div" size={200} weight={600} align="start" pr={500}>
                  Weight
                </Text>
              </th>
              <th scope="col">
                <Text as="div" size={200} weight={600} align="start">
                  Emitted classes
                </Text>
              </th>
            </tr>
          </thead>
          <tbody>
            {elements.map((element) => {
              const defaults = elementDefaults[element];
              const emitted = defaultClasses(element);

              return (
                <tr key={element}>
                  <th scope="row">
                    <Text as="div" family="mono" size={200} align="start" pr={500}>
                      {element}
                    </Text>
                  </th>
                  <td>
                    <Text as="div" family="mono" size={200} pr={500}>
                      {defaults.size ?? "(no class)"}
                    </Text>
                  </td>
                  <td>
                    <Text as="div" family="mono" size={200} pr={500}>
                      {defaults.weight ?? "(no class)"}
                    </Text>
                  </td>
                  <td>
                    <Text as="div" family="mono" size={200} color="muted">
                      {emitted === "" ? "(no class)" : emitted}
                    </Text>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-headings">
        <Text as="h2" id="typography-headings">
          Heading ramp
        </Text>
        <Text as="p" color="muted">
          The heading elements from the same defaults map, each rendered with no size or weight prop.
        </Text>
        <Grid columns={12} gap={300}>
          {headings.map((heading) => (
            <SpecRow key={heading} token={`as="${heading}"`} emitted={defaultClasses(heading)}>
              <Text as={heading}>{sample}</Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-override">
        <Text as="h2" id="typography-override">
          Size overrides the element default
        </Text>
        <Text as="p" color="muted">
          Both lines below are h2 elements. The first takes the element default; the second sets size
          to the largest step. Same document structure, different visual level.
        </Text>
        <Stack gap={400}>
          <Stack gap={100}>
            <Text as="p" family="mono" size={200} color="muted">
              {`<Text as="h2">`}
            </Text>
            <Text as="h2">Section heading at the h2 default</Text>
          </Stack>
          <Stack gap={100}>
            <Text as="p" family="mono" size={200} color="muted">
              {`<Text as="h2" size={900}>`}
            </Text>
            <Text as="h2" size={900}>
              Section heading at size 900
            </Text>
          </Stack>
        </Stack>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-no-class">
        <Text as="h2" id="typography-no-class">
          Elements that leave an axis unset
        </Text>
        <Text as="p" color="muted">
          strong and em are the two entries whose defaults emit no size class. Both sit inline in a
          plain paragraph below, where an inherited size is what they should show.
        </Text>
        <Text as="p">
          Plain paragraph text at the p default of size 400 and weight 400.{" "}
          <Text as="strong">This strong</Text> sets weight and no size, so it goes bold at the
          paragraph size. <Text as="em">This em</Text> sets neither, so it matches the paragraph in
          both and only the browser italic marks it.
        </Text>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-color">
        <Text as="h2" id="typography-color">
          Colors
        </Text>
        <Text as="p" color="muted">
          Every color resolves to a shadcn semantic variable, so the set follows the theme toggle in
          the toolbar. inverse is the value meant for a dark surface, so it is shown on a primary
          background.
        </Text>
        <Grid columns={{ base: 1, sm: 2, lg: 3 }} gap={400}>
          {colors.map((color) =>
            // inverse resolves to the primary foreground color. On the page
            // background it would be near-invisible, so it gets the surface it
            // is meant for.
            color === "inverse" ? (
              <Box key={color} bg="primary" border="all" radius="md" p={400}>
                <Text as="p" family="mono" size={200} color={color}>
                  {`color="${color}"`}
                </Text>
                <Text as="p" family="mono" size={100} color={color}>
                  {colorMap[color]}
                </Text>
              </Box>
            ) : (
              <Box key={color} border="all" radius="md" p={400}>
                <Text as="p" family="mono" size={200} color={color}>
                  {`color="${color}"`}
                </Text>
                <Text as="p" family="mono" size={100} color={color}>
                  {colorMap[color]}
                </Text>
              </Box>
            ),
          )}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-family">
        <Text as="h2" id="typography-family">
          Families
        </Text>
        <Grid columns={12} gap={300}>
          {families.map((family) => (
            <SpecRow key={family} token={`family="${family}"`} emitted={familyMap[family]}>
              <Text as="p" family={family}>
                {sample}
              </Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>

      <Stack as="section" gap={300} aria-labelledby="typography-transform">
        <Text as="h2" id="typography-transform">
          Transforms
        </Text>
        <Grid columns={12} gap={300}>
          {transforms.map((transform) => (
            <SpecRow
              key={transform}
              token={`transform="${transform}"`}
              emitted={transformMap[transform]}
            >
              <Text as="p" transform={transform}>
                {sample}
              </Text>
            </SpecRow>
          ))}
        </Grid>
      </Stack>
    </Stack>
  ),
};
