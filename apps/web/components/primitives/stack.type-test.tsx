import { Stack } from "./stack";

// @ts-expect-error Stack does not permit className escape hatches.
<Stack className="flex-row" />;

// @ts-expect-error Stack does not permit inline style escape hatches.
<Stack style={{ flexDirection: "row" }} />;

// @ts-expect-error Stack accepts only the shared layout element union.
<Stack as="button" />;

// @ts-expect-error Stack directions are limited to flex direction tokens.
<Stack direction="diagonal" />;

// @ts-expect-error Stack alignment is limited to flex alignment tokens.
<Stack align="left" />;

// @ts-expect-error Stack justification is limited to flex justification tokens.
<Stack justify="space-between" />;

// @ts-expect-error Stack wrapping is limited to supported flex wrap modes.
<Stack wrap="reverse" />;

// @ts-expect-error Stack gaps are limited to generated spacing steps.
<Stack gap={450} />;

// @ts-expect-error Stack horizontal gaps are limited to generated spacing steps.
<Stack gapX={450} />;

// @ts-expect-error Stack vertical gaps are limited to generated spacing steps.
<Stack gapY={450} />;

// @ts-expect-error Stack width is limited to the generated sizing values.
<Stack width="screen" />;

// @ts-expect-error Stack height is limited to the generated sizing values.
<Stack height="fit" />;

// @ts-expect-error Stack minimum height is limited to the generated sizing values.
<Stack minHeight="auto" />;
