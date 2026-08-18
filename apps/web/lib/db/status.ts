import type { ISql } from "postgres";

import { getSql } from "./client";

// Counts are cast to int so postgres.js hands back numbers; count() is int8,
// which the driver otherwise returns as a string. Every figure here is far
// below 2^31.
export interface FetchHealthRow {
  jobs_harvested: number;
  companies_tracked: number;
  skills_identified: number;
  jobs_classified: number;
  jobs_open: number;
  runs_started_7d: number;
  runs_succeeded_7d: number;
  runs_failed_7d: number;
  last_run_at: Date | null;
  collecting_since: Date | null;
}

// Takes the connection as an argument so the db test can run this exact query
// on a pool or a transaction of its own; getSql() calls next/server's
// connection(), which throws outside a request scope. ISql is the base both
// Sql and TransactionSql extend.
export async function selectFetchHealth(sql: ISql): Promise<FetchHealthRow> {
  const [row] = await sql<FetchHealthRow[]>`
    SELECT
      (SELECT count(*)::int FROM job_postings) AS jobs_harvested,
      (SELECT count(*)::int FROM companies) AS companies_tracked,
      (SELECT count(DISTINCT skill_id)::int FROM job_posting_skills) AS skills_identified,
      (SELECT count(DISTINCT job_posting_id)::int FROM classifications) AS jobs_classified,
      (SELECT count(*)::int FROM open_postings) AS jobs_open,
      -- Frequency counts every run started, including ones still in progress.
      -- The error rate below divides by concluded runs only, so an in-flight
      -- run cannot read as a silent success.
      (SELECT count(*)::int FROM fetch_runs
        WHERE started_at > now() - interval '7 days') AS runs_started_7d,
      (SELECT (count(*) FILTER (WHERE status = 'success'))::int FROM fetch_runs
        WHERE started_at > now() - interval '7 days') AS runs_succeeded_7d,
      (SELECT (count(*) FILTER (WHERE status = 'failed'))::int FROM fetch_runs
        WHERE started_at > now() - interval '7 days') AS runs_failed_7d,
      (SELECT max(started_at) FROM fetch_runs) AS last_run_at,
      (SELECT min(started_at) FROM fetch_runs) AS collecting_since
  `;

  return row;
}

export async function getFetchHealth(): Promise<FetchHealthRow> {
  return selectFetchHealth(await getSql());
}
