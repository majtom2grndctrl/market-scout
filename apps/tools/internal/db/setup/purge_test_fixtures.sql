-- One-time data repair: remove Vitest fixture rows that leaked into the
-- development database before the DB-backed suites were pointed at
-- market_scout_test. This is operational SQL, not a numbered migration — a
-- migration versions schema across environments, and a fresh database has
-- nothing to purge.
--
-- Fixture rows are identified structurally, never by timestamp: a company by
-- board_token LIKE '%vitest%', a taxonomy row by slug LIKE '%vitest%'. Every
-- delete is predicate-driven, so a second run matches zero rows and a retry
-- after a failed attempt is safe.
--
-- If any statement aborts on a foreign key, STOP. An abort means a fixture row
-- is referenced by real data; widening a predicate would delete real history.
--
-- Scope limit: Vitest fixtures only. The Go //go:build integration suites also
-- write companies with ats set to the development database, under their own
-- markers — board_token prefixes 'market-scout-fetch-runs-test-',
-- 'market-scout-loctexts-', 'mcp-action-add-' and 'mcp-save-enrich-'. Because
-- ats is set, queries/fetcher.sql's WHERE ats IS NOT NULL keeps fetching them,
-- and their t.Cleanup teardown is best-effort — the location_texts one only
-- logs a failed delete. None of those tokens match '%vitest%', so all of them
-- survive this purge. That is deliberate: testing-guide.md §1 scopes pointing
-- the Go suites at market_scout_test as a separate follow-up, and quietly
-- widening a destructive script's blast radius is worse than leaving rows.
-- A clean census here does not mean the database is fixture-free.
--
-- OPERATOR RUN SEQUENCE
--
-- Run every step as the owner role. Host psql and pg_dump are not installed;
-- all four steps go through the database container. Quiesce first: no fetcher
-- and no batch-enrich run may write between the before-census and the
-- after-census, or normal writes mask an over-delete. The three psql
-- invocations are separate processes — nothing depends on sharing a session,
-- only on running them back-to-back against a quiet database.
--
-- 0. Dump. COMMIT is unrecoverable without one.
--
--     docker exec market-scout-db pg_dump -U market_scout -d market_scout -Fc \
--       > ~/market_scout-prepurge-$(date -u +%Y%m%dT%H%M%SZ).dump
--
--    The dump streams over stdout and lands in the operator's home directory
--    on the host, not inside the container. Restore with pg_restore.
--
-- 1. Census, before. Its leading pre-flight check must report zero cross-linked
--    snapshots; a non-zero count means this script will abort at step 4, by
--    design. Investigate before going further — do not widen a predicate.
--
--     docker exec -i market-scout-db psql -U market_scout -d market_scout -f - \
--       < internal/db/setup/purge_test_fixtures_counts.sql
--
-- 2. Purge. ON_ERROR_STOP makes psql stop reading the file at the first error;
--    the BEGIN/COMMIT wrapper is what discards a partial purge, and it does so
--    whether or not that flag is set.
--
--     docker exec -i market-scout-db psql -v ON_ERROR_STOP=1 -U market_scout \
--       -d market_scout -f - < internal/db/setup/purge_test_fixtures.sql
--
-- 3. Census, after. Reconcile against step 1: every table in the delete order
--    below must drop by exactly the count this script reported.
--
--     docker exec -i market-scout-db psql -U market_scout -d market_scout -f - \
--       < internal/db/setup/purge_test_fixtures_counts.sql

BEGIN;

-- Each table is its own statement, not one chained data-modifying CTE. CTEs in a
-- single statement share one snapshot, so the link rows that step 1 cascades away
-- would still be visible to steps 7-9 and those deletes would abort on their
-- ON DELETE RESTRICT foreign keys.

-- 1. classifications. Carries no board_token; reachable only through
-- job_postings.company_id. Deleting it first is load-bearing: job_posting_roles,
-- job_posting_specializations and job_posting_skills cascade from here
-- (ON DELETE CASCADE on classification_id), and until they are gone their
-- ON DELETE RESTRICT foreign keys to canonical_roles, specializations and skills
-- block steps 7-9.
WITH d AS (
    DELETE FROM classifications
    WHERE job_posting_id IN (
        SELECT jp.id
        FROM job_postings jp
        JOIN companies c ON c.id = jp.company_id
        WHERE c.board_token LIKE '%vitest%'
    )
    RETURNING 1
)
SELECT 'classifications' AS relation, count(*) AS deleted FROM d;

-- 2. posting_snapshots. Also has no board_token; scoped on the posting side
-- only, deliberately. A snapshot whose fetch_run_id points at a fixture run but
-- whose job_posting_id belongs to a real company is real collected history, and
-- posting_snapshots is append-only — nothing can recreate the row. Matching it
-- here to keep step 4 moving would be exactly the widening the header forbids.
-- Left alone it blocks step 4 instead: posting_snapshots.fetch_run_id has no
-- ON DELETE clause (NO ACTION), so the fetch_runs delete aborts and the
-- transaction rolls back. That abort is the STOP signal, not a problem to route
-- around. purge_test_fixtures_counts.sql leads with a pre-flight check that
-- surfaces the same condition before this script runs.
WITH d AS (
    DELETE FROM posting_snapshots
    WHERE job_posting_id IN (
        SELECT jp.id
        FROM job_postings jp
        JOIN companies c ON c.id = jp.company_id
        WHERE c.board_token LIKE '%vitest%'
    )
    RETURNING 1
)
SELECT 'posting_snapshots' AS relation, count(*) AS deleted FROM d;

-- 3. job_postings. posting_snapshots.job_posting_id is ON DELETE RESTRICT, so
-- step 2 must precede this.
WITH d AS (
    DELETE FROM job_postings
    WHERE company_id IN (
        SELECT id FROM companies WHERE board_token LIKE '%vitest%'
    )
    RETURNING 1
)
SELECT 'job_postings' AS relation, count(*) AS deleted FROM d;

-- 4. fetch_runs. fetch_runs.company_id has no ON DELETE clause (NO ACTION), so
-- these must go before the companies delete.
WITH d AS (
    DELETE FROM fetch_runs
    WHERE company_id IN (
        SELECT id FROM companies WHERE board_token LIKE '%vitest%'
    )
    RETURNING 1
)
SELECT 'fetch_runs' AS relation, count(*) AS deleted FROM d;

-- 5. companies. job_postings.company_id is ON DELETE RESTRICT.
WITH d AS (
    DELETE FROM companies
    WHERE board_token LIKE '%vitest%'
    RETURNING 1
)
SELECT 'companies' AS relation, count(*) AS deleted FROM d;

-- 6. canonical_role_dimensions. Has neither board_token nor slug, so it is
-- matched from both sides. The canonical_role_id side would cascade from step 7
-- anyway; the dimension_id side would not — that FK is ON DELETE RESTRICT, and a
-- fixture dimension mapped to a real canonical role would otherwise block
-- step 10.
WITH d AS (
    DELETE FROM canonical_role_dimensions
    WHERE canonical_role_id IN (
        SELECT id FROM canonical_roles WHERE slug LIKE '%vitest%'
    )
       OR dimension_id IN (
        SELECT id FROM role_dimensions WHERE slug LIKE '%vitest%'
    )
    RETURNING 1
)
SELECT 'canonical_role_dimensions' AS relation, count(*) AS deleted FROM d;

-- 7. canonical_roles. Blocked by job_posting_roles.role_id (RESTRICT) until
-- step 1's cascade cleared those rows. An abort here means a real classification
-- references a fixture role.
WITH d AS (
    DELETE FROM canonical_roles
    WHERE slug LIKE '%vitest%'
    RETURNING 1
)
SELECT 'canonical_roles' AS relation, count(*) AS deleted FROM d;

-- 8. specializations. Blocked by job_posting_specializations.specialization_id
-- (RESTRICT) until step 1's cascade cleared those rows.
WITH d AS (
    DELETE FROM specializations
    WHERE slug LIKE '%vitest%'
    RETURNING 1
)
SELECT 'specializations' AS relation, count(*) AS deleted FROM d;

-- 9. skills. Blocked by job_posting_skills.skill_id (RESTRICT) until step 1's
-- cascade cleared those rows.
WITH d AS (
    DELETE FROM skills
    WHERE slug LIKE '%vitest%'
    RETURNING 1
)
SELECT 'skills' AS relation, count(*) AS deleted FROM d;

-- 10. role_dimensions. Blocked by canonical_role_dimensions.dimension_id
-- (RESTRICT) until step 6 cleared both sides of that link.
WITH d AS (
    DELETE FROM role_dimensions
    WHERE slug LIKE '%vitest%'
    RETURNING 1
)
SELECT 'role_dimensions' AS relation, count(*) AS deleted FROM d;

COMMIT;
