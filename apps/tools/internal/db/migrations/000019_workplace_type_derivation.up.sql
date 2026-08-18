-- Derive workplace type in the read model without changing append-only
-- snapshots. Local performance check (2026-08-14, 66,092 snapshots): after
-- warm-up, EXPLAIN (ANALYZE, TIMING OFF) had a 227.567ms median across five
-- runs. That run measured raw_data stored as pglz, and predates the run-scoped
-- predicate on current_snapshot below; future compressed raw_data values are
-- configured to use lz4.

CREATE OR REPLACE VIEW open_postings_display AS
SELECT
    open_postings.job_posting_id,
    latest_successful_fetch_runs.company_id,
    companies.name AS company_name,
    latest_successful_fetch_runs.started_at AS run_started_at,
    current_snapshot.title,
    current_snapshot.location_text,
    current_snapshot.workplace_type,
    current_snapshot.compensation_min,
    current_snapshot.compensation_max,
    current_snapshot.compensation_currency,
    current_classification.id AS classification_id,
    current_classification.seniority,
    workplace_resolution.workplace_type_resolved,
    CASE
        WHEN workplace_resolution.workplace_type_resolved IS NULL THEN NULL
        WHEN current_snapshot.workplace_type IS NOT NULL THEN 'ats'
        WHEN raw_resolution.workplace_type_resolved IS NOT NULL THEN 'raw_data'
        ELSE 'location_text'
    END AS workplace_type_source
FROM open_postings
JOIN latest_successful_fetch_runs
    ON latest_successful_fetch_runs.fetch_run_id = open_postings.fetch_run_id
JOIN companies ON companies.id = latest_successful_fetch_runs.company_id
LEFT JOIN LATERAL (
    SELECT
        posting_snapshots.title,
        posting_snapshots.location_text,
        posting_snapshots.workplace_type,
        posting_snapshots.compensation_min,
        posting_snapshots.compensation_max,
        posting_snapshots.compensation_currency,
        posting_snapshots.raw_data
    FROM posting_snapshots
    WHERE posting_snapshots.job_posting_id = open_postings.job_posting_id
        -- Content comes from the same run that established openness. Without
        -- this, a snapshot written by a later failed run — or by no run at all —
        -- could supply the displayed row. open_postings only admits a posting
        -- that has a snapshot in this run, so the predicate never empties the
        -- lateral and never changes row counts.
        AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id
    ORDER BY posting_snapshots.fetched_at DESC, posting_snapshots.id DESC
    LIMIT 1
) AS current_snapshot ON TRUE
LEFT JOIN LATERAL (
    SELECT classifications.id, classifications.seniority
    FROM classifications
    WHERE classifications.job_posting_id = open_postings.job_posting_id
    ORDER BY classifications.classified_at DESC, classifications.id DESC
    LIMIT 1
) AS current_classification ON TRUE
LEFT JOIN LATERAL (
    -- Concatenation forces one fresh in-memory datum; OFFSET 0 prevents the
    -- planner from inlining it and turning each tier-2 lookup into a detoast.
    SELECT
        r.doc->>'telecommuting' AS telecommuting,
        r.doc->'metadata' AS metadata
    FROM (
        SELECT current_snapshot.raw_data || '{}'::jsonb AS doc
        WHERE current_snapshot.workplace_type IS NULL
        OFFSET 0
    ) AS r
) AS rd ON TRUE
LEFT JOIN LATERAL (
    SELECT
        bool_or(
            m.entry->>'name' = 'Remote'
            AND m.entry->>'value' = 'true'
        ) AS remote_true,
        bool_or(
            m.entry->>'name' = 'Remote'
            AND m.entry->>'value' = 'false'
        ) AS remote_false,
        -- Some boards send Location Type as a one-element array. slot.value is
        -- the expanded element and is NULL for a scalar value, so the coalesce
        -- reads both shapes the way the sibling aggregates below do.
        max(CASE
            WHEN m.entry->>'name' = 'Location Type'
                THEN coalesce(slot.value, m.entry->>'value')
        END) AS location_type,
        bool_or(
            m.entry->>'name' = 'Job Posting Location'
            AND slot.value ~* '\yremote\y'
        ) AS job_location_remote,
        bool_or(
            m.entry->>'name' = 'Job Posting Location'
            AND slot.value ~* '\y(office|headquarters|hq)\y'
        ) AS job_location_onsite
    FROM jsonb_array_elements(
        CASE
            WHEN jsonb_typeof(rd.metadata) = 'array' THEN rd.metadata
            ELSE '[]'::jsonb
        END
    ) AS m(entry)
    -- The WHERE documents the metadata-array contract; the CASE above makes
    -- the set-returning function safe even if the planner evaluates it first.
    LEFT JOIN LATERAL jsonb_array_elements_text(
        CASE
            WHEN jsonb_typeof(m.entry->'value') = 'array' THEN m.entry->'value'
            ELSE '[]'::jsonb
        END
    ) AS slot(value) ON TRUE
    WHERE jsonb_typeof(rd.metadata) = 'array'
) AS md ON TRUE
LEFT JOIN LATERAL (
    SELECT CASE
        WHEN rd.telecommuting = 'true' THEN 'remote'
        WHEN md.remote_true THEN 'remote'
        WHEN md.location_type ~* '^on-site' THEN 'onsite'
        WHEN md.location_type ~* '^remote' THEN 'remote'
        WHEN md.location_type ~* '^hybrid' THEN 'hybrid'
        WHEN md.job_location_remote THEN 'remote'
        WHEN md.job_location_onsite THEN 'onsite'
    END AS workplace_type_resolved,
    CASE
        WHEN rd.telecommuting = 'false' OR md.remote_false THEN TRUE
        ELSE FALSE
    END AS false_boolean
) AS raw_resolution ON TRUE
LEFT JOIN LATERAL (
    SELECT regexp_replace(
        btrim(regexp_replace(
            translate(current_snapshot.location_text, U&'\00A0', ' '),
            '\s+',
            ' ',
            'g'
        )),
        'remote[- ]friendly',
        '',
        'gi'
    ) AS location_text
) AS normalized_location ON TRUE
LEFT JOIN LATERAL (
    SELECT CASE
        -- Reported ATS and structured raw data outrank location-text inference.
        WHEN current_snapshot.workplace_type IS NOT NULL
            THEN current_snapshot.workplace_type
        WHEN raw_resolution.workplace_type_resolved IS NOT NULL
            THEN raw_resolution.workplace_type_resolved
        -- A false raw boolean is evidence against tier 3 only after every raw
        -- branch has had the chance to provide a three-value answer, and only
        -- against 'remote': it contradicts remote location text but agrees with
        -- hybrid and onsite, which therefore still resolve.
        WHEN normalized_location.location_text ~* '\yhybrid\y' THEN 'hybrid'
        WHEN normalized_location.location_text ~* '\yremote\y'
            AND NOT raw_resolution.false_boolean THEN 'remote'
        WHEN normalized_location.location_text ~* '\yon[- ]?site\y' THEN 'onsite'
    END AS workplace_type_resolved
) AS workplace_resolution ON TRUE;

-- This changes the compression setting for future writes only; existing
-- append-only snapshots remain untouched.
ALTER TABLE posting_snapshots
    ALTER COLUMN raw_data SET COMPRESSION lz4;
