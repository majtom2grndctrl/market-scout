// Two kinds of "no". Absent vocabulary is a compile error and never reaches
// this file — nothing here can describe a measure the enums don't hold. What
// remains is runtime: a shape the model got wrong, or a crossing the
// Composability rules don't sanction.

export type Result<T, E> =
  | { readonly ok: true; readonly value: T }
  | { readonly ok: false; readonly error: E };

export const GRAMMAR_ISSUE_CODES = [
  "shape",
  "version_unsupported",
  "duplicate_link_key",
  "cohort_not_allowed",
  "encoding_needs_week",
  "encoding_needs_per_posting_measure",
  "encoding_needs_signed_measure",
  "encoding_needs_two_groupings",
  "encoding_needs_at_most_one_grouping",
  "measure_needs_grouping",
  "measure_needs_window",
  "measure_needs_distribution_encoding",
] as const;

export type GrammarIssueCode = (typeof GRAMMAR_ISSUE_CODES)[number];

export interface GrammarIssue {
  readonly code: GrammarIssueCode;
  readonly message: string;
  readonly path: readonly (string | number)[];
}

// Every rejection collapses to one error so callers get a single refusal to
// narrate, but the issues survive intact — the Inspector marks the offending
// slot from `path`, and a refusal names its reason rather than "invalid".
export interface GrammarError {
  readonly message: string;
  readonly issues: readonly GrammarIssue[];
}

export function grammarError(issues: readonly GrammarIssue[]): GrammarError {
  const message = issues
    .map((issue) => `${issue.path.join(".") || "composition"}: ${issue.message}`)
    .join("; ");
  return { message, issues };
}
