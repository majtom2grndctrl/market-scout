-- Remove taxonomy minted by the 2026-09-16-0843 batch-enrich run that violates a
-- stated rule of the classifier contract. Every slug here was created by that run
-- and carries one or two links, so the blast radius is a handful of tags.
--
-- Scope is deliberately narrow. Slugs that merely read as broad but predate the run
-- are NOT touched: 'technical-leadership' (253 links) and 'process-optimization'
-- (8 links) are established vocabulary, and one agent misreported them as newly
-- minted. Retiring established terms is a separate decision about the whole
-- taxonomy, not cleanup after one run.
--
-- The near-duplicate clusters that run exposed -- iam/iam-systems/identity-management,
-- soc-2/soc2-compliance/soc-2-compliance, the campaign and seo families -- are also
-- out of scope. Those need a merge with a chosen canonical form, and most of the
-- members predate this run, so they touch historical classifications this run never
-- wrote. Follow 000025's pattern when that decision is made.
--
-- Order is load-bearing, exactly as in 000025: job_posting_skills_skill_id_fkey and
-- job_posting_specializations_specialization_id_fkey are ON DELETE RESTRICT, so the
-- link rows must go before the taxonomy rows. The classifications themselves survive;
-- they simply lose these tags.

-- ---------------------------------------------------------------------------
-- 1. Geography is not taxonomy.
--
-- project.md: geography enters the analysis surface as a curated `market` dimension,
-- never as raw location text or a taxonomy term. 'japan-market' (posting 41684) and
-- 'south-korea-market' (posting 41767) create a second, uncurated geography axis the
-- read model does not know about, so a market grouping and a specialization grouping
-- would answer the same question differently. Both postings also encode region in the
-- title, which the role-slug rules already say to strip; the market dimension recovers
-- the geography from posting location.
-- ---------------------------------------------------------------------------

DELETE FROM job_posting_specializations
WHERE specialization_id IN (
    SELECT id FROM specializations WHERE slug IN ('japan-market', 'south-korea-market')
);

DELETE FROM specializations WHERE slug IN ('japan-market', 'south-korea-market');

-- ---------------------------------------------------------------------------
-- 2. 'emerging-technologies' names no domain, industry, or product area.
--
-- Minted as a specialization on posting 41857. The specializations axis is domain;
-- a term that means "things that are new" cannot be grouped on and does not narrow
-- anything. Dropped rather than re-homed: there is no specific concept to preserve.
-- ---------------------------------------------------------------------------

DELETE FROM job_posting_specializations
WHERE specialization_id IN (
    SELECT id FROM specializations WHERE slug = 'emerging-technologies'
);

DELETE FROM specializations WHERE slug = 'emerging-technologies';

-- ---------------------------------------------------------------------------
-- 3. Factually wrong slugs -- the concept named is not the concept in the source.
--
-- 'vlm-inference'  (posting 40756): the description says vLLM, an inference server.
--                  VLM is a vision-language model. Different thing entirely.
-- 'geo-targeting'  (posting 42903): minted from an "SEO/GEO Lead" title, read as
--                  geographic targeting. In a search-marketing title GEO is
--                  Generative Engine Optimization.
--
-- These are not style problems. Left in place they are wrong data that future agents
-- would near-match against and legitimately reuse.
-- ---------------------------------------------------------------------------

DELETE FROM job_posting_skills
WHERE skill_id IN (SELECT id FROM skills WHERE slug IN ('vlm-inference', 'geo-targeting'));

DELETE FROM skills WHERE slug IN ('vlm-inference', 'geo-targeting');

-- ---------------------------------------------------------------------------
-- 4. Bundled slugs -- more than one concept in a single term.
--
-- 'aws-gcp-azure'        (posting 40756): three cloud platforms in one slug. Each
--                        already exists individually; a posting naming all three
--                        should carry three tags, and one naming fewer should not be
--                        credited with the rest.
-- 'icd-snomed-standards' (posting 40736): two terminology standards in one slug, and
--                        overlapping the 'healthcare-terminology-standards'
--                        specialization the same agent minted.
--
-- A bundled slug cannot be filtered or counted honestly: it is simultaneously true
-- and false for most postings that carry it.
-- ---------------------------------------------------------------------------

DELETE FROM job_posting_skills
WHERE skill_id IN (SELECT id FROM skills WHERE slug IN ('aws-gcp-azure', 'icd-snomed-standards'));

DELETE FROM skills WHERE slug IN ('aws-gcp-azure', 'icd-snomed-standards');

-- ---------------------------------------------------------------------------
-- 5. Umbrella skills minted by this run.
--
-- 'technical-expertise' (43107), 'technical-acumen' (41857), 'systems-knowledge'
-- (41794, 41907), 'operations-management' (43015), 'infrastructure' (41817),
-- 'cross-functional-alignment' (41716), 'compliance' (40607).
--
-- Each names a competency so broad it cannot discriminate between postings, which is
-- the only thing a skill tag is for. 'compliance' additionally sits beside the
-- pre-existing 'soc-2' and 'iso-27001' and the 'compliance-engineer' role, where it
-- adds no signal any of them lack.
--
-- This section is the most judgment-dependent in the file. If the intent is to keep
-- broad competencies as a deliberate tier, drop this statement and keep the rest --
-- sections 1 through 4 are rule violations, this one is a taste call.
-- ---------------------------------------------------------------------------

DELETE FROM job_posting_skills
WHERE skill_id IN (
    SELECT id FROM skills WHERE slug IN (
        'technical-expertise', 'technical-acumen', 'systems-knowledge',
        'operations-management', 'infrastructure', 'cross-functional-alignment',
        'compliance'
    )
);

DELETE FROM skills WHERE slug IN (
    'technical-expertise', 'technical-acumen', 'systems-knowledge',
    'operations-management', 'infrastructure', 'cross-functional-alignment',
    'compliance'
);
