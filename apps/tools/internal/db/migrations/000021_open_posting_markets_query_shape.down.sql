-- Restore the exact 000020 view definition when rolling back the query-shape
-- correction.

CREATE OR REPLACE VIEW open_posting_markets AS
WITH matched_markets AS (
    SELECT DISTINCT
        open_postings.job_posting_id,
        market_seeds.slug,
        market_seeds.name,
        market_seeds.kind
    FROM open_postings
    JOIN LATERAL (
        SELECT posting_snapshots.location_texts
        FROM posting_snapshots
        WHERE posting_snapshots.job_posting_id = open_postings.job_posting_id
            AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id
        ORDER BY posting_snapshots.fetched_at DESC, posting_snapshots.id DESC
        LIMIT 1
    ) AS current_snapshot ON TRUE
    CROSS JOIN LATERAL unnest(current_snapshot.location_texts) AS location(location_text)
    JOIN market_seeds
        ON market_seeds.slug <> 'unmapped'
    CROSS JOIN LATERAL unnest(market_seeds.patterns) AS pattern(value)
    WHERE location.location_text ~* ('\y' || pattern.value || '\y')
)
SELECT job_posting_id, slug, name, kind
FROM matched_markets

UNION ALL

SELECT
    open_postings.job_posting_id,
    unmapped.slug,
    unmapped.name,
    unmapped.kind
FROM open_postings
JOIN market_seeds AS unmapped ON unmapped.slug = 'unmapped'
WHERE NOT EXISTS (
    SELECT 1
    FROM matched_markets
    WHERE matched_markets.job_posting_id = open_postings.job_posting_id
);
