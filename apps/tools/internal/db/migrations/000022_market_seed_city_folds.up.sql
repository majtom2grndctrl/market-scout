-- Extend the curated hub dictionary with the approved SF Bay Area folds and
-- the observed Redmond, WA / Redmond, Washington Seattle fold. Keep this
-- correction data-only so the open_posting_markets view contract is unchanged.

UPDATE market_seeds
SET patterns = patterns || ARRAY['palo alto', 'menlo park', 'sunnyvale', 'san jose']
WHERE slug = 'sf-bay-area';

UPDATE market_seeds
SET patterns = patterns || ARRAY['redmond']
WHERE slug = 'seattle';
