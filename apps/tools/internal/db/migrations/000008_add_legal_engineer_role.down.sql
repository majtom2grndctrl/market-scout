-- Removes the 'legal-engineer' canonical role.
-- If any classification references this role via job_posting_roles, the RESTRICT FK
-- (job_posting_roles.role_id → canonical_roles.id ON DELETE RESTRICT) will block
-- this delete. On a database with enrichment history against legal engineer postings,
-- delete rows from job_posting_roles where role_id = (SELECT id FROM canonical_roles
-- WHERE slug = 'legal-engineer') before retrying.
DELETE FROM canonical_roles WHERE slug = 'legal-engineer';
