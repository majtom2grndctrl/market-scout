-- Data-integrity fix: 'staff-engineer' (name 'Staff Software Engineer') encodes
-- seniority in the role slug, splitting one population across two dimensions.
-- 50 postings read software-engineer + seniority='staff'; 20 read staff-engineer
-- + seniority='staff'. A grouping by role and a grouping by seniority therefore
-- answer the same question differently, which is exactly the failure the
-- composition grammar cannot absorb.
--
-- All 20 rows already carry seniority='staff', so folding the role loses nothing
-- the seniority dimension does not already hold. No classification carries both
-- slugs, so the UPDATE cannot collide on the job_posting_roles primary key.
--
-- Order is load-bearing: job_posting_roles_role_id_fkey is ON DELETE RESTRICT,
-- so the remap must precede the delete. canonical_role_dimensions is
-- ON DELETE CASCADE and needs no explicit cleanup.
--
-- The root cause is prompt-side, not schema-side: apps/tools/cmd/batch-enrich
-- already forbids seniority modifiers in role slugs, but the parallel-subagent
-- path had drifted from it. Both contracts now carry the rule.

UPDATE job_posting_roles
SET role_id = (SELECT id FROM canonical_roles WHERE slug = 'software-engineer')
WHERE role_id = (SELECT id FROM canonical_roles WHERE slug = 'staff-engineer');

DELETE FROM canonical_roles WHERE slug = 'staff-engineer';
