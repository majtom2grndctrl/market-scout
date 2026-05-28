ALTER TABLE posting_snapshots
    ADD COLUMN source_first_published_at timestamptz,
    ADD COLUMN source_last_modified_at timestamptz;
