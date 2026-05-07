CREATE INDEX idx_fetch_runs_company_id_started_at
    ON fetch_runs (company_id, started_at DESC);

CREATE INDEX idx_posting_snapshots_fetch_run_id
    ON posting_snapshots (fetch_run_id)
    WHERE fetch_run_id IS NOT NULL;
