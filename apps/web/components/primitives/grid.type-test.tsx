import { Grid, GridItem } from "./grid";

// @ts-expect-error Grid accepts only the shared layout element union.
<Grid as="button" />;

// @ts-expect-error Grid columns must use the generated one-through-twelve scale.
<Grid columns={13} />;

// @ts-expect-error Auto-fit columns are valid only as a bare value, not responsively.
<Grid columns={{ md: "auto" }} />;

// @ts-expect-error Auto-fit grids require a generated track-width token.
<Grid columns="auto" />;

// @ts-expect-error Grid track widths are limited to generated tokens.
<Grid columns="auto" minItemWidth="2xl" />;

// @ts-expect-error Grid gaps are limited to generated spacing steps.
<Grid gap={450} />;

// @ts-expect-error Grid horizontal gaps are limited to generated spacing steps.
<Grid gapX={450} />;

// @ts-expect-error Grid vertical gaps are limited to generated spacing steps.
<Grid gapY={450} />;

// @ts-expect-error GridItem accepts only the shared layout element union.
<GridItem as="button" />;

// @ts-expect-error Column spans must use the generated span values.
<GridItem colSpan={13} />;

// @ts-expect-error Row spans must use the generated span values.
<GridItem rowSpan={7} />;
