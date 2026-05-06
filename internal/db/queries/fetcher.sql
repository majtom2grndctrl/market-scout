-- name: ListCompaniesWithATS :many
SELECT *
FROM companies
WHERE ats IS NOT NULL
ORDER BY name;

-- name: UpsertJobPosting :one
-- DO UPDATE is used rather than DO NOTHING so RETURNING id fires on conflict
-- (DO NOTHING returns no rows on conflict, and the caller needs the existing
-- row's id to write the snapshot).
--
-- In practice source_id is stable for a given source_url, so the SET is
-- effectively a no-op by value — but it still writes a new heap tuple each
-- run, which autovacuum reclaims. Accepted trade-off: avoids a separate
-- SELECT path and also silently corrects source_id if an ATS ever reassigns
-- an ID to the same URL.
--
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
    raw_data
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);
