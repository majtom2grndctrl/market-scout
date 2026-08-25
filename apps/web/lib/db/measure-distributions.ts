import type { MeasureContext, MeasureRow } from "./measure-engine";
import { createMeasureScope } from "./measure-scope";

export async function runDistributionMeasure(context: MeasureContext): Promise<readonly MeasureRow[]> {
  const { sql, composition } = context;
  const scope = createMeasureScope(sql, composition);
  const where = scope.filters ?? sql`TRUE`;
  const duration = durationExpression(context);
  const order = orderClause(sql, composition.sort);
  const limit = composition.limit == null ? undefined : sql`LIMIT ${composition.limit}`;

  const rows = await sql<DistributionSqlRow[]>`
    SELECT
      ${scope.keys ?? sql`'{}'::jsonb`} AS keys,
      ${duration} AS value
    FROM ${scope.cohort} AS cohort
    JOIN job_postings AS posting ON posting.id = cohort.job_posting_id
    JOIN posting_lifespans AS lifespan ON lifespan.job_posting_id = cohort.job_posting_id
    ${scope.joins ?? sql``}
    WHERE ${where}
    ${order}
    ${limit ?? sql``}
  `;

  return rows.map((row) => ({ keys: row.keys, value: row.value }));
}

interface DistributionSqlRow {
  readonly keys: Record<string, string>;
  readonly value: number;
}

function durationExpression(context: MeasureContext) {
  const { sql, composition, now } = context;

  switch (composition.measure) {
    case "age":
      return sql`(EXTRACT(EPOCH FROM (${now} - lifespan.first_seen)) / 86400)::double precision`;
    case "lifespan":
      return sql`(EXTRACT(EPOCH FROM (lifespan.last_seen - lifespan.first_seen)) / 86400)::double precision`;
    case "count":
    case "delta":
    case "rate":
    case "share":
      throw new Error(`aggregate measure "${composition.measure}" reached the distribution runner`);
  }
}

function orderClause(sql: MeasureContext["sql"], sort: "asc" | "desc" | undefined) {
  if (sort === "asc") return sql`ORDER BY value ASC, keys ASC`;
  if (sort === "desc") return sql`ORDER BY value DESC, keys ASC`;
  return sql`ORDER BY keys ASC`;
}
