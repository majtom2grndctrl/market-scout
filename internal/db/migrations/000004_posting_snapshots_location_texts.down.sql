-- Destructive: drops captured multi-market location strings. No backfill exists.
ALTER TABLE posting_snapshots
    DROP COLUMN location_texts;
