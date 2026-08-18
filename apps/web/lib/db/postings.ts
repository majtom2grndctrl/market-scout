import type { ISql } from "postgres";

import { getSql } from "./client";

export interface OpenPostingsDisplayRow {
  job_posting_id: string;
  company_id: string;
  company_name: string;
  run_started_at: Date;
  title: string | null;
  location_text: string | null;
  workplace_type: string | null;
  compensation_min: string | null;
  compensation_max: string | null;
  compensation_currency: string | null;
  classification_id: string | null;
  seniority: string | null;
  workplace_type_resolved: string | null;
  workplace_type_source: string | null;
}

// The page renders every returned row in one server-rendered payload (no
// pagination yet), so the result set is capped here. Rows beyond the cap are
// simply not shown; increase this once the page paginates instead of
// rendering the full list.
const MAX_OPEN_POSTINGS = 500;

// Takes the connection as an argument so the db test can run this exact query
// on its read-only client; getSql() calls next/server's connection(), which
// throws outside a request scope.
export async function selectOpenPostings(sql: ISql): Promise<OpenPostingsDisplayRow[]> {
  return sql<OpenPostingsDisplayRow[]>`
    SELECT
      job_posting_id,
      company_id,
      company_name,
      run_started_at,
      title,
      location_text,
      workplace_type,
      compensation_min,
      compensation_max,
      compensation_currency,
      classification_id,
      seniority,
      workplace_type_resolved,
      workplace_type_source
    FROM open_postings_display
    ORDER BY run_started_at DESC, job_posting_id DESC
    LIMIT ${MAX_OPEN_POSTINGS}
  `;
}

export async function listOpenPostings(): Promise<OpenPostingsDisplayRow[]> {
  return selectOpenPostings(await getSql());
}
