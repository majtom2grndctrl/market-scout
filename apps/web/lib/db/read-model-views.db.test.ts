import { randomUUID } from "node:crypto";

import postgres from "postgres";
import { describe, expect, it } from "vitest";

describe("read-model views", () => {
  it("keeps the prior successful snapshot open after a later failed run, displays content only from the run that established openness, excludes null-run snapshots, and prefers the newest ID when timestamps tie", async (context) => {
    const ownerDsn = process.env.DATABASE_URL;
    const readOnlyDsn = process.env.DATABASE_URL_RO;
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    const owner = postgres(ownerDsn);
    const readOnly = postgres(readOnlyDsn);
    const marker = `vitest-read-model-${randomUUID()}`;
    let companyId: string | undefined;

    try {
      const [company] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Read Model ${marker}`}, 'greenhouse', ${marker})
        RETURNING id
      `;
      companyId = company.id;

      const [successfulRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${companyId}, '2026-08-01T12:00:00Z', '2026-08-01T12:01:00Z', 'success', 1)
        RETURNING id
      `;
      const [failedRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, error_message)
        VALUES (${companyId}, '2026-08-02T12:00:00Z', '2026-08-02T12:01:00Z', 'failed', 'fixture failure')
        RETURNING id
      `;

      const [openPosting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${companyId}, 'ats', ${`https://example.test/${marker}/open`})
        RETURNING id
      `;
      const [nullRunPosting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${companyId}, 'ats', ${`https://example.test/${marker}/null-run`})
        RETURNING id
      `;
      const [unclassifiedPosting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${companyId}, 'ats', ${`https://example.test/${marker}/unclassified`})
        RETURNING id
      `;

      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, location_text, workplace_type, raw_data)
        VALUES (${openPosting.id}, '2026-07-31T12:00:00Z', ${successfulRun.id}, 'Older fixture role', 'Office', 'onsite', ${JSON.stringify({ marker })})
      `;
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, location_text, workplace_type, raw_data)
        VALUES (${openPosting.id}, '2026-08-01T12:00:00Z', ${successfulRun.id}, 'Open fixture role', 'Remote', 'remote', ${JSON.stringify({ marker })})
      `;
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, location_text, workplace_type, raw_data)
        VALUES (${openPosting.id}, '2026-08-01T12:00:00Z', ${successfulRun.id}, 'Tied fixture role', 'Hybrid', 'hybrid', ${JSON.stringify({ marker })})
      `;
      // The next two snapshots are newer than every successful-run snapshot for
      // the open posting, so they win an unscoped "latest snapshot" lookup. They
      // must not: openness comes from successfulRun, and so must the displayed
      // content. Attaching them to the open posting is what makes the display
      // assertion below test the run-scoping rather than pass vacuously.
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, location_text, workplace_type, raw_data)
        VALUES (${openPosting.id}, '2026-08-02T12:00:00Z', ${failedRun.id}, 'Failed-run fixture role', 'Office', 'onsite', ${JSON.stringify({ marker })})
      `;
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, location_text, workplace_type, raw_data)
        VALUES (${openPosting.id}, '2026-08-02T12:00:00Z', NULL, 'Null-run fixture role for open posting', 'Office', 'onsite', ${JSON.stringify({ marker })})
      `;
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, raw_data)
        VALUES (${nullRunPosting.id}, '2026-08-01T12:00:00Z', NULL, 'Null-run fixture role', ${JSON.stringify({ marker })})
      `;
      await owner`
        INSERT INTO posting_snapshots (job_posting_id, fetched_at, fetch_run_id, title, raw_data)
        VALUES (${unclassifiedPosting.id}, '2026-08-01T12:00:00Z', ${successfulRun.id}, 'Unclassified fixture role', ${JSON.stringify({ marker })})
      `;

      const [supersededClassification] = await owner<{ id: string }[]>`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${openPosting.id}, 'fixture-model', 'v1', '2026-08-01T12:00:00Z', 'junior')
        RETURNING id
      `;
      await owner`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${openPosting.id}, 'fixture-model', 'v2', '2026-08-02T12:00:00Z', 'senior')
      `;
      const [tiedCurrentClassification] = await owner<{ id: string }[]>`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${openPosting.id}, 'fixture-model', 'v3', '2026-08-02T12:00:00Z', 'lead')
        RETURNING id
      `;
      const [role] = await owner<{ id: string }[]>`
        INSERT INTO canonical_roles (slug, name)
        VALUES (${`${marker}-role`}, 'Fixture role')
        RETURNING id
      `;
      const [secondRole] = await owner<{ id: string }[]>`
        INSERT INTO canonical_roles (slug, name)
        VALUES (${`${marker}-second-role`}, 'Second fixture role')
        RETURNING id
      `;
      const [supersededRole] = await owner<{ id: string }[]>`
        INSERT INTO canonical_roles (slug, name)
        VALUES (${`${marker}-superseded-role`}, 'Superseded fixture role')
        RETURNING id
      `;
      const [specialization] = await owner<{ id: string }[]>`
        INSERT INTO specializations (slug, name)
        VALUES (${`${marker}-specialization`}, 'Fixture specialization')
        RETURNING id
      `;
      const [supersededSpecialization] = await owner<{ id: string }[]>`
        INSERT INTO specializations (slug, name)
        VALUES (${`${marker}-superseded-specialization`}, 'Superseded fixture specialization')
        RETURNING id
      `;
      const [skill] = await owner<{ id: string }[]>`
        INSERT INTO skills (slug, name)
        VALUES (${`${marker}-skill`}, 'Fixture skill')
        RETURNING id
      `;
      const [supersededSkill] = await owner<{ id: string }[]>`
        INSERT INTO skills (slug, name)
        VALUES (${`${marker}-superseded-skill`}, 'Superseded fixture skill')
        RETURNING id
      `;
      const [dimension] = await owner<{ id: string }[]>`
        INSERT INTO role_dimensions (slug, name)
        VALUES (${`${marker}-dimension`}, 'Fixture dimension')
        RETURNING id
      `;
      const [supersededDimension] = await owner<{ id: string }[]>`
        INSERT INTO role_dimensions (slug, name)
        VALUES (${`${marker}-superseded-dimension`}, 'Superseded fixture dimension')
        RETURNING id
      `;

      await owner`
        INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
        VALUES (${role.id}, ${dimension.id}), (${secondRole.id}, ${dimension.id}), (${supersededRole.id}, ${supersededDimension.id})
      `;
      await owner`
        INSERT INTO job_posting_roles (classification_id, role_id)
        VALUES (${supersededClassification.id}, ${supersededRole.id}), (${tiedCurrentClassification.id}, ${role.id}), (${tiedCurrentClassification.id}, ${secondRole.id})
      `;
      await owner`
        INSERT INTO job_posting_specializations (classification_id, specialization_id)
        VALUES (${supersededClassification.id}, ${supersededSpecialization.id}), (${tiedCurrentClassification.id}, ${specialization.id})
      `;
      await owner`
        INSERT INTO job_posting_skills (classification_id, skill_id)
        VALUES (${supersededClassification.id}, ${supersededSkill.id}), (${tiedCurrentClassification.id}, ${skill.id})
      `;

      const openRows = await readOnly<{ job_posting_id: string; fetch_run_id: string }[]>`
        SELECT job_posting_id, fetch_run_id
        FROM open_postings
        WHERE job_posting_id = ${openPosting.id}
      `;
      const nullRunRows = await readOnly<{ job_posting_id: string }[]>`
        SELECT job_posting_id
        FROM open_postings
        WHERE job_posting_id = ${nullRunPosting.id}
      `;
      const displayRows = await readOnly<
        {
          job_posting_id: string;
          run_started_at: Date;
          title: string;
          location_text: string;
          workplace_type: string;
          classification_id: string;
          seniority: string;
        }[]
      >`
        SELECT job_posting_id, run_started_at, title, location_text, workplace_type, classification_id, seniority
        FROM open_postings_display
        WHERE job_posting_id = ${openPosting.id}
      `;
      const unclassifiedDisplayRows = await readOnly<
        { job_posting_id: string; classification_id: string | null; seniority: string | null }[]
      >`
        SELECT job_posting_id, classification_id, seniority
        FROM open_postings_display
        WHERE job_posting_id = ${unclassifiedPosting.id}
      `;
      const taxonomyRows = await readOnly<
        { term_kind: string; slug: string; name: string }[]
      >`
        SELECT term_kind, slug, name
        FROM open_posting_taxonomy
        WHERE job_posting_id = ${openPosting.id}
        ORDER BY term_kind, slug
      `;

      expect(openRows).toEqual([
        {
          job_posting_id: openPosting.id,
          fetch_run_id: successfulRun.id,
        },
      ]);
      expect(nullRunRows).toEqual([]);
      expect(displayRows).toEqual([
        {
          job_posting_id: openPosting.id,
          run_started_at: new Date('2026-08-01T12:00:00Z'),
          title: 'Tied fixture role',
          location_text: 'Hybrid',
          workplace_type: 'hybrid',
          classification_id: tiedCurrentClassification.id,
          seniority: 'lead',
        },
      ]);
      expect(unclassifiedDisplayRows).toEqual([
        {
          job_posting_id: unclassifiedPosting.id,
          classification_id: null,
          seniority: null,
        },
      ]);
      expect(taxonomyRows).toEqual([
        {
          term_kind: 'dimension',
          slug: `${marker}-dimension`,
          name: 'Fixture dimension',
        },
        {
          term_kind: 'role',
          slug: `${marker}-role`,
          name: 'Fixture role',
        },
        {
          term_kind: 'role',
          slug: `${marker}-second-role`,
          name: 'Second fixture role',
        },
        {
          term_kind: 'skill',
          slug: `${marker}-skill`,
          name: 'Fixture skill',
        },
        {
          term_kind: 'specialization',
          slug: `${marker}-specialization`,
          name: 'Fixture specialization',
        },
      ]);
    } finally {
      if (companyId !== undefined) {
        await owner`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM job_posting_roles WHERE classification_id IN (SELECT id FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId}))`;
        await owner`DELETE FROM job_posting_specializations WHERE classification_id IN (SELECT id FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId}))`;
        await owner`DELETE FROM job_posting_skills WHERE classification_id IN (SELECT id FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId}))`;
        await owner`DELETE FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM job_postings WHERE company_id = ${companyId}`;
        await owner`DELETE FROM fetch_runs WHERE company_id = ${companyId}`;
        await owner`DELETE FROM companies WHERE id = ${companyId}`;
      }
      await owner`
        DELETE FROM canonical_role_dimensions
        WHERE canonical_role_id IN (SELECT id FROM canonical_roles WHERE slug LIKE ${`${marker}%`})
           OR dimension_id IN (SELECT id FROM role_dimensions WHERE slug LIKE ${`${marker}%`})
      `;
      await owner`DELETE FROM canonical_roles WHERE slug LIKE ${`${marker}%`}`;
      await owner`DELETE FROM specializations WHERE slug LIKE ${`${marker}%`}`;
      await owner`DELETE FROM skills WHERE slug LIKE ${`${marker}%`}`;
      await owner`DELETE FROM role_dimensions WHERE slug LIKE ${`${marker}%`}`;
      await Promise.all([owner.end(), readOnly.end()]);
    }
  });

  it("derives workplace type with provenance from ATS, raw data, and normalized location text", async (context) => {
    const ownerDsn = process.env.DATABASE_URL;
    const readOnlyDsn = process.env.DATABASE_URL_RO;
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    const owner = postgres(ownerDsn);
    const readOnly = postgres(readOnlyDsn);
    const marker = `vitest-workplace-type-${randomUUID()}`;
    let companyId: string | undefined;

    const fixtures = [
      {
        key: "ats-precedence",
        signal: "ATS workplace_type onsite",
        locationText: "US-Remote",
        workplaceType: "onsite",
        rawData: {},
        resolved: "onsite",
        source: "ats",
      },
      {
        key: "telecommuting-true",
        signal: "telecommuting true",
        locationText: "Berlin",
        workplaceType: null,
        rawData: { telecommuting: true },
        resolved: "remote",
        source: "raw_data",
      },
      {
        key: "telecommuting-false",
        signal: "telecommuting false",
        locationText: "US-Remote",
        workplaceType: null,
        rawData: { telecommuting: false },
        resolved: null,
        source: null,
      },
      {
        key: "remote-false",
        signal: "Remote false",
        locationText: "US-Remote",
        workplaceType: null,
        rawData: { metadata: [{ name: "Remote", value: false }] },
        resolved: null,
        source: null,
      },
      {
        key: "remote-true",
        signal: "Remote true",
        locationText: "Austin, TX",
        workplaceType: null,
        rawData: { metadata: [{ name: "Remote", value: true }] },
        resolved: "remote",
        source: "raw_data",
      },
      {
        key: "telecommuting-false-location-type-hybrid",
        signal: "telecommuting false with Location Type Hybrid (Travel-Required)",
        locationText: "Austin, TX",
        workplaceType: null,
        rawData: {
          telecommuting: false,
          metadata: [
            { name: "Location Type", value: "Hybrid (Travel-Required)" },
          ],
        },
        resolved: "hybrid",
        source: "raw_data",
      },
      {
        key: "location-type-array",
        signal: "Location Type as a one-element array",
        locationText: "Austin, TX",
        workplaceType: null,
        rawData: { metadata: [{ name: "Location Type", value: ["Remote"] }] },
        resolved: "remote",
        source: "raw_data",
      },
      {
        key: "remote-false-location-hybrid",
        signal: "Remote false with hybrid location text",
        locationText: "Hybrid - New York",
        workplaceType: null,
        rawData: { metadata: [{ name: "Remote", value: false }] },
        resolved: "hybrid",
        source: "location_text",
      },
      {
        key: "remote-false-location-onsite",
        signal: "Remote false with onsite location text",
        locationText: "Onsite - New York",
        workplaceType: null,
        rawData: { metadata: [{ name: "Remote", value: false }] },
        resolved: "onsite",
        source: "location_text",
      },
      {
        key: "location-type-onsite",
        signal: "Location Type On-Site",
        locationText: "Austin, TX",
        workplaceType: null,
        rawData: { metadata: [{ name: "Location Type", value: "On-Site" }] },
        resolved: "onsite",
        source: "raw_data",
      },
      {
        key: "location-type-hybrid",
        signal: "Location Type Hybrid (Travel-Required)",
        locationText: "Austin, TX",
        workplaceType: null,
        rawData: {
          metadata: [
            { name: "Location Type", value: "Hybrid (Travel-Required)" },
          ],
        },
        resolved: "hybrid",
        source: "raw_data",
      },
      {
        key: "location-type-unrecognized",
        signal: "Location Type Flexible",
        locationText: "US-Remote",
        workplaceType: null,
        rawData: { metadata: [{ name: "Location Type", value: "Flexible" }] },
        resolved: "remote",
        source: "location_text",
      },
      {
        key: "location-type-null",
        signal: "Location Type JSON null",
        locationText: "US-Remote",
        workplaceType: null,
        rawData: { metadata: [{ name: "Location Type", value: null }] },
        resolved: "remote",
        source: "location_text",
      },
      {
        key: "job-location-remote",
        signal: "Job Posting Location Germany - Remote",
        locationText: "Munich",
        workplaceType: null,
        rawData: {
          metadata: [
            { name: "Job Posting Location", value: ["Germany - Remote"] },
          ],
        },
        resolved: "remote",
        source: "raw_data",
      },
      {
        key: "job-location-onsite",
        signal: "Job Posting Location Israel - Office",
        locationText: "Tel Aviv",
        workplaceType: null,
        rawData: {
          metadata: [
            { name: "Job Posting Location", value: ["Israel - Office"] },
          ],
        },
        resolved: "onsite",
        source: "raw_data",
      },
      {
        key: "job-location-headquarters",
        signal: "Job Posting Location MD - Columbia - Headquarters",
        locationText: "Columbia, MD",
        workplaceType: null,
        rawData: {
          metadata: [
            {
              name: "Job Posting Location",
              value: ["MD - Columbia - Headquarters"],
            },
          ],
        },
        resolved: "onsite",
        source: "raw_data",
      },
      {
        key: "job-location-empty",
        signal: "Job Posting Location empty array",
        locationText: "US-Remote",
        workplaceType: null,
        rawData: { metadata: [{ name: "Job Posting Location", value: [] }] },
        resolved: "remote",
        source: "location_text",
      },
      {
        key: "metadata-null",
        signal: "metadata JSON null",
        locationText: "Hybrid- Fremont, CA",
        workplaceType: null,
        rawData: { metadata: null },
        resolved: "hybrid",
        source: "location_text",
      },
      {
        key: "remote-friendly",
        signal: "no raw signal",
        locationText: "Remote-Friendly, United States",
        workplaceType: null,
        rawData: {},
        resolved: null,
        source: null,
      },
      {
        key: "location-remote",
        signal: "no raw signal",
        locationText: "US-Remote",
        workplaceType: null,
        rawData: {},
        resolved: "remote",
        source: "location_text",
      },
      {
        key: "location-hybrid",
        signal: "no raw signal",
        locationText: "Hybrid- Fremont, CA",
        workplaceType: null,
        rawData: {},
        resolved: "hybrid",
        source: "location_text",
      },
      {
        key: "location-hybrid-whitespace",
        signal: "no raw signal",
        locationText: " Hybrid- Fremont,  CA ",
        workplaceType: null,
        rawData: {},
        resolved: "hybrid",
        source: "location_text",
      },
      {
        key: "location-remote-friendly-nbsp",
        signal: "no raw signal",
        locationText: "Remote\u00a0Friendly, United States",
        workplaceType: null,
        rawData: {},
        resolved: null,
        source: null,
      },
      {
        key: "location-hybrid-over-remote",
        signal: "no raw signal",
        locationText: "Hybrid- Remote, CA",
        workplaceType: null,
        rawData: {},
        resolved: "hybrid",
        source: "location_text",
      },
      {
        key: "location-onsite",
        signal: "no raw signal",
        locationText: "Onsite- Salem, OR",
        workplaceType: null,
        rawData: {},
        resolved: "onsite",
        source: "location_text",
      },
      {
        key: "location-none",
        signal: "no raw signal",
        locationText: "Sacramento, CA",
        workplaceType: null,
        rawData: {},
        resolved: null,
        source: null,
      },
    ];

    try {
      const [company] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Workplace Type ${marker}`}, 'greenhouse', ${marker})
        RETURNING id
      `;
      companyId = company.id;

      const [successfulRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${companyId}, '2026-08-03T12:00:00Z', '2026-08-03T12:01:00Z', 'success', ${fixtures.length})
        RETURNING id
      `;

      const expectedByPostingId = new Map<
        string,
        {
          key: string;
          signal: string;
          resolved: string | null;
          source: string | null;
        }
      >();
      for (const fixture of fixtures) {
        const [posting] = await owner<{ id: string }[]>`
          INSERT INTO job_postings (company_id, source_type, source_url)
          VALUES (${companyId}, 'ats', ${`https://example.test/${marker}/${fixture.key}`})
          RETURNING id
        `;
        expectedByPostingId.set(posting.id, fixture);
        await owner`
          INSERT INTO posting_snapshots (
            job_posting_id,
            fetched_at,
            fetch_run_id,
            title,
            location_text,
            workplace_type,
            raw_data
          )
          VALUES (
            ${posting.id},
            '2026-08-03T12:00:00Z',
            ${successfulRun.id},
            ${`Workplace fixture: ${fixture.signal}`},
            ${fixture.locationText},
            ${fixture.workplaceType},
            ${owner.json(fixture.rawData)}
          )
        `;
      }

      const columns = await readOnly<{ column_name: string; data_type: string }[]>`
        SELECT column_name, data_type
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'open_postings_display'
        ORDER BY ordinal_position
      `;
      expect(columns).toEqual([
        { column_name: "job_posting_id", data_type: "bigint" },
        { column_name: "company_id", data_type: "bigint" },
        { column_name: "company_name", data_type: "text" },
        { column_name: "run_started_at", data_type: "timestamp with time zone" },
        { column_name: "title", data_type: "text" },
        { column_name: "location_text", data_type: "text" },
        { column_name: "workplace_type", data_type: "text" },
        { column_name: "compensation_min", data_type: "bigint" },
        { column_name: "compensation_max", data_type: "bigint" },
        { column_name: "compensation_currency", data_type: "text" },
        { column_name: "classification_id", data_type: "bigint" },
        { column_name: "seniority", data_type: "text" },
        { column_name: "workplace_type_resolved", data_type: "text" },
        { column_name: "workplace_type_source", data_type: "text" },
      ]);

      const rows = await readOnly<
        {
          job_posting_id: string;
          workplace_type_resolved: string | null;
          workplace_type_source: string | null;
        }[]
      >`
        SELECT *
        FROM open_postings_display
        WHERE company_id = ${companyId}
        ORDER BY job_posting_id
      `;

      expect(rows).toHaveLength(fixtures.length);
      expect(Object.keys(rows[0] ?? {})).toEqual(columns.map(({ column_name }) => column_name));
      expect(
        rows.map((row) => ({
          key: expectedByPostingId.get(row.job_posting_id)?.key,
          signal: expectedByPostingId.get(row.job_posting_id)?.signal,
          resolved: row.workplace_type_resolved,
          source: row.workplace_type_source,
        })),
      ).toEqual(
        expect.arrayContaining(
          fixtures.map(({ key, signal, resolved, source }) => ({
            key,
            signal,
            resolved,
            source,
          })),
        ),
      );
    } finally {
      if (companyId !== undefined) {
        await owner`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM job_postings WHERE company_id = ${companyId}`;
        await owner`DELETE FROM fetch_runs WHERE company_id = ${companyId}`;
        await owner`DELETE FROM companies WHERE id = ${companyId}`;
      }
      await Promise.all([owner.end(), readOnly.end()]);
    }
  });
});
