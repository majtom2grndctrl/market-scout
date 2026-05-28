-- Destructive: drops fetch-run linkage, captured descriptions, and compensation
-- fields from snapshots. No backfill exists.
ALTER TABLE posting_snapshots
    DROP COLUMN compensation_period,
    DROP COLUMN compensation_currency,
    DROP COLUMN compensation_max,
    DROP COLUMN compensation_min,
    DROP COLUMN description_text,
    DROP COLUMN fetch_run_id;

DROP TABLE fetch_runs;
