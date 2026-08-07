import { Text } from "./text";

// @ts-expect-error Text elements are limited to the generated text-bearing element union.
<Text as="button" />;

// @ts-expect-error Text sizes must use the generated type scale.
<Text size={450} />;

// @ts-expect-error Weight must use the generated weight scale.
<Text weight={800} />;

// @ts-expect-error Leading is a closed base-tier prop.
<Text leading="loose" />;

// @ts-expect-error Tracking is a closed base-tier prop.
<Text tracking="widest" />;

// @ts-expect-error Color is limited to semantic foreground values.
<Text color="secondary" />;

// @ts-expect-error Family is limited to the generated family values.
<Text family="serif" />;

// @ts-expect-error Text alignment is limited to start, center, and end.
<Text align="justify" />;

// @ts-expect-error Transform is a closed base-tier prop.
<Text transform="sentence" />;

// @ts-expect-error Truncate is a boolean prop.
<Text truncate="true" />;

// @ts-expect-error Line clamp must use the generated one-through-six scale.
<Text lineClamp={7} />;

// @ts-expect-error Text wrap is limited to the generated text wrapping values.
<Text wrap="wrap" />;

<Text as="p" cite="https://example.com" dateTime="2026-08-04" htmlFor="email" />;
