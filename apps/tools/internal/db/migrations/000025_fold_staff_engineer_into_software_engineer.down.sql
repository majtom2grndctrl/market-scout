-- Reverse 000025 as far as the data allows.
--
-- This down migration is deliberately partial. The up migration folded 20
-- job_posting_roles rows into 'software-engineer', and which 20 is not
-- recoverable afterward: they became indistinguishable from the 50 postings
-- that already read software-engineer + seniority='staff'. Reconstructing
-- membership by seniority would capture all 70, which is a different error,
-- not a restoration. So this file restores the role row and its dimensions
-- only; the fold itself stands.
--
-- Restore membership from a backup if it is genuinely needed.

INSERT INTO canonical_roles (slug, name) VALUES
    ('staff-engineer', 'Staff Software Engineer')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
SELECT cr.id, rd.id
FROM canonical_roles cr
CROSS JOIN role_dimensions rd
WHERE cr.slug = 'staff-engineer'
  AND rd.slug IN ('data', 'engineering', 'operations', 'product', 'research')
ON CONFLICT DO NOTHING;
