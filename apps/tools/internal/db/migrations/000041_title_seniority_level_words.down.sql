-- Restore 000036's patterns for the four ranks 000041 widened. No rows were
-- added or removed, so the arrays are the whole reversal.

UPDATE title_seniority_seeds
SET patterns = ARRAY['intern', 'internship', 'co-?op']
WHERE slug = 'intern';

UPDATE title_seniority_seeds
SET patterns = ARRAY['junior', 'jr\.?', 'entry[ -]level', 'new grad(uate)?']
WHERE slug = 'junior';

UPDATE title_seniority_seeds
SET patterns = ARRAY['mid[ -]level']
WHERE slug = 'mid';

UPDATE title_seniority_seeds
SET patterns = ARRAY['lead']
WHERE slug = 'lead';
