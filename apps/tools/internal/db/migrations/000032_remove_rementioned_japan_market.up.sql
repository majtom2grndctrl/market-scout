-- Remove 'japan-market', re-minted as specializations id 11342 on 2026-09-17 at
-- 17:44:14 by a classification run that named it explicitly, in its own agent
-- brief, as the forbidden example. 000029 (2026-09-16) deleted this same slug for
-- the same reason: geography is not taxonomy, per project.md -- geography enters
-- this project's analysis surface as a curated `market` dimension, never as a
-- taxonomy term. 000031 added the retired_slugs registry so mcp.save_enrichment
-- now rejects minting any registered slug, but deliberately left this row alone:
-- a slug that already EXISTS is treated as reuse and is never blocked, so the
-- registry could not clean up what predated it. This migration is that cleanup.
--
-- Queried at authoring time: 'japan-market' existed only in specializations (id
-- 11342), referenced by exactly one job_posting_specializations row, attached to
-- classification 6255 (job_posting_id 45165).
--
-- Also checked: every other slug in retired_slugs (the 13 others 000029 removed)
-- against all three taxonomy tables (canonical_roles, specializations, skills).
-- None of them have come back. 'japan-market' is the only re-mint.
--
-- Order is load-bearing, exactly as in 000029: job_posting_specializations_specialization_id_fkey
-- is ON DELETE RESTRICT, so the link row must go before the taxonomy row. The
-- classification itself survives; it simply loses this tag.

DELETE FROM job_posting_specializations
WHERE specialization_id IN (
    SELECT id FROM specializations WHERE slug = 'japan-market'
);

DELETE FROM specializations WHERE slug = 'japan-market';
