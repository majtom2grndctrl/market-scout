-- name: InsertFetchRun :one
INSERT INTO fetch_runs (company_id, started_at, status)
VALUES ($1, $2, 'in_progress')
RETURNING id;

-- name: MarkFetchRunSuccess :exec
UPDATE fetch_runs
SET status = 'success', completed_at = $2, postings_count = $3
WHERE id = $1;

-- name: MarkFetchRunFailed :exec
UPDATE fetch_runs
SET status = 'failed', completed_at = $2, error_message = $3
WHERE id = $1;

-- name: ListLatestFetchRunsByCompany :many
SELECT DISTINCT ON (fr.company_id)
    c.name,
    fr.status,
    fr.started_at,
    fr.completed_at,
    fr.postings_count,
    fr.error_message
FROM fetch_runs fr
JOIN companies c ON c.id = fr.company_id
ORDER BY fr.company_id, fr.started_at DESC;
