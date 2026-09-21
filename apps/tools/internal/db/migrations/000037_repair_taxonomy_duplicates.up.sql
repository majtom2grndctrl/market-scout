-- Repair duplicate and corrupted rows in the `skills` and `specializations`
-- taxonomy tables. `canonical_roles` is deliberately untouched: role dedup is
-- being planned separately, and folding a role has a second blast radius
-- (canonical_role_dimensions) this migration does not want to reason about.
--
-- Two distinct defects are entangled in the same symptom -- two rows carrying
-- the same `name` -- and they need opposite fixes.
--
--   1. DUPLICATE SLUGS. Two slugs name one concept: 'golang' and 'go',
--      'postgres' and 'postgresql', 'gcp' and 'google-cloud'. A grouping by
--      skill splits one population across two rows, so every count involving
--      either slug is wrong. The fix is a merge, on the 000025 model: repoint
--      the join rows, delete the retired row, record the retirement.
--
--   2. CORRUPTED NAMES. Two slugs name different concepts but one row's `name`
--      was overwritten with the other's: slug 'javascript' named "HTML/CSS",
--      'tableau' named "Data Visualization", 'embeddings' named "Large
--      Language Models", 'tensorflow' named "PyTorch or TensorFlow". These are
--      not duplicates. The row asserts something false, and merging it would
--      destroy a concept. The fix is a name repair, on the 000030 section 3
--      model: the slug stays, the link count stays, only `name` changes.
--
-- Telling them apart is a judgment call per row, and 000034 already settled
-- which signal to trust when they disagree: the slug. 000034 measured `name`
-- as unreliable (110 exact-name pairs in `skills`, 42.7% of them scoring below
-- 0.3 on slug similarity) and demoted it out of the similarity gate's
-- substitution branch for exactly that reason. This migration applies the same
-- rule by hand. Where slug and name disagree, the slug wins: 'system-design'
-- carrying the name "ML Systems Design" is a general system-design skill with
-- a borrowed label, not an ML one.
--
-- Scope of the merge set is deliberately narrow. A pair is merged only when the
-- two slugs are the same term written differently -- an orthographic variant
-- (hyphenation, spacing, plural, word order), an alias ('golang'/'go'), or the
-- same concept with a qualifier added or dropped ('airflow'/'apache-airflow').
-- Pairs whose slugs name genuinely different words are NOT merged even when
-- their names currently match, because merging two distinct concepts is
-- invisible afterward while leaving a duplicate is not. Left alone on that
-- rule, among others: 'enterprise-software'/'enterprise-saas',
-- 'payment-processing'/'payments-operations',
-- 'financial-services'/'financial-services-insurance',
-- 'data-collection-operations'/'data-operations', and
-- 'ai-development'/'ai-development-tools' -- each a plausible pair of distinct
-- domains. They get a name repair instead, which makes the distinction visible
-- so a human can merge them later if it turns out there isn't one.
--
-- 'cpp' ("C++"), 'csharp' ("C#") and 'c' ("C") survive as three rows. They
-- collapse together only under a normalizer that strips punctuation -- the same
-- similarity('C++','C#') = 1.000 hazard 000033 documented. They are three
-- languages.
--
-- Counts, measured against the development database before this migration:
--
--                              skills   specializations
--   rows                         2652               417
--   exact-name duplicate groups   106                16
--   merged away (rows deleted)     64                 7
--   names repaired                 76                11
--   exact-name groups after         0                 0
--
-- Join-row arithmetic: 588 job_posting_skills rows and 82
-- job_posting_specializations rows are repointed. Zero of them collide with a
-- pair the survivor already held, so the join tables keep 35231 and 12340 rows
-- and no classification loses a term. The INSERT ... ON CONFLICT DO NOTHING
-- below handles collisions anyway: the primary keys are (classification_id,
-- skill_id) and (classification_id, specialization_id), a classification that
-- referenced both the retired term and its survivor would collide, and a plain
-- UPDATE would abort the migration partway through.

-- ---------------------------------------------------------------------------
-- 1. The link archive.
--
-- 000031's retired_slugs records that a slug was retired and why. It does not
-- record which classifications pointed at it, and once the join rows are
-- repointed that fact is unrecoverable -- so the down migration could not put
-- them back. retired_slug_links is that record: one row per (retired slug x
-- classification) link this migration moves, plus one row with a NULL
-- classification_id for a retired slug that carried no links at all, so a
-- zero-link slug is still recoverable.
--
-- `collided` marks a link whose survivor pair already existed. The up
-- migration's INSERT was a no-op for those, so the down migration must not
-- delete them -- they predate this migration.
--
-- The table is dropped by the down migration: it holds nothing but this
-- migration's own undo record.
-- ---------------------------------------------------------------------------

CREATE TABLE retired_slug_links (
    id                   bigserial PRIMARY KEY,
    table_name           text        NOT NULL,
    retired_slug         text        NOT NULL,
    retired_id           bigint      NOT NULL,
    retired_name         text        NOT NULL,
    retired_created_at   timestamptz NOT NULL,
    survivor_slug        text        NOT NULL,
    classification_id    bigint,
    collided             boolean     NOT NULL DEFAULT false,
    retired_by_migration text        NOT NULL,
    CONSTRAINT retired_slug_links_table_name_check
        CHECK (table_name IN ('canonical_roles', 'specializations', 'skills'))
);

CREATE INDEX retired_slug_links_lookup_idx
    ON retired_slug_links (retired_by_migration, table_name, retired_slug);

-- ---------------------------------------------------------------------------
-- 2. The merge maps.
--
-- Held in temp tables so the archive write, the repoint, the delete and the
-- retired_slugs registration all read one list instead of four copies of it.
-- Each row is (retired slug, survivor slug, why they are the same term). The
-- survivor is the slug that reads as the concept's canonical spelling; where
-- both read equally well, the one already carrying the group's name.
-- Dropped explicitly at the end rather than ON COMMIT DROP, so the file does
-- not depend on how the runner wraps it.
-- ---------------------------------------------------------------------------

CREATE TEMP TABLE merge_map_skills (
    retired_slug  text PRIMARY KEY,
    survivor_slug text NOT NULL,
    variant       text NOT NULL
);

INSERT INTO merge_map_skills (retired_slug, survivor_slug, variant) VALUES
    ('ab-testing'                      , 'a-b-testing'                    , 'orthographic variant'),
    ('airflow'                         , 'apache-airflow'                 , 'qualifier variant'),
    ('flink'                           , 'apache-flink'                   , 'qualifier variant'),
    ('apis'                            , 'api'                            , 'orthographic variant'),
    ('ashby'                           , 'ashby-ats'                      , 'qualifier variant'),
    ('c-cpp'                           , 'cpp'                            , 'orthographic variant'),
    ('c-plus-plus'                     , 'cpp'                            , 'orthographic variant'),
    ('cicd-pipelines'                  , 'ci-cd-pipelines'                , 'orthographic variant'),
    ('cicd-systems'                    , 'ci-cd-systems'                  , 'orthographic variant'),
    ('cloud-formation'                 , 'cloudformation'                 , 'orthographic variant'),
    ('commercial-negotiation'          , 'commercial-negotiations'        , 'orthographic variant'),
    ('cuda-gpu'                        , 'cuda'                           , 'qualifier variant'),
    ('credit-modeling'                 , 'credit-risk-modeling'           , 'qualifier variant'),
    ('doe'                             , 'design-of-experiments'          , 'alias'),
    ('experimental-design'             , 'design-of-experiments'          , 'alias'),
    ('english'                         , 'english-language'               , 'qualifier variant'),
    ('fea'                             , 'finite-element-analysis'        , 'alias'),
    ('golang'                          , 'go'                             , 'alias'),
    ('ga4'                             , 'google-analytics'               , 'alias'),
    ('google-cloud'                    , 'gcp'                            , 'alias'),
    ('high-voltage'                    , 'high-voltage-systems'           , 'qualifier variant'),
    ('ip-law'                          , 'intellectual-property-law'      , 'alias'),
    ('issues-management'               , 'issue-management'               , 'orthographic variant'),
    ('korean-language'                 , 'korean-fluency'                 , 'alias'),
    ('leadership-communications'       , 'leadership-communication'       , 'orthographic variant'),
    ('linux-systems'                   , 'linux'                          , 'qualifier variant'),
    ('mandarin'                        , 'mandarin-chinese'               , 'qualifier variant'),
    ('materials-characterization'      , 'material-characterization'      , 'orthographic variant'),
    ('materials-selection'             , 'material-selection'             , 'orthographic variant'),
    ('sales-methodology-meddic'        , 'meddic-methodology'             , 'orthographic variant'),
    ('microsoft-azure'                 , 'azure'                          , 'qualifier variant'),
    ('next-js'                         , 'nextjs'                         , 'orthographic variant'),
    ('node'                            , 'nodejs'                         , 'orthographic variant'),
    ('node-js'                         , 'nodejs'                         , 'orthographic variant'),
    ('nosql'                           , 'nosql-databases'                , 'qualifier variant'),
    ('oscilloscopes'                   , 'oscilloscope'                   , 'orthographic variant'),
    ('org-design'                      , 'organizational-design'          , 'alias'),
    ('partner-strategy'                , 'partnership-strategy'           , 'orthographic variant'),
    ('plc'                             , 'plc-programming'                , 'qualifier variant'),
    ('postgres'                        , 'postgresql'                     , 'alias'),
    ('demo'                            , 'product-demo'                   , 'qualifier variant'),
    ('product-demos'                   , 'product-demo'                   , 'orthographic variant'),
    ('product-demonstration'           , 'product-demo'                   , 'alias'),
    ('product-demonstrations'          , 'product-demo'                   , 'alias'),
    ('pulsed-power'                    , 'pulsed-power-systems'           , 'qualifier variant'),
    ('rag-retrieval'                   , 'rag'                            , 'qualifier variant'),
    ('renewals-expansion'              , 'renewals-and-expansion'         , 'orthographic variant'),
    ('r-language'                      , 'r'                              , 'qualifier variant'),
    ('saas-administration'             , 'saas-platform-administration'   , 'qualifier variant'),
    ('sales-navigator'                 , 'linkedin-sales-navigator'       , 'qualifier variant'),
    ('sql-programming'                 , 'sql'                            , 'qualifier variant'),
    ('system-administration'           , 'systems-administration'         , 'orthographic variant'),
    ('system-integrations'             , 'systems-integration'            , 'orthographic variant'),
    ('system-integrator-expertise'     , 'systems-integrator-expertise'   , 'orthographic variant'),
    ('technical-drawings'              , 'technical-drawing'              , 'orthographic variant'),
    ('value-selling'                   , 'value-based-selling'            , 'orthographic variant'),
    ('solution-engineering'            , 'solutions-engineering'          , 'orthographic variant'),
    ('solution-architecture-and-design', 'solution-architecture'          , 'qualifier variant'),
    ('storage-architecture'            , 'storage-architecture-and-design', 'qualifier variant'),
    ('bd-strategy'                     , 'business-development-strategy'  , 'alias'),
    ('greenhouse-ats'                  , 'greenhouse'                     , 'qualifier variant'),
    ('complex-negotiation'             , 'complex-deal-negotiation'       , 'orthographic variant'),
    ('system-architecture'             , 'systems-architecture'           , 'orthographic variant'),
    ('system-design'                   , 'systems-design'                 , 'orthographic variant');
CREATE TEMP TABLE merge_map_specializations (
    retired_slug  text PRIMARY KEY,
    survivor_slug text NOT NULL,
    variant       text NOT NULL
);

INSERT INTO merge_map_specializations (retired_slug, survivor_slug, variant) VALUES
    ('agtech'                         , 'agricultural-technology'       , 'alias'),
    ('financial-planning-and-analysis', 'financial-planning-analysis'   , 'orthographic variant'),
    ('identity-access-management'     , 'identity-and-access-management', 'orthographic variant'),
    ('state-local-government'         , 'state-and-local-government'    , 'orthographic variant'),
    ('partnership-led-growth'         , 'partner-led-growth'            , 'orthographic variant'),
    ('supply-chain-security'          , 'software-supply-chain-security', 'qualifier variant'),
    ('systems-integrator-partnerships', 'system-integrator-partnerships', 'orthographic variant');
-- ---------------------------------------------------------------------------
-- 3. Guard against a database whose taxonomy contradicts the maps.
--
-- An absent slug is not an error. Nothing seeds these tables from a migration
-- -- every row in them was minted at runtime -- so a fresh database, and the
-- fixture-only test database, have an empty taxonomy and nothing to merge.
-- Those rows are simply skipped by the joins below; the retirements are still
-- registered, which is the point of the registry.
--
-- Two states are errors, because both produce a silent partial merge: a
-- retired slug present without its survivor, which would delete links rather
-- than move them; and a survivor that is itself retired elsewhere in the map,
-- which would leave links pointing at a row this migration then deletes.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    v_bad text;
BEGIN
    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_skills m
    WHERE EXISTS (SELECT 1 FROM skills x WHERE x.slug = m.retired_slug)
      AND NOT EXISTS (SELECT 1 FROM skills x WHERE x.slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'skills merge map: survivor missing for %', v_bad;
    END IF;

    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_skills m
    WHERE EXISTS (SELECT 1 FROM merge_map_skills x WHERE x.retired_slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'skills merge map: survivor is itself retired for %', v_bad;
    END IF;

    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_specializations m
    WHERE EXISTS (SELECT 1 FROM specializations x WHERE x.slug = m.retired_slug)
      AND NOT EXISTS (SELECT 1 FROM specializations x WHERE x.slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'specializations merge map: survivor missing for %', v_bad;
    END IF;

    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_specializations m
    WHERE EXISTS (SELECT 1 FROM merge_map_specializations x WHERE x.retired_slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'specializations merge map: survivor is itself retired for %', v_bad;
    END IF;
END
$$;

-- ---------------------------------------------------------------------------
-- 4. Archive every link about to move, before moving it.
--
-- The LEFT JOIN is load-bearing: it emits one row with a NULL
-- classification_id for a retired slug that carries no links, which is what
-- lets the down migration recreate that row too.
-- ---------------------------------------------------------------------------

INSERT INTO retired_slug_links (
    table_name, retired_slug, retired_id, retired_name, retired_created_at,
    survivor_slug, classification_id, collided, retired_by_migration)
SELECT 'skills', r.slug, r.id, r.name, r.created_at, s.slug, j.classification_id,
       j.classification_id IS NOT NULL AND EXISTS (
           SELECT 1 FROM job_posting_skills x
           WHERE x.classification_id = j.classification_id AND x.skill_id = s.id),
       '000037_repair_taxonomy_duplicates'
FROM merge_map_skills m
JOIN skills r ON r.slug = m.retired_slug
JOIN skills s ON s.slug = m.survivor_slug
LEFT JOIN job_posting_skills j ON j.skill_id = r.id;

INSERT INTO retired_slug_links (
    table_name, retired_slug, retired_id, retired_name, retired_created_at,
    survivor_slug, classification_id, collided, retired_by_migration)
SELECT 'specializations', r.slug, r.id, r.name, r.created_at, s.slug, j.classification_id,
       j.classification_id IS NOT NULL AND EXISTS (
           SELECT 1 FROM job_posting_specializations x
           WHERE x.classification_id = j.classification_id AND x.specialization_id = s.id),
       '000037_repair_taxonomy_duplicates'
FROM merge_map_specializations m
JOIN specializations r ON r.slug = m.retired_slug
JOIN specializations s ON s.slug = m.survivor_slug
LEFT JOIN job_posting_specializations j ON j.specialization_id = r.id;

-- ---------------------------------------------------------------------------
-- 5. Repoint the links, then drop the retired rows.
--
-- INSERT ... ON CONFLICT DO NOTHING rather than UPDATE: a classification that
-- referenced both the retired term and its survivor collides on the primary
-- key, and an UPDATE would abort. DISTINCT covers the other collision shape --
-- two retired slugs folding into one survivor on the same classification
-- ('node' and 'node-js' both into 'nodejs').
--
-- Order is load-bearing, same as 000025: the skill_id / specialization_id
-- foreign keys are ON DELETE RESTRICT, so every link must be off the retired
-- row before the row can go.
-- ---------------------------------------------------------------------------

INSERT INTO job_posting_skills (classification_id, skill_id)
SELECT DISTINCT j.classification_id, s.id
FROM job_posting_skills j
JOIN skills r ON r.id = j.skill_id
JOIN merge_map_skills m ON m.retired_slug = r.slug
JOIN skills s ON s.slug = m.survivor_slug
ON CONFLICT (classification_id, skill_id) DO NOTHING;

DELETE FROM job_posting_skills j
USING skills r, merge_map_skills m
WHERE r.id = j.skill_id AND m.retired_slug = r.slug;

DELETE FROM skills r
USING merge_map_skills m
WHERE m.retired_slug = r.slug;

INSERT INTO job_posting_specializations (classification_id, specialization_id)
SELECT DISTINCT j.classification_id, s.id
FROM job_posting_specializations j
JOIN specializations r ON r.id = j.specialization_id
JOIN merge_map_specializations m ON m.retired_slug = r.slug
JOIN specializations s ON s.slug = m.survivor_slug
ON CONFLICT (classification_id, specialization_id) DO NOTHING;

DELETE FROM job_posting_specializations j
USING specializations r, merge_map_specializations m
WHERE r.id = j.specialization_id AND m.retired_slug = r.slug;

DELETE FROM specializations r
USING merge_map_specializations m
WHERE m.retired_slug = r.slug;

-- ---------------------------------------------------------------------------
-- 6. Register the retirements in 000031's registry, so mcp.save_enrichment
-- refuses to mint them again.
--
-- table_name is scoped to the table the slug was retired from, not NULL. Every
-- 000031 seed row used NULL because its reason held in any table -- geography
-- is not taxonomy anywhere. These reasons do not: 'postgres' is a duplicate of
-- a *skill*, and says nothing about whether it could be a legitimate
-- specialization. This is the table-specific case 000031's nullable column was
-- left open for.
--
-- The reason text names the survivor, because the registry's error message is
-- handed back to the agent that tripped it. A blocked mint that says which
-- slug to use instead costs one round trip; one that only says no costs a
-- re-mint under a third spelling.
-- ---------------------------------------------------------------------------

INSERT INTO retired_slugs (slug, table_name, retired_by_migration, reason)
SELECT m.retired_slug, 'skills', '000037_repair_taxonomy_duplicates',
       format('Duplicate slug, merged into %L: the two slugs are one term (%s), so a grouping by skill split one population across two rows. Use %L.',
              m.survivor_slug, m.variant, m.survivor_slug)
FROM merge_map_skills m
ON CONFLICT (slug, table_name) DO NOTHING;

INSERT INTO retired_slugs (slug, table_name, retired_by_migration, reason)
SELECT m.retired_slug, 'specializations', '000037_repair_taxonomy_duplicates',
       format('Duplicate slug, merged into %L: the two slugs are one term (%s), so a grouping by specialization split one population across two rows. Use %L.',
              m.survivor_slug, m.variant, m.survivor_slug)
FROM merge_map_specializations m
ON CONFLICT (slug, table_name) DO NOTHING;

DROP TABLE merge_map_skills;
DROP TABLE merge_map_specializations;

-- ---------------------------------------------------------------------------
-- 7. Repair the corrupted names.
--
-- These rows keep their slug, their id and every link. Only `name` changes,
-- from a label borrowed off a different concept to one derived from the row's
-- own slug -- title-cased, with acronyms and product names capitalized as they
-- read elsewhere in the same table ('oauth' -> "OAuth", 'fastapi' ->
-- "FastAPI", 'javascript' -> "JavaScript"), following 000030.
--
-- Where a name group had no owner -- no row whose slug matched the shared name
-- -- one row keeps the established compound name and the rest are derived from
-- their slugs. 'sso-saml-oauth' keeps "SSO, SAML, OAuth Configuration" while
-- 'oauth' and 'authentication' become "OAuth" and "Authentication";
-- 'technical-writing' keeps "Technical Writing & Documentation" while
-- 'documentation' and 'user-documentation' get their own.
--
-- Two of these are judgment, not derivation, and are worth naming. 'analytical'
-- and 'analytics' both read "Analytics & Data-Driven Problem Solving" with 435
-- and 329 links; 'analytics' keeps the compound and 'analytical' becomes
-- "Analytical Thinking", which is what the bare adjective means as a skill.
-- 'investor-relations' read "Relationship Management" -- a borrowed label that
-- only surfaced because repairing 'relationship-management' collided with it.
-- ---------------------------------------------------------------------------

UPDATE skills SET name = 'Analytical Thinking'            WHERE slug = 'analytical';
UPDATE skills SET name = 'Architecture'                   WHERE slug = 'architecture';
UPDATE skills SET name = 'Authentication'                 WHERE slug = 'authentication';
UPDATE skills SET name = 'Balance Sheet Reconciliation'   WHERE slug = 'balance-sheet-reconciliation';
UPDATE skills SET name = 'Candidate Management'           WHERE slug = 'candidate-management';
UPDATE skills SET name = 'Coaching'                       WHERE slug = 'coaching';
UPDATE skills SET name = 'Communication'                  WHERE slug = 'communication';
UPDATE skills SET name = 'Complex Deal Negotiation'       WHERE slug = 'complex-deal-negotiation';
UPDATE skills SET name = 'Complex Sales Negotiations'     WHERE slug = 'complex-sales-negotiations';
UPDATE skills SET name = 'Complex Stakeholder Navigation' WHERE slug = 'complex-stakeholder-navigation';
UPDATE skills SET name = 'Corrosion Science'              WHERE slug = 'corrosion-science';
UPDATE skills SET name = 'Critical Thinking'              WHERE slug = 'critical-thinking';
UPDATE skills SET name = 'Customer Engagement'            WHERE slug = 'customer-engagement';
UPDATE skills SET name = 'Customer Solutions Engineering' WHERE slug = 'customer-solutions-engineering';
UPDATE skills SET name = 'Data Analysis'                  WHERE slug = 'data-analysis';
UPDATE skills SET name = 'Data-Driven Workflows'          WHERE slug = 'data-driven-workflows';
UPDATE skills SET name = 'Data Orchestration'             WHERE slug = 'data-orchestration';
UPDATE skills SET name = 'Design Thinking'                WHERE slug = 'design-thinking';
UPDATE skills SET name = 'Diagnostics'                    WHERE slug = 'diagnostics';
UPDATE skills SET name = 'Discovery'                      WHERE slug = 'discovery';
UPDATE skills SET name = 'Documentation'                  WHERE slug = 'documentation';
UPDATE skills SET name = 'Due Diligence'                  WHERE slug = 'due-diligence';
UPDATE skills SET name = 'Email Outreach'                 WHERE slug = 'email-outreach';
UPDATE skills SET name = 'Embeddings'                     WHERE slug = 'embeddings';
UPDATE skills SET name = 'FastAPI'                        WHERE slug = 'fastapi';
UPDATE skills SET name = 'Financial Operations'           WHERE slug = 'financial-operations';
UPDATE skills SET name = 'Hazard Analysis'                WHERE slug = 'hazard-analysis';
UPDATE skills SET name = 'HR Systems'                     WHERE slug = 'hr-systems';
UPDATE skills SET name = 'Industrial Controls'            WHERE slug = 'industrial-controls';
UPDATE skills SET name = 'Instrumentation'                WHERE slug = 'instrumentation';
UPDATE skills SET name = 'Investor Relations'             WHERE slug = 'investor-relations';
UPDATE skills SET name = 'JavaScript'                     WHERE slug = 'javascript';
UPDATE skills SET name = 'Launch Strategy'                WHERE slug = 'launch-strategy';
UPDATE skills SET name = 'Leadership'                     WHERE slug = 'leadership';
UPDATE skills SET name = 'Manufacturing Processes'        WHERE slug = 'manufacturing-processes';
UPDATE skills SET name = 'Mentoring'                      WHERE slug = 'mentoring';
UPDATE skills SET name = 'Metrics-Driven Decisions'       WHERE slug = 'metrics-driven-decisions';
UPDATE skills SET name = 'Multiphysics Modeling'          WHERE slug = 'multiphysics-modeling';
UPDATE skills SET name = 'Negotiation'                    WHERE slug = 'negotiation';
UPDATE skills SET name = 'Networking Basics'              WHERE slug = 'networking-basics';
UPDATE skills SET name = 'OAuth'                          WHERE slug = 'oauth';
UPDATE skills SET name = 'Onboarding'                     WHERE slug = 'onboarding';
UPDATE skills SET name = 'Optimization'                   WHERE slug = 'optimization';
UPDATE skills SET name = 'Performance Profiling'          WHERE slug = 'performance-profiling';
UPDATE skills SET name = 'Presentation'                   WHERE slug = 'presentation';
UPDATE skills SET name = 'Presentation Skills'            WHERE slug = 'presentation-skills';
UPDATE skills SET name = 'Process Control'                WHERE slug = 'process-control';
UPDATE skills SET name = 'Product Knowledge'              WHERE slug = 'product-knowledge';
UPDATE skills SET name = 'Product Messaging'              WHERE slug = 'product-messaging';
UPDATE skills SET name = 'Product Sensibility'            WHERE slug = 'product-sensibility';
UPDATE skills SET name = 'Product Strategy'               WHERE slug = 'product-strategy';
UPDATE skills SET name = 'PyTorch'                        WHERE slug = 'pytorch';
UPDATE skills SET name = 'Quality Assurance'              WHERE slug = 'quality-assurance';
UPDATE skills SET name = 'Recruitment'                    WHERE slug = 'recruitment';
UPDATE skills SET name = 'Relationship Management'        WHERE slug = 'relationship-management';
UPDATE skills SET name = 'Rotor Dynamics'                 WHERE slug = 'rotor-dynamics';
UPDATE skills SET name = 'Sales Leadership'               WHERE slug = 'sales-leadership';
UPDATE skills SET name = 'Sales Metrics'                  WHERE slug = 'sales-metrics';
UPDATE skills SET name = 'Sales Process Improvement'      WHERE slug = 'sales-process-improvement';
UPDATE skills SET name = 'Secure Code Review'             WHERE slug = 'secure-code-review';
UPDATE skills SET name = 'Solution Architecture & Design' WHERE slug = 'solution-architecture';
UPDATE skills SET name = 'Sourcing Tools Fluency'         WHERE slug = 'sourcing-tools-fluency';
UPDATE skills SET name = 'Systems Engineering'            WHERE slug = 'systems-engineering';
UPDATE skills SET name = 'Tableau'                        WHERE slug = 'tableau';
UPDATE skills SET name = 'Talent Strategy'                WHERE slug = 'talent-strategy';
UPDATE skills SET name = 'Team Leadership'                WHERE slug = 'team-leadership';
UPDATE skills SET name = 'Technical Account Management'   WHERE slug = 'technical-account-management';
UPDATE skills SET name = 'Technical Decision Making'      WHERE slug = 'technical-decision-making';
UPDATE skills SET name = 'Technical Planning'             WHERE slug = 'technical-planning';
UPDATE skills SET name = 'TensorFlow'                     WHERE slug = 'tensorflow';
UPDATE skills SET name = 'Terraform'                      WHERE slug = 'terraform';
UPDATE skills SET name = 'Testing'                        WHERE slug = 'testing';
UPDATE skills SET name = 'Troubleshooting'                WHERE slug = 'troubleshooting';
UPDATE skills SET name = 'UI Design'                      WHERE slug = 'ui-design';
UPDATE skills SET name = 'User Documentation'             WHERE slug = 'user-documentation';
UPDATE skills SET name = 'Validation'                     WHERE slug = 'validation';

UPDATE specializations SET name = 'AI Development'        WHERE slug = 'ai-development';
UPDATE specializations SET name = 'Behavioral Science'    WHERE slug = 'behavioral-science';
UPDATE specializations SET name = 'Business Automation'   WHERE slug = 'business-automation';
UPDATE specializations SET name = 'Data Operations'       WHERE slug = 'data-operations';
UPDATE specializations SET name = 'Data Products'         WHERE slug = 'data-products';
UPDATE specializations SET name = 'Enterprise SaaS'       WHERE slug = 'enterprise-saas';
UPDATE specializations SET name = 'Financial Services'    WHERE slug = 'financial-services';
UPDATE specializations SET name = 'Industrial Automation' WHERE slug = 'industrial-automation';
UPDATE specializations SET name = 'Payments Operations'   WHERE slug = 'payments-operations';
UPDATE specializations SET name = 'Performance Marketing' WHERE slug = 'performance-marketing';
UPDATE specializations SET name = 'Trust & Verification'  WHERE slug = 'trust-and-verification';
