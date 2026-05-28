-- Destructive: drops captured ATS timestamps. No backfill exists.
ALTER TABLE posting_snapshots
    DROP COLUMN source_last_modified_at,
    DROP COLUMN source_first_published_at;
