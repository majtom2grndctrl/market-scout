-- Queries that drive the batch-enrich Go command and the MCP enrichment_preview
-- tool. Selection uses a LATERAL join to the latest posting_snapshots row per
-- job_posting, an optional ILIKE focus prefilter, and a caller-chosen ordering
-- to process the backlog in stable priority. The Forced variant drops the NOT
-- EXISTS classifications guard so already-classified postings are eligible for
-- re-enrichment under --force.
--
-- The join to companies surfaces company_name, which enrichment_preview reports
-- in its sample. companies.name is NOT NULL, so the column is non-nullable.
--
-- @row_limit is named (not `limit`) because `limit` is a reserved word in
-- sqlc's named-parameter syntax. Column aliases (`posting_id`, `description_text`,
-- `company_name`) pin the generated struct field names.
--
-- @newest_first drives ORDER BY direction without string-interpolating a
-- column/direction into SQL (sqlc can't parameterize ORDER BY directly). Only
-- one of the two CASE expressions ever produces a non-NULL value per row —
-- the other is NULL for every row and so is a no-op tie-breaker — so this
-- reduces to a plain ASC or DESC sort on first_seen_at (NOT NULL, so no NULLS
-- ordering concern) depending on the bound boolean. Default false preserves
-- the pre-existing oldest-first behavior for callers (like cmd/batch-enrich)
-- that don't set it.

-- name: ListUnclassifiedPostings :many
SELECT jp.id AS posting_id,
       jp.company_id,
       c.name AS company_name,
       s.title,
       s.description_text
FROM job_postings jp
JOIN companies c ON c.id = jp.company_id
JOIN LATERAL (
    SELECT title, description_text
    FROM posting_snapshots
    WHERE job_posting_id = jp.id
    ORDER BY fetched_at DESC
    LIMIT 1
) s ON true
WHERE s.description_text IS NOT NULL
  AND (@focus::text = '' OR (s.title ILIKE '%' || @focus::text || '%' OR s.description_text ILIKE '%' || @focus::text || '%'))
  AND NOT EXISTS (
      SELECT 1 FROM classifications WHERE job_posting_id = jp.id
  )
ORDER BY CASE WHEN @newest_first::bool THEN jp.first_seen_at END DESC,
         CASE WHEN NOT @newest_first::bool THEN jp.first_seen_at END ASC
LIMIT @row_limit::int;

-- name: ListUnclassifiedPostingsForced :many
SELECT jp.id AS posting_id,
       jp.company_id,
       c.name AS company_name,
       s.title,
       s.description_text
FROM job_postings jp
JOIN companies c ON c.id = jp.company_id
JOIN LATERAL (
    SELECT title, description_text
    FROM posting_snapshots
    WHERE job_posting_id = jp.id
    ORDER BY fetched_at DESC
    LIMIT 1
) s ON true
WHERE s.description_text IS NOT NULL
  AND (@focus::text = '' OR (s.title ILIKE '%' || @focus::text || '%' OR s.description_text ILIKE '%' || @focus::text || '%'))
ORDER BY CASE WHEN @newest_first::bool THEN jp.first_seen_at END DESC,
         CASE WHEN NOT @newest_first::bool THEN jp.first_seen_at END ASC
LIMIT @row_limit::int;

-- name: ListClassifiedAmong :many
-- Returns the subset of @ids that already have at least one classifications row.
-- Used under --force to flag postings that will produce duplicate-write surprises
-- in re-enrichment. The ::bigint[] cast is required for sqlc to emit a pq.Array
-- parameter so a Go []int64 marshals correctly through database/sql.
SELECT job_posting_id AS posting_id
FROM classifications
WHERE job_posting_id = ANY(@ids::bigint[])
GROUP BY job_posting_id;

-- PostingExists reports whether a job_postings row exists for the given id. The
-- MCP save_enrichment action calls it through the read-only pool to reject a
-- nonexistent posting before invoking the action function.
-- name: PostingExists :one
SELECT EXISTS (SELECT 1 FROM job_postings WHERE id = $1) AS exists;

-- name: ListCanonicalRoles :many
SELECT id, slug, name FROM canonical_roles ORDER BY slug;

-- name: ListSpecializations :many
SELECT id, slug, name FROM specializations ORDER BY slug;

-- name: ListSkills :many
SELECT id, slug, name FROM skills ORDER BY slug;

-- name: ListRoleDimensions :many
SELECT id, slug, name FROM role_dimensions ORDER BY slug;
