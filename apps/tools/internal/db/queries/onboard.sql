-- name: FindCompanyDedupStatus :one
-- Given (ats, board_token), returns whether a matching company exists and
-- whether it has any posting_snapshots within the recency window.
--
-- Recency window: snapshots from now() - (recency_days days) and later count
-- as "recent". The caller passes the watchlist's 30-day threshold as an
-- integer; pushing the interval construction into SQL avoids a clock-skew
-- gap between app server and database, and avoids the awkward
-- interval-as-int64 type sqlc emits for raw interval parameters.
--
-- Returns zero rows if no company matches. Returns exactly one row if a
-- company exists, with has_recent_snapshot=false when the company has no
-- postings or no snapshots in the window.
SELECT
    c.id AS company_id,
    EXISTS (
        SELECT 1
        FROM job_postings jp
        JOIN posting_snapshots ps ON ps.job_posting_id = jp.id
        WHERE jp.company_id = c.id
          AND ps.fetched_at >= now() - make_interval(days => sqlc.arg(recency_days)::int)
    ) AS has_recent_snapshot
FROM companies c
WHERE c.ats = sqlc.arg(ats)
  AND c.board_token = sqlc.arg(board_token);
