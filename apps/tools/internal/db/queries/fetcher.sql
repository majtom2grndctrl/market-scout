-- name: ListCompaniesWithATS :many
SELECT *
FROM companies
WHERE ats IS NOT NULL
ORDER BY name;

-- name: UpsertJobPosting :one
-- DO UPDATE (not DO NOTHING) so RETURNING id fires on conflict — the caller
-- needs the row id to write the snapshot. SET source_id is usually a no-op by
-- value but self-heals if an ATS reassigns the id for an existing source_url.
-- Append-only semantics apply to posting_snapshots, not this table.
INSERT INTO job_postings (company_id, source_type, source_url, source_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (company_id, source_url) DO UPDATE SET source_id = EXCLUDED.source_id
RETURNING id;

-- name: InsertPostingSnapshot :exec
INSERT INTO posting_snapshots (
    job_posting_id,
    fetched_at,
    title,
    location_text,
    department,
    team,
    employment_type,
    workplace_type,
    posted_at,
    job_url,
    raw_data,
    source_first_published_at,
    source_last_modified_at,
    location_texts,
    fetch_run_id,
    description_text,
    compensation_min,
    compensation_max,
    compensation_currency,
    compensation_period
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20);

-- name: ListLatestDescriptionsByCompany :many
-- Latest snapshot's description_text per job_posting for a company.
-- Skips postings whose latest snapshot has NULL description_text.
-- ::text cast is intentional: forces sqlc to emit a plain string instead of
-- sql.NullString. The outer WHERE guarantees non-null at the DB level; the
-- cast surfaces that guarantee in the generated Go type.
SELECT job_posting_id, description_text
FROM (
    SELECT DISTINCT ON (ps.job_posting_id)
        ps.job_posting_id,
        ps.description_text::text AS description_text
    FROM posting_snapshots ps
    JOIN job_postings jp ON jp.id = ps.job_posting_id
    WHERE jp.company_id = $1
    ORDER BY ps.job_posting_id, ps.fetched_at DESC
) latest
WHERE description_text IS NOT NULL;
