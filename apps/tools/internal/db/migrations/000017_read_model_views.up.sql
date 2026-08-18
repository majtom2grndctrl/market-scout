-- Read models define "open" once for the web app and the read-only MCP query
-- gateway. Only successful runs define openness: failed and in-progress runs
-- may be incomplete, so treating their absence as closure would be false.
-- market_scout_readonly can read these views through the live cluster's default
-- privileges and, on a fresh cluster, setup/readonly_role.sql's GRANT SELECT ON
-- ALL TABLES IN SCHEMA public. Roles are provisioned outside migrations.
--
-- Local performance check of open_postings_display as defined below (2026-08-08,
-- 61,642 snapshots): after warm-up, EXPLAIN (ANALYZE, TIMING OFF) took 33-47ms
-- across five runs. That number describes this migration's view only — 000019
-- replaces open_postings_display with a much heavier definition and carries its
-- own measurement. The existing fetch_run_id index already gave snapshot index
-- scans. A transaction-scoped (fetch_run_id, job_posting_id) trial gave an
-- index-only scan but took 38ms, within that range, so this migration
-- intentionally does not add it.

CREATE VIEW latest_successful_fetch_runs AS
SELECT DISTINCT ON (fetch_runs.company_id)
    fetch_runs.id AS fetch_run_id,
    fetch_runs.company_id,
    fetch_runs.started_at
FROM fetch_runs
WHERE fetch_runs.status = 'success'
ORDER BY fetch_runs.company_id, fetch_runs.started_at DESC, fetch_runs.id DESC;

CREATE VIEW open_postings AS
SELECT DISTINCT
    posting_snapshots.job_posting_id,
    latest_successful_fetch_runs.fetch_run_id
FROM latest_successful_fetch_runs
JOIN posting_snapshots
    ON posting_snapshots.fetch_run_id = latest_successful_fetch_runs.fetch_run_id
JOIN job_postings
    ON job_postings.id = posting_snapshots.job_posting_id
    AND job_postings.company_id = latest_successful_fetch_runs.company_id;

CREATE VIEW open_postings_display AS
SELECT
    open_postings.job_posting_id,
    latest_successful_fetch_runs.company_id,
    companies.name AS company_name,
    latest_successful_fetch_runs.started_at AS run_started_at,
    current_snapshot.title,
    current_snapshot.location_text,
    current_snapshot.workplace_type,
    current_snapshot.compensation_min,
    current_snapshot.compensation_max,
    current_snapshot.compensation_currency,
    current_classification.id AS classification_id,
    current_classification.seniority
FROM open_postings
JOIN latest_successful_fetch_runs
    ON latest_successful_fetch_runs.fetch_run_id = open_postings.fetch_run_id
JOIN companies ON companies.id = latest_successful_fetch_runs.company_id
-- Per-posting laterals preserve index scans; DISTINCT ON instead sorts and
-- materializes all snapshots.
LEFT JOIN LATERAL (
    SELECT
        posting_snapshots.title,
        posting_snapshots.location_text,
        posting_snapshots.workplace_type,
        posting_snapshots.compensation_min,
        posting_snapshots.compensation_max,
        posting_snapshots.compensation_currency
    FROM posting_snapshots
    WHERE posting_snapshots.job_posting_id = open_postings.job_posting_id
    ORDER BY posting_snapshots.fetched_at DESC
    LIMIT 1
) AS current_snapshot ON TRUE
LEFT JOIN LATERAL (
    SELECT classifications.id, classifications.seniority
    FROM classifications
    WHERE classifications.job_posting_id = open_postings.job_posting_id
    ORDER BY classifications.classified_at DESC
    LIMIT 1
) AS current_classification ON TRUE;

CREATE VIEW open_posting_taxonomy AS
SELECT
    open_postings_display.job_posting_id,
    'role'::text AS term_kind,
    canonical_roles.slug,
    canonical_roles.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_roles ON canonical_roles.id = job_posting_roles.role_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'specialization'::text AS term_kind,
    specializations.slug,
    specializations.name
FROM open_postings_display
JOIN job_posting_specializations
    ON job_posting_specializations.classification_id = open_postings_display.classification_id
JOIN specializations ON specializations.id = job_posting_specializations.specialization_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'skill'::text AS term_kind,
    skills.slug,
    skills.name
FROM open_postings_display
JOIN job_posting_skills
    ON job_posting_skills.classification_id = open_postings_display.classification_id
JOIN skills ON skills.id = job_posting_skills.skill_id

UNION ALL

SELECT DISTINCT
    open_postings_display.job_posting_id,
    'dimension'::text AS term_kind,
    role_dimensions.slug,
    role_dimensions.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_role_dimensions
    ON canonical_role_dimensions.canonical_role_id = job_posting_roles.role_id
JOIN role_dimensions ON role_dimensions.id = canonical_role_dimensions.dimension_id;
