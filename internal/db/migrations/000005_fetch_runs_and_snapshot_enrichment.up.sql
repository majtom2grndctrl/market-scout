-- Tracks each fetch attempt per company so snapshots can be tied back to a run
-- (success/failure, error context, postings count). Snapshots gain a nullable
-- FK to fetch_runs (existing rows stay NULL) plus description and compensation
-- fields captured from richer ATS payloads.
CREATE TABLE fetch_runs (
    id             bigserial PRIMARY KEY,
    company_id     bigint NOT NULL REFERENCES companies(id),
    started_at     timestamptz NOT NULL,
    completed_at   timestamptz,
    status         text NOT NULL CHECK (status IN ('in_progress','success','failed')),
    error_message  text,
    postings_count integer
);

ALTER TABLE posting_snapshots
    ADD COLUMN fetch_run_id          bigint REFERENCES fetch_runs(id),
    ADD COLUMN description_text      text,
    ADD COLUMN compensation_min      bigint,
    ADD COLUMN compensation_max      bigint,
    ADD COLUMN compensation_currency text,
    ADD COLUMN compensation_period   text CHECK (compensation_period IN ('hour','day','week','month','year'));
