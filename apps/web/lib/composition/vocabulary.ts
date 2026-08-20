// The vocabulary is closed on purpose. Compensation and geography are absent
// rather than refused per question, so an unanswerable question is
// uncomposable and the "no" comes for free — a compile error for us, and a
// value the model's schema never offers it.

export const MEASURES = ["count", "delta", "age", "lifespan", "rate", "share"] as const;
export type Measure = (typeof MEASURES)[number];

// Declared order is canonical order: normalization reorders `groupBy` to match,
// so two compositions differing only in how the model listed their groupings
// serialize to the same link.
export const GROUPINGS = [
  "company",
  "role",
  "specialization",
  "skill",
  "seniority",
  "function",
  "week",
] as const;
export type Grouping = (typeof GROUPINGS)[number];

// `week` is missing because `window` already scopes time; a filter and a
// modifier competing for the same axis would give one question two spellings.
export const FILTER_DIMENSIONS = [
  "company",
  "role",
  "specialization",
  "skill",
  "seniority",
  "function",
] as const satisfies readonly Grouping[];
export type FilterDimension = (typeof FILTER_DIMENSIONS)[number];

export const COHORTS = ["open", "closed", "all"] as const;
export type Cohort = (typeof COHORTS)[number];

export const SORTS = ["asc", "desc"] as const;
export type Sort = (typeof SORTS)[number];

// No `scorecard`, no `set-intersection`. A composition is one measure, so the
// encoding enum cannot promise a shape one measure has no way to feed.
export const ENCODINGS = [
  "line",
  "area",
  "diverging_bars",
  "ranked_bars",
  "histogram",
  "stacked_bar",
  "table",
] as const;
export type Encoding = (typeof ENCODINGS)[number];

export interface Filter {
  readonly dim: FilterDimension;
  readonly value: string;
}

// Weekly only, so the window carries no unit to choose. ~15 fetch days across
// the corpus cannot honestly support a finer grain.
export interface Window {
  readonly weeks: number;
}

// Canonical form, as `parseComposition` returns it: `cohort` always filled,
// and every modifier the composition does not use simply absent. A value that
// came *out of* `parseComposition` round-trips through the link to a deep-equal
// value; that is a property of the parse, not of the type. The type admits
// crossings the rules refuse and filter values the parse trims, and
// serialization is deliberately total — so a hand-built value can serialize to
// a link that reads back as a refusal. Deliberate: the gate is on the way in.
export interface Composition {
  readonly measure: Measure;
  readonly cohort: Cohort;
  readonly groupBy?: readonly Grouping[];
  readonly filter?: readonly Filter[];
  readonly window?: Window;
  readonly sort?: Sort;
  readonly limit?: number;
  readonly encoding: Encoding;
}
