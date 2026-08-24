-- Reverse only the patterns appended by 000022, preserving the original seed
-- arrays installed by 000020.

UPDATE market_seeds
SET patterns = patterns[1:array_length(patterns, 1) - 4]
WHERE slug = 'sf-bay-area';

UPDATE market_seeds
SET patterns = patterns[1:array_length(patterns, 1) - 1]
WHERE slug = 'seattle';
