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
