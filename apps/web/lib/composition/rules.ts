import type { GrammarError, GrammarIssue } from "./errors";
import { grammarError } from "./errors";
import type { Cohort, Composition, Encoding, FilterDimension, Grouping, Measure } from "./vocabulary";
import { FILTER_DIMENSIONS, GROUPINGS } from "./vocabulary";

// Cohort is a free modifier on the count measures — currently-open, closed, or
// all is the question being asked. On the time measures it is intrinsic: `age`
// *is* the open cohort and `lifespan` the closed one, so it is fixed rather
// than chosen. First entry doubles as the default, which is why the free
// measures list `open` first.
const COHORTS_BY_MEASURE = {
  count: ["open", "closed", "all"],
  delta: ["open", "closed", "all"],
  age: ["open"],
  lifespan: ["closed"],
  rate: ["all"],
  share: ["open"],
} as const satisfies Record<Measure, readonly [Cohort, ...Cohort[]]>;

export function allowedCohorts(measure: Measure): readonly Cohort[] {
  return COHORTS_BY_MEASURE[measure];
}

export function defaultCohort(measure: Measure): Cohort {
  return COHORTS_BY_MEASURE[measure][0];
}

// Distributions of one posting-level duration, not aggregates over a group.
const PER_POSTING_MEASURES = ["age", "lifespan"] as const satisfies readonly Measure[];

// The encodings that render a per-posting measure without inventing an
// aggregate: `histogram` shows the distribution, `table` the rows themselves.
const DISTRIBUTION_ENCODINGS = ["histogram", "table"] as const satisfies readonly Encoding[];

// The only measure that can come back negative, and so the only one a bar chart
// may render on both sides of zero.
const SIGNED_MEASURES = ["delta"] as const satisfies readonly Measure[];

// Grouping over a classified dimension slices the ~35% of the corpus that
// carries a classification, so the composition states that debt structurally
// and the engine fills the denominator. A filter on one of those dimensions
// slices the corpus exactly as hard, so `validate` counts filters too.
// `seniority` is non-null within a classification but still a third of all
// postings, so it is classified too.
//
// Keyed by `Grouping` itself, not a hand-picked subset: `Record<Grouping,
// boolean>` requires an entry for every member of the union, so a grouping
// added to the vocabulary without a line here is a compile error, not a
// grouping that silently reads as full-corpus.
const REQUIRES_DENOMINATOR = {
  company: false,
  week: false,
  role: true,
  specialization: true,
  skill: true,
  seniority: true,
  function: true,
} as const satisfies Record<Grouping, boolean>;

export function requiresDenominator(grouping: Grouping): boolean {
  return REQUIRES_DENOMINATOR[grouping];
}

// A refusal here carries the same `GrammarError` every other public failure
// carries, so a caller narrating "no" reaches for one shape whether the
// composition arrived from the model, from a link, or already typed.
export type Validation =
  | {
      readonly ok: true;
      readonly requiresDenominator: boolean;
      readonly denominatorGroupings: readonly Grouping[];
    }
  | { readonly ok: false; readonly error: GrammarError };

// The validation core: rejects any crossing the Composability table does not
// sanction, and marks coverage debt on the ones it does. Coverage debt counts
// filters alongside groupings — restricting to `skill: "go"` buys the same ~35%
// slice that grouping by `skill` does.
export function validate(composition: Composition): Validation {
  const issues = crossingIssues(composition);
  if (issues.length > 0) return { ok: false, error: grammarError(issues) };

  const filterDimensions = orderFilters(composition.filter ?? []).map((filter) => filter.dim);
  const denominatorGroupings = orderGroupings([
    ...(composition.groupBy ?? []),
    ...filterDimensions,
  ]).filter(requiresDenominator);

  return {
    ok: true,
    requiresDenominator: denominatorGroupings.length > 0,
    denominatorGroupings,
  };
}

// Shared with the zod refinement, and normalizing here is what keeps the two
// honest. `parseComposition` normalizes before the refinement runs; a typed
// caller of `validate` has no way to know it must. Un-normalized, a `groupBy`
// listing one grouping twice reads as two — the opposite verdict on the same
// composition. `orderGroupings` is idempotent, so the second pass costs nothing.
export function crossingIssues(composition: Composition): readonly GrammarIssue[] {
  const { measure, cohort, encoding } = composition;
  const groupBy = orderGroupings(composition.groupBy ?? []);
  const issues: GrammarIssue[] = [];

  const cohorts = allowedCohorts(measure);
  if (!contains(cohorts, cohort)) {
    issues.push({
      code: "cohort_not_allowed",
      path: ["cohort"],
      message: `measure "${measure}" takes cohort ${quoteList(cohorts)}, not "${cohort}"`,
    });
  }

  if ((encoding === "line" || encoding === "area") && !groupBy.includes("week")) {
    issues.push({
      code: "encoding_needs_week",
      path: ["encoding"],
      message: `encoding "${encoding}" plots over time, so groupBy must include "week"`,
    });
  }

  if (encoding === "histogram" && !contains(PER_POSTING_MEASURES, measure)) {
    issues.push({
      code: "encoding_needs_per_posting_measure",
      path: ["encoding"],
      message: `encoding "histogram" renders a per-posting distribution, so measure must be ${quoteList(PER_POSTING_MEASURES)}, not "${measure}"`,
    });
  }

  // The inverse of the histogram rule, and the reason it can't be folded into
  // it: a per-posting measure is a distribution wherever it lands, so any
  // non-distribution encoding would have to collapse it to an aggregate the
  // composition never names — mean, median, p90 — the silent default this
  // grammar exists to refuse.
  if (contains(PER_POSTING_MEASURES, measure) && !contains(DISTRIBUTION_ENCODINGS, encoding)) {
    issues.push({
      code: "measure_needs_distribution_encoding",
      path: ["encoding"],
      message: `measure "${measure}" is a per-posting duration, so encoding must be ${quoteList(DISTRIBUTION_ENCODINGS)}, not "${encoding}"`,
    });
  }

  // One grouping turns the distribution into small multiples. Two would be a
  // grid of them, a form the Composability section hands to chart-primitives
  // rather than sanctioning here.
  if (encoding === "histogram" && groupBy.length > 1) {
    issues.push({
      code: "encoding_needs_at_most_one_grouping",
      path: ["groupBy"],
      message: `encoding "histogram" takes at most one grouping (small multiples), got ${groupBy.length}`,
    });
  }

  if (encoding === "diverging_bars" && !contains(SIGNED_MEASURES, measure)) {
    issues.push({
      code: "encoding_needs_signed_measure",
      path: ["encoding"],
      message: `encoding "diverging_bars" needs a signed measure (${quoteList(SIGNED_MEASURES)}), not "${measure}"`,
    });
  }

  if (encoding === "stacked_bar" && groupBy.length !== 2) {
    issues.push({
      code: "encoding_needs_two_groupings",
      path: ["groupBy"],
      message: `encoding "stacked_bar" needs exactly two groupings, got ${groupBy.length}`,
    });
  }

  if (measure === "share" && groupBy.length === 0) {
    issues.push({
      code: "measure_needs_grouping",
      path: ["groupBy"],
      message: `measure "share" normalizes within a grouping, so it needs at least one`,
    });
  }

  // Delta is a signed change *over* a window. Without one the engine has to
  // invent the interval the numbers are differenced across — the silent default
  // this grammar exists to prevent, and one that would let two links reading as
  // different questions compute the same thing.
  if (measure === "delta" && composition.window == null) {
    issues.push({
      code: "measure_needs_window",
      path: ["window"],
      message: `measure "delta" is a signed change over a window, so it needs one`,
    });
  }

  return issues;
}

// Reordering to the enum's declared order also drops repeats, so one set of
// groupings reaches the engine — and the link — in exactly one spelling.
export function orderGroupings(groupings: readonly Grouping[]): readonly Grouping[] {
  return GROUPINGS.filter((grouping) => groupings.includes(grouping));
}

export function orderFilters<T extends { dim: FilterDimension; value: string }>(
  filters: readonly T[],
): readonly T[] {
  const unique = new Map(filters.map((filter) => [`${filter.dim}=${filter.value}`, filter]));
  return [...unique.values()].sort(
    (a, b) =>
      FILTER_DIMENSIONS.indexOf(a.dim) - FILTER_DIMENSIONS.indexOf(b.dim) ||
      compare(a.value, b.value),
  );
}

// Array.includes on a readonly tuple narrows its argument to that tuple's
// union, which defeats the point — these calls ask whether a wider value is in
// the list at all.
function contains<T extends string>(values: readonly T[], value: string): boolean {
  return (values as readonly string[]).includes(value);
}

function compare(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

function quoteList(values: readonly string[]): string {
  return values.map((value) => `"${value}"`).join(" | ");
}
