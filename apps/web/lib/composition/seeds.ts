import type { Composition } from "./vocabulary";

// The five catalog views that reduce to a single measure. They seed cold-start
// chips and prove the grammar spans the catalog it replaces; Company Profile
// (several compositions in one layout) and Skill Overlap (co-occurrence) do not
// reduce, so they are not here. Each one passes `validate` — a seed that cannot
// is a mis-derived seed, not a rule to loosen.

// Who is ramping, one company against another over time. `week` in the grouping
// is what earns the line.
export const DEMAND_TREND = {
  measure: "count",
  cohort: "open",
  groupBy: ["company", "week"],
  encoding: "line",
} as const satisfies Composition;

// Who moved up or down this window. The one signed measure, so the one
// composition that renders on both sides of zero.
export const MOVERS = {
  measure: "delta",
  cohort: "open",
  groupBy: ["company"],
  window: { weeks: 2 },
  sort: "desc",
  limit: 20,
  encoding: "diverging_bars",
} as const satisfies Composition;

// How fast roles fill. Ungrouped, so it reads the whole closed population as one
// distribution — the bimodal split is the finding, and grouping would hide it.
export const ROLE_LIFECYCLE = {
  measure: "lifespan",
  cohort: "closed",
  encoding: "histogram",
} as const satisfies Composition;

// Who hires junior people. `share` normalizes each company's bar to a
// composition, which is what makes small and large boards comparable.
export const SENIORITY_MIX = {
  measure: "share",
  cohort: "open",
  groupBy: ["company", "seniority"],
  encoding: "stacked_bar",
} as const satisfies Composition;

// Engineering against sales against design. Grouped over a classified
// dimension, so it carries a denominator the engine fills.
export const DEMAND_BY_FUNCTION = {
  measure: "count",
  cohort: "open",
  groupBy: ["function"],
  sort: "desc",
  encoding: "ranked_bars",
} as const satisfies Composition;

export const SEED_COMPOSITIONS = {
  demandTrend: DEMAND_TREND,
  movers: MOVERS,
  roleLifecycle: ROLE_LIFECYCLE,
  seniorityMix: SENIORITY_MIX,
  demandByFunction: DEMAND_BY_FUNCTION,
} as const satisfies Record<string, Composition>;

export type SeedName = keyof typeof SEED_COMPOSITIONS;
