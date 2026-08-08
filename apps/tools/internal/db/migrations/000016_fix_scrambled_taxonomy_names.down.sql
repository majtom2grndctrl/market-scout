-- Reverse 000016: restore the exact prior names captured before this fix.

-- skills: confirmed mismatches
UPDATE skills SET name = 'Mathematica' WHERE slug = 'matlab';
UPDATE skills SET name = 'Vendor Management and Coordination' WHERE slug = 'budget-management';
UPDATE skills SET name = 'Quality Operations' WHERE slug = 'quality-control';

-- skills: previously empty names
UPDATE skills SET name = '' WHERE slug = 'cli';
UPDATE skills SET name = '' WHERE slug = 'apache-spark';
UPDATE skills SET name = '' WHERE slug = 'campaign-planning';
UPDATE skills SET name = '' WHERE slug = 'clojure';
UPDATE skills SET name = '' WHERE slug = 'cold-calling';
UPDATE skills SET name = '' WHERE slug = 'database-systems';
UPDATE skills SET name = '' WHERE slug = 'data-pipeline';
UPDATE skills SET name = '' WHERE slug = 'functional-programming';
UPDATE skills SET name = '' WHERE slug = 'gdpr';
UPDATE skills SET name = '' WHERE slug = 'mlops-tooling';
UPDATE skills SET name = '' WHERE slug = 'predictive-modeling';
UPDATE skills SET name = '' WHERE slug = 'territory-management';
UPDATE skills SET name = '' WHERE slug = 'zendesk';

-- specializations: confirmed mismatches
UPDATE specializations SET name = 'Cloud Infrastructure (infrastructure-capex-accounting)' WHERE slug = 'cloud-native-deployment';
UPDATE specializations SET name = 'Observability Systems' WHERE slug = 'real-time-systems';
UPDATE specializations SET name = 'State & Local Government Sector' WHERE slug = 'public-safety';

-- specializations: previously empty names
UPDATE specializations SET name = '' WHERE slug = 'account-based-marketing';
UPDATE specializations SET name = '' WHERE slug = 'cloud-platforms-infrastructure';
UPDATE specializations SET name = '' WHERE slug = 'integration-engineering';
UPDATE specializations SET name = '' WHERE slug = 'luxury-travel';
UPDATE specializations SET name = '' WHERE slug = 'martech';
UPDATE specializations SET name = '' WHERE slug = 'mechanical-design';
