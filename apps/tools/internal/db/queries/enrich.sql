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
-- @dedup collapses postings that describe the same job into one work unit. A
-- work unit is a connected component over two edge relations, both scoped within
-- a single company:
--
--   * the text edge: identical md5(description_text). A board that lists one job
--     once per location emits byte-identical text, and identical text always
--     classifies identically.
--   * the requisition edge: identical requisition_key AND identical normalized
--     title. One requisition whose description was edited between fetches keeps
--     its key and its title while its text drifts.
--
-- The requisition edge is gated on the title because at Greenhouse and Ashby the
-- requisition key identifies a job *family*, not a posting. Over the corpus, 152
-- of the 271 multi-posting requisition-key groups (463 postings) span more than
-- one normalized title; ungated, that merged four distinct Stripe Staff SWE roles
-- into one classification. Normalization is load-bearing, not cosmetic: raw-title
-- comparison finds 112 uniform groups where normalized finds 119, and the delta
-- includes a Stripe pair that differs only by a trailing space.
--
-- Components, not a scalar key, because the two edges chain: A and B share text,
-- B and C share a gated requisition key, so A, B and C are one job. The recursive
-- CTE walks that closure; the representative is the lowest posting id in the
-- component.
--
-- description_text IS NOT NULL is filtered in `candidate`, before any edge is
-- built. Hashing coalesce(description_text, '') instead would give every
-- description-less posting the same hash and collapse a company's entire
-- description-less backlog into one unit.
--
-- The edges are themselves gated on @dedup so that a non-dedup call pays nothing
-- for the self-joins and every posting lands in a component of one.
--
-- When @dedup is true, @row_limit bounds work units rather than postings, and
-- every posting in a selected unit is returned even when that carries the result
-- past the limit. A unit split across the limit would be classified in two halves
-- on two different runs, independently, which is the inconsistency dedup exists
-- to prevent. Callers must therefore expect len(rows) >= @row_limit and read
-- is_representative to find the one posting whose text needs classifying.
--
-- When @dedup is false the query keeps its original row-per-posting semantics:
-- @row_limit bounds postings and is_representative is true for every row. That is
-- the path cmd/batch-enrich still takes; it classifies each posting independently
-- and has no notion of writing one result to a unit's siblings.
--
-- The slot column carries the window position over work units, so a unit's
-- members share a slot and survive the limit together. With @dedup false every
-- posting is its own unit, which makes the same dense_rank a per-posting
-- ordinal — the row-per-posting semantics are preserved, not special-cased.
--
-- @newest_first drives ORDER BY direction without string-interpolating a
-- column/direction into SQL (sqlc can't parameterize ORDER BY directly). Only
-- the CASE expressions for the chosen direction produce non-NULL values;
-- the other direction is a no-op. Posting id is the final direction-matched
-- tie breaker, so a cohort cutoff remains deterministic when first_seen_at
-- values tie. Callers resolve the direction; internal/enrich/selection defaults
-- it to newest-first.
--
-- @max_per_company caps how many work units one company contributes, and the
-- wave ordering that goes with it is deliberate under-sampling of the largest
-- boards, not proportional sampling. `capped` ranks each company's units
-- independently (company_slot), then `waved` orders by company_slot *before*
-- recency, so a wave takes every company's newest unit, then every company's
-- second-newest, and so on. Ordering alone does the spreading; the cap is the
-- hard ceiling that stops one board from refilling the tail of a large wave.
--
-- Without it the oldest-first backlog was structurally single-company: a
-- 50-unit wave on 2026-09-21 drew 58 postings, all OpenAI, all first seen
-- 2026-07-20. The corpus is ~43% OpenAI/Stripe/Anthropic, and the question the
-- project answers — which job titles are emerging — needs the *classified* set
-- broader than the corpus, or title analysis reports three companies' naming
-- conventions.
--
-- The cap counts work units, matching @row_limit; a capped company can still
-- return more than @max_per_company postings when its selected units have
-- siblings. A value <= 0 disables the cap.
--
-- Note: postings whose latest snapshot has no description_text never enter
-- `candidate` at all. Workday (~120 postings) and Workable (~30) currently
-- store none, so they are invisible to selection. That is a fetcher gap, not a
-- selection one.

-- name: ListUnclassifiedPostings :many
WITH RECURSIVE candidate AS (
    SELECT jp.id AS posting_id,
           jp.company_id,
           jp.first_seen_at,
           c.name AS company_name,
           s.title,
           s.description_text,
           md5(s.description_text) AS text_hash,
           lower(regexp_replace(btrim(s.title), '\s+', ' ', 'g')) AS norm_title,
           nullif(s.requisition_key, '') AS req_key
    FROM job_postings jp
    JOIN companies c ON c.id = jp.company_id
    JOIN LATERAL (
        SELECT title, description_text, requisition_key
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
),
edge AS (
    SELECT a.posting_id AS src, b.posting_id AS dst
    FROM candidate a
    JOIN candidate b ON b.company_id = a.company_id
                    AND b.text_hash = a.text_hash
                    AND b.posting_id <> a.posting_id
    WHERE @dedup::bool
    UNION
    SELECT a.posting_id AS src, b.posting_id AS dst
    FROM candidate a
    JOIN candidate b ON b.company_id = a.company_id
                    AND b.req_key = a.req_key
                    AND b.norm_title = a.norm_title
                    AND b.posting_id <> a.posting_id
    WHERE @dedup::bool AND a.req_key IS NOT NULL
),
reach AS (
    SELECT posting_id AS anchor, posting_id AS member FROM candidate
    UNION
    SELECT r.anchor, e.dst FROM reach r JOIN edge e ON e.src = r.member
),
unit AS (
    SELECT anchor AS posting_id, min(member) AS unit_id
    FROM reach
    GROUP BY anchor
),
keyed AS (
    SELECT c.posting_id,
           c.company_id,
           c.first_seen_at,
           c.company_name,
           c.title,
           c.description_text,
           (c.company_id::text || '|unit:' || u.unit_id::text)::text AS dedup_key
    FROM candidate c
    JOIN unit u ON u.posting_id = c.posting_id
),
grouped AS (
    SELECT dedup_key,
           company_id,
           min(first_seen_at) AS unit_oldest,
           max(first_seen_at) AS unit_newest,
           min(posting_id)    AS unit_min_id,
           max(posting_id)    AS unit_max_id
    FROM keyed
    GROUP BY dedup_key, company_id
),
capped AS (
    SELECT g.dedup_key,
           g.unit_oldest,
           g.unit_newest,
           g.unit_min_id,
           g.unit_max_id,
           row_number() OVER (
               PARTITION BY g.company_id
               ORDER BY CASE WHEN @newest_first::bool THEN g.unit_newest END DESC,
                        CASE WHEN @newest_first::bool THEN g.unit_max_id END DESC,
                        CASE WHEN NOT @newest_first::bool THEN g.unit_oldest END ASC,
                        CASE WHEN NOT @newest_first::bool THEN g.unit_min_id END ASC) AS company_slot
    FROM grouped g
),
waved AS (
    SELECT c.dedup_key,
           dense_rank() OVER (
               ORDER BY c.company_slot ASC,
                        CASE WHEN @newest_first::bool THEN c.unit_newest END DESC,
                        CASE WHEN @newest_first::bool THEN c.unit_max_id END DESC,
                        CASE WHEN NOT @newest_first::bool THEN c.unit_oldest END ASC,
                        CASE WHEN NOT @newest_first::bool THEN c.unit_min_id END ASC) AS slot
    FROM capped c
    WHERE @max_per_company::int <= 0 OR c.company_slot <= @max_per_company::int
),
ranked AS (
    SELECT k.posting_id,
           k.company_id,
           k.company_name,
           k.title,
           k.description_text,
           k.dedup_key,
           CASE WHEN @dedup::bool
                THEN (row_number() OVER (PARTITION BY k.dedup_key ORDER BY k.posting_id) = 1)
                ELSE true
           END AS is_representative,
           w.slot
    FROM keyed k
    JOIN waved w ON w.dedup_key = k.dedup_key
)
SELECT posting_id,
       company_id,
       company_name,
       title,
       description_text,
       dedup_key,
       is_representative
FROM ranked
WHERE slot <= @row_limit::int
ORDER BY slot, posting_id;

-- name: ListUnclassifiedPostingsForced :many
WITH RECURSIVE candidate AS (
    SELECT jp.id AS posting_id,
           jp.company_id,
           jp.first_seen_at,
           c.name AS company_name,
           s.title,
           s.description_text,
           md5(s.description_text) AS text_hash,
           lower(regexp_replace(btrim(s.title), '\s+', ' ', 'g')) AS norm_title,
           nullif(s.requisition_key, '') AS req_key
    FROM job_postings jp
    JOIN companies c ON c.id = jp.company_id
    JOIN LATERAL (
        SELECT title, description_text, requisition_key
        FROM posting_snapshots
        WHERE job_posting_id = jp.id
        ORDER BY fetched_at DESC
        LIMIT 1
    ) s ON true
    WHERE s.description_text IS NOT NULL
      AND (@focus::text = '' OR (s.title ILIKE '%' || @focus::text || '%' OR s.description_text ILIKE '%' || @focus::text || '%'))
),
edge AS (
    SELECT a.posting_id AS src, b.posting_id AS dst
    FROM candidate a
    JOIN candidate b ON b.company_id = a.company_id
                    AND b.text_hash = a.text_hash
                    AND b.posting_id <> a.posting_id
    WHERE @dedup::bool
    UNION
    SELECT a.posting_id AS src, b.posting_id AS dst
    FROM candidate a
    JOIN candidate b ON b.company_id = a.company_id
                    AND b.req_key = a.req_key
                    AND b.norm_title = a.norm_title
                    AND b.posting_id <> a.posting_id
    WHERE @dedup::bool AND a.req_key IS NOT NULL
),
reach AS (
    SELECT posting_id AS anchor, posting_id AS member FROM candidate
    UNION
    SELECT r.anchor, e.dst FROM reach r JOIN edge e ON e.src = r.member
),
unit AS (
    SELECT anchor AS posting_id, min(member) AS unit_id
    FROM reach
    GROUP BY anchor
),
keyed AS (
    SELECT c.posting_id,
           c.company_id,
           c.first_seen_at,
           c.company_name,
           c.title,
           c.description_text,
           (c.company_id::text || '|unit:' || u.unit_id::text)::text AS dedup_key
    FROM candidate c
    JOIN unit u ON u.posting_id = c.posting_id
),
grouped AS (
    SELECT dedup_key,
           company_id,
           min(first_seen_at) AS unit_oldest,
           max(first_seen_at) AS unit_newest,
           min(posting_id)    AS unit_min_id,
           max(posting_id)    AS unit_max_id
    FROM keyed
    GROUP BY dedup_key, company_id
),
capped AS (
    SELECT g.dedup_key,
           g.unit_oldest,
           g.unit_newest,
           g.unit_min_id,
           g.unit_max_id,
           row_number() OVER (
               PARTITION BY g.company_id
               ORDER BY CASE WHEN @newest_first::bool THEN g.unit_newest END DESC,
                        CASE WHEN @newest_first::bool THEN g.unit_max_id END DESC,
                        CASE WHEN NOT @newest_first::bool THEN g.unit_oldest END ASC,
                        CASE WHEN NOT @newest_first::bool THEN g.unit_min_id END ASC) AS company_slot
    FROM grouped g
),
waved AS (
    SELECT c.dedup_key,
           dense_rank() OVER (
               ORDER BY c.company_slot ASC,
                        CASE WHEN @newest_first::bool THEN c.unit_newest END DESC,
                        CASE WHEN @newest_first::bool THEN c.unit_max_id END DESC,
                        CASE WHEN NOT @newest_first::bool THEN c.unit_oldest END ASC,
                        CASE WHEN NOT @newest_first::bool THEN c.unit_min_id END ASC) AS slot
    FROM capped c
    WHERE @max_per_company::int <= 0 OR c.company_slot <= @max_per_company::int
),
ranked AS (
    SELECT k.posting_id,
           k.company_id,
           k.company_name,
           k.title,
           k.description_text,
           k.dedup_key,
           CASE WHEN @dedup::bool
                THEN (row_number() OVER (PARTITION BY k.dedup_key ORDER BY k.posting_id) = 1)
                ELSE true
           END AS is_representative,
           w.slot
    FROM keyed k
    JOIN waved w ON w.dedup_key = k.dedup_key
)
SELECT posting_id,
       company_id,
       company_name,
       title,
       description_text,
       dedup_key,
       is_representative
FROM ranked
WHERE slot <= @row_limit::int
ORDER BY slot, posting_id;

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
