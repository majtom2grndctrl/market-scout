import type { Fragment, ISql } from "postgres";

import type { Cohort, Composition, GrammarError, Grouping, Measure, Result } from "../composition";
import { validate } from "../composition";
import { orderFilters, orderGroupings } from "../composition/rules";

import { getSql } from "./client";
import { runAggregateMeasure } from "./measure-aggregates";
import { runDistributionMeasure } from "./measure-distributions";
import { createMeasureScope } from "./measure-scope";

export interface MeasureRow {
  readonly keys: Partial<Record<Grouping, string>>;
  readonly value: number;
  // A weekly time series (`rate`, and week-grouped `count`/`delta`/`share`)
  // marks a failed- or absent-run week so the chart breaks its line instead of
  // drawing through missing data. The flag carries the signal; value stays 0,
  // keeping "0 real" distinct from "0 unknown."
  readonly gap?: boolean;
}

export interface MeasureResult {
  readonly measure: Measure;
  readonly cohort: Cohort;
  readonly groupBy: readonly Grouping[];
  readonly rows: readonly MeasureRow[];
  readonly denominator?: { readonly classified: number; readonly total: number };
}

export interface RunCompositionOptions {
  readonly now?: Date;
}

export interface MeasureContext {
  readonly sql: ISql;
  readonly composition: Composition;
  // A missing injected clock remains SQL `now()`, so every timestamp inside a
  // query is evaluated by one database clock rather than separate JS calls.
  readonly now: Fragment;
}

interface DenominatorRow {
  readonly classified: number;
  readonly total: number;
}

export async function runCompositionWith(
  sql: ISql,
  composition: Composition,
  options: RunCompositionOptions = {},
): Promise<Result<MeasureResult, GrammarError>> {
  const validation = validate(composition);
  if (!validation.ok) return { ok: false, error: validation.error };

  // Typed callers may skip parseComposition(), whose transform establishes the
  // grammar's canonical group/filter order. Validation is deliberately robust
  // to that input; normalize here as well so SQL aliases and result metadata
  // observe the same one-spelling contract.
  const canonicalComposition = canonicalizeComposition(composition);
  const context: MeasureContext = {
    sql,
    composition: canonicalComposition,
    now: options.now == null ? sql`now()` : sql`${options.now}::timestamptz`,
  };
  const rows = await runMeasure(context);
  const denominator = validation.requiresDenominator ? await selectDenominator(context) : undefined;

  return {
    ok: true,
    value: {
      measure: canonicalComposition.measure,
      cohort: canonicalComposition.cohort,
      groupBy: canonicalComposition.groupBy ?? [],
      rows,
      ...(denominator != null && { denominator }),
    },
  };
}

function canonicalizeComposition(composition: Composition): Composition {
  const groupBy = orderGroupings(composition.groupBy ?? []);
  const filter = orderFilters(composition.filter ?? []);

  return {
    measure: composition.measure,
    cohort: composition.cohort,
    ...(groupBy.length > 0 && { groupBy }),
    ...(filter.length > 0 && { filter }),
    ...(composition.window != null && { window: composition.window }),
    ...(composition.sort != null && { sort: composition.sort }),
    ...(composition.limit != null && { limit: composition.limit }),
    encoding: composition.encoding,
  };
}

export async function runComposition(
  composition: Composition,
  options: RunCompositionOptions = {},
): Promise<Result<MeasureResult, GrammarError>> {
  return runCompositionWith(await getSql(), composition, options);
}

async function runMeasure(context: MeasureContext): Promise<readonly MeasureRow[]> {
  switch (context.composition.measure) {
    case "count":
    case "share":
    case "delta":
    case "rate":
      return runAggregateMeasure(context);
    case "age":
    case "lifespan":
      return runDistributionMeasure(context);
  }
}

async function selectDenominator(context: MeasureContext): Promise<MeasureResult["denominator"]> {
  const scope = createMeasureScope(context.sql, context.composition);
  const where = scope.filters ?? context.sql`TRUE`;
  const [row] = await context.sql<DenominatorRow[]>`
    SELECT
      count(*) FILTER (
        WHERE EXISTS (
          SELECT 1
          FROM latest_classifications AS classification
          WHERE classification.job_posting_id = cohort.job_posting_id
        )
      )::int AS classified,
      count(*)::int AS total
    FROM ${scope.cohort} AS cohort
    JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
    WHERE ${where}
  `;

  return row;
}
