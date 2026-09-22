import { randomUUID } from "node:crypto";

import postgres from "postgres";
import { describe, expect, it } from "vitest";

import { testDsns } from "./test-dsn";

describe("market derivation", () => {
  it("derives curated markets from run-scoped snapshot location arrays", async (context) => {
    const { ownerDsn, readOnlyDsn } = testDsns();
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    const owner = postgres(ownerDsn);
    const readOnly = postgres(readOnlyDsn);
    const marker = `vitest-market-derivation-${randomUUID()}`;
    let companyId: string | undefined;

    const fixtures: Array<{
      key: string;
      locationText: string | null;
      locationTexts: string[] | null;
    }> = [
      {
        key: "sf",
        locationText: "San Francisco, CA",
        locationTexts: ["San Francisco, CA"],
      },
      {
        key: "everett",
        locationText: "Everett, WA",
        locationTexts: ["Everett, WA"],
      },
      {
        key: "san-jose",
        locationText: "San Jose, CA",
        locationTexts: ["San Jose, CA"],
      },
      {
        key: "south-san-francisco",
        locationText: "South San Francisco, CA",
        locationTexts: ["South San Francisco, CA"],
      },
      {
        key: "arlington",
        locationText: "Arlington, VA",
        locationTexts: ["Arlington, VA"],
      },
      {
        key: "newark",
        locationText: "Newark, NJ",
        locationTexts: ["Newark, NJ"],
      },
      {
        key: "redmond",
        locationText: "Redmond, WA",
        locationTexts: ["Redmond, WA"],
      },
      {
        key: "woodinville",
        locationText: "Woodinville, WA",
        locationTexts: ["Woodinville, WA"],
      },
      {
        key: "issaquah",
        locationText: "Issaquah, WA",
        locationTexts: ["Issaquah, WA"],
      },
      {
        key: "run-scoped-snapshot",
        locationText: "New York, NY",
        locationTexts: ["New York, NY"],
      },
      {
        key: "sf-and-new-york",
        locationText: "San Francisco, CA | New York City, NY",
        locationTexts: ["San Francisco, CA | New York City, NY"],
      },
      {
        key: "remote-us",
        locationText: "US - Remote",
        locationTexts: ["US - Remote"],
      },
      {
        key: "us-region",
        locationText: "United States",
        locationTexts: ["United States"],
      },
      {
        key: "not-applicable",
        locationText: "N/A",
        locationTexts: ["N/A"],
      },
      { key: "empty", locationText: "", locationTexts: [""] },
      { key: "null-array", locationText: null, locationTexts: null },
      { key: "empty-array", locationText: "", locationTexts: [] },
    ];
    const postingIdByKey = new Map<string, string>();

    try {
      const [company] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Market Derivation ${marker}`}, 'greenhouse', ${marker})
        RETURNING id
      `;
      companyId = company.id;

      const [run] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${companyId}, '2026-08-21T12:00:00Z', '2026-08-21T12:01:00Z', 'success', ${fixtures.length})
        RETURNING id
      `;

      for (const fixture of fixtures) {
        const [posting] = await owner<{ id: string }[]>`
          INSERT INTO job_postings (company_id, source_type, source_url)
          VALUES (${companyId}, 'ats', ${`https://example.test/${marker}/${fixture.key}`})
          RETURNING id
        `;
        postingIdByKey.set(fixture.key, posting.id);
        await owner`
          INSERT INTO posting_snapshots (
            job_posting_id,
            fetched_at,
            fetch_run_id,
            title,
            location_text,
            location_texts,
            raw_data
          )
          VALUES (
            ${posting.id},
            '2026-08-21T12:00:00Z',
            ${run.id},
            ${`Market fixture: ${fixture.key}`},
            ${fixture.locationText},
            ${fixture.locationTexts},
            ${owner.json({ marker })}
          )
        `;
      }

      const [failedRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, error_message)
        VALUES (${companyId}, '2026-08-22T12:00:00Z', '2026-08-22T12:01:00Z', 'failed', 'fixture failure')
        RETURNING id
      `;

      // These snapshots deliberately share the successful run's timestamp: the
      // second insert has the higher ID and must supply this posting's market.
      await owner`
        INSERT INTO posting_snapshots (
          job_posting_id,
          fetched_at,
          fetch_run_id,
          title,
          location_text,
          location_texts,
          raw_data
        )
        VALUES (
          ${postingIdByKey.get("run-scoped-snapshot")!},
          '2026-08-21T12:00:00Z',
          ${run.id},
          'Market fixture: run-scoped-snapshot tied current',
          'Seattle, WA',
          ${["Seattle, WA"]},
          ${owner.json({ marker })}
        )
      `;
      await owner`
        INSERT INTO posting_snapshots (
          job_posting_id,
          fetched_at,
          fetch_run_id,
          title,
          location_text,
          location_texts,
          raw_data
        )
        VALUES (
          ${postingIdByKey.get("run-scoped-snapshot")!},
          '2026-08-22T12:00:00Z',
          ${failedRun.id},
          'Market fixture: run-scoped-snapshot failed',
          'San Francisco, CA',
          ${["San Francisco, CA"]},
          ${owner.json({ marker })}
        )
      `;
      await owner`
        INSERT INTO posting_snapshots (
          job_posting_id,
          fetched_at,
          fetch_run_id,
          title,
          location_text,
          location_texts,
          raw_data
        )
        VALUES (
          ${postingIdByKey.get("run-scoped-snapshot")!},
          '2026-08-23T12:00:00Z',
          NULL,
          'Market fixture: run-scoped-snapshot null run',
          'Washington, DC',
          ${["Washington, DC"]},
          ${owner.json({ marker })}
        )
      `;

      async function marketsFor(key: string) {
        return readOnly<{ slug: string; name: string; kind: string }[]>`
          SELECT slug, name, kind
          FROM open_posting_markets
          WHERE job_posting_id = ${postingIdByKey.get(key)!}
          ORDER BY slug
        `;
      }

      expect(await marketsFor("sf")).toEqual([
        { slug: "sf-bay-area", name: "SF Bay Area", kind: "hub" },
      ]);
      expect(await marketsFor("everett")).toEqual([
        { slug: "seattle", name: "Seattle", kind: "hub" },
      ]);
      expect(await marketsFor("san-jose")).toEqual([
        { slug: "sf-bay-area", name: "SF Bay Area", kind: "hub" },
      ]);
      expect(await marketsFor("south-san-francisco")).toEqual([
        { slug: "sf-bay-area", name: "SF Bay Area", kind: "hub" },
      ]);
      expect(await marketsFor("arlington")).toEqual([
        { slug: "washington-dc", name: "Washington, DC", kind: "hub" },
      ]);
      expect(await marketsFor("newark")).toEqual([
        { slug: "new-york", name: "New York", kind: "hub" },
      ]);
      expect(await marketsFor("redmond")).toEqual([
        { slug: "seattle", name: "Seattle", kind: "hub" },
      ]);
      expect(await marketsFor("woodinville")).toEqual([
        { slug: "seattle", name: "Seattle", kind: "hub" },
      ]);
      expect(await marketsFor("issaquah")).toEqual([
        { slug: "seattle", name: "Seattle", kind: "hub" },
      ]);
      expect(await marketsFor("run-scoped-snapshot")).toEqual([
        { slug: "seattle", name: "Seattle", kind: "hub" },
      ]);
      expect(await marketsFor("sf-and-new-york")).toEqual([
        { slug: "new-york", name: "New York", kind: "hub" },
        { slug: "sf-bay-area", name: "SF Bay Area", kind: "hub" },
      ]);
      expect(await marketsFor("remote-us")).toEqual(
        expect.arrayContaining([
          {
            slug: "remote-us",
            name: "Remote — United States",
            kind: "remote",
          },
        ]),
      );
      expect(await marketsFor("us-region")).toEqual([
        { slug: "us", name: "United States", kind: "region" },
      ]);
      expect(await marketsFor("not-applicable")).toEqual([
        { slug: "unmapped", name: "Unmapped", kind: "region" },
      ]);
      expect(await marketsFor("empty")).toEqual([
        { slug: "unmapped", name: "Unmapped", kind: "region" },
      ]);
      expect(await marketsFor("null-array")).toEqual([
        { slug: "unmapped", name: "Unmapped", kind: "region" },
      ]);
      expect(await marketsFor("empty-array")).toEqual([
        { slug: "unmapped", name: "Unmapped", kind: "region" },
      ]);

      const taxonomyRows = await readOnly<{ term_kind: string; slug: string; name: string }[]>`
        SELECT term_kind, slug, name
        FROM open_posting_taxonomy
        WHERE job_posting_id = ${postingIdByKey.get("sf")!}
        ORDER BY term_kind, slug
      `;
      expect(taxonomyRows).toEqual([
        {
          term_kind: "market",
          slug: "sf-bay-area",
          name: "SF Bay Area",
        },
        {
          term_kind: "title_head",
          slug: "market-fixture-sf",
          name: "market fixture: sf",
        },
        {
          term_kind: "title_seniority",
          slug: "unstated",
          name: "Unstated",
        },
      ]);
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
