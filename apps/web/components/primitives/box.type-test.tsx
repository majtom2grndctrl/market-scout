import { Box } from "./box";

// @ts-expect-error Box accepts only the shared layout element union.
<Box as="button" />;

// @ts-expect-error Box backgrounds are limited to semantic surface tokens.
<Box bg="white" />;

// @ts-expect-error Box radii are limited to the shadcn radius ladder.
<Box radius="5xl" />;

// @ts-expect-error Box borders are limited to the supported edge set.
<Box border="center" />;

// @ts-expect-error Box shadows are limited to Tailwind's supported scale.
<Box shadow="2xl" />;

// @ts-expect-error Box overflow is limited to the supported overflow modes.
<Box overflow="scroll" />;

// @ts-expect-error Box width is limited to the generated sizing values.
<Box width="screen" />;

// @ts-expect-error Box height is limited to the generated sizing values.
<Box height="fit" />;

// @ts-expect-error Box minimum height is limited to the generated sizing values.
<Box minHeight="auto" />;

// @ts-expect-error Auto horizontal margins are reserved for Container.
<Box mx="auto" />;
