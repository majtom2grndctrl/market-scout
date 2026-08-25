import type { Fragment, ISql } from "postgres";

import type { Cohort, Composition, Grouping } from "../composition";

// The engine only interpolates fragments selected from these fixed lookups.
// Composition values can choose a lookup entry, but never become SQL syntax.
interface GroupingSource {
  readonly join: (sql: ISql) => Fragment;
  readonly keyValue: (sql: ISql) => Fragment;
  readonly filter: (sql: ISql, value: string) => Fragment;
}

export interface MeasureScope {
  readonly cohort: Fragment;
  readonly groupBy: readonly Grouping[];
  readonly joins?: Fragment;
  readonly filters?: Fragment;
  readonly keys?: Fragment;
}

export function createMeasureScope(sql: ISql, composition: Composition): MeasureScope {
  const groupBy = composition.groupBy ?? [];
  const joins = joinFragments(
    sql,
    groupBy.map((grouping) => GROUPING_SOURCES[grouping].join(sql)),
    " ",
  );
  const filters = joinFragments(
    sql,
    (composition.filter ?? []).map((filter) => GROUPING_SOURCES[filter.dim].filter(sql, filter.value)),
    " AND ",
  );
  const keyValues = joinFragments(
    sql,
    groupBy.map((grouping) => GROUPING_SOURCES[grouping].keyValue(sql)),
    ", ",
  );

  return {
    cohort: cohortSource(sql, composition.cohort),
    groupBy,
    ...(joins != null && { joins }),
    ...(filters != null && { filters }),
    ...(keyValues != null && { keys: sql`jsonb_build_object(${keyValues})` }),
  };
}

function cohortSource(sql: ISql, cohort: Cohort): Fragment {
  switch (cohort) {
    case "open":
      return sql`open_postings`;
    case "closed":
      return sql`closed_postings`;
    case "all":
      return sql`all_seen_postings`;
  }
}

const GROUPING_SOURCES = {
  company: {
    join: (sql) => sql`
      JOIN companies AS grouping_company ON grouping_company.id = posting.company_id
    `,
    keyValue: (sql) => sql`'company', grouping_company.name`,
    filter: (sql, value) => sql`posting.company_id = ${value}`,
  },
  market: {
    join: (sql) => sql`
      JOIN posting_taxonomy AS grouping_market
        ON grouping_market.job_posting_id = cohort.job_posting_id
        AND grouping_market.term_kind = 'market'
    `,
    keyValue: (sql) => sql`'market', grouping_market.slug`,
    filter: (sql, value) => sql`
      EXISTS (
        SELECT 1
        FROM posting_taxonomy AS filter_market
        WHERE filter_market.job_posting_id = cohort.job_posting_id
          AND filter_market.term_kind = 'market'
          AND filter_market.slug = ${value}
      )
    `,
  },
  role: {
    join: (sql) => sql`
      JOIN posting_taxonomy AS grouping_role
        ON grouping_role.job_posting_id = cohort.job_posting_id
        AND grouping_role.term_kind = 'role'
    `,
    keyValue: (sql) => sql`'role', grouping_role.slug`,
    filter: (sql, value) => sql`
      EXISTS (
        SELECT 1
        FROM posting_taxonomy AS filter_role
        WHERE filter_role.job_posting_id = cohort.job_posting_id
          AND filter_role.term_kind = 'role'
          AND filter_role.slug = ${value}
      )
    `,
  },
  specialization: {
    join: (sql) => sql`
      JOIN posting_taxonomy AS grouping_specialization
        ON grouping_specialization.job_posting_id = cohort.job_posting_id
        AND grouping_specialization.term_kind = 'specialization'
    `,
    keyValue: (sql) => sql`'specialization', grouping_specialization.slug`,
    filter: (sql, value) => sql`
      EXISTS (
        SELECT 1
        FROM posting_taxonomy AS filter_specialization
        WHERE filter_specialization.job_posting_id = cohort.job_posting_id
          AND filter_specialization.term_kind = 'specialization'
          AND filter_specialization.slug = ${value}
      )
    `,
  },
  skill: {
    join: (sql) => sql`
      JOIN posting_taxonomy AS grouping_skill
        ON grouping_skill.job_posting_id = cohort.job_posting_id
        AND grouping_skill.term_kind = 'skill'
    `,
    keyValue: (sql) => sql`'skill', grouping_skill.slug`,
    filter: (sql, value) => sql`
      EXISTS (
        SELECT 1
        FROM posting_taxonomy AS filter_skill
        WHERE filter_skill.job_posting_id = cohort.job_posting_id
          AND filter_skill.term_kind = 'skill'
          AND filter_skill.slug = ${value}
      )
    `,
  },
  seniority: {
    join: (sql) => sql`
      JOIN latest_classifications AS grouping_seniority
        ON grouping_seniority.job_posting_id = cohort.job_posting_id
    `,
    keyValue: (sql) => sql`'seniority', grouping_seniority.seniority`,
    filter: (sql, value) => sql`
      EXISTS (
        SELECT 1
        FROM latest_classifications AS filter_seniority
        WHERE filter_seniority.job_posting_id = cohort.job_posting_id
          AND filter_seniority.seniority = ${value}
      )
    `,
  },
  function: {
    join: (sql) => sql`
      JOIN posting_taxonomy AS grouping_function
        ON grouping_function.job_posting_id = cohort.job_posting_id
        AND grouping_function.term_kind = 'dimension'
    `,
    keyValue: (sql) => sql`'function', grouping_function.slug`,
    filter: (sql, value) => sql`
      EXISTS (
        SELECT 1
        FROM posting_taxonomy AS filter_function
        WHERE filter_function.job_posting_id = cohort.job_posting_id
          AND filter_function.term_kind = 'dimension'
          AND filter_function.slug = ${value}
      )
    `,
  },
  week: {
    join: (sql) => sql`
      JOIN posting_lifespans AS grouping_week
        ON grouping_week.job_posting_id = cohort.job_posting_id
    `,
    keyValue: (sql) => sql`
      'week', to_char(
        date_trunc('week', grouping_week.first_seen AT TIME ZONE 'UTC'),
        'YYYY-MM-DD'
      )
    `,
    // `week` is deliberately absent from FILTER_DIMENSIONS. This exists only
    // to make the lookup total over Grouping; the engine never selects it.
    filter: (sql) => sql`TRUE`,
  },
} as const satisfies Record<Grouping, GroupingSource>;

function joinFragments(sql: ISql, fragments: readonly Fragment[], separator: " " | " AND " | ", ") {
  const [first, ...rest] = fragments;
  if (first == null) return undefined;

  return rest.reduce(
    (joined, fragment) =>
      separator === " "
        ? sql`${joined} ${fragment}`
        : separator === " AND "
          ? sql`${joined} AND ${fragment}`
          : sql`${joined}, ${fragment}`,
    first,
  );
}
