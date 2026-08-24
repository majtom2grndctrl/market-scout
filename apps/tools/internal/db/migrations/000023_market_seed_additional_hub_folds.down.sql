-- Reverse only the patterns appended by 000023, preserving the dictionary
-- installed by the preceding migrations.

UPDATE market_seeds
SET patterns = patterns[1:array_length(patterns, 1) - 1]
WHERE slug = 'sf-bay-area';

UPDATE market_seeds
SET patterns = patterns[1:array_length(patterns, 1) - 1]
WHERE slug = 'washington-dc';

UPDATE market_seeds
SET patterns = patterns[1:array_length(patterns, 1) - 1]
WHERE slug = 'new-york';

UPDATE market_seeds
SET patterns = patterns[1:array_length(patterns, 1) - 2]
WHERE slug = 'seattle';
