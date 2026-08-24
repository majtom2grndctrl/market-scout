-- Restore the pre-000020 taxonomy before dropping the market derivation it
-- depends on. The existing four-column read-model contract is unchanged.
CREATE OR REPLACE VIEW open_posting_taxonomy AS
SELECT
    open_postings_display.job_posting_id,
    'role'::text AS term_kind,
    canonical_roles.slug,
    canonical_roles.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_roles ON canonical_roles.id = job_posting_roles.role_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'specialization'::text AS term_kind,
    specializations.slug,
    specializations.name
FROM open_postings_display
JOIN job_posting_specializations
    ON job_posting_specializations.classification_id = open_postings_display.classification_id
JOIN specializations ON specializations.id = job_posting_specializations.specialization_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'skill'::text AS term_kind,
    skills.slug,
    skills.name
FROM open_postings_display
JOIN job_posting_skills
    ON job_posting_skills.classification_id = open_postings_display.classification_id
JOIN skills ON skills.id = job_posting_skills.skill_id

UNION ALL

SELECT DISTINCT
    open_postings_display.job_posting_id,
    'dimension'::text AS term_kind,
    role_dimensions.slug,
    role_dimensions.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_role_dimensions
    ON canonical_role_dimensions.canonical_role_id = job_posting_roles.role_id
JOIN role_dimensions ON role_dimensions.id = canonical_role_dimensions.dimension_id;

DROP VIEW open_posting_markets;
DROP TABLE market_seeds;
