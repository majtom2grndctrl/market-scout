-- Reverse 000030 in full.
--
-- Unlike 000029's down migration, this one is completely reversible: the up
-- migration never deleted a row, it only changed `name` values, and every
-- prior value is known and recorded below rather than lost. Dropping the
-- CHECK constraints is trivially reversible. Restoring the 24 previously-
-- empty names is reversible because the prior value was exactly '' -- that
-- is the defect this migration fixed, so restoring it is just writing it
-- back. Restoring the three corrupted "Cloud Platforms" names is reversible
-- for the same reason: the shared prior value is recorded here.

-- ---------------------------------------------------------------------------
-- 1. Drop the CHECK constraints first -- restoring blank names below would
-- otherwise violate them.
-- ---------------------------------------------------------------------------

ALTER TABLE skills DROP CONSTRAINT IF EXISTS skills_name_not_blank;
ALTER TABLE specializations DROP CONSTRAINT IF EXISTS specializations_name_not_blank;
ALTER TABLE canonical_roles DROP CONSTRAINT IF EXISTS canonical_roles_name_not_blank;

-- ---------------------------------------------------------------------------
-- 2. Restore the three corrupted skill names to their shared prior value,
-- "Cloud Platforms". 'cloud-platforms' already read "Cloud Platforms" and
-- is left as is.
-- ---------------------------------------------------------------------------

UPDATE skills SET name = 'Cloud Platforms' WHERE slug = 'aws';
UPDATE skills SET name = 'Cloud Platforms' WHERE slug = 'gcp';
UPDATE skills SET name = 'Cloud Platforms' WHERE slug = 'azure';

-- ---------------------------------------------------------------------------
-- 3. Restore the 17 skills and 7 specializations that had name = '' before
-- this migration ran.
-- ---------------------------------------------------------------------------

UPDATE skills SET name = '' WHERE slug IN (
    'apis', 'ats-integration', 'ats-platforms', 'client-onboarding',
    'conversational-ai', 'data-integrity', 'deal-desk', 'dutch-language',
    'hris-integration', 'hr-tech-integration', 'hr-technology',
    'hubspot-administration', 'meeting-coordination', 'office-operations',
    'operational-excellence', 'saas-implementation', 'travel-coordination'
);

UPDATE specializations SET name = '' WHERE slug IN (
    'defense-national-security', 'digital-native-companies',
    'internal-controls', 'international-accounting', 'international-tax',
    'policy-research', 'social-impact'
);
