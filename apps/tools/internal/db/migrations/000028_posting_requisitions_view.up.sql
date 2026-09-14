-- Expose the ATS's own requisition identifier to the read model, so consumers
-- can count requisitions alongside postings. Some boards list one requisition
-- once per location; posting count overstates hiring for those companies. This
-- view gives a second denominator without discarding a row.
--
-- One row per open posting: open_postings is DISTINCT over
-- (job_posting_id, fetch_run_id), and a posting belongs to exactly one company,
-- so only that company's single latest successful run can match. Openness is
-- derived here rather than re-derived from fetch_runs, because
-- 000017_read_model_views defines it once for the whole read model.
--
-- requisition_key is never NULL. A posting whose current snapshot carries no
-- key falls back to its own identity, prefixed with 'posting:' so a synthesized
-- key can never collide with a real one. requisition_source mirrors
-- workplace_type_source in 000019_workplace_type_derivation: without it a
-- fallback is indistinguishable from a key the platform actually supplied.
--
-- Keys are board-scoped, not global -- Greenhouse numbers per board, Workday
-- per tenant, and Workable's `code` is human-typed ('2026-37', 'SQ204'). There
-- are no cross-company collisions among today's keys, but two Workable boards
-- numbering by year-week collide on their first overlap. company_id travels
-- with the key so consumers count distinct over (company_id, requisition_key).
--
-- company_id comes from job_postings rather than latest_successful_fetch_runs.
-- open_postings already requires the two to agree, so the values are equal by
-- construction and this path is one join shorter.

CREATE VIEW posting_requisitions AS
SELECT
    open_postings.job_posting_id,
    job_postings.company_id,
    COALESCE(
        current_snapshot.requisition_key,
        'posting:' || open_postings.job_posting_id
    ) AS requisition_key,
    CASE
        WHEN current_snapshot.requisition_key IS NOT NULL THEN 'ats'
        ELSE 'posting'
    END AS requisition_source
FROM open_postings
JOIN job_postings ON job_postings.id = open_postings.job_posting_id
LEFT JOIN LATERAL (
    SELECT posting_snapshots.requisition_key
    FROM posting_snapshots
    WHERE posting_snapshots.job_posting_id = open_postings.job_posting_id
        -- The key comes from the same run that established openness. This
        -- predicate is the one in 000019_workplace_type_derivation, not the
        -- older lateral in 000017_read_model_views, which predates it: without
        -- it a snapshot written by a later failed run could supply the key, and
        -- a green fixture test would never show it. open_postings only admits a
        -- posting with a snapshot in this run, so the predicate never empties
        -- the lateral and never changes row counts.
        AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id
    ORDER BY posting_snapshots.fetched_at DESC, posting_snapshots.id DESC
    LIMIT 1
) AS current_snapshot ON TRUE;
