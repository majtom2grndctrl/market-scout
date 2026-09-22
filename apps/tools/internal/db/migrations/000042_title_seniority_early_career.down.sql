-- Restore 000041's patterns for the one rank 000042 widened. No rows were
-- added or removed, so the array is the whole reversal.

UPDATE title_seniority_seeds
SET patterns = ARRAY['junior', 'jr\.?', 'entry[ -]level', 'new grad(uate)?', 'university grad(uate)?']
WHERE slug = 'junior';
