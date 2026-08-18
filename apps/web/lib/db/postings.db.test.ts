import { randomUUID } from "node:crypto";

import postgres from "postgres";
import { describe, expect, it } from "vitest";

describe("selectOpenPostings", () => {
  it("returns every explicitly selected view column through the read-only client", async (context) => {
    const ownerDsn = process.env.DATABASE_URL;
    const readOnlyDsn = process.env.DATABASE_URL_RO;
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    const { selectOpenPostings } = await import("./postings");
    const owner = postgres(ownerDsn);
    const readOnly = postgres(readOnlyDsn);
    const marker = `vitest-postings-${randomUUID()}`;
    let companyId: string | undefined;

    try {
      const [company] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Postings ${marker}`}, 'greenhouse', ${marker})
        RETURNING id
      `;
      companyId = company.id;

      const [run] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${company.id}, now(), now(), 'success', 1)
        RETURNING id
      `;
      const [posting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${company.id}, 'ats', ${`https://example.test/${marker}`})
        RETURNING id
      `;
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, workplace_type, raw_data)
        VALUES (${posting.id}, now(), ${run.id}, 'Fixture role', 'remote', ${JSON.stringify({ marker })})
      `;

      const row = (await selectOpenPostings(readOnly)).find(
        (candidate) => candidate.job_posting_id === posting.id,
      );

      expect(row).toBeDefined();
      expect(Object.keys(row ?? {})).toEqual([
        "job_posting_id",
        "company_id",
        "company_name",
        "run_started_at",
        "title",
        "location_text",
        "workplace_type",
        "compensation_min",
        "compensation_max",
        "compensation_currency",
        "classification_id",
        "seniority",
        "workplace_type_resolved",
        "workplace_type_source",
      ]);
    } finally {
      if (companyId !== undefined) {
        await owner`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM job_postings WHERE company_id = ${companyId}`;
        await owner`DELETE FROM fetch_runs WHERE company_id = ${companyId}`;
        await owner`DELETE FROM companies WHERE id = ${companyId}`;
      }
      await Promise.all([owner.end(), readOnly.end()]);
    }
  });
});
