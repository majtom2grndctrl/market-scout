import type { Route } from "next";

// Href construction only — no titles. `Route` is written once, in the satisfies
// constraint, so a renamed or deleted route fails to compile here rather than
// silently at every call site. Every key keeps its precise inferred type, which
// a `: Paths` annotation would widen away.
export const paths = {
  home: "/",
  status: "/status",
  postings: "/postings",
} satisfies Record<string, Route | ((...args: never[]) => Route)>;

export type Paths = typeof paths;
