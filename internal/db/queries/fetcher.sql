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
    location_texts
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);
