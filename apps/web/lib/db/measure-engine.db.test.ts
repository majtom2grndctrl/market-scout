import { randomUUID } from "node:crypto";

import type { ISql } from "postgres";
import postgres from "postgres";
import { describe, expect, it } from "vitest";

import type { Composition } from "../composition";

import { runCompositionWith } from "./measure-engine";
import { testDsns } from "./test-dsn";

const now = new Date("2099-02-02T12:00:00Z");

describe("runCompositionWith", () => {
  it("computes every measure through the read-only client from cohort-safe read models", async (context) => {
    const { ownerDsn, readOnlyDsn } = testDsns();
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    const owner = postgres(ownerDsn);
    const readOnly = postgres(readOnlyDsn);
    const marker = `vitest-measure-engine-${randomUUID()}`;
    let companyId: string | undefined;
    let secondaryCompanyId: string | undefined;
    let emptyCompanyId: string | undefined;
    let noRunCompanyId: string | undefined;
    let currentRoleSlug: string | undefined;
    let supersededRoleSlug: string | undefined;

    try {
      const [company] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Measure Engine ${marker}`}, 'greenhouse', ${marker})
        RETURNING id
      `;
      companyId = company.id;
      const fixtureCompanyId = company.id;
      const fixtureCompanyName = `Vitest Measure Engine ${marker}`;

      const [firstRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${fixtureCompanyId}, '2099-01-05T12:00:00Z', '2099-01-05T12:01:00Z', 'success', 4)
        RETURNING id
      `;
      const [zeroRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${fixtureCompanyId}, '2099-01-12T12:00:00Z', '2099-01-12T12:01:00Z', 'success', 0)
        RETURNING id
      `;
      const [failedRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, error_message)
        VALUES (${fixtureCompanyId}, '2099-01-19T12:00:00Z', '2099-01-19T12:01:00Z', 'failed', 'fixture failure')
        RETURNING id
      `;
      const [currentRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${fixtureCompanyId}, '2099-01-26T12:00:00Z', '2099-01-26T12:01:00Z', 'success', 3)
        RETURNING id
      `;

      async function insertPosting(key: string) {
        const [posting] = await owner<{ id: string }[]>`
          INSERT INTO job_postings (company_id, source_type, source_url)
          VALUES (${fixtureCompanyId}, 'ats', ${`https://example.test/${marker}/${key}`})
          RETURNING id
        `;
        return posting;
      }

      async function snapshot(
        postingId: string,
        runId: string,
        fetchedAt: string,
        locations: readonly string[],
      ) {
        await owner`
          INSERT INTO posting_snapshots (
            job_posting_id, fetched_at, fetch_run_id, title, location_text, location_texts, raw_data
          )
          VALUES (
            ${postingId}, ${fetchedAt}, ${runId}, ${`Fixture ${marker}`}, ${locations.join(" | ")}, ${locations}, ${owner.json({ marker })}
          )
        `;
      }

      const [secondaryCompany] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Measure Engine Secondary ${marker}`}, 'greenhouse', ${`secondary-${marker}`})
        RETURNING id
      `;
      secondaryCompanyId = secondaryCompany.id;
      const [secondaryFirstRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${secondaryCompanyId}, '2099-01-05T12:00:00Z', '2099-01-05T12:01:00Z', 'success', 1)
        RETURNING id
      `;
      const [secondaryZeroRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${secondaryCompanyId}, '2099-01-19T12:00:00Z', '2099-01-19T12:01:00Z', 'success', 0)
        RETURNING id
      `;
      const [secondaryLowerTieRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${secondaryCompanyId}, '2099-01-26T12:00:00Z', '2099-01-26T12:01:00Z', 'success', 1)
        RETURNING id
      `;
      const [secondaryHigherTieRun] = await owner<{ id: string }[]>`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES (${secondaryCompanyId}, '2099-01-26T12:00:00Z', '2099-01-26T12:01:00Z', 'success', 1)
        RETURNING id
      `;
      const [secondaryPosting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${secondaryCompanyId}, 'ats', ${`https://example.test/${marker}/secondary`})
        RETURNING id
      `;
      const [secondaryLowerTiePosting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${secondaryCompanyId}, 'ats', ${`https://example.test/${marker}/secondary-lower-tie`})
        RETURNING id
      `;
      const [secondaryHigherTiePosting] = await owner<{ id: string }[]>`
        INSERT INTO job_postings (company_id, source_type, source_url)
        VALUES (${secondaryCompanyId}, 'ats', ${`https://example.test/${marker}/secondary-higher-tie`})
        RETURNING id
      `;
      await snapshot(secondaryPosting.id, secondaryFirstRun.id, '2099-01-05T12:00:00Z', ['Portland, OR']);
      await snapshot(secondaryPosting.id, secondaryZeroRun.id, '2099-01-19T12:00:00Z', ['Portland, OR']);
      await snapshot(secondaryLowerTiePosting.id, secondaryLowerTieRun.id, '2099-01-26T12:00:00Z', ['Moon base']);
      // Equal fetched_at values select the later snapshot id for posting_markets.
      await snapshot(secondaryHigherTiePosting.id, secondaryHigherTieRun.id, '2099-01-26T12:00:00Z', ['Moon base']);
      await snapshot(secondaryHigherTiePosting.id, secondaryHigherTieRun.id, '2099-01-26T12:00:00Z', ['Seattle, WA']);

      const [emptyCompany] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Measure Engine Empty ${marker}`}, 'greenhouse', ${`empty-${marker}`})
        RETURNING id
      `;
      emptyCompanyId = emptyCompany.id;
      const fixtureEmptyCompanyId = emptyCompany.id;
      const emptyCompanyName = `Vitest Measure Engine Empty ${marker}`;
      await owner`
        INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
        VALUES
          (${emptyCompanyId}, '2099-01-05T12:00:00Z', '2099-01-05T12:01:00Z', 'success', 0),
          (${emptyCompanyId}, '2099-02-03T12:00:00Z', '2099-02-03T12:01:00Z', 'success', 0)
      `;
      const [noRunCompany] = await owner<{ id: string }[]>`
        INSERT INTO companies (name, ats, board_token)
        VALUES (${`Vitest Measure Engine No Runs ${marker}`}, 'greenhouse', ${`no-runs-${marker}`})
        RETURNING id
      `;
      noRunCompanyId = noRunCompany.id;
      const fixtureNoRunCompanyId = noRunCompany.id;

      const multiMarket = await insertPosting("multi-market");
      const unmappedOpen = await insertPosting("unmapped-open");
      const closedMarket = await insertPosting("closed-market");
      const singleRun = await insertPosting("single-run");
      const lateOpen = await insertPosting("late-open");

      await snapshot(
        multiMarket.id,
        firstRun.id,
        "2099-01-05T12:00:00Z",
        ["San Francisco, CA | New York, NY"],
      );
      await snapshot(unmappedOpen.id, firstRun.id, "2099-01-05T12:00:00Z", ["Moon base"]);
      await snapshot(closedMarket.id, firstRun.id, "2099-01-05T12:00:00Z", ["Seattle, WA"]);
      await snapshot(singleRun.id, firstRun.id, "2099-01-05T12:00:00Z", ["Mars colony"]);
      await snapshot(multiMarket.id, zeroRun.id, "2099-01-12T12:00:00Z", ["San Francisco, CA | New York, NY"]);
      await snapshot(closedMarket.id, zeroRun.id, "2099-01-12T12:00:00Z", ["Seattle, WA"]);
      // A failed run cannot establish openness or advance a lifespan.
      await snapshot(closedMarket.id, failedRun.id, "2099-01-19T12:00:00Z", ["New York, NY"]);
      await snapshot(multiMarket.id, currentRun.id, "2099-01-26T12:00:00Z", ["San Francisco, CA | New York, NY"]);
      await snapshot(unmappedOpen.id, currentRun.id, "2099-01-26T12:00:00Z", ["Moon base"]);
      await snapshot(lateOpen.id, currentRun.id, "2099-01-26T12:00:00Z", ["Seattle, WA"]);

      currentRoleSlug = `${marker}-current-role`;
      supersededRoleSlug = `${marker}-superseded-role`;
      const [currentRole] = await owner<{ id: string }[]>`
        INSERT INTO canonical_roles (slug, name)
        VALUES (${currentRoleSlug}, 'Current fixture role')
        RETURNING id
      `;
      const [supersededRole] = await owner<{ id: string }[]>`
        INSERT INTO canonical_roles (slug, name)
        VALUES (${supersededRoleSlug}, 'Superseded fixture role')
        RETURNING id
      `;
      const [currentDimension] = await owner<{ id: string }[]>`
        INSERT INTO role_dimensions (slug, name)
        VALUES (${`${marker}-current-function`}, 'Current fixture function')
        RETURNING id
      `;
      const [supersededDimension] = await owner<{ id: string }[]>`
        INSERT INTO role_dimensions (slug, name)
        VALUES (${`${marker}-superseded-function`}, 'Superseded fixture function')
        RETURNING id
      `;
      await owner`
        INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
        VALUES
          (${currentRole.id}, ${currentDimension.id}),
          (${supersededRole.id}, ${supersededDimension.id})
      `;

      const [supersededClassification] = await owner<{ id: string }[]>`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${multiMarket.id}, 'fixture-model', 'v1', '2099-01-11T12:00:00Z', 'junior')
        RETURNING id
      `;
      const [currentClassification] = await owner<{ id: string }[]>`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${multiMarket.id}, 'fixture-model', 'v2', '2099-01-11T12:00:00Z', 'senior')
        RETURNING id
      `;
      await owner`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${closedMarket.id}, 'fixture-model', 'v1', '2099-01-11T12:00:00Z', 'mid')
      `;
      await owner`
        INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
        VALUES (${singleRun.id}, 'fixture-model', 'v1', '2099-01-11T12:00:00Z', 'lead')
      `;
      await owner`
        INSERT INTO job_posting_roles (classification_id, role_id)
        VALUES
          (${supersededClassification.id}, ${supersededRole.id}),
          (${currentClassification.id}, ${currentRole.id})
      `;

      const companyFilter = [{ dim: "company" as const, value: companyId }];
      const result = async (composition: Composition) => {
        const measured = await runCompositionWith(readOnly, composition, { now });
        expect(measured.ok).toBe(true);
        if (!measured.ok) throw new Error("fixture composition unexpectedly failed validation");
        return measured.value;
      };

      const [latestClassification] = await readOnly<{ classification_id: string }[]>`
        SELECT classification_id
        FROM latest_classifications
        WHERE job_posting_id = ${multiMarket.id}
      `;
      expect(latestClassification).toEqual({ classification_id: currentClassification.id });

      const marketsFromTiedSnapshots = await readOnly<{ slug: string }[]>`
        SELECT slug
        FROM posting_markets
        WHERE job_posting_id = ${secondaryHigherTiePosting.id}
      `;
      expect(marketsFromTiedSnapshots).toEqual([{ slug: "seattle" }]);

      const tiedAsOfPostings = await readOnly<{ job_posting_id: string }[]>`
        SELECT job_posting_id
        FROM open_postings_as_of('2099-01-26T12:00:00Z')
        WHERE job_posting_id = ${secondaryLowerTiePosting.id}
          OR job_posting_id = ${secondaryHigherTiePosting.id}
      `;
      expect(tiedAsOfPostings).toEqual([{ job_posting_id: secondaryHigherTiePosting.id }]);

      await readOnly.begin("isolation level repeatable read", async (snapshot) => {
        const directOpen = await snapshot<{ count: number }[]>`
          SELECT count(*)::int AS count FROM open_postings
        `;
        // The requisition expectation has to be measured inside the same
        // repeatable-read snapshot as the posting count, or a concurrent fetch
        // could move one number and not the other.
        const directRequisitions = await snapshot<{ count: number }[]>`
          SELECT count(DISTINCT (company_id, requisition_key))::int AS count
          FROM posting_requisitions
        `;
        const measured = await runCompositionWith(
          snapshot,
          { measure: "count", cohort: "open", encoding: "ranked_bars" },
          { now },
        );
        expect(measured).toMatchObject({ ok: true });
        if (!measured.ok) throw new Error("unfiltered count unexpectedly failed validation");
        expect(measured.value.rows).toEqual([
          { keys: {}, value: directOpen[0].count, requisitions: directRequisitions[0].count },
        ]);
      });

      // No fixture posting here carries an ATS requisition_key, so the view's
      // 'posting:<id>' fallback gives each open posting its own requisition.
      expect(
        (await result({ measure: "count", cohort: "open", filter: companyFilter, encoding: "ranked_bars" })).rows,
      ).toEqual([{ keys: {}, value: 3, requisitions: 3 }]);

      const directlyTypedDuplicates = await result({
        measure: "count",
        cohort: "open",
        groupBy: ["company", "company"],
        filter: [...companyFilter, ...companyFilter],
        encoding: "ranked_bars",
      });
      expect(directlyTypedDuplicates.groupBy).toEqual(["company"]);
      expect(directlyTypedDuplicates.rows).toEqual([
        { keys: { company: fixtureCompanyName }, value: 3, requisitions: 3 },
      ]);

      expect(
        (await result({
          measure: "delta",
          cohort: "open",
          filter: companyFilter,
          window: { weeks: 2 },
          encoding: "diverging_bars",
        })).rows,
      ).toEqual([{ keys: {}, value: 1 }]);

      const rate = await result({
        measure: "rate",
        cohort: "all",
        groupBy: ["week"],
        filter: companyFilter,
        encoding: "line",
      });
      const rateByWeek = new Map(rate.rows.map((row) => [row.keys.week, row]));
      expect(rateByWeek.get("2099-01-05")).toMatchObject({ value: 4 });
      expect(rateByWeek.get("2099-01-12")).toEqual({ keys: { week: "2099-01-12" }, value: 0 });
      expect(rateByWeek.get("2099-01-19")).toEqual({ keys: { week: "2099-01-19" }, value: 0, gap: true });
      expect(rateByWeek.get("2099-01-26")).toMatchObject({ value: 1 });
      expect(rateByWeek.get("2099-02-02")).toEqual({ keys: { week: "2099-02-02" }, value: 0, gap: true });

      const rateByCompany = await result({
        measure: "rate",
        cohort: "all",
        groupBy: ["company", "week"],
        filter: companyFilter,
        encoding: "line",
      });
      const primaryRateRows = rateByCompany.rows;
      expect(primaryRateRows).toHaveLength(5);
      expect(primaryRateRows.every((row) => row.keys.week !== undefined && row.keys.company !== undefined)).toBe(true);
      expect(primaryRateRows.find((row) => row.keys.week === "2099-01-19")).toEqual({
        keys: { company: fixtureCompanyName, week: "2099-01-19" },
        value: 0,
        gap: true,
      });

      const seededCompanyRate = await result({
        measure: "rate",
        cohort: "all",
        groupBy: ["company", "week"],
        filter: [{ dim: "company", value: fixtureEmptyCompanyId }],
        encoding: "line",
      });
      expect(seededCompanyRate.groupBy).toEqual(["company", "week"]);
      expect(seededCompanyRate.rows).toHaveLength(5);
      expect(seededCompanyRate.rows.every((row) => row.keys.company === emptyCompanyName)).toBe(true);
      expect(seededCompanyRate.rows.find((row) => row.keys.week === "2099-02-02")).toEqual({
        keys: { company: emptyCompanyName, week: "2099-02-02" },
        value: 0,
        gap: true,
      });

      expect(
        (
          await result({
            measure: "rate",
            cohort: "all",
            groupBy: ["company", "week"],
            filter: [{ dim: "company", value: fixtureNoRunCompanyId }],
            encoding: "line",
          })
        ).rows,
      ).toEqual([]);

      const multiCompanyMarketRate = await result({
        measure: "rate",
        cohort: "all",
        groupBy: ["week"],
        filter: [{ dim: "market", value: "seattle" }],
        encoding: "line",
      });
      expect(multiCompanyMarketRate.rows.find((row) => row.keys.week === "2099-01-19")).toEqual({
        keys: { week: "2099-01-19" },
        value: 0,
      });

      const topRateSeries = await result({
        measure: "rate",
        cohort: "all",
        groupBy: ["market", "week"],
        filter: companyFilter,
        sort: "desc",
        limit: 1,
        encoding: "line",
      });
      expect(topRateSeries.rows).toHaveLength(5);
      expect(topRateSeries.rows.every((row) => row.keys.market === "seattle")).toBe(true);
      expect(topRateSeries.rows.map((row) => row.keys.week)).toEqual([
        "2099-01-05",
        "2099-01-12",
        "2099-01-19",
        "2099-01-26",
        "2099-02-02",
      ]);

      // A week-grouped `count` breaks its line on the failed-run week exactly as
      // `rate` does, and — because it buckets by first-seen — reproduces rate's
      // per-week new-posting counts over the same fixture.
      const countByWeek = await result({
        measure: "count",
        cohort: "all",
        groupBy: ["week"],
        filter: companyFilter,
        encoding: "line",
      });
      expect(countByWeek.rows).toHaveLength(5);
      const countByWeekMap = new Map(countByWeek.rows.map((row) => [row.keys.week, row]));
      expect(countByWeekMap.get("2099-01-05")).toEqual({ keys: { week: "2099-01-05" }, value: 4 });
      // 2099-01-12 had a successful run but no new postings: a genuine zero, NOT a gap.
      expect(countByWeekMap.get("2099-01-12")).toEqual({ keys: { week: "2099-01-12" }, value: 0 });
      expect(countByWeekMap.get("2099-01-19")).toEqual({ keys: { week: "2099-01-19" }, value: 0, gap: true });
      expect(countByWeekMap.get("2099-01-26")).toEqual({ keys: { week: "2099-01-26" }, value: 1 });
      expect(countByWeekMap.get("2099-02-02")).toEqual({ keys: { week: "2099-02-02" }, value: 0, gap: true });

      // Company-grouped: gap granularity is per-(company, week), mirroring rate.
      const countByCompanyWeek = await result({
        measure: "count",
        cohort: "all",
        groupBy: ["company", "week"],
        filter: companyFilter,
        encoding: "line",
      });
      expect(countByCompanyWeek.rows).toHaveLength(5);
      expect(countByCompanyWeek.rows.find((row) => row.keys.week === "2099-01-05")).toEqual({
        keys: { company: fixtureCompanyName, week: "2099-01-05" },
        value: 4,
      });
      expect(countByCompanyWeek.rows.find((row) => row.keys.week === "2099-01-19")).toEqual({
        keys: { company: fixtureCompanyName, week: "2099-01-19" },
        value: 0,
        gap: true,
      });

      // A week-grouped `delta` walks the same calendar: the signed diff surfaces
      // on real weeks, the failed/absent-run weeks surface as gaps, and a week
      // whose diff is genuinely zero stays ungapped.
      const deltaByWeek = await result({
        measure: "delta",
        cohort: "open",
        groupBy: ["week"],
        filter: companyFilter,
        window: { weeks: 2 },
        encoding: "line",
      });
      expect(deltaByWeek.rows).toHaveLength(5);
      const deltaByWeekMap = new Map(deltaByWeek.rows.map((row) => [row.keys.week, row]));
      expect(deltaByWeekMap.get("2099-01-05")).toEqual({ keys: { week: "2099-01-05" }, value: 0 });
      expect(deltaByWeekMap.get("2099-01-12")).toEqual({ keys: { week: "2099-01-12" }, value: 0 });
      expect(deltaByWeekMap.get("2099-01-19")).toEqual({ keys: { week: "2099-01-19" }, value: 0, gap: true });
      expect(deltaByWeekMap.get("2099-01-26")).toEqual({ keys: { week: "2099-01-26" }, value: 1 });
      expect(deltaByWeekMap.get("2099-02-02")).toEqual({ keys: { week: "2099-02-02" }, value: 0, gap: true });

      // A `share` over a non-company grouping + week: the gap is scope-wide per
      // week and applied to EVERY group key in that week. `market` is
      // multi-valued (multiMarket counts under two markets), so this also pins
      // that the 0-valued gap/genuine-zero rows leave the global normalizer —
      // the sum of real assignments — summing to 1.0. (`share` is open-cohort.)
      const shareByMarketWeek = await result({
        measure: "share",
        cohort: "open",
        groupBy: ["market", "week"],
        filter: companyFilter,
        encoding: "line",
      });
      // four markets (sf-bay-area, new-york, seattle, unmapped) across five weeks
      expect(shareByMarketWeek.rows).toHaveLength(20);
      expect(shareByMarketWeek.rows.reduce((total, row) => total + row.value, 0)).toBeCloseTo(1);
      const marketGapRows = shareByMarketWeek.rows.filter((row) => row.keys.week === "2099-01-19");
      expect(marketGapRows).toHaveLength(4);
      expect(marketGapRows.every((row) => row.value === 0 && row.gap === true)).toBe(true);
      expect(new Set(marketGapRows.map((row) => row.keys.market))).toEqual(
        new Set(["sf-bay-area", "new-york", "seattle", "unmapped"]),
      );
      // 2099-01-12 has a successful run but no new postings: genuine zeros, no gap.
      const marketZeroRows = shareByMarketWeek.rows.filter((row) => row.keys.week === "2099-01-12");
      expect(marketZeroRows).toHaveLength(4);
      expect(marketZeroRows.every((row) => row.value === 0 && row.gap === undefined)).toBe(true);
      const seattleArrival = shareByMarketWeek.rows.find(
        (row) => row.keys.market === "seattle" && row.keys.week === "2099-01-26",
      );
      expect(seattleArrival?.value).toBeCloseTo(0.25);
      expect(seattleArrival?.gap).toBeUndefined();

      // The same scaffold over a classified dimension. Only one open posting is
      // classified in this fixture (senior), so the series is a single key, but
      // it still breaks its line on the failed-run week and normalizes to 1.0.
      const shareBySeniorityWeek = await result({
        measure: "share",
        cohort: "open",
        groupBy: ["seniority", "week"],
        filter: companyFilter,
        encoding: "line",
      });
      expect(shareBySeniorityWeek.rows).toHaveLength(5);
      expect(shareBySeniorityWeek.rows.reduce((total, row) => total + row.value, 0)).toBeCloseTo(1);
      const seniorityByWeek = new Map(shareBySeniorityWeek.rows.map((row) => [row.keys.week, row]));
      expect(seniorityByWeek.get("2099-01-05")).toEqual({
        keys: { seniority: "senior", week: "2099-01-05" },
        value: 1,
      });
      expect(seniorityByWeek.get("2099-01-12")).toEqual({
        keys: { seniority: "senior", week: "2099-01-12" },
        value: 0,
      });
      expect(seniorityByWeek.get("2099-01-19")).toEqual({
        keys: { seniority: "senior", week: "2099-01-19" },
        value: 0,
        gap: true,
      });

      expect(
        (await result({ measure: "age", cohort: "open", filter: companyFilter, encoding: "histogram" })).rows.map(
          (row) => row.value,
        ),
      ).toEqual([28, 28, 7]);
      expect(
        (await result({ measure: "lifespan", cohort: "closed", filter: companyFilter, encoding: "histogram" })).rows.map(
          (row) => row.value,
        ),
      ).toEqual([7, 0]);

      const share = await result({
        measure: "share",
        cohort: "open",
        groupBy: ["market"],
        filter: companyFilter,
        encoding: "ranked_bars",
      });
      expect(share.rows.reduce((total, row) => total + row.value, 0)).toBeCloseTo(1);
      expect(new Map(share.rows.map((row) => [row.keys.market, row.value]))).toEqual(
        new Map([
          ["new-york", 0.25],
          ["seattle", 0.25],
          ["sf-bay-area", 0.25],
          ["unmapped", 0.25],
        ]),
      );

      expect(
        (await result({
          measure: "count",
          cohort: "closed",
          groupBy: ["market"],
          filter: companyFilter,
          encoding: "ranked_bars",
        })).rows,
      ).toEqual([
        { keys: { market: "seattle" }, value: 1 },
        { keys: { market: "unmapped" }, value: 1 },
      ]);

      const openDenominator = await result({
        measure: "count",
        cohort: "open",
        groupBy: ["seniority"],
        filter: companyFilter,
        encoding: "ranked_bars",
      });
      expect(openDenominator.denominator).toEqual({ classified: 1, total: 3 });
      const closedDenominator = await result({
        measure: "count",
        cohort: "closed",
        groupBy: ["seniority"],
        filter: companyFilter,
        encoding: "ranked_bars",
      });
      expect(closedDenominator.denominator).toEqual({ classified: 2, total: 2 });
      expect(
        (await result({
          measure: "count",
          cohort: "open",
          groupBy: ["function"],
          filter: companyFilter,
          encoding: "ranked_bars",
        })).rows,
      ).toEqual([{ keys: { function: `${marker}-current-function` }, value: 1, requisitions: 1 }]);
      expect(
        (await result({
          measure: "count",
          cohort: "open",
          filter: [{ dim: "role", value: supersededRoleSlug }],
          encoding: "ranked_bars",
        })).rows,
      ).toEqual([{ keys: {}, value: 0, requisitions: 0 }]);

      const literalMetacharacters = await result({
        measure: "count",
        cohort: "open",
        filter: [{ dim: "role", value: `${currentRoleSlug}' OR '1'='1` }],
        encoding: "ranked_bars",
      });
      expect(literalMetacharacters.rows).toEqual([{ keys: {}, value: 0, requisitions: 0 }]);
    } finally {
      if (companyId !== undefined) {
        await owner`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM job_postings WHERE company_id = ${companyId}`;
        await owner`DELETE FROM fetch_runs WHERE company_id = ${companyId}`;
        await owner`DELETE FROM companies WHERE id = ${companyId}`;
      }
      if (secondaryCompanyId !== undefined) {
        await owner`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${secondaryCompanyId})`;
        await owner`DELETE FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${secondaryCompanyId})`;
        await owner`DELETE FROM job_postings WHERE company_id = ${secondaryCompanyId}`;
        await owner`DELETE FROM fetch_runs WHERE company_id = ${secondaryCompanyId}`;
        await owner`DELETE FROM companies WHERE id = ${secondaryCompanyId}`;
      }
      if (emptyCompanyId !== undefined) {
        await owner`DELETE FROM fetch_runs WHERE company_id = ${emptyCompanyId}`;
        await owner`DELETE FROM companies WHERE id = ${emptyCompanyId}`;
      }
      if (noRunCompanyId !== undefined) {
        await owner`DELETE FROM companies WHERE id = ${noRunCompanyId}`;
      }
      if (currentRoleSlug !== undefined || supersededRoleSlug !== undefined) {
        await owner`
          DELETE FROM canonical_role_dimensions
          WHERE canonical_role_id IN (
            SELECT id FROM canonical_roles WHERE slug IN (${currentRoleSlug ?? ""}, ${supersededRoleSlug ?? ""})
          )
        `;
        await owner`DELETE FROM canonical_roles WHERE slug IN (${currentRoleSlug ?? ""}, ${supersededRoleSlug ?? ""})`;
      }
      await owner`DELETE FROM role_dimensions WHERE slug LIKE ${`${marker}%`}`;
      await Promise.all([owner.end(), readOnly.end()]);
    }
    // This suite drives every measure end-to-end over a live Postgres in one
    // test; the weekly-series gap cases add several calendar-walk queries, so it
    // runs past Vitest's 5s default.
  }, 30_000);

  // `posting_requisitions` answers "how many jobs, not listings". Only a `count`
  // over the open cohort, grouped so no row holds part of a requisition, can
  // carry that number — so these cases pin both the number and the rows that
  // must not carry one.
  it("counts requisitions beside postings for open counts, and omits the key everywhere else", async (context) => {
    const { ownerDsn, readOnlyDsn } = testDsns();
    if (!ownerDsn || !readOnlyDsn) {
      context.skip();
      return;
    }

    const owner = postgres(ownerDsn);
    const readOnly = postgres(readOnlyDsn);
    const marker = `vitest-requisitions-${randomUUID()}`;
    const companyIds: string[] = [];

    try {
      async function insertCompany(label: string) {
        const name = `Vitest Requisitions ${label} ${marker}`;
        const [company] = await owner<{ id: string }[]>`
          INSERT INTO companies (name, ats, board_token)
          VALUES (${name}, 'greenhouse', ${`${label}-${marker}`})
          RETURNING id
        `;
        companyIds.push(company.id);
        return { id: company.id, name };
      }

      async function insertRun(companyId: string, startedAt: string, postingsCount: number) {
        const [run] = await owner<{ id: string }[]>`
          INSERT INTO fetch_runs (company_id, started_at, completed_at, status, postings_count)
          VALUES (${companyId}, ${startedAt}, ${startedAt}, 'success', ${postingsCount})
          RETURNING id
        `;
        return run.id;
      }

      // `requisition_key` is seeded the way the fetcher writes it — a column on
      // the snapshot — because the view reads the snapshot belonging to the run
      // that established openness, not the posting row.
      async function insertPosting(
        companyId: string,
        runId: string,
        fetchedAt: string,
        key: string,
        requisitionKey: string | null,
        locations: readonly string[] = ["Seattle, WA"],
      ) {
        const [posting] = await owner<{ id: string }[]>`
          INSERT INTO job_postings (company_id, source_type, source_url)
          VALUES (${companyId}, 'ats', ${`https://example.test/${marker}/${key}`})
          RETURNING id
        `;
        await owner`
          INSERT INTO posting_snapshots (
            job_posting_id, fetched_at, fetch_run_id, title, location_text, location_texts,
            raw_data, requisition_key
          )
          VALUES (
            ${posting.id}, ${fetchedAt}, ${runId}, ${`Fixture ${key}`}, ${locations.join(" | ")},
            ${locations}, ${owner.json({ marker })}, ${requisitionKey}
          )
        `;
        return posting.id;
      }

      async function classifyAs(postingId: string, roleId: string) {
        const [classification] = await owner<{ id: string }[]>`
          INSERT INTO classifications (job_posting_id, model, prompt_version, classified_at, seniority)
          VALUES (${postingId}, 'fixture-model', 'v1', '2099-01-26T12:00:00Z', 'senior')
          RETURNING id
        `;
        await owner`
          INSERT INTO job_posting_roles (classification_id, role_id)
          VALUES (${classification.id}, ${roleId})
        `;
      }

      // A Greenhouse board that lists one requisition once per location: five
      // open postings, three requisition keys. The sixth posting appears only in
      // the earlier run, so it is closed.
      const fanOut = await insertCompany("Fan Out");
      const fanOutEarlierRun = await insertRun(fanOut.id, "2099-01-19T12:00:00Z", 6);
      const fanOutCurrentRun = await insertRun(fanOut.id, "2099-01-26T12:00:00Z", 5);
      await insertPosting(fanOut.id, fanOutCurrentRun, "2099-01-26T12:00:00Z", "fanout-sf", "4771001");
      await insertPosting(fanOut.id, fanOutCurrentRun, "2099-01-26T12:00:00Z", "fanout-nyc", "4771001");
      await insertPosting(fanOut.id, fanOutCurrentRun, "2099-01-26T12:00:00Z", "fanout-sea", "4771002");
      await insertPosting(fanOut.id, fanOutCurrentRun, "2099-01-26T12:00:00Z", "fanout-aus", "4771002");
      await insertPosting(fanOut.id, fanOutCurrentRun, "2099-01-26T12:00:00Z", "fanout-rem", "4771003");
      await insertPosting(fanOut.id, fanOutEarlierRun, "2099-01-19T12:00:00Z", "fanout-closed", "4771004");

      // A board whose platform supplies no key at all. The view's
      // 'posting:<id>' fallback must give each posting its own requisition, so
      // the company reports one requisition per posting rather than one total.
      const noKeys = await insertCompany("No Keys");
      const noKeysRun = await insertRun(noKeys.id, "2099-01-26T12:00:00Z", 4);
      await insertPosting(noKeys.id, noKeysRun, "2099-01-26T12:00:00Z", "nokeys-a", null);
      await insertPosting(noKeys.id, noKeysRun, "2099-01-26T12:00:00Z", "nokeys-b", null);
      await insertPosting(noKeys.id, noKeysRun, "2099-01-26T12:00:00Z", "nokeys-c", null);
      await insertPosting(noKeys.id, noKeysRun, "2099-01-26T12:00:00Z", "nokeys-d", null);

      // The per-location listing in its purest form: one requisition, two open
      // postings, two different markets. Grouping by `market` therefore puts the
      // siblings in different rows.
      const twoMarket = await insertCompany("Two Market");
      const twoMarketRun = await insertRun(twoMarket.id, "2099-01-26T12:00:00Z", 2);
      const twoMarketKey = `two-market-${marker}`;
      await insertPosting(twoMarket.id, twoMarketRun, "2099-01-26T12:00:00Z", "twomarket-nyc", twoMarketKey, [
        "New York, NY",
      ]);
      await insertPosting(twoMarket.id, twoMarketRun, "2099-01-26T12:00:00Z", "twomarket-sf", twoMarketKey, [
        "San Francisco, CA",
      ]);

      // Two companies spelling the same requisition key. Keys are board-scoped,
      // so these are two jobs and not one; a distinct count over the key alone
      // would merge them. The shared role exists to give one query group that
      // spans both companies — the only place the merge is visible, since a
      // company-grouped row already holds a single company.
      const collidingKey = `colliding-${marker}`;
      const sharedRoleSlug = `${marker}-shared-role`;
      const [sharedRole] = await owner<{ id: string }[]>`
        INSERT INTO canonical_roles (slug, name)
        VALUES (${sharedRoleSlug}, 'Shared fixture role')
        RETURNING id
      `;
      const collidingA = await insertCompany("Colliding A");
      const collidingARun = await insertRun(collidingA.id, "2099-01-26T12:00:00Z", 2);
      const collidingB = await insertCompany("Colliding B");
      const collidingBRun = await insertRun(collidingB.id, "2099-01-26T12:00:00Z", 1);
      for (const [companyId, runId, key] of [
        [collidingA.id, collidingARun, "colliding-a-1"],
        [collidingA.id, collidingARun, "colliding-a-2"],
        [collidingB.id, collidingBRun, "colliding-b-1"],
      ] as const) {
        await classifyAs(
          await insertPosting(companyId, runId, "2099-01-26T12:00:00Z", key, collidingKey),
          sharedRole.id,
        );
      }

      const measure = async (composition: Composition) => {
        const measured = await runCompositionWith(readOnly, composition, { now });
        expect(measured.ok).toBe(true);
        if (!measured.ok) throw new Error("fixture composition unexpectedly failed validation");
        return measured.value;
      };
      const byCompany = (companyId: string, cohort: Composition["cohort"]): Composition => ({
        measure: "count",
        cohort,
        groupBy: ["company"],
        filter: [{ dim: "company", value: companyId }],
        encoding: "ranked_bars",
      });

      expect((await measure(byCompany(fanOut.id, "open"))).rows).toEqual([
        { keys: { company: fanOut.name }, value: 5, requisitions: 3 },
      ]);
      expect((await measure(byCompany(noKeys.id, "open"))).rows).toEqual([
        { keys: { company: noKeys.name }, value: 4, requisitions: 4 },
      ]);

      // `posting_requisitions` covers open postings only. An absent key, not a
      // 0, is what says "this row cannot answer."
      for (const cohort of ["closed", "all"] as const) {
        const rows = (await measure(byCompany(fanOut.id, cohort))).rows;
        expect(rows).toEqual([{ keys: { company: fanOut.name }, value: cohort === "closed" ? 1 : 6 }]);
        expect(rows.every((row) => !("requisitions" in row))).toBe(true);
      }

      // The weekly path resolves each week from the run calendar, and the view
      // answers only for the current snapshot — so no week row can carry a
      // requisition count. `open` is the load-bearing cohort here: under `all`
      // the cohort rule alone would suppress the column.
      const byWeek = (await measure({
        measure: "count",
        cohort: "open",
        groupBy: ["week"],
        filter: [{ dim: "company", value: fanOut.id }],
        encoding: "line",
      })).rows;
      expect(byWeek.length).toBeGreaterThan(0);
      expect(byWeek.every((row) => !("requisitions" in row))).toBe(true);

      // `market` splits a requisition's siblings across group rows: both rows
      // below would say `requisitions: 1` for the same job, and a consumer
      // totalling the series would read two jobs out of one. `value` fans out
      // the same way but stays an honest posting count, so only the requisition
      // column is suppressed.
      const byMarket = (await measure({
        measure: "count",
        cohort: "open",
        groupBy: ["market"],
        filter: [{ dim: "company", value: twoMarket.id }],
        encoding: "ranked_bars",
      })).rows;
      expect(byMarket).toEqual([
        { keys: { market: "new-york" }, value: 1 },
        { keys: { market: "sf-bay-area" }, value: 1 },
      ]);
      expect(byMarket.every((row) => !("requisitions" in row))).toBe(true);
      // Ungrouped, the same two postings correctly resolve to one job.
      expect((await measure(byCompany(twoMarket.id, "open"))).rows).toEqual([
        { keys: { company: twoMarket.name }, value: 2, requisitions: 1 },
      ]);

      // `share` is a proportion, so an absolute second count beside it would
      // mean nothing. Grouped by `company` on purpose: under a sibling-splitting
      // grouping this assertion would still pass with the share half of the gate
      // deleted.
      const shareRows = (await measure({
        measure: "share",
        cohort: "open",
        groupBy: ["company"],
        filter: [{ dim: "company", value: fanOut.id }],
        encoding: "ranked_bars",
      })).rows;
      expect(shareRows).toEqual([{ keys: { company: fanOut.name }, value: 1 }]);
      expect(shareRows.every((row) => !("requisitions" in row))).toBe(true);

      // One group spanning both colliding companies: three postings, two jobs.
      // Counting distinct over the key alone reports 1 here.
      const collisionFilter = [{ dim: "role" as const, value: sharedRoleSlug }];
      expect(
        (await measure({ measure: "count", cohort: "open", filter: collisionFilter, encoding: "ranked_bars" })).rows,
      ).toEqual([{ keys: {}, value: 3, requisitions: 2 }]);
      expect(
        (await measure({
          measure: "count",
          cohort: "open",
          groupBy: ["company"],
          filter: collisionFilter,
          encoding: "ranked_bars",
        })).rows,
      ).toEqual([
        { keys: { company: collidingA.name }, value: 2, requisitions: 1 },
        { keys: { company: collidingB.name }, value: 1, requisitions: 1 },
      ]);
    } finally {
      for (const companyId of companyIds) {
        await owner`DELETE FROM posting_snapshots WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM classifications WHERE job_posting_id IN (SELECT id FROM job_postings WHERE company_id = ${companyId})`;
        await owner`DELETE FROM job_postings WHERE company_id = ${companyId}`;
        await owner`DELETE FROM fetch_runs WHERE company_id = ${companyId}`;
        await owner`DELETE FROM companies WHERE id = ${companyId}`;
      }
      // After the classifications, whose cascade takes job_posting_roles with
      // them: the role FK is ON DELETE RESTRICT.
      await owner`DELETE FROM canonical_roles WHERE slug LIKE ${`${marker}%`}`;
      await Promise.all([owner.end(), readOnly.end()]);
    }
  }, 30_000);

  it("refuses an invalid composition before invoking the SQL client", async () => {
    let calls = 0;
    const sql = new Proxy((() => undefined) as unknown as ISql, {
      apply() {
        calls += 1;
        return undefined;
      },
    });

    const result = await runCompositionWith(sql, {
      measure: "count",
      cohort: "open",
      encoding: "line",
    });

    expect(result.ok).toBe(false);
    expect(calls).toBe(0);
  });

  it("refuses rate without its required weekly axis before invoking the SQL client", async () => {
    let calls = 0;
    const sql = new Proxy((() => undefined) as unknown as ISql, {
      apply() {
        calls += 1;
        return undefined;
      },
    });

    const result = await runCompositionWith(sql, {
      measure: "rate",
      cohort: "all",
      encoding: "ranked_bars",
    });

    expect(result).toMatchObject({
      ok: false,
      error: { issues: [{ code: "measure_needs_week", path: ["groupBy"] }] },
    });
    expect(calls).toBe(0);
  });
});
