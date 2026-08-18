-- Roll back the derived workplace-type columns by rebuilding the display view
-- from migration 000018, then recreating the taxonomy view that depends on it.
-- Dropping is necessary because CREATE OR REPLACE cannot remove columns from a
-- view. Recreating the views does not grant SELECT: default privileges apply
-- only when market_scout_readonly exists and the migration owner configured
-- them; otherwise operators must grant and verify read-only access separately.

DROP VIEW open_posting_taxonomy;
DROP VIEW open_postings_display;

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
    ORDER BY posting_snapshots.fetched_at DESC, posting_snapshots.id DESC
    LIMIT 1
) AS current_snapshot ON TRUE
LEFT JOIN LATERAL (
    SELECT classifications.id, classifications.seniority
    FROM classifications
    WHERE classifications.job_posting_id = open_postings.job_posting_id
    ORDER BY classifications.classified_at DESC, classifications.id DESC
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

-- Before the up migration the column named no algorithm and followed
-- default_toast_compression. `default` restores that deferral; naming pglz
-- would assert a setting the column never carried.
ALTER TABLE posting_snapshots
    ALTER COLUMN raw_data SET COMPRESSION default;
