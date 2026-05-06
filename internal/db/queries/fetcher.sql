-- name: ListCompaniesWithATS :many
SELECT *
FROM companies
WHERE ats IS NOT NULL
ORDER BY name;

-- name: UpsertJobPosting :one
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
    raw_data
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
