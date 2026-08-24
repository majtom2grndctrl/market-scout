-- Extend the curated hub dictionary with the remaining observed city folds.
-- This correction changes seed data only; open_posting_markets keeps its
-- existing derivation contract.

UPDATE market_seeds
SET patterns = patterns || ARRAY['south san francisco']
WHERE slug = 'sf-bay-area';

UPDATE market_seeds
SET patterns = patterns || ARRAY['arlington,? (virginia|va)']
WHERE slug = 'washington-dc';

UPDATE market_seeds
SET patterns = patterns || ARRAY['newark,? (new jersey|nj)']
WHERE slug = 'new-york';

UPDATE market_seeds
SET patterns = patterns || ARRAY['woodinville', 'issaquah']
WHERE slug = 'seattle';
