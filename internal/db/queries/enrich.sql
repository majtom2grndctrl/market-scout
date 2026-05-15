-- Queries that drive the batch-enrich Go command. Selection uses a LATERAL join
-- to the latest posting_snapshots row per job_posting, an optional ILIKE focus
-- prefilter, and oldest-first ordering to process the backlog in stable priority.
-- The Forced variant drops the NOT EXISTS classifications guard so already-
-- classified postings are eligible for re-enrichment under --force.
--
-- @row_limit is named (not `limit`) because `limit` is a reserved word in
-- sqlc's named-parameter syntax. Column aliases (`posting_id`, `description_text`)
-- pin the generated struct field names (PostingID, DescriptionText).

-- name: ListUnclassifiedPostings :many
SELECT jp.id AS posting_id,
       jp.company_id,
       s.title,
       s.description_text
FROM job_postings jp
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
ORDER BY jp.first_seen_at ASC
LIMIT @row_limit::int;

-- name: ListUnclassifiedPostingsForced :many
SELECT jp.id AS posting_id,
       jp.company_id,
       s.title,
       s.description_text
FROM job_postings jp
JOIN LATERAL (
    SELECT title, description_text
    FROM posting_snapshots
    WHERE job_posting_id = jp.id
    ORDER BY fetched_at DESC
    LIMIT 1
) s ON true
WHERE s.description_text IS NOT NULL
  AND (@focus::text = '' OR (s.title ILIKE '%' || @focus::text || '%' OR s.description_text ILIKE '%' || @focus::text || '%'))
ORDER BY jp.first_seen_at ASC
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

-- name: ListCanonicalRoles :many
SELECT id, slug, name FROM canonical_roles ORDER BY slug;

-- name: ListSpecializations :many
SELECT id, slug, name FROM specializations ORDER BY slug;

-- name: ListSkills :many
SELECT id, slug, name FROM skills ORDER BY slug;

-- name: ListRoleDimensions :many
SELECT id, slug, name FROM role_dimensions ORDER BY slug;
