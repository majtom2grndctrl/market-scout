-- Data-integrity fix: several canonical_roles/specializations/skills rows minted
-- by save_enrichment ended up with a slug and name describing two different,
-- unrelated concepts (an agent scrambled which name attached to which slug in
-- a batch mint), or were left with an empty name entirely. The upsert in
-- apps/tools/internal/db/queries/classifications.sql is a slug-keyed no-op on
-- conflict, so a wrong name sticks forever until corrected directly here.
--
-- Only `name` is touched. `slug` values are never changed — see
-- agent-context/lib/developer-guide.md §5.7 and the audit that produced this
-- migration for the usage evidence behind each correction.

-- skills: confirmed mismatches
UPDATE skills SET name = 'MATLAB' WHERE slug = 'matlab';
UPDATE skills SET name = 'Budget Management' WHERE slug = 'budget-management';
UPDATE skills SET name = 'Quality Control' WHERE slug = 'quality-control';

-- skills: empty names, filled from slug + usage evidence
UPDATE skills SET name = 'CLI' WHERE slug = 'cli';
UPDATE skills SET name = 'Apache Spark' WHERE slug = 'apache-spark';
UPDATE skills SET name = 'Campaign Planning' WHERE slug = 'campaign-planning';
UPDATE skills SET name = 'Clojure' WHERE slug = 'clojure';
UPDATE skills SET name = 'Cold Calling' WHERE slug = 'cold-calling';
UPDATE skills SET name = 'Database Systems' WHERE slug = 'database-systems';
UPDATE skills SET name = 'Data Pipeline' WHERE slug = 'data-pipeline';
UPDATE skills SET name = 'Functional Programming' WHERE slug = 'functional-programming';
UPDATE skills SET name = 'GDPR' WHERE slug = 'gdpr';
UPDATE skills SET name = 'MLOps Tooling' WHERE slug = 'mlops-tooling';
UPDATE skills SET name = 'Predictive Modeling' WHERE slug = 'predictive-modeling';
UPDATE skills SET name = 'Territory Management' WHERE slug = 'territory-management';
UPDATE skills SET name = 'Zendesk' WHERE slug = 'zendesk';

-- specializations: confirmed mismatches
UPDATE specializations SET name = 'Cloud-Native Deployment' WHERE slug = 'cloud-native-deployment';
UPDATE specializations SET name = 'Real-Time Systems' WHERE slug = 'real-time-systems';
UPDATE specializations SET name = 'Public Safety' WHERE slug = 'public-safety';

-- specializations: empty names, filled from slug + usage evidence
UPDATE specializations SET name = 'Account-Based Marketing' WHERE slug = 'account-based-marketing';
UPDATE specializations SET name = 'Cloud Platforms & Infrastructure' WHERE slug = 'cloud-platforms-infrastructure';
UPDATE specializations SET name = 'Integration Engineering' WHERE slug = 'integration-engineering';
UPDATE specializations SET name = 'Luxury Travel' WHERE slug = 'luxury-travel';
UPDATE specializations SET name = 'Martech' WHERE slug = 'martech';
UPDATE specializations SET name = 'Mechanical Design' WHERE slug = 'mechanical-design';
