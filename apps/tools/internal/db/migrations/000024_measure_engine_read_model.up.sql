-- Extend the read model beyond the current open cohort. Only successful runs
-- establish cohort membership; failed and in-progress runs carry no closure
-- signal. `market_scout_readonly` receives SELECT through the configured
-- default privileges (see setup/readonly_role.sql); that operational script
-- grants this read function explicitly to market_scout_readonly. It is not
-- SECURITY DEFINER and is unavailable to PUBLIC.
--
-- Local performance check of the posting_lifespans aggregation (2026-08-24,
-- 70,670 snapshots): after one warm-up, five EXPLAIN (ANALYZE, TIMING OFF)
-- runs took 29.275, 29.513, 29.763, 30.328, and 30.473ms; median 29.763ms.
-- The query reads only ids and timestamps, and the existing snapshot indexes
-- were sufficient, so this migration adds no index.

CREATE VIEW all_seen_postings AS
SELECT DISTINCT
    posting_snapshots.job_posting_id
FROM posting_snapshots
JOIN fetch_runs
    ON fetch_runs.id = posting_snapshots.fetch_run_id
    AND fetch_runs.status = 'success'
JOIN job_postings
    ON job_postings.id = posting_snapshots.job_posting_id
    AND job_postings.company_id = fetch_runs.company_id;

CREATE VIEW closed_postings AS
SELECT all_seen_postings.job_posting_id
FROM all_seen_postings
WHERE NOT EXISTS (
    SELECT 1
    FROM open_postings
    WHERE open_postings.job_posting_id = all_seen_postings.job_posting_id
);

CREATE VIEW posting_lifespans AS
SELECT
    posting_snapshots.job_posting_id,
    min(fetch_runs.started_at) AS first_seen,
    max(fetch_runs.started_at) AS last_seen,
    EXISTS (
        SELECT 1
        FROM closed_postings
        WHERE closed_postings.job_posting_id = posting_snapshots.job_posting_id
    ) AS is_closed
FROM posting_snapshots
JOIN fetch_runs
    ON fetch_runs.id = posting_snapshots.fetch_run_id
    AND fetch_runs.status = 'success'
JOIN job_postings
    ON job_postings.id = posting_snapshots.job_posting_id
    AND job_postings.company_id = fetch_runs.company_id
GROUP BY posting_snapshots.job_posting_id;

CREATE VIEW latest_classifications AS
SELECT
    all_seen_postings.job_posting_id,
    current_classification.id AS classification_id,
    current_classification.seniority,
    current_classification.classified_at
FROM all_seen_postings
JOIN LATERAL (
    SELECT
        classifications.id,
        classifications.seniority,
        classifications.classified_at
    FROM classifications
    WHERE classifications.job_posting_id = all_seen_postings.job_posting_id
    ORDER BY classifications.classified_at DESC, classifications.id DESC
    LIMIT 1
) AS current_classification ON TRUE;

CREATE VIEW posting_markets AS
-- Materialize the latest successful snapshot set and its market matches once.
-- `posting_taxonomy` consumers filter this branch by term_kind, but cannot
-- supply a posting-id predicate through the matched/unmapped UNION. Without
-- these boundaries, the planner expands the regex derivation twice: once for
-- matched rows and again for the unmapped anti-join.
WITH latest_successful_snapshots AS MATERIALIZED (
    SELECT DISTINCT ON (posting_snapshots.job_posting_id)
        posting_snapshots.job_posting_id,
        posting_snapshots.location_texts
    FROM posting_snapshots
    JOIN fetch_runs
        ON fetch_runs.id = posting_snapshots.fetch_run_id
        AND fetch_runs.status = 'success'
    JOIN job_postings
        ON job_postings.id = posting_snapshots.job_posting_id
        AND job_postings.company_id = fetch_runs.company_id
    ORDER BY
        posting_snapshots.job_posting_id,
        posting_snapshots.fetched_at DESC,
        posting_snapshots.id DESC
),
market_patterns AS MATERIALIZED (
    SELECT
        market_seeds.slug,
        market_seeds.name,
        market_seeds.kind,
        string_agg(pattern.value, '\y|\y') AS expression
    FROM market_seeds
    CROSS JOIN LATERAL unnest(market_seeds.patterns) AS pattern(value)
    WHERE market_seeds.slug <> 'unmapped'
    GROUP BY market_seeds.slug, market_seeds.name, market_seeds.kind
),
matched_markets AS MATERIALIZED (
    SELECT DISTINCT
        latest_successful_snapshots.job_posting_id,
        market_patterns.slug,
        market_patterns.name,
        market_patterns.kind
    FROM latest_successful_snapshots
    CROSS JOIN LATERAL unnest(latest_successful_snapshots.location_texts) AS location(location_text)
    JOIN market_patterns
        ON location.location_text ~* ('\y(' || market_patterns.expression || ')\y')
)
SELECT job_posting_id, slug, name, kind
FROM matched_markets

UNION ALL

SELECT
    latest_successful_snapshots.job_posting_id,
    unmapped.slug,
    unmapped.name,
    unmapped.kind
FROM latest_successful_snapshots
JOIN market_seeds AS unmapped ON unmapped.slug = 'unmapped'
WHERE NOT EXISTS (
    SELECT 1
    FROM matched_markets
    WHERE matched_markets.job_posting_id = latest_successful_snapshots.job_posting_id
);

CREATE VIEW posting_taxonomy AS
SELECT
    latest_classifications.job_posting_id,
    'role'::text AS term_kind,
    canonical_roles.slug,
    canonical_roles.name
FROM latest_classifications
JOIN job_posting_roles
    ON job_posting_roles.classification_id = latest_classifications.classification_id
JOIN canonical_roles ON canonical_roles.id = job_posting_roles.role_id

UNION ALL

SELECT
    latest_classifications.job_posting_id,
    'specialization'::text AS term_kind,
    specializations.slug,
    specializations.name
FROM latest_classifications
JOIN job_posting_specializations
    ON job_posting_specializations.classification_id = latest_classifications.classification_id
JOIN specializations ON specializations.id = job_posting_specializations.specialization_id

UNION ALL

SELECT
    latest_classifications.job_posting_id,
    'skill'::text AS term_kind,
    skills.slug,
    skills.name
FROM latest_classifications
JOIN job_posting_skills
    ON job_posting_skills.classification_id = latest_classifications.classification_id
JOIN skills ON skills.id = job_posting_skills.skill_id

UNION ALL

SELECT DISTINCT
    latest_classifications.job_posting_id,
    'dimension'::text AS term_kind,
    role_dimensions.slug,
    role_dimensions.name
FROM latest_classifications
JOIN job_posting_roles
    ON job_posting_roles.classification_id = latest_classifications.classification_id
JOIN canonical_role_dimensions
    ON canonical_role_dimensions.canonical_role_id = job_posting_roles.role_id
JOIN role_dimensions ON role_dimensions.id = canonical_role_dimensions.dimension_id

UNION ALL

SELECT
    posting_markets.job_posting_id,
    'market'::text AS term_kind,
    posting_markets.slug,
    posting_markets.name
FROM posting_markets;

-- Availability is company-grain so rate can distinguish one company's gap
-- from another company's successful zero-posting week.
CREATE VIEW fetch_success_weeks AS
SELECT DISTINCT
    fetch_runs.company_id,
    date_trunc('week', fetch_runs.started_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS week
FROM fetch_runs
WHERE fetch_runs.status = 'success';

CREATE FUNCTION public.open_postings_as_of(p_as_of timestamptz)
RETURNS TABLE (job_posting_id bigint)
LANGUAGE sql
STABLE
AS $$
    WITH latest_successful_fetch_runs AS (
        SELECT DISTINCT ON (fetch_runs.company_id)
            fetch_runs.company_id,
            fetch_runs.id AS fetch_run_id
        FROM fetch_runs
        WHERE fetch_runs.status = 'success'
            AND fetch_runs.started_at <= p_as_of
        ORDER BY fetch_runs.company_id, fetch_runs.started_at DESC, fetch_runs.id DESC
    )
    SELECT DISTINCT posting_snapshots.job_posting_id
    FROM latest_successful_fetch_runs
    JOIN posting_snapshots
        ON posting_snapshots.fetch_run_id = latest_successful_fetch_runs.fetch_run_id
    JOIN job_postings
        ON job_postings.id = posting_snapshots.job_posting_id
        AND job_postings.company_id = latest_successful_fetch_runs.company_id;
$$;

REVOKE ALL ON FUNCTION public.open_postings_as_of(timestamptz) FROM PUBLIC;
