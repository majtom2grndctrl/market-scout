-- Restore the original timestamp-only ordering if migration 000018 is rolled back.

CREATE OR REPLACE VIEW open_postings_display AS
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
