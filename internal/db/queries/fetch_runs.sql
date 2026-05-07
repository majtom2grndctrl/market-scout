-- $1: company_id, $2: started_at
-- name: InsertFetchRun :one
INSERT INTO fetch_runs (company_id, started_at, status)
VALUES ($1, $2, 'in_progress')
RETURNING id;

-- $1: id, $2: completed_at, $3: postings_count
-- name: MarkFetchRunSuccess :exec
UPDATE fetch_runs
SET status = 'success', completed_at = $2, postings_count = $3
WHERE id = $1;

-- $1: id, $2: completed_at, $3: error_message
-- name: MarkFetchRunFailed :exec
UPDATE fetch_runs
SET status = 'failed', completed_at = $2, error_message = $3
WHERE id = $1;
