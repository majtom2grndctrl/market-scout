-- Row-count census for the fixture purge, run immediately before and
-- immediately after purge_test_fixtures.sql with no fetcher or batch-enrich
-- run in between. The operator reconciles: every table in the purge's delete
-- order must drop by exactly the count that script reported, the three link
-- tables must drop by the number of rows the deleted classifications owned
-- (they cascade), and every other table must be unchanged.
--
-- Read-only. Companion to internal/db/setup/purge_test_fixtures.sql; that
-- file's header carries the full operator run sequence.
--
--     docker exec -i market-scout-db psql -U market_scout -d market_scout -f - \
--       < internal/db/setup/purge_test_fixtures_counts.sql
--
-- Every fixture_rows predicate mirrors the purge statement of the same step, so
-- the expected drop is readable off this table before anything is deleted.
--
-- open_postings is a view, not a purge target: its count is included because the
-- purge removes fixture postings from the open cohort and the drop is the only
-- way to observe that.

-- Pre-flight. Must be zero before the purge runs.
--
-- The purge scopes its posting_snapshots delete to the job_posting_id arm, so a
-- snapshot on a real posting that points at a fixture company's fetch run
-- survives that step and then aborts the fetch_runs step on
-- posting_snapshots.fetch_run_id (NO ACTION). The abort is correct — the row is
-- append-only collected history that no rerun can rebuild — but it is cheaper
-- to see the condition here than to watch a purge roll back. Non-zero means
-- stop and investigate the cross-link; it never means widen the purge.
SELECT
    'cross_linked_snapshots' AS preflight_check,
    count(*) AS rows_found
FROM posting_snapshots ps
JOIN job_postings jp ON jp.id = ps.job_posting_id
JOIN companies owner_c ON owner_c.id = jp.company_id
WHERE ps.fetch_run_id IN (
        SELECT fr.id FROM fetch_runs fr
        JOIN companies c ON c.id = fr.company_id
        WHERE c.board_token LIKE '%vitest%')
  AND owner_c.board_token NOT LIKE '%vitest%';

SELECT step, relation, total_rows, fixture_rows
FROM (
    VALUES
        ( 1, 'classifications',
          (SELECT count(*) FROM classifications),
          (SELECT count(*) FROM classifications cl
             WHERE cl.job_posting_id IN (
                 SELECT jp.id FROM job_postings jp
                 JOIN companies c ON c.id = jp.company_id
                 WHERE c.board_token LIKE '%vitest%')) ),
        ( 2, 'posting_snapshots',
          (SELECT count(*) FROM posting_snapshots),
          (SELECT count(*) FROM posting_snapshots ps
             WHERE ps.job_posting_id IN (
                 SELECT jp.id FROM job_postings jp
                 JOIN companies c ON c.id = jp.company_id
                 WHERE c.board_token LIKE '%vitest%')) ),
        ( 3, 'job_postings',
          (SELECT count(*) FROM job_postings),
          (SELECT count(*) FROM job_postings jp
             WHERE jp.company_id IN (
                 SELECT id FROM companies WHERE board_token LIKE '%vitest%')) ),
        ( 4, 'fetch_runs',
          (SELECT count(*) FROM fetch_runs),
          (SELECT count(*) FROM fetch_runs fr
             WHERE fr.company_id IN (
                 SELECT id FROM companies WHERE board_token LIKE '%vitest%')) ),
        ( 5, 'companies',
          (SELECT count(*) FROM companies),
          (SELECT count(*) FROM companies WHERE board_token LIKE '%vitest%') ),
        ( 6, 'canonical_role_dimensions',
          (SELECT count(*) FROM canonical_role_dimensions),
          (SELECT count(*) FROM canonical_role_dimensions crd
             WHERE crd.canonical_role_id IN (
                 SELECT id FROM canonical_roles WHERE slug LIKE '%vitest%')
                OR crd.dimension_id IN (
                 SELECT id FROM role_dimensions WHERE slug LIKE '%vitest%')) ),
        ( 7, 'canonical_roles',
          (SELECT count(*) FROM canonical_roles),
          (SELECT count(*) FROM canonical_roles WHERE slug LIKE '%vitest%') ),
        ( 8, 'specializations',
          (SELECT count(*) FROM specializations),
          (SELECT count(*) FROM specializations WHERE slug LIKE '%vitest%') ),
        ( 9, 'skills',
          (SELECT count(*) FROM skills),
          (SELECT count(*) FROM skills WHERE slug LIKE '%vitest%') ),
        (10, 'role_dimensions',
          (SELECT count(*) FROM role_dimensions),
          (SELECT count(*) FROM role_dimensions WHERE slug LIKE '%vitest%') ),
        -- Cascades from the classifications delete; never targeted directly.
        (11, 'job_posting_roles (cascade)',
          (SELECT count(*) FROM job_posting_roles),
          (SELECT count(*) FROM job_posting_roles jpr
             WHERE jpr.classification_id IN (
                 SELECT cl.id FROM classifications cl
                 JOIN job_postings jp ON jp.id = cl.job_posting_id
                 JOIN companies c ON c.id = jp.company_id
                 WHERE c.board_token LIKE '%vitest%')) ),
        (12, 'job_posting_specializations (cascade)',
          (SELECT count(*) FROM job_posting_specializations),
          (SELECT count(*) FROM job_posting_specializations jps
             WHERE jps.classification_id IN (
                 SELECT cl.id FROM classifications cl
                 JOIN job_postings jp ON jp.id = cl.job_posting_id
                 JOIN companies c ON c.id = jp.company_id
                 WHERE c.board_token LIKE '%vitest%')) ),
        (13, 'job_posting_skills (cascade)',
          (SELECT count(*) FROM job_posting_skills),
          (SELECT count(*) FROM job_posting_skills jpk
             WHERE jpk.classification_id IN (
                 SELECT cl.id FROM classifications cl
                 JOIN job_postings jp ON jp.id = cl.job_posting_id
                 JOIN companies c ON c.id = jp.company_id
                 WHERE c.board_token LIKE '%vitest%')) ),
        -- Not a purge target. A row is fixture-owned when its posting is: the
        -- view pairs a posting only with a fetch run of that posting's own
        -- company, so the purge drops exactly these rows and leaves the rest.
        (14, 'open_postings (view)',
          (SELECT count(*) FROM open_postings),
          (SELECT count(*) FROM open_postings op
             WHERE op.job_posting_id IN (
                 SELECT jp.id FROM job_postings jp
                 JOIN companies c ON c.id = jp.company_id
                 WHERE c.board_token LIKE '%vitest%')) )
) AS census(step, relation, total_rows, fixture_rows)
ORDER BY step;
