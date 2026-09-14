-- Drops the extracted requisition keys. Lossless in practice: raw_data is
-- retained on every snapshot, so re-running the up migration reconstructs the
-- same values from the same source.
ALTER TABLE posting_snapshots
    DROP COLUMN requisition_key;
