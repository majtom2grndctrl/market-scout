-- Fix three defects in the `name` column across the taxonomy tables
-- (canonical_roles, specializations, skills), then add a CHECK that
-- prevents the first defect from recurring.
--
-- Data must be fixed before the CHECK is added: an ALTER TABLE ... ADD
-- CONSTRAINT validates every existing row, and it fails outright on the
-- 24 rows below that currently have name = ''.
--
-- canonical_roles has no rows affected by either defect as of this writing;
-- it is included in the CHECK for parity because all three tables share the
-- same (slug, name) shape and mcp.save_enrichment can mint rows in any of
-- them.

-- ---------------------------------------------------------------------------
-- 1. Empty names on 17 skills, all minted 2026-09-13, name = '' throughout.
--
-- The columns are NOT NULL but nothing ever enforced non-empty, and
-- mcp.save_enrichment does not validate the name it is given. Names below
-- are derived by title-casing the kebab-case slug, with acronyms (ATS, AI,
-- HR, HRIS, SaaS, API) capitalized as they read elsewhere in this same
-- table (see 'api' -> 'API', 'hris' -> 'HRIS', 'ats-management' -> 'ATS
-- Platform Management ...), and 'HubSpot' kept as the product's own
-- capitalization, matching the existing 'hubspot' -> 'HubSpot' row.
-- ---------------------------------------------------------------------------

UPDATE skills SET name = 'APIs'                   WHERE slug = 'apis';
UPDATE skills SET name = 'ATS Integration'        WHERE slug = 'ats-integration';
UPDATE skills SET name = 'ATS Platforms'          WHERE slug = 'ats-platforms';
UPDATE skills SET name = 'Client Onboarding'      WHERE slug = 'client-onboarding';
UPDATE skills SET name = 'Conversational AI'      WHERE slug = 'conversational-ai';
UPDATE skills SET name = 'Data Integrity'         WHERE slug = 'data-integrity';
UPDATE skills SET name = 'Deal Desk'              WHERE slug = 'deal-desk';
UPDATE skills SET name = 'Dutch Language'         WHERE slug = 'dutch-language';
UPDATE skills SET name = 'HRIS Integration'       WHERE slug = 'hris-integration';
UPDATE skills SET name = 'HR Tech Integration'    WHERE slug = 'hr-tech-integration';
UPDATE skills SET name = 'HR Technology'          WHERE slug = 'hr-technology';
UPDATE skills SET name = 'HubSpot Administration' WHERE slug = 'hubspot-administration';
UPDATE skills SET name = 'Meeting Coordination'   WHERE slug = 'meeting-coordination';
UPDATE skills SET name = 'Office Operations'      WHERE slug = 'office-operations';
UPDATE skills SET name = 'Operational Excellence' WHERE slug = 'operational-excellence';
UPDATE skills SET name = 'SaaS Implementation'    WHERE slug = 'saas-implementation';
UPDATE skills SET name = 'Travel Coordination'    WHERE slug = 'travel-coordination';

-- ---------------------------------------------------------------------------
-- 2. Empty names on 7 specializations, also minted 2026-09-13, name = ''.
--
-- The task that requested this migration cited "17 skills and 1
-- specialization." A direct query found 7 empty specializations, not 1;
-- the set below is what actually exists, confirmed by query rather than
-- taken from that count. Same title-casing approach as above; no acronyms
-- among these slugs. 'digital-native-companies' keeps the hyphen in
-- "Digital-Native" as a single compound modifier, matching how the term is
-- normally written.
-- ---------------------------------------------------------------------------

UPDATE specializations SET name = 'Defense & National Security'  WHERE slug = 'defense-national-security';
UPDATE specializations SET name = 'Digital-Native Companies'     WHERE slug = 'digital-native-companies';
UPDATE specializations SET name = 'Internal Controls'            WHERE slug = 'internal-controls';
UPDATE specializations SET name = 'International Accounting'    WHERE slug = 'international-accounting';
UPDATE specializations SET name = 'International Tax'            WHERE slug = 'international-tax';
UPDATE specializations SET name = 'Policy Research'               WHERE slug = 'policy-research';
UPDATE specializations SET name = 'Social Impact'                 WHERE slug = 'social-impact';

-- ---------------------------------------------------------------------------
-- 3. Four distinct skill slugs sharing the corrupted name "Cloud Platforms".
--
-- 'cloud-platforms' (315 uses), 'aws' (192), 'gcp' (86), 'azure' (62) are
-- four legitimately different slugs -- a general cloud-platforms skill and
-- three specific vendors -- with a broken name column that collapsed them
-- to one label. This is a name-only fix: none of the four slugs are merged
-- or deleted, and their link counts are untouched.
--
-- 'cloud-platforms' already reads "Cloud Platforms," so its statement is a
-- no-op re-assertion, kept here for symmetry with the other three and so a
-- future reader sees all four decided together.
-- ---------------------------------------------------------------------------

UPDATE skills SET name = 'Cloud Platforms'          WHERE slug = 'cloud-platforms';
UPDATE skills SET name = 'AWS'                       WHERE slug = 'aws';
UPDATE skills SET name = 'Google Cloud Platform'     WHERE slug = 'gcp';
UPDATE skills SET name = 'Microsoft Azure'           WHERE slug = 'azure';

-- ---------------------------------------------------------------------------
-- 4. Prevent recurrence: no blank (or whitespace-only) name in any of the
-- three taxonomy tables. Added last, now that no existing row violates it.
-- ---------------------------------------------------------------------------

ALTER TABLE canonical_roles
    ADD CONSTRAINT canonical_roles_name_not_blank CHECK (btrim(name) <> '');

ALTER TABLE specializations
    ADD CONSTRAINT specializations_name_not_blank CHECK (btrim(name) <> '');

ALTER TABLE skills
    ADD CONSTRAINT skills_name_not_blank CHECK (btrim(name) <> '');
