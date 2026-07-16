-- name: FindCompanyDedupStatus :one
-- Given (ats, board_token), returns whether a matching company exists and
-- whether it has any posting_snapshots within the recency window.
--
-- Recency window: snapshots from now() - (recency_days days) and later count
-- as "recent". The caller passes the recency window in days; watchlist callers
-- default to 30. Pushing the interval construction into SQL avoids a
-- clock-skew gap between app server and database, and avoids the awkward
-- interval-as-int64 type sqlc emits for raw interval parameters.
--
-- Returns zero rows if no company matches. Returns exactly one row if a
-- company exists, with has_recent_snapshot=false when the company has no
-- postings or no snapshots in the window.
SELECT
    c.id AS company_id,
    c.name,
    c.ats,
    c.board_token,
    c.industry,
    c.careers_page_url,
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

-- name: FindCompaniesByNormalizedNames :many
-- Given a batch of raw candidate names, returns all matching companies for each
-- candidate's normalized name. This preserves collisions for human
-- disambiguation. Normalization is the watchlist dedup rule: strip punctuation
-- and whitespace by keeping only alphanumeric characters, then lowercase and
-- compare for equality.
--
-- input_index is 1-based, matching WITH ORDINALITY from the input array.
SELECT
    candidate_names.input_index,
    c.id AS company_id,
    c.name,
    c.ats,
    c.board_token,
    c.industry,
    c.careers_page_url,
    EXISTS (
        SELECT 1
        FROM job_postings jp
        JOIN posting_snapshots ps ON ps.job_posting_id = jp.id
        WHERE jp.company_id = c.id
          AND ps.fetched_at >= now() - make_interval(days => sqlc.arg(recency_days)::int)
    ) AS has_recent_snapshot
FROM (
    SELECT
        ordinality::int AS input_index,
        lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')) AS normalized_name
    FROM unnest(@candidate_names::text[]) WITH ORDINALITY AS raw_names(name, ordinality)
) candidate_names
JOIN (
    SELECT
        id,
        name,
        ats,
        board_token,
        industry,
        careers_page_url,
        lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')) AS normalized_name
    FROM companies
) c ON c.normalized_name = candidate_names.normalized_name
ORDER BY candidate_names.input_index, c.id;

-- name: FindCompaniesByCareersURLHost :many
-- Given a batch of candidate careers URLs, returns companies whose careers-page
-- host matches after lowercasing and removing an optional www prefix.
-- input_index is 1-based, matching WITH ORDINALITY from the input array.
SELECT
    candidate_urls.input_index,
    c.id AS company_id,
    c.name,
    c.ats,
    c.board_token,
    c.industry,
    c.careers_page_url,
    EXISTS (
        SELECT 1
        FROM job_postings jp
        JOIN posting_snapshots ps ON ps.job_posting_id = jp.id
        WHERE jp.company_id = c.id
          AND ps.fetched_at >= now() - make_interval(days => sqlc.arg(recency_days)::int)
    ) AS has_recent_snapshot
FROM (
    SELECT
        ordinality::int AS input_index,
        lower(regexp_replace(url, '^https?://(www\.)?([^/]+).*$', '\2')) AS careers_url_host
    FROM unnest(@candidate_urls::text[]) WITH ORDINALITY AS raw_urls(url, ordinality)
) candidate_urls
JOIN (
    SELECT
        id,
        name,
        ats,
        board_token,
        industry,
        careers_page_url,
        lower(regexp_replace(careers_page_url, '^https?://(www\.)?([^/]+).*$', '\2')) AS careers_url_host
    FROM companies
) c ON c.careers_url_host = candidate_urls.careers_url_host
ORDER BY candidate_urls.input_index, c.id;

-- name: FindCompaniesByNameSimilarity :many
-- Given a batch of raw candidate names, returns companies whose normalized
-- names meet the caller's trigram-similarity threshold. input_index is 1-based,
-- matching WITH ORDINALITY from the input array.
WITH candidate_names AS (
    SELECT
        ordinality::int AS input_index,
        lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')) AS normalized_name
    FROM unnest(@candidate_names::text[]) WITH ORDINALITY AS raw_names(name, ordinality)
), normalized_companies AS (
    SELECT
        id,
        name,
        ats,
        board_token,
        industry,
        careers_page_url,
        lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')) AS normalized_name
    FROM companies
), scored_matches AS (
    SELECT
        candidate_names.input_index,
        c.*,
        similarity(candidate_names.normalized_name, c.normalized_name)::float8 AS score
    FROM candidate_names
    CROSS JOIN normalized_companies c
)
SELECT
    scored_matches.input_index,
    scored_matches.id AS company_id,
    scored_matches.name,
    scored_matches.ats,
    scored_matches.board_token,
    scored_matches.industry,
    scored_matches.careers_page_url,
    EXISTS (
        SELECT 1
        FROM job_postings jp
        JOIN posting_snapshots ps ON ps.job_posting_id = jp.id
        WHERE jp.company_id = scored_matches.id
          AND ps.fetched_at >= now() - make_interval(days => sqlc.arg(recency_days)::int)
    ) AS has_recent_snapshot,
    scored_matches.score
FROM scored_matches
WHERE scored_matches.score >= sqlc.arg(similarity_threshold)::float8
ORDER BY scored_matches.input_index, scored_matches.score DESC, scored_matches.id;
