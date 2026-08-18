import { randomUUID } from "node:crypto";

import postgres from "postgres";
import { describe, expect, it } from "vitest";

import type { FetchHealthRow } from "./status";

// The query aggregates the whole database, so fixtures are measured as a delta
// rather than against an absolute total.
function delta(before: FetchHealthRow, after: FetchHealthRow) {
  return {
    jobs_harvested: after.jobs_harvested - before.jobs_harvested,
    companies_tracked: after.companies_tracked - before.companies_tracked,
    skills_identified: after.skills_identified - before.skills_identified,
    jobs_classified: after.jobs_classified - before.jobs_classified,
    jobs_open: after.jobs_open - before.jobs_open,
    runs_started_7d: after.runs_started_7d - before.runs_started_7d,
    runs_succeeded_7d: after.runs_succeeded_7d - before.runs_succeeded_7d,
    runs_failed_7d: after.runs_failed_7d - before.runs_failed_7d,
  };
}

// Thrown to roll the fixture transaction back once assertions have run.
const rollback = new Error("rollback fixture transaction");

describe("getFetchHealth", () => {
  it("counts a re-fetched posting once, a multi-classification posting once, a skill once across classifications, and counts 7-day runs both as started and split by status", async (context) => {
    const ownerDsn = process.env.DATABASE_URL;
    const readOnlyDsn = process.env.DATABASE_URL_RO;
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    // Imported here, not at module scope: lib/db/client.ts throws on import when
    // DATABASE_URL_RO is unset, which would fail collection instead of skipping.
    const { selectFetchHealth } = await import("./status");

    const owner = postgres(ownerDsn);
    const marker = `vitest-status-${randomUUID()}`;

    try {
      // Repeatable read pins one snapshot for both counts, so rows committed by
      // the other db test file running in parallel cannot move the delta.
      // Rolling back afterwards is the whole teardown.
      await owner.begin("isolation level repeatable read", async (tx) => {
        const before = await selectFetchHealth(tx);

        const [company] = await tx<{ id: string }[]>`
          INSERT INTO companies (name, ats, board_token)
          VALUES (${`Vitest Status ${marker}`}, 'greenhouse', ${marker})
          RETURNING id
        `;

        // Inside the 7-day window: two successes, one failure, and one
        // in_progress that counts toward runs_started_7d but toward neither
        // status split. Outside it: one success, one failure, which no 7-day
        // figure may see.
        const [currentRun] = await tx<{ id: string }[]>`
          INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
          VALUES (${company.id}, now() - interval '1 day', now() - interval '1 day', 'success', 2)
          RETURNING id
        `;
        await tx`
          INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
          VALUES
            (${company.id}, now() - interval '3 days', now() - interval '3 days', 'success', 1),
            (${company.id}, now() - interval '2 days', now() - interval '2 days', 'failed', 0),
            (${company.id}, now() - interval '1 hour', NULL, 'in_progress', NULL),
            (${company.id}, now() - interval '30 days', now() - interval '30 days', 'success', 1),
            (${company.id}, now() - interval '30 days', now() - interval '30 days', 'failed', 0)
        `;

        const [refetchedPosting] = await tx<{ id: string }[]>`
          INSERT INTO job_postings (company_id, source_type, source_url)
          VALUES (${company.id}, 'ats', ${`https://example.test/${marker}/refetched`})
          RETURNING id
        `;
        const [classifiedPosting] = await tx<{ id: string }[]>`
          INSERT INTO job_postings (company_id, source_type, source_url)
          VALUES (${company.id}, 'ats', ${`https://example.test/${marker}/classified`})
          RETURNING id
        `;

        // Three snapshots over two postings: the counts diverge only if a
        // metric is reading snapshots where it should read postings.
        await tx`
          INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, raw_data)
          VALUES
            (${refetchedPosting.id}, now() - interval '1 day', ${currentRun.id}, 'Refetched fixture role', ${JSON.stringify({ marker })}),
            (${refetchedPosting.id}, now() - interval '23 hours', ${currentRun.id}, 'Refetched fixture role', ${JSON.stringify({ marker })}),
            (${classifiedPosting.id}, now() - interval '1 day', ${currentRun.id}, 'Classified fixture role', ${JSON.stringify({ marker })})
        `;

        const [firstClassification] = await tx<{ id: string }[]>`
          INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
          VALUES (${classifiedPosting.id}, 'fixture-model', 'v1', now() - interval '2 days', 'junior')
          RETURNING id
        `;
        const [secondClassification] = await tx<{ id: string }[]>`
          INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
          VALUES (${classifiedPosting.id}, 'fixture-model', 'v2', now() - interval '1 day', 'senior')
          RETURNING id
        `;

        // One skill, carried by both classifications.
        const [skill] = await tx<{ id: string }[]>`
          INSERT INTO skills (slug, name)
          VALUES (${`${marker}-skill`}, 'Fixture skill')
          RETURNING id
        `;
        await tx`
          INSERT INTO job_posting_skills (classification_id, skill_id)
          VALUES (${firstClassification.id}, ${skill.id}), (${secondClassification.id}, ${skill.id})
        `;

        const after = await selectFetchHealth(tx);

        const measured = delta(before, after);

        // Six runs inserted, four inside the window: runs_started_7d is 4, not
        // 6 (the 30-day rows are out of window) and not 3 (in_progress started,
        // so frequency counts it).
        expect(measured).toEqual({
          jobs_harvested: 2,
          companies_tracked: 1,
          skills_identified: 1,
          jobs_classified: 1,
          jobs_open: 2,
          runs_started_7d: 4,
          runs_succeeded_7d: 2,
          runs_failed_7d: 1,
        });

        // The asymmetry the page depends on, stated as a property rather than
        // as literals: frequency counts started runs, the error rate divides by
        // concluded ones. Rewriting the frequency subquery as a sum of the two
        // status counts would satisfy the literals above only by changing them,
        // but it can never satisfy this.
        expect(measured.runs_started_7d).toBeGreaterThan(
          measured.runs_succeeded_7d + measured.runs_failed_7d,
        );

        throw rollback;
      });
    } catch (error) {
      if (error !== rollback) {
        throw error;
      }
    } finally {
      await owner.end();
    }
  });

  it("is readable through the read-only role", async (context) => {
    const readOnlyDsn = process.env.DATABASE_URL_RO;
    if (!readOnlyDsn) {
      context.skip();
      return;
    }

    const { selectFetchHealth } = await import("./status");
    const readOnly = postgres(readOnlyDsn);

    try {
      // The page runs on this DSN; a missing grant on any counted table or on
      // open_postings would surface here rather than as a 500.
      const row = await selectFetchHealth(readOnly);

      expect(row.jobs_harvested).toBeTypeOf("number");
      expect(row.companies_tracked).toBeTypeOf("number");
      expect(row.skills_identified).toBeTypeOf("number");
      expect(row.jobs_classified).toBeTypeOf("number");
      expect(row.jobs_open).toBeTypeOf("number");
      expect(row.runs_started_7d).toBeTypeOf("number");
      expect(row.runs_succeeded_7d).toBeTypeOf("number");
      expect(row.runs_failed_7d).toBeTypeOf("number");
    } finally {
      await readOnly.end();
    }
  });
});
