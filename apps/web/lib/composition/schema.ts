import { z } from "zod";
import { zodToJsonSchema } from "zod-to-json-schema";

import type { GrammarError, GrammarIssue, GrammarIssueCode, Result } from "./errors";
import { GRAMMAR_ISSUE_CODES, grammarError } from "./errors";
import { crossingIssues, defaultCohort, orderFilters, orderGroupings } from "./rules";
import type { Composition } from "./vocabulary";
import { COHORTS, ENCODINGS, FILTER_DIMENSIONS, GROUPINGS, MEASURES, SORTS } from "./vocabulary";

const filterInput = z
  .object({
    dim: z.enum(FILTER_DIMENSIONS).describe("Dimension to scope to."),
    value: z.string().trim().min(1).describe("Exact value on that dimension, e.g. a company name."),
  })
  .strict();

// Ten years at weekly grain. The corpus holds ~15 fetch days (live, 2026-08-15),
// so any ceiling is generous; this one is small enough that the value can never
// stringify in exponential notation, which the link reader's digit-only pattern
// could not read back.
const MAX_WINDOW_WEEKS = 520;

// No encoding renders a thousand groups, and the closed vocabulary has no
// dimension with a thousand distinct values. The bound is a shape constraint,
// not a rendering preference.
const MAX_LIMIT = 1000;

// A count the link can spell. `deserializeComposition` hands every value back as
// a string, so a digit-string is a legitimate spelling of one; nothing else is.
// Bare `z.coerce.number()` runs `Number()` first, which turns `true` into a
// fabricated "top 1" and `"0x14"` into 20 — a model that got the type wrong
// deserves a named refusal, not a number the grammar invented for it.
function linkableCount(max: number) {
  return z.preprocess(
    (value) => (typeof value === "string" && /^\d+$/.test(value) ? Number(value) : value),
    z.number().int().min(1).max(max),
  );
}

// Modifiers read `.nullable().describe().optional()`, and the order is
// load-bearing. The exported tool schema reports optional fields as
// nullable-and-required, so an explicit null from the model has to parse as
// "not set"; and the export drops the outermost optional wrapper, so a
// description attached there would vanish from the artifact the model reads.
// json-schema.test.ts asserts a description on every exported slot, so a
// reordered chain fails there rather than shipping a schema the model reads
// blind.
//
// `.strict()` because the export declares `additionalProperties: false`: an
// unnamed key is a refusal on both sides. Stripping instead would let a
// camelCase/snake_case slip through as a valid, silently different question.
// The nested `filterInput` and `window` objects carry `.strict()` for the same
// reason: the openAi target emits `additionalProperties: false` on every
// object, so a plain inner object would strip a nested key the export rejects —
// the divergence inverted one level down.
const compositionInput = z
  .object({
    measure: z.enum(MEASURES).describe("The one quantity this composition resolves."),
    cohort: z
      .enum(COHORTS)
      .nullable()
      .describe(
        "Which postings count. Free on count and delta; fixed elsewhere — age is open, lifespan closed, rate all, share open. Omit to take the default for the measure.",
      )
      .optional(),
    groupBy: z
      .array(z.enum(GROUPINGS))
      .nullable()
      .describe("Dimensions to resolve the measure over. Empty means one figure for the corpus.")
      .optional(),
    filter: z
      .array(filterInput)
      .nullable()
      .describe("Scopes the corpus before the measure runs.")
      .optional(),
    window: z
      .object({
        weeks: linkableCount(MAX_WINDOW_WEEKS).describe("Length in whole weeks."),
      })
      .strict()
      .nullable()
      .describe("Time span the measure covers. Weekly grain only. Required by the delta measure.")
      .optional(),
    sort: z.enum(SORTS).nullable().describe("Order of the result.").optional(),
    limit: linkableCount(MAX_LIMIT)
      .nullable()
      .describe("Keep only the first N groups, or rows for an ungrouped result.")
      .optional(),
    encoding: z.enum(ENCODINGS).describe("How the result renders. Must fit the result shape."),
  })
  .strict()
  .describe(
    "One measure resolved over a grouping and filter and rendered by an encoding, plus cohort, sort, limit and window modifiers.",
  );

type CompositionInput = z.infer<typeof compositionInput>;

// Normalizing before the refinements runs the rules against the same value the
// caller gets back, and gives serialization one form to emit.
export const compositionSchema = compositionInput
  .transform(normalize)
  .superRefine((composition, ctx) => {
    for (const issue of crossingIssues(composition)) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: [...issue.path],
        message: issue.message,
        params: { grammarCode: issue.code },
      });
    }
  });

// The only path from a model-emitted object to a typed composition. Never
// throws: an invalid crossing is an answer the surface has to narrate, not an
// exception it has to catch.
export function parseComposition(input: unknown): Result<Composition, GrammarError> {
  const parsed = compositionSchema.safeParse(input);
  if (parsed.success) return { ok: true, value: parsed.data };
  return { ok: false, error: grammarError(parsed.error.issues.map(toGrammarIssue)) };
}

// The tool contract. Looser than `parseComposition` on crossings — it accepts
// `lifespan` + `open`, which the parse rejects — and that asymmetry is the
// keystone: a crossing the model can express is a crossing it can be corrected
// on. On shape the two agree, except that the export declares `limit` and
// `weeks` as integers while the parse also takes the digit-string a link hands
// back.
//
// `effectStrategy` is pinned rather than left to the library default, because
// the keystone rests on it: it is what unwraps the transform and refinement
// above instead of hardening them into structure.
//
// Encoding the crossing rules here would mean enumerating every legal crossing
// as a union — outside the function-calling subset, and one more place to drift.
export const COMPOSITION_JSON_SCHEMA = zodToJsonSchema(compositionSchema, {
  name: "composition",
  nameStrategy: "title",
  target: "openAi",
  $refStrategy: "none",
  effectStrategy: "input",
});

function normalize(input: CompositionInput): Composition {
  const groupBy = orderGroupings(input.groupBy ?? []);
  const filter = orderFilters(input.filter ?? []);

  return {
    measure: input.measure,
    cohort: input.cohort ?? defaultCohort(input.measure),
    ...(groupBy.length > 0 && { groupBy }),
    ...(filter.length > 0 && { filter }),
    ...(input.window != null && { window: { weeks: input.window.weeks } }),
    ...(input.sort != null && { sort: input.sort }),
    ...(input.limit != null && { limit: input.limit }),
    encoding: input.encoding,
  };
}

function toGrammarIssue(issue: z.ZodIssue): GrammarIssue {
  return { code: issueCode(issue), message: issue.message, path: issue.path };
}

// A refinement tags itself so the caller can branch on which rule refused;
// everything else zod raises is the model getting the shape wrong.
function issueCode(issue: z.ZodIssue): GrammarIssueCode {
  const tagged = issue.code === "custom" ? issue.params?.grammarCode : undefined;
  return (GRAMMAR_ISSUE_CODES as readonly unknown[]).includes(tagged)
    ? (tagged as GrammarIssueCode)
    : "shape";
}
