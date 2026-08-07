import { Container } from "./container";

// @ts-expect-error Container does not permit className escape hatches.
<Container className="max-w-7xl" />;

// @ts-expect-error Container does not permit inline style escape hatches.
<Container style={{ maxWidth: "80rem" }} />;

// @ts-expect-error Container accepts only the shared layout element union.
<Container as="button" />;

// @ts-expect-error Container width is limited to the generated named width tokens.
<Container maxWidth="prose" />;

<Container mx={{ base: 0, md: "auto" }} />;
