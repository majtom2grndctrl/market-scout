-- Capture the ATS's own requisition identifier per snapshot, so the read model
-- can count requisitions alongside postings. Some boards list one requisition
-- many times -- one row per location -- which makes posting count overstate
-- hiring for those companies. The key is what lets those rows collapse without
-- discarding any of them.
--
-- The column lives on posting_snapshots, not job_postings, because it is an
-- observed field like title: a board that reassigns a key should leave history
-- intact. It is a plain column, not a generated one, because a stored generated
-- column may only reference its own row and the extraction path depends on
-- companies.ats, two joins away.
--
-- No index. idx_posting_snapshots_posting_fetched on
-- (job_posting_id, fetched_at DESC) already serves the current-snapshot lateral
-- the read-model view uses; a second index on an 84k-row append-only table
-- would be write amplification with no reader.

ALTER TABLE posting_snapshots
    ADD COLUMN requisition_key text;

-- One-time backfill over existing history. Adapters own extraction going
-- forward, but they cannot reach rows already written, so raw_data is the right
-- source here. This UPDATE does not contradict the append-only snapshot rule in
-- developer-guide.md 5.7: that rule governs the fetcher's write path, not a
-- migration populating a newly added column.
--
-- Branch on companies.ats rather than sniffing raw_data for a known key. Gem
-- boards are Greenhouse-shaped and carry internal_job_id in their payload, so a
-- sniffing backfill would populate Gem rows and break the NULL contract below.
--
-- Extraction uses ->> throughout so a JSON-null yields SQL NULL. 356 Greenhouse
-- snapshots across 20 postings carry internal_job_id as JSON null, and 412
-- Workable snapshots carry code as JSON null; -> followed by a cast would turn
-- those into the string 'null'. nullif then folds the empty string (66 Workable
-- snapshots) into NULL as well.

UPDATE posting_snapshots s
SET requisition_key = nullif(s.raw_data->>'internal_job_id', '')
FROM job_postings p
JOIN companies c ON c.id = p.company_id
WHERE p.id = s.job_posting_id
  AND c.ats = 'greenhouse';

-- Workday exposes the requisition id as the first bulletFields entry.
UPDATE posting_snapshots s
SET requisition_key = nullif(s.raw_data->'bulletFields'->>0, '')
FROM job_postings p
JOIN companies c ON c.id = p.company_id
WHERE p.id = s.job_posting_id
  AND c.ats = 'workday';

UPDATE posting_snapshots s
SET requisition_key = nullif(s.raw_data->>'code', '')
FROM job_postings p
JOIN companies c ON c.id = p.company_id
WHERE p.id = s.job_posting_id
  AND c.ats = 'workable';

-- ashby, lever, and gem rows stay NULL. Those platforms expose no identifier
-- distinct from posting identity, so there is nothing honest to extract; the
-- read model falls back to posting identity for them.
