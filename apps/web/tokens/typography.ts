export const text = {
  100: { size: "0.75rem", lineHeight: "1.4" },
  200: { size: "0.8125rem", lineHeight: "1.4" },
  300: { size: "0.875rem", lineHeight: "1.5" },
  400: { size: "1rem", lineHeight: "1.6" },
  500: { size: "1.25rem", lineHeight: "1.5" },
  600: { size: "1.5rem", lineHeight: "1.4" },
  700: { size: "2rem", lineHeight: "1.3" },
  800: { size: "2.5rem", lineHeight: "1.2" },
  900: { size: "3.5rem", lineHeight: "1.1" },
} as const;

export const weight = {
  400: "400",
  500: "500",
  600: "600",
  700: "700",
} as const;

export const leading = {
  tight: "1.1",
  snug: "1.3",
  normal: "1.5",
  relaxed: "1.7",
} as const;

export const tracking = {
  tight: "-0.02em",
  normal: "0",
  wide: "0.04em",
  wider: "0.08em",
} as const;

export const elementDefaults = {
  h1: { size: 900, weight: 700 },
  h2: { size: 800, weight: 700 },
  h3: { size: 700, weight: 600 },
  h4: { size: 600, weight: 600 },
  h5: { size: 500, weight: 600 },
  h6: { size: 400, weight: 600 },
  p: { size: 400, weight: 400 },
  span: { size: 400, weight: 400 },
  div: { size: 400, weight: 400 },
  label: { size: 300, weight: 500 },
  code: { size: 300, weight: 400 },
  kbd: { size: 300, weight: 400 },
  samp: { size: 300, weight: 400 },
  strong: { weight: 700 },
  em: {},
  blockquote: { size: 500, weight: 400 },
  small: { size: 300, weight: 400 },
  figcaption: { size: 200, weight: 400 },
  caption: { size: 200, weight: 400 },
  li: { size: 400, weight: 400 },
  dd: { size: 400, weight: 400 },
  cite: { size: 400, weight: 400 },
  abbr: { size: 400, weight: 400 },
  time: { size: 400, weight: 400 },
  dt: { size: 400, weight: 600 },
} as const;
