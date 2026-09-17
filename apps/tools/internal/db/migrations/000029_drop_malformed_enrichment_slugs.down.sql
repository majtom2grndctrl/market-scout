-- Reverse 000029 as far as the data allows.
--
-- This down migration is deliberately partial, for the same reason 000025's is: the
-- up migration deleted link rows, and which classification carried which tag is not
-- recoverable from the surviving schema. Restoring the taxonomy rows gives back the
-- vocabulary; it does not re-tag the postings.
--
-- Membership is recorded in the run report at
-- agent-output/batch-enrich/2026-09-16-0843.md and in the up migration's comments,
-- which name every affected posting. Re-tagging from those is a manual step, and in
-- most cases the right move is to re-classify the posting rather than restore a slug
-- the contract rejects.
--
-- Restore from a backup if genuine membership recovery is needed.

INSERT INTO specializations (slug, name) VALUES
    ('japan-market',          'Japan Markets'),
    ('south-korea-market',    'South Korea Markets'),
    ('emerging-technologies', 'Emerging Technologies')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO skills (slug, name) VALUES
    ('vlm-inference',              'VLM Inference'),
    ('geo-targeting',              'Geo Targeting'),
    ('aws-gcp-azure',              'AWS / GCP / Azure'),
    ('icd-snomed-standards',       'ICD & SNOMED Standards'),
    ('technical-expertise',        'Technical Expertise'),
    ('technical-acumen',           'Technical Acumen'),
    ('systems-knowledge',          'Systems Knowledge'),
    ('operations-management',      'Operations Management'),
    ('infrastructure',             'Infrastructure Architecture & Operations'),
    ('cross-functional-alignment', 'Cross-functional Alignment'),
    ('compliance',                 'General Compliance Knowledge')
ON CONFLICT (slug) DO NOTHING;
