// The typed contract the agent composes analyses from. Pure by construction —
// nothing here reaches a database. The only route from a composition to numbers
// is the engine consuming a value this module already validated.

export type { GrammarError, GrammarIssue, GrammarIssueCode, Result } from "./errors";
export { GRAMMAR_ISSUE_CODES } from "./errors";

export type {
  Cohort,
  Composition,
  Encoding,
  Filter,
  FilterDimension,
  Grouping,
  Measure,
  Sort,
  Window,
} from "./vocabulary";
export {
  COHORTS,
  ENCODINGS,
  FILTER_DIMENSIONS,
  GROUPINGS,
  MEASURES,
  SORTS,
} from "./vocabulary";

export type { Validation } from "./rules";
export { allowedCohorts, defaultCohort, requiresDenominator, validate } from "./rules";

export { COMPOSITION_JSON_SCHEMA, compositionSchema, parseComposition } from "./schema";

export { deserializeComposition, serializeComposition } from "./serialize";

export type { SeedName } from "./seeds";
export {
  DEMAND_BY_FUNCTION,
  DEMAND_TREND,
  MOVERS,
  ROLE_LIFECYCLE,
  SEED_COMPOSITIONS,
  SENIORITY_MIX,
} from "./seeds";
