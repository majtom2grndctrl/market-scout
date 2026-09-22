-- Bring `canonical_roles` under the name-collision gate, and widen the gate's
-- normalizer to catch internal-spacing variants.
--
-- 000038 blocks a mint whose normalized name already exists in its target
-- table. Two gaps were left open on purpose and are closed here.
--
-- GAP 1 -- ROLES WERE EXCLUDED. 000038's rule was affordable for
-- `specializations` and `skills` only because 000037 had already brought their
-- normalized-name collisions to zero. `canonical_roles` was excluded, in
-- 000038's words, because "a rule that fires against existing corruption is a
-- rule workers learn to route around". 000037 had skipped roles because
-- folding a role has a second blast radius, `canonical_role_dimensions`, that
-- it did not want to reason about. Sections 1-4 below do that reasoning. There
-- are exactly three collision clusters, six rows; they are merged on 000037's
-- model, and section 6 drops the exclusion.
--
-- GAP 2 -- THE NORMALIZER MISSED INTERNAL SPACING. The gate compared
-- lower(btrim(regexp_replace(name, '\s+', ' ', 'g'))) -- case and whitespace
-- runs only. That is correctly conservative about punctuation and must stay
-- that way: a punctuation-stripping normalizer folds 'c' "C", 'cpp' "C++" and
-- 'csharp' "C#" into one term, which is the similarity('C++','C#') = 1.000
-- hazard 000033 documented and 000037 made a point of surviving. But it also
-- misses the case where the only difference is whether a space is there at
-- all, and two of those were live in `skills`: 'power-bi' "Power BI" against
-- 'powerbi' "PowerBI", and 'soc-2-compliance' "SOC 2 Compliance" against
-- 'soc2-compliance' "SOC2 Compliance". Section 6 compares
-- lower(regexp_replace(name, '\s', '', 'g')) instead -- all whitespace
-- removed, punctuation untouched -- and section 4 merges the two pairs it
-- surfaces.
--
-- The wider key is strictly broader than the old one, not merely different:
-- two names equal under collapse-and-trim are still equal with every space
-- removed. So nothing the old gate blocked becomes mintable.
--
-- PRE-FLIGHT, run against the development database before this migration and
-- the reason it is being shipped rather than reported as unsafe. Across all
-- three tables, the whitespace-removing key finds exactly two pairs the
-- collapsing key does not:
--
--   skills  powerbi          "PowerBI"          ~ power-bi         "Power BI"
--   skills  soc2-compliance  "SOC2 Compliance"  ~ soc-2-compliance "SOC 2 Compliance"
--
-- Both are genuine duplicates of one term. Zero false positives: no pair of
-- distinct concepts merely concatenates alike. 'c' / 'cpp' / 'csharp' keep
-- three distinct keys ("c", "c++", "c#"), as does every other punctuation-
-- bearing name in the vocabulary. Had the check produced even one false
-- positive the finding would have been reported and the change dropped --
-- blocking beats guessing is the entire premise of 000038's gate, and a gate
-- that blocks a legitimate distinct concept teaches workers to route around it
-- exactly as a gate that fires on existing corruption does.
--
-- WHY MERGE AND NOT RENAME, for the three role clusters. 000037 had two tools:
-- merge a pair of slugs that are one term written twice, and repair a name
-- borrowed off a different concept. Each cluster here is judged the same way
-- 000037 judged its sixty-odd, and all three come out as merges:
--
--   'business-operations' / 'business-operations-manager', both "Business
--   Operations Manager". The qualifier-added variant of 000037's
--   'airflow'/'apache-airflow'. The populations settle it: both slugs carry
--   the same mix of titles -- "Head of Business Operations" under one,
--   "Director, Business Operations" under the other, plus specialists and
--   analysts under both. One concept, two spellings.
--
--   'sales-development-rep' / 'sales-development-representative', both "Sales
--   Development Representative". An abbreviation alias, 000037's
--   'org-design'/'organizational-design'.
--
--   'solutions-architect-director' / 'solutions-architecture-director', both
--   "Solutions Architecture Director". A word-form variant, 000037's
--   'system-design'/'systems-design'.
--
-- SURVIVOR CHOICE follows 000037's rule -- the slug that reads as the
-- concept's canonical spelling -- not link count. 'sales-development-rep'
-- carries 71 links against its survivor's 41 and is retired anyway, because
-- the links move and the abbreviation is the spelling that keeps coming back.
--
-- ONE NAME REPAIR, and it is worth naming because it is judgment rather than
-- derivation. The survivor 'business-operations' keeps 26 links whose titles
-- are mostly not managers -- analysts, specialists, leads, directors, one COO
-- -- so the inherited name "Business Operations Manager" asserts something
-- false about most of its own population. 000037's rule for a slug and a name
-- that disagree is that the slug wins, so the survivor is renamed "Business
-- Operations". No other role carries that name, so the repair creates no new
-- collision. The other two clusters keep their names unchanged: there the
-- shared name is correct for both spellings.
--
-- ROLE DIMENSIONS, the blast radius 000037 declined. `canonical_role_dimensions`
-- is a set of dimension tags per role, and the two rows of a cluster disagree
-- about it -- 'business-operations' carries five tags and
-- 'business-operations-manager' four, a subset. The merge takes the UNION, on
-- the same reasoning as the join rows: a dimension tag is an assertion that
-- this role belongs to that dimension, and dropping one loses information
-- while keeping one that only the retired row carried loses nothing. Every
-- retired row's tags are archived first, so the down migration restores them
-- exactly.
--
-- THE REPAIRS ARE HISTORICAL; THE GATE CHANGE IS NOT. Sections 1-5 describe
-- five rows that exist in one database, the one this was written against. On a
-- fresh or empty database they match nothing, and that is a successful no-op,
-- not a skipped step: every guard below asks whether a slug is present before
-- treating its absence as a problem, every archive and repoint is a join that
-- yields no rows, and the two assertions compare zero against zero. Only the
-- retirement registry in section 5 writes unconditionally, deliberately and on
-- 000037's precedent -- a registry entry saying 'sales-development-rep' is a
-- duplicate spelling of 'sales-development-representative' is worth having
-- before the duplicate is minted, not only after. Section 6 is the part that
-- carries forward to every database: it changes what the gate does, not what
-- any particular row contains.
--
-- WHY NOT A UNIQUE INDEX on the normalized name, now that all three tables are
-- clean: unchanged from 000038. An index cannot tell minting from reuse,
-- surfaces as an unstructured constraint violation instead of a structured
-- error naming the collider, and forecloses the rare collision a human decides
-- is legitimate.
--
-- Measured against the development database before this migration. A database
-- that was never enriched reads zero on every line, which is the no-op case:
--
--                                  canonical_roles   skills
--   normalized-name collision clusters (old key)   3        0
--   additional clusters (new key)                  0        2
--   rows merged away                               3        2
--   join rows repointed                            84       6
--   join-row collisions with the survivor          0        0
--   role dimension tags moved to the survivor      2        -
--   collision clusters after, under the new key    0        0
--
-- Section 7 asserts the last line rather than trusting it, and section 4
-- asserts that no classification lost a term.

-- ---------------------------------------------------------------------------
-- 1. The merge map and the dimension archive.
--
-- The link archive is 000037's `retired_slug_links`, reused rather than
-- reinvented: its `retired_by_migration` column and its index exist so more
-- than one migration can write to it, and its table_name CHECK already admits
-- 'canonical_roles'. One caveat, recorded because it is not obvious:
-- 000037's down migration drops that table outright. A full `migrate down`
-- runs this file's down first and is unaffected, but 000037's down run
-- directly against a database still carrying 000039/000040 would take this
-- migration's undo record with it.
--
-- Role dimensions need their own archive; `retired_slug_links` has no column
-- for them and bending one of its columns into a second meaning would make
-- both harder to read. This table is created and dropped by this migration.
-- ---------------------------------------------------------------------------

CREATE TABLE retired_role_dimensions (
    id                   bigserial PRIMARY KEY,
    retired_slug         text   NOT NULL,
    retired_id           bigint NOT NULL,
    dimension_slug       text   NOT NULL,
    -- true when the survivor already carried this dimension, so the up
    -- migration's INSERT was a no-op and the down migration must not remove it.
    collided             boolean NOT NULL,
    retired_by_migration text   NOT NULL
);

CREATE TEMP TABLE merge_map_roles (
    retired_slug  text PRIMARY KEY,
    survivor_slug text NOT NULL,
    variant       text NOT NULL
);

INSERT INTO merge_map_roles (retired_slug, survivor_slug, variant) VALUES
    ('business-operations-manager' , 'business-operations'            , 'qualifier variant'),
    ('sales-development-rep'       , 'sales-development-representative', 'alias'),
    ('solutions-architect-director', 'solutions-architecture-director' , 'orthographic variant');

CREATE TEMP TABLE merge_map_skills_spacing (
    retired_slug  text PRIMARY KEY,
    survivor_slug text NOT NULL,
    variant       text NOT NULL
);

INSERT INTO merge_map_skills_spacing (retired_slug, survivor_slug, variant) VALUES
    ('powerbi'        , 'power-bi'        , 'spacing variant'),
    ('soc2-compliance', 'soc-2-compliance', 'spacing variant');

-- ---------------------------------------------------------------------------
-- 2. Guard against a database whose taxonomy contradicts the maps.
--
-- 000037's guards, verbatim in intent. An absent slug is not an error: nothing
-- seeds these tables from a migration, so a fresh database and the fixture-only
-- test database have an empty taxonomy and nothing to merge. Two states are
-- errors because both produce a silent partial merge -- a retired slug present
-- without its survivor, and a survivor that is itself retired elsewhere in the
-- map.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    v_bad text;
BEGIN
    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_roles m
    WHERE EXISTS (SELECT 1 FROM canonical_roles x WHERE x.slug = m.retired_slug)
      AND NOT EXISTS (SELECT 1 FROM canonical_roles x WHERE x.slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'canonical_roles merge map: survivor missing for %', v_bad;
    END IF;

    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_roles m
    WHERE EXISTS (SELECT 1 FROM merge_map_roles x WHERE x.retired_slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'canonical_roles merge map: survivor is itself retired for %', v_bad;
    END IF;

    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_skills_spacing m
    WHERE EXISTS (SELECT 1 FROM skills x WHERE x.slug = m.retired_slug)
      AND NOT EXISTS (SELECT 1 FROM skills x WHERE x.slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'skills merge map: survivor missing for %', v_bad;
    END IF;

    SELECT string_agg(retired_slug, ', ') INTO v_bad
    FROM merge_map_skills_spacing m
    WHERE EXISTS (SELECT 1 FROM merge_map_skills_spacing x WHERE x.retired_slug = m.survivor_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'skills merge map: survivor is itself retired for %', v_bad;
    END IF;
END
$$;

-- ---------------------------------------------------------------------------
-- 3. Archive every link and every dimension tag about to move, before moving
-- it.
--
-- The LEFT JOIN is load-bearing, exactly as in 000037 section 4: it emits one
-- row with a NULL classification_id for a retired slug that carries no links,
-- which is what lets the down migration recreate that row too.
-- ---------------------------------------------------------------------------

INSERT INTO retired_slug_links (
    table_name, retired_slug, retired_id, retired_name, retired_created_at,
    survivor_slug, classification_id, collided, retired_by_migration)
SELECT 'canonical_roles', r.slug, r.id, r.name, r.created_at, s.slug, j.classification_id,
       j.classification_id IS NOT NULL AND EXISTS (
           SELECT 1 FROM job_posting_roles x
           WHERE x.classification_id = j.classification_id AND x.role_id = s.id),
       '000040_roles_name_gate_and_spacing'
FROM merge_map_roles m
JOIN canonical_roles r ON r.slug = m.retired_slug
JOIN canonical_roles s ON s.slug = m.survivor_slug
LEFT JOIN job_posting_roles j ON j.role_id = r.id;

INSERT INTO retired_slug_links (
    table_name, retired_slug, retired_id, retired_name, retired_created_at,
    survivor_slug, classification_id, collided, retired_by_migration)
SELECT 'skills', r.slug, r.id, r.name, r.created_at, s.slug, j.classification_id,
       j.classification_id IS NOT NULL AND EXISTS (
           SELECT 1 FROM job_posting_skills x
           WHERE x.classification_id = j.classification_id AND x.skill_id = s.id),
       '000040_roles_name_gate_and_spacing'
FROM merge_map_skills_spacing m
JOIN skills r ON r.slug = m.retired_slug
JOIN skills s ON s.slug = m.survivor_slug
LEFT JOIN job_posting_skills j ON j.skill_id = r.id;

INSERT INTO retired_role_dimensions (
    retired_slug, retired_id, dimension_slug, collided, retired_by_migration)
SELECT r.slug, r.id, d.slug,
       EXISTS (SELECT 1 FROM canonical_role_dimensions x
               WHERE x.canonical_role_id = s.id AND x.dimension_id = d.id),
       '000040_roles_name_gate_and_spacing'
FROM merge_map_roles m
JOIN canonical_roles r ON r.slug = m.retired_slug
JOIN canonical_roles s ON s.slug = m.survivor_slug
JOIN canonical_role_dimensions crd ON crd.canonical_role_id = r.id
JOIN role_dimensions d ON d.id = crd.dimension_id;

-- ---------------------------------------------------------------------------
-- 4. Repoint the links and the dimension tags, then drop the retired rows.
--
-- INSERT ... ON CONFLICT DO NOTHING rather than UPDATE, for 000037's reason: a
-- classification that referenced both the retired role and its survivor
-- collides on the primary key and an UPDATE would abort the migration partway
-- through. Measured beforehand, none of the 84 role links and none of the 6
-- skill links collides -- but the shape has to survive a database where one
-- does.
--
-- Order is load-bearing, same as 000025 and 000037: the foreign keys are
-- ON DELETE RESTRICT, so every link must be off the retired row before the row
-- can go.
--
-- The dimension union is the same INSERT ... ON CONFLICT: a tag the survivor
-- already carries is left alone, a tag only the retired row carried moves
-- across.
--
-- The DO block at the end is the proof that nothing was lost. It counts the
-- distinct (classification, role) pairs the merge map touches before and after
-- the repoint, in terms of the surviving slug, and raises if a classification
-- came out the other side with fewer role terms than it went in with.
-- ---------------------------------------------------------------------------

-- Every classification that currently holds a retired term or its survivor,
-- counted once per classification: the number of classifications that must
-- still hold the survivor once the merge is done.
CREATE TEMP TABLE merge_preflight AS
SELECT 'roles'::text AS kind, count(DISTINCT j.classification_id) AS n
FROM job_posting_roles j
JOIN canonical_roles r ON r.id = j.role_id
WHERE EXISTS (SELECT 1 FROM merge_map_roles m
              WHERE m.retired_slug = r.slug OR m.survivor_slug = r.slug)
UNION ALL
SELECT 'skills', count(DISTINCT j.classification_id)
FROM job_posting_skills j
JOIN skills k ON k.id = j.skill_id
WHERE EXISTS (SELECT 1 FROM merge_map_skills_spacing m
              WHERE m.retired_slug = k.slug OR m.survivor_slug = k.slug);

INSERT INTO job_posting_roles (classification_id, role_id)
SELECT DISTINCT j.classification_id, s.id
FROM job_posting_roles j
JOIN canonical_roles r ON r.id = j.role_id
JOIN merge_map_roles m ON m.retired_slug = r.slug
JOIN canonical_roles s ON s.slug = m.survivor_slug
ON CONFLICT (classification_id, role_id) DO NOTHING;

INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
SELECT DISTINCT s.id, crd.dimension_id
FROM canonical_role_dimensions crd
JOIN canonical_roles r ON r.id = crd.canonical_role_id
JOIN merge_map_roles m ON m.retired_slug = r.slug
JOIN canonical_roles s ON s.slug = m.survivor_slug
ON CONFLICT DO NOTHING;

DELETE FROM canonical_role_dimensions crd
USING canonical_roles r, merge_map_roles m
WHERE r.id = crd.canonical_role_id AND m.retired_slug = r.slug;

DELETE FROM job_posting_roles j
USING canonical_roles r, merge_map_roles m
WHERE r.id = j.role_id AND m.retired_slug = r.slug;

DELETE FROM canonical_roles r
USING merge_map_roles m
WHERE m.retired_slug = r.slug;

INSERT INTO job_posting_skills (classification_id, skill_id)
SELECT DISTINCT j.classification_id, s.id
FROM job_posting_skills j
JOIN skills k ON k.id = j.skill_id
JOIN merge_map_skills_spacing m ON m.retired_slug = k.slug
JOIN skills s ON s.slug = m.survivor_slug
ON CONFLICT (classification_id, skill_id) DO NOTHING;

DELETE FROM job_posting_skills j
USING skills k, merge_map_skills_spacing m
WHERE k.id = j.skill_id AND m.retired_slug = k.slug;

DELETE FROM skills k
USING merge_map_skills_spacing m
WHERE m.retired_slug = k.slug;

DO $$
DECLARE
    v_before bigint;
    v_after  bigint;
BEGIN
    SELECT n INTO v_before FROM merge_preflight WHERE kind = 'roles';
    SELECT count(DISTINCT j.classification_id) INTO v_after
    FROM job_posting_roles j
    JOIN canonical_roles r ON r.id = j.role_id
    JOIN merge_map_roles m ON m.survivor_slug = r.slug;
    IF v_after <> v_before THEN
        RAISE EXCEPTION
            'canonical_roles merge lost a classification: % held a merged role before, % hold the survivor after',
            v_before, v_after;
    END IF;

    SELECT n INTO v_before FROM merge_preflight WHERE kind = 'skills';
    SELECT count(DISTINCT j.classification_id) INTO v_after
    FROM job_posting_skills j
    JOIN skills k ON k.id = j.skill_id
    JOIN merge_map_skills_spacing m ON m.survivor_slug = k.slug;
    IF v_after <> v_before THEN
        RAISE EXCEPTION
            'skills merge lost a classification: % held a merged skill before, % hold the survivor after',
            v_before, v_after;
    END IF;

    -- Every archived dimension tag must now be on the survivor.
    IF EXISTS (
        SELECT 1
        FROM retired_role_dimensions a
        JOIN retired_slug_links l
          ON l.retired_by_migration = a.retired_by_migration
         AND l.table_name = 'canonical_roles'
         AND l.retired_slug = a.retired_slug
        JOIN canonical_roles s ON s.slug = l.survivor_slug
        JOIN role_dimensions d ON d.slug = a.dimension_slug
        WHERE a.retired_by_migration = '000040_roles_name_gate_and_spacing'
          AND NOT EXISTS (
              SELECT 1 FROM canonical_role_dimensions x
              WHERE x.canonical_role_id = s.id AND x.dimension_id = d.id)
    ) THEN
        RAISE EXCEPTION 'canonical_roles merge lost a role dimension';
    END IF;
END
$$;

DROP TABLE merge_preflight;

-- ---------------------------------------------------------------------------
-- 5. Register the retirements in 000031's registry, so mcp.save_enrichment
-- refuses to mint them again, and repair the one name that disagrees with its
-- slug.
--
-- The reason text names the survivor, for 000037's reason: the registry's
-- error message is handed back to the agent that tripped it, and a blocked
-- mint that says which slug to use instead costs one round trip where one that
-- only says no costs a re-mint under a third spelling.
-- ---------------------------------------------------------------------------

INSERT INTO retired_slugs (slug, table_name, retired_by_migration, reason)
SELECT m.retired_slug, 'canonical_roles', '000040_roles_name_gate_and_spacing',
       format('Duplicate slug, merged into %L: the two slugs are one role (%s), so a grouping by role split one population across two rows. Use %L.',
              m.survivor_slug, m.variant, m.survivor_slug)
FROM merge_map_roles m
ON CONFLICT (slug, table_name) DO NOTHING;

INSERT INTO retired_slugs (slug, table_name, retired_by_migration, reason)
SELECT m.retired_slug, 'skills', '000040_roles_name_gate_and_spacing',
       format('Duplicate slug, merged into %L: the two slugs are one term differing only in internal spacing (%s). Use %L.',
              m.survivor_slug, m.variant, m.survivor_slug)
FROM merge_map_skills_spacing m
ON CONFLICT (slug, table_name) DO NOTHING;

DROP TABLE merge_map_roles;
DROP TABLE merge_map_skills_spacing;

-- The survivor's inherited name asserts "Manager" over a population that is
-- mostly not managers. Slug wins over name, following 000037 section 7.
UPDATE canonical_roles SET name = 'Business Operations' WHERE slug = 'business-operations';

-- ---------------------------------------------------------------------------
-- 6. Extend the gate.
--
-- The whole function is re-declared because that is the only way to change a
-- plpgsql body. The changes from 000039 are confined to the name_collision
-- block -- now one set-based statement over all three tables instead of two
-- hand-copied loops over two -- and to the comparison key, which both that
-- block and Phase A's advisory `exact_name` axis now take from the same
-- expression. The advisory-lock wrapper `mcp.save_enrichment` is untouched, so
-- writes stay serialized as 000026 arranged, and 000039's advisory audit is
-- carried through unchanged.
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION mcp.save_enrichment_unlocked(
    p_payload        jsonb,
    p_model          text,
    p_prompt_version text
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
DECLARE
    v_posting_id      bigint;
    v_seniority       text;
    v_notes           text;
    v_errors          jsonb := '[]'::jsonb;
    v_class_id        bigint;
    v_elem            jsonb;
    v_dim             text;
    v_slug            text;
    v_name            text;
    v_id              bigint;
    v_inserted        boolean;
    v_new_roles       jsonb := '[]'::jsonb;
    v_new_specs       jsonb := '[]'::jsonb;
    v_new_skills      jsonb := '[]'::jsonb;
    v_idx             int;
    v_retired_reason  text;
    -- ---- Mint-only name collision (migrations 000038, 000040) ------------
    -- 000038 ran this check as two hand-copied loops holding the collider's
    -- slug and name in scalars. 000040 brought `canonical_roles` into scope,
    -- which would have made a third copy, and replaced all three with the one
    -- set-based statement below -- so the scalars are gone.
    -- Slug shape: lowercase ASCII letters/digits, optional single-hyphen
    -- separators, no leading/trailing/consecutive hyphens. Mirrors slugPattern
    -- in internal/enrich/classify/validate.go.
    c_slug_pattern constant text := '^[a-z0-9]+(-[a-z0-9]+)*$';
    -- 64-char slug ceiling. Mirrors classify.MaxSlugLen.
    c_max_slug_len constant int := 64;
    -- Closed seniority set. Identical to validSeniorities in
    -- internal/enrich/classify/validate.go and the classifications.seniority CHECK.
    c_seniorities  constant text[] := ARRAY[
        'intern', 'junior', 'mid', 'senior', 'staff',
        'principal', 'lead', 'director', 'unknown'
    ];
    -- Within-payload cross-array collision tracking: first array each slug was
    -- seen in, so a slug appearing in a second array fires slug_collision.
    v_owner        text;
    v_array        text;

    -- ---- Near-duplicate suppression (migration 000033) --------------------
    -- The three taxonomy arrays are named identically to their target tables,
    -- which is what lets every phase below iterate over them by name instead
    -- of being written out three times.
    c_tables constant text[] := ARRAY['canonical_roles', 'specializations', 'skills'];
    -- Which axis may trigger a substitution (migration 000034). 'slug' compares
    -- similarity(proposed.slug, existing.slug); 'name' compares the two names.
    -- There is deliberately no value meaning "either" -- combining the axes is
    -- 000033's behaviour and is what the header above measures as unsafe. The
    -- Phase A CASE has no ELSE, so an unrecognised value yields NULL, compares
    -- false against the threshold, and every near match degrades to an advisory
    -- candidate. A typo mints; it never remaps.
    c_substitute_axis constant text := 'slug';
    -- At or above this, ON c_substitute_axis, a proposal is remapped onto the
    -- existing row. Exact normalized-name equality no longer substitutes on its
    -- own; since 000034 it is advisory only.
    c_substitute_at   constant numeric := 0.95;
    -- At or above this (and below c_substitute_at) a proposal still mints, but
    -- its near matches are reported back as advisory candidates.
    c_advisory_at     constant numeric := 0.60;
    -- At or above this, two entries in the SAME payload array are one concept
    -- twice and all but one are dropped.
    c_payload_dup_at  constant numeric := 0.85;
    -- How many advisory candidates to report per minted slug.
    c_max_candidates  constant int := 3;

    -- v_payload is the working copy the write phase reads. p_payload stays the
    -- payload as submitted, so every check above keeps seeing exactly what the
    -- agent sent and every reported path indexes the submitted arrays.
    v_payload      jsonb;
    v_subs         jsonb := '[]'::jsonb;   -- substitutions[] for the envelope
    v_cands        jsonb := '[]'::jsonb;   -- similarity_candidates[]
    v_drops        jsonb := '[]'::jsonb;   -- dropped[]
    v_sub_map      jsonb := '{}'::jsonb;   -- {table: {idx: {slug, name}}}
    v_uses_map     jsonb := '{}'::jsonb;   -- {table: {slug: existing_use_count}}
    v_out          jsonb;
    v_result       jsonb;
    v_order        int[];
    v_keep_idx     int[];
    v_i            int;
    v_j            int;
    v_kept         int;
    v_sim          numeric;
    v_best         numeric;
    v_survivor     jsonb;
    v_has_novel    boolean;
    v_needs_dedup  boolean;
BEGIN
    v_posting_id := (p_payload ->> 'posting_id')::bigint;
    v_seniority  := p_payload -> 'classification' ->> 'seniority';
    v_notes      := p_payload -> 'classification' ->> 'notes';

    -- ---- Re-check invariants the database owns, accumulating structured errors.
    -- These mirror the Go-side checks; they exist so a payload that slipped past
    -- (or bypassed) Go validation cannot corrupt the taxonomy. On any violation
    -- the function returns the errors and writes nothing.

    -- Posting must exist.
    IF NOT EXISTS (SELECT 1 FROM public.job_postings WHERE id = v_posting_id) THEN
        v_errors := v_errors || jsonb_build_object(
            'path', 'posting_id',
            'code', 'posting_not_found',
            'message', format('job posting %s does not exist', v_posting_id)
        );
    END IF;

    -- Seniority must be in the closed set. Re-checked here instead of relying on
    -- the table CHECK to RAISE (which would surface to the caller as db_error
    -- rather than a structured invalid_seniority).
    IF NOT (v_seniority = ANY (c_seniorities)) THEN
        v_errors := v_errors || jsonb_build_object(
            'path', 'classification.seniority',
            'code', 'invalid_seniority',
            'message', format('%s is not a valid seniority', coalesce(v_seniority, '(null)'))
        );
    END IF;

    -- Slug shape and length, plus duplicate-within-array, for each minting array.
    -- v_idx tracks position so paths read <array>[i].slug, matching Go.
    FOR v_array IN SELECT unnest(ARRAY['canonical_roles', 'specializations', 'skills'])
    LOOP
        v_idx := 0;
        DECLARE
            v_seen text[] := ARRAY[]::text[];
        BEGIN
            FOR v_slug IN
                SELECT (e ->> 'slug')
                FROM jsonb_array_elements(coalesce(p_payload -> v_array, '[]'::jsonb)) AS e
            LOOP
                -- Duplicate slug within this array.
                IF v_slug = ANY (v_seen) THEN
                    v_errors := v_errors || jsonb_build_object(
                        'path', format('%s[%s].slug', v_array, v_idx),
                        'code', 'duplicate_slug',
                        'message', format('%s appears more than once', v_slug)
                    );
                ELSE
                    v_seen := v_seen || v_slug;
                END IF;

                -- Slug length, then shape. An over-length slug reports
                -- slug_too_long; any other malformed slug reports invalid_slug.
                IF length(coalesce(v_slug, '')) > c_max_slug_len THEN
                    v_errors := v_errors || jsonb_build_object(
                        'path', format('%s[%s].slug', v_array, v_idx),
                        'code', 'slug_too_long',
                        'message', format('%s is too long; slugs are at most %s characters', v_slug, c_max_slug_len)
                    );
                ELSIF coalesce(v_slug, '') !~ c_slug_pattern THEN
                    v_errors := v_errors || jsonb_build_object(
                        'path', format('%s[%s].slug', v_array, v_idx),
                        'code', 'invalid_slug',
                        'message', format('%s is not a valid slug', coalesce(v_slug, '(null)'))
                    );
                END IF;

                v_idx := v_idx + 1;
            END LOOP;
        END;
    END LOOP;

    -- Within-payload cross-array collision: the same slug value in two different
    -- payload arrays. role_dimensions is a closed seeded set the agent never mints
    -- into, so only the three minting arrays participate (matches Go).
    -- Track first-seen array per slug using two parallel arrays (no hstore dep).
    DECLARE
        v_seen_slugs  text[] := ARRAY[]::text[];
        v_seen_arrays text[] := ARRAY[]::text[];
        v_pos         int;
    BEGIN
        FOR v_array IN SELECT unnest(ARRAY['canonical_roles', 'specializations', 'skills'])
        LOOP
            v_idx := 0;
            FOR v_slug IN
                SELECT (e ->> 'slug')
                FROM jsonb_array_elements(coalesce(p_payload -> v_array, '[]'::jsonb)) AS e
            LOOP
                v_pos := array_position(v_seen_slugs, v_slug);
                IF v_pos IS NOT NULL AND v_seen_arrays[v_pos] IS DISTINCT FROM v_array THEN
                    v_owner := v_seen_arrays[v_pos];
                    v_errors := v_errors || jsonb_build_object(
                        'path', format('%s[%s].slug', v_array, v_idx),
                        'code', 'slug_collision',
                        'message', format('%s appears in both %s and %s; a slug belongs to one table only',
                            v_slug, least(v_owner, v_array), greatest(v_owner, v_array))
                    );
                ELSIF v_pos IS NULL THEN
                    v_seen_slugs  := v_seen_slugs || v_slug;
                    v_seen_arrays := v_seen_arrays || v_array;
                END IF;
                v_idx := v_idx + 1;
            END LOOP;
        END LOOP;
    END;

    -- Unknown role dimensions: every dimensions[] slug must exist in role_dimensions.
    v_idx := 0;
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(p_payload -> 'canonical_roles', '[]'::jsonb))
    LOOP
        IF jsonb_array_length(coalesce(v_elem -> 'dimensions', '[]'::jsonb)) = 0 THEN
            v_errors := v_errors || jsonb_build_object(
                'path', format('canonical_roles[%s].dimensions', v_idx),
                'code', 'empty_dimensions',
                'message', format('canonical_role %s has no dimensions', v_elem ->> 'slug')
            );
        END IF;
        FOR v_dim IN SELECT jsonb_array_elements_text(coalesce(v_elem -> 'dimensions', '[]'::jsonb))
        LOOP
            IF NOT EXISTS (SELECT 1 FROM public.role_dimensions WHERE slug = v_dim) THEN
                v_errors := v_errors || jsonb_build_object(
                    'path', format('canonical_roles[%s].dimensions', v_idx),
                    'code', 'unknown_dimension',
                    'message', format('%s is not a known role dimension', v_dim)
                );
            END IF;
        END LOOP;
        v_idx := v_idx + 1;
    END LOOP;

    -- Cross-table slug ownership: a payload slug must not already exist in a
    -- different taxonomy table. canonical_roles vs (specializations, skills,
    -- role_dimensions); specializations vs (canonical_roles, skills,
    -- role_dimensions); skills vs (canonical_roles, specializations,
    -- role_dimensions).
    v_idx := 0;
    FOR v_slug IN SELECT jsonb_path_query(p_payload, '$.canonical_roles[*].slug') #>> '{}'
    LOOP
        IF EXISTS (SELECT 1 FROM public.specializations WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('canonical_roles[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a specializations', v_slug));
        ELSIF EXISTS (SELECT 1 FROM public.skills WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('canonical_roles[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a skills', v_slug));
        ELSIF EXISTS (SELECT 1 FROM public.role_dimensions WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('canonical_roles[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a role_dimensions', v_slug));
        END IF;
        v_idx := v_idx + 1;
    END LOOP;

    v_idx := 0;
    FOR v_slug IN SELECT jsonb_path_query(p_payload, '$.specializations[*].slug') #>> '{}'
    LOOP
        IF EXISTS (SELECT 1 FROM public.canonical_roles WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('specializations[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a canonical_roles', v_slug));
        ELSIF EXISTS (SELECT 1 FROM public.skills WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('specializations[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a skills', v_slug));
        ELSIF EXISTS (SELECT 1 FROM public.role_dimensions WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('specializations[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a role_dimensions', v_slug));
        END IF;
        v_idx := v_idx + 1;
    END LOOP;

    v_idx := 0;
    FOR v_slug IN SELECT jsonb_path_query(p_payload, '$.skills[*].slug') #>> '{}'
    LOOP
        IF EXISTS (SELECT 1 FROM public.canonical_roles WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('skills[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a canonical_roles', v_slug));
        ELSIF EXISTS (SELECT 1 FROM public.specializations WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('skills[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a specializations', v_slug));
        ELSIF EXISTS (SELECT 1 FROM public.role_dimensions WHERE slug = v_slug) THEN
            v_errors := v_errors || jsonb_build_object('path', format('skills[%s].slug', v_idx), 'code', 'slug_collision', 'message', format('%s is already a role_dimensions', v_slug));
        END IF;
        v_idx := v_idx + 1;
    END LOOP;

    -- Retired-slug registry (migration 000031): a payload slug that is about to
    -- be MINTED must not match a retired entry scoped to its own table (or to
    -- NULL, all tables). A slug already present in its own table is reuse, not
    -- minting, and is never blocked here regardless of retirement history.
    --
    -- This runs against the payload as submitted, deliberately BEFORE the
    -- 000033 gate reshapes anything. A retired slug that happens to be a close
    -- match for something existing is still rejected rather than quietly
    -- remapped: 000029/000031 retired those terms because the agent's judgement
    -- about them was wrong, and silently accepting the payload would hide that.
    v_idx := 0;
    FOR v_slug IN SELECT jsonb_path_query(p_payload, '$.canonical_roles[*].slug') #>> '{}'
    LOOP
        IF NOT EXISTS (SELECT 1 FROM public.canonical_roles WHERE slug = v_slug) THEN
            SELECT reason INTO v_retired_reason
            FROM public.retired_slugs
            WHERE slug = v_slug AND (table_name = 'canonical_roles' OR table_name IS NULL)
            ORDER BY (table_name IS NULL)
            LIMIT 1;

            IF v_retired_reason IS NOT NULL THEN
                v_errors := v_errors || jsonb_build_object(
                    'path', format('canonical_roles[%s].slug', v_idx),
                    'code', 'retired_slug',
                    'message', format('%s was retired: %s', v_slug, v_retired_reason)
                );
            END IF;
        END IF;
        v_idx := v_idx + 1;
    END LOOP;

    v_idx := 0;
    FOR v_slug IN SELECT jsonb_path_query(p_payload, '$.specializations[*].slug') #>> '{}'
    LOOP
        IF NOT EXISTS (SELECT 1 FROM public.specializations WHERE slug = v_slug) THEN
            SELECT reason INTO v_retired_reason
            FROM public.retired_slugs
            WHERE slug = v_slug AND (table_name = 'specializations' OR table_name IS NULL)
            ORDER BY (table_name IS NULL)
            LIMIT 1;

            IF v_retired_reason IS NOT NULL THEN
                v_errors := v_errors || jsonb_build_object(
                    'path', format('specializations[%s].slug', v_idx),
                    'code', 'retired_slug',
                    'message', format('%s was retired: %s', v_slug, v_retired_reason)
                );
            END IF;
        END IF;
        v_idx := v_idx + 1;
    END LOOP;

    v_idx := 0;
    FOR v_slug IN SELECT jsonb_path_query(p_payload, '$.skills[*].slug') #>> '{}'
    LOOP
        IF NOT EXISTS (SELECT 1 FROM public.skills WHERE slug = v_slug) THEN
            SELECT reason INTO v_retired_reason
            FROM public.retired_slugs
            WHERE slug = v_slug AND (table_name = 'skills' OR table_name IS NULL)
            ORDER BY (table_name IS NULL)
            LIMIT 1;

            IF v_retired_reason IS NOT NULL THEN
                v_errors := v_errors || jsonb_build_object(
                    'path', format('skills[%s].slug', v_idx),
                    'code', 'retired_slug',
                    'message', format('%s was retired: %s', v_slug, v_retired_reason)
                );
            END IF;
        END IF;
        v_idx := v_idx + 1;
    END LOOP;

    -- Mint-only name collision (migrations 000038, 000040): a payload term
    -- that is about to be MINTED must not carry the normalized name of a row
    -- its target table already holds. Reuse of an existing slug is never
    -- blocked -- only minting a second row for a name the taxonomy already
    -- spells.
    --
    -- This BLOCKS; it does not substitute. 000033 substituted on this axis and
    -- 000034 demoted it to advisory for good reason: a name-matched, slug-
    -- dissimilar pair is as likely to be a corrupted label as a duplicate
    -- concept, and remapping onto the wrong row discards a concept invisibly.
    -- Blocking guesses nothing. It writes no link, names the collider, and
    -- hands the worker the two answers that are both correct: reuse the
    -- existing slug, or pick a name that actually distinguishes the concepts.
    --
    -- 000040 changed two things.
    --
    -- SCOPE. 000038 covered `specializations` and `skills` only: 000037 had
    -- brought their normalized-name collisions to zero, `canonical_roles`
    -- still carried three, and a rule that fires against existing corruption
    -- is a rule workers learn to route around. Section 4 of this migration
    -- cleared those three, so roles rejoin. 000038's two hand-copied loops
    -- would have become three; they are one set-based statement instead,
    -- built on the same `proposed` / `existing` / `novel` CTEs Phase A uses
    -- below, so the two checks cannot disagree about what "novel" means.
    --
    -- NORMALIZER. 000038 compared lower(btrim(collapse whitespace)), which
    -- catches "SOC 2  Compliance" against "soc 2 compliance" but not
    -- "SOC2 Compliance", and not "PowerBI" against "Power BI". Both of those
    -- pairs were live in `skills` and section 4 of this migration merges them.
    -- The comparison key is now lower(remove ALL whitespace), which is
    -- strictly broader: two names equal under the old key are still equal
    -- under this one, because removing spaces from equal strings gives equal
    -- strings. Punctuation is deliberately still untouched. Stripping it would
    -- fold 'c' "C", 'cpp' "C++" and 'csharp' "C#" into one term -- the
    -- similarity('C++','C#') = 1.000 hazard 000033 documented and 000037 made
    -- a point of surviving. Measured across all three tables before this
    -- migration, the wider key found exactly two pairs the old one missed,
    -- both of them the genuine duplicates named above, and zero pairs of
    -- distinct concepts that merely concatenate alike.
    --
    -- Position mirrors the retired-slug registry above, and for the same
    -- reason: it reads the payload as submitted, before the 000033 gate
    -- reshapes anything, and it joins that check's error batch so a payload
    -- with several problems reports all of them in one reply.
    WITH t(tbl) AS (
        VALUES ('canonical_roles'), ('specializations'), ('skills')
    ),
    proposed AS (
        SELECT t.tbl,
               (e.ord - 1)::int AS idx,
               e.value ->> 'slug' AS slug,
               coalesce(e.value ->> 'name', '') AS name
        FROM t
        CROSS JOIN LATERAL jsonb_array_elements(coalesce(p_payload -> t.tbl, '[]'::jsonb))
             WITH ORDINALITY AS e(value, ord)
    ),
    existing AS (
        SELECT 'canonical_roles'::text AS tbl, id, slug, name FROM public.canonical_roles
        UNION ALL
        SELECT 'specializations', id, slug, name FROM public.specializations
        UNION ALL
        SELECT 'skills', id, slug, name FROM public.skills
    ),
    -- A slug already present in its target table is reuse, and reuse is never
    -- blocked. An empty or whitespace-only name has no key to collide on.
    novel AS (
        SELECT p.*
        FROM proposed p
        WHERE lower(regexp_replace(p.name, '\s', '', 'g')) <> ''
          AND NOT EXISTS (
              SELECT 1 FROM existing x WHERE x.tbl = p.tbl AND x.slug = p.slug
          )
    ),
    -- One collider per proposal, chosen the way 000038 chose it: the
    -- name-equal row whose slug is closest to the proposed one, ties broken by
    -- id. Naming the nearest collider is what makes "reuse this instead"
    -- actionable when several rows share the name.
    collider AS (
        SELECT DISTINCT ON (n.tbl, n.idx)
               n.tbl, n.idx, n.slug, n.name,
               x.slug AS existing_slug, x.name AS existing_name
        FROM novel n
        JOIN existing x
          ON x.tbl = n.tbl
         AND lower(regexp_replace(x.name, '\s', '', 'g'))
           = lower(regexp_replace(n.name, '\s', '', 'g'))
        ORDER BY n.tbl, n.idx, public.similarity(x.slug, n.slug) DESC, x.id
    )
    SELECT v_errors || coalesce((
               SELECT jsonb_agg(jsonb_build_object(
                          'path', format('%s[%s].name', c.tbl, c.idx),
                          'code', 'name_collision',
                          'table', c.tbl,
                          'proposed_slug', c.slug,
                          'proposed_name', c.name,
                          'existing_slug', c.existing_slug,
                          'existing_name', c.existing_name,
                          'slug_similarity', round(public.similarity(c.slug, c.existing_slug)::numeric, 3),
                          'name_similarity', round(public.similarity(c.name, c.existing_name)::numeric, 3),
                          'message', format(
                              '%s already has %L named %L; minting %L under the same name would split one term across two rows. Reuse %L, or name the new concept so it is distinguishable.',
                              c.tbl, c.existing_slug, c.existing_name, c.slug, c.existing_slug))
                      ORDER BY c.tbl, c.idx)
               FROM collider c), '[]'::jsonb)
    INTO v_errors;

    IF jsonb_array_length(v_errors) > 0 THEN
        RETURN jsonb_build_object('ok', false, 'errors', v_errors);
    END IF;

    -- =======================================================================
    -- Near-duplicate suppression (migration 000033).
    --
    -- Every check above has passed, so nothing below can make the call fail.
    -- These phases only reshape the payload; the write phase then reads
    -- v_payload instead of p_payload.
    -- =======================================================================

    v_payload := p_payload;

    -- ---- Phase A: substitution and advisory candidates. -------------------
    --
    -- One statement for all three arrays. `existing` is the union of the three
    -- taxonomy tables tagged by table name, so each proposal is only ever
    -- compared against its own table. `novel` narrows to proposals that would
    -- MINT -- a slug already present in its target table is reuse and is never
    -- touched, which is what keeps this from second-guessing the taxonomy the
    -- agent was handed.
    --
    -- The two axes are scored separately and do different jobs (000034). The
    -- slug axis alone decides substitution: it is the axis measured clean, two
    -- intra-table hits at 0.95 across all three tables, both genuine word-order
    -- restatements. The name axis feeds only `sim`, the advisory score, because
    -- `name` is not a reliable identity signal in this database -- 110 skill
    -- pairs share an exact normalized name and 47 of them score under 0.3 on
    -- slug, i.e. are different concepts wearing the same string.
    --
    -- So a restatement whose slug shares no trigrams with its target
    -- (data-orchestration vs data-pipeline-architecture: 0.150 on slug, 1.000
    -- on name) is no longer remapped. It mints, and the name match comes back
    -- as an advisory candidate tagged match:'exact_name'. That costs one round
    -- trip, where a remap would be right for that pair and wrong for
    -- llm / embeddings, silently, with no way for the agent to tell which.
    --
    -- Tie-breaking is total on both rankings. Substitution: axis score
    -- descending, then the candidate slug alphabetically. Advisory: exact name
    -- match first, then the combined score, then the candidate slug. Two runs
    -- over the same data always pick the same target.

    -- Cheap pre-check, so a payload with nothing to substitute pays nothing.
    -- v_has_novel is three indexed slug lookups per element; without it the
    -- Phase A statement below scans all three taxonomy tables even when every
    -- proposed slug already exists and there is nothing to substitute.
    WITH t(tbl) AS (
        VALUES ('canonical_roles'), ('specializations'), ('skills')
    ),
    p AS (
        SELECT t.tbl, e.value ->> 'slug' AS slug
        FROM t
        CROSS JOIN LATERAL jsonb_array_elements(coalesce(p_payload -> t.tbl, '[]'::jsonb)) AS e(value)
    )
    SELECT EXISTS (
            SELECT 1 FROM p WHERE CASE p.tbl
                WHEN 'canonical_roles' THEN NOT EXISTS (SELECT 1 FROM public.canonical_roles r WHERE r.slug = p.slug)
                WHEN 'specializations' THEN NOT EXISTS (SELECT 1 FROM public.specializations s WHERE s.slug = p.slug)
                ELSE NOT EXISTS (SELECT 1 FROM public.skills k WHERE k.slug = p.slug)
            END)
    INTO v_has_novel;

    IF v_has_novel THEN
        WITH t(tbl) AS (
            VALUES ('canonical_roles'), ('specializations'), ('skills')
        ),
        proposed AS (
            SELECT t.tbl,
                   (e.ord - 1)::int AS idx,
                   e.value ->> 'slug' AS slug,
                   coalesce(e.value ->> 'name', '') AS name
            FROM t
            CROSS JOIN LATERAL jsonb_array_elements(coalesce(p_payload -> t.tbl, '[]'::jsonb))
                 WITH ORDINALITY AS e(value, ord)
        ),
        existing AS (
            SELECT 'canonical_roles'::text AS tbl, slug, name FROM public.canonical_roles
            UNION ALL
            SELECT 'specializations'::text, slug, name FROM public.specializations
            UNION ALL
            SELECT 'skills'::text, slug, name FROM public.skills
        ),
        novel AS (
            SELECT p.*
            FROM proposed p
            WHERE NOT EXISTS (
                SELECT 1 FROM existing x WHERE x.tbl = p.tbl AND x.slug = p.slug
            )
        ),
        scored AS (
            -- Both axes are computed once, in the inner select, and everything
            -- downstream reads those two columns. The axis CASE picks between
            -- values already in hand, so narrowing the branch costs nothing.
            SELECT b.*,
                   greatest(b.slug_sim, b.name_sim) AS sim,
                   CASE c_substitute_axis
                       WHEN 'slug' THEN b.slug_sim
                       WHEN 'name' THEN b.name_sim
                   END AS sub_sim
            FROM (
                SELECT n.tbl, n.idx, n.slug, n.name,
                       x.slug AS match_slug,
                       x.name AS match_name,
                       -- Normalized-name equality, on the same key the mint-only
                       -- name_collision block above uses (000040): lower, all
                       -- whitespace removed, punctuation untouched. Compared as text,
                       -- not as trigrams, so 'C++' and 'C#' do not match here even
                       -- though pg_trgm scores them 1.000. One key for both sites --
                       -- two would drift, and a gate that blocks on one definition
                       -- while advising on another is worse than either alone.
                       -- Advisory only since 000034 -- it no longer substitutes.
                       (lower(regexp_replace(n.name, '\s', '', 'g'))
                          = lower(regexp_replace(x.name, '\s', '', 'g'))) AS exact_name,
                       public.similarity(n.slug, x.slug)::numeric AS slug_sim,
                       public.similarity(n.name, x.name)::numeric AS name_sim
                FROM novel n
                JOIN existing x ON x.tbl = n.tbl
            ) b
        ),
        -- Substitution ranking, over the substitutable rows only. It is separate
        -- from the advisory ranking below on purpose: the advisory order puts
        -- exact-name rows first, and an exact-name row scoring 0.063 on slug can
        -- outrank a genuine 1.000 slug match. Sharing one row_number would hand
        -- rn = 1 to the row that can no longer substitute and lose the one that
        -- can.
        chosen AS (
            SELECT q.*
            FROM (
                SELECT s.*,
                       row_number() OVER (
                           PARTITION BY s.tbl, s.idx
                           ORDER BY s.sub_sim DESC, s.match_slug ASC
                       ) AS sub_rn
                FROM scored s
                WHERE s.sub_sim >= c_substitute_at
            ) q
            WHERE q.sub_rn = 1
        ),
        ranked AS (
            SELECT s.*,
                   row_number() OVER (
                       PARTITION BY s.tbl, s.idx
                       ORDER BY s.exact_name DESC, s.sim DESC, s.match_slug ASC
                   ) AS rn
            FROM scored s
            WHERE s.exact_name OR s.sim >= c_advisory_at
        ),
        advisory AS (
            SELECT r.*
            FROM ranked r
            WHERE r.rn <= c_max_candidates
              AND NOT EXISTS (SELECT 1 FROM chosen c WHERE c.tbl = r.tbl AND c.idx = r.idx)
        )
        SELECT
            coalesce((
                SELECT jsonb_agg(jsonb_build_object(
                           'path', format('%s[%s].slug', c.tbl, c.idx),
                           'table', c.tbl,
                           'proposed_slug', c.slug,
                           'proposed_name', c.name,
                           'substituted_slug', c.match_slug,
                           'substituted_name', c.match_name,
                           -- One branch survives, so `match` names the axis
                           -- that fired rather than which of two branches did,
                           -- and `similarity` is that axis's score -- the number
                           -- that actually cleared the threshold, not the
                           -- combined one, which can be higher for the wrong
                           -- reason.
                           'match', c_substitute_axis || '_similarity',
                           'similarity', round(c.sub_sim, 3))
                       ORDER BY c.tbl, c.idx)
                FROM chosen c), '[]'::jsonb),
            coalesce((
                SELECT jsonb_object_agg(q.tbl, q.m)
                FROM (
                    SELECT c.tbl,
                           jsonb_object_agg(c.idx::text,
                               jsonb_build_object('slug', c.match_slug, 'name', c.match_name)) AS m
                    FROM chosen c GROUP BY c.tbl
                ) q), '{}'::jsonb),
            coalesce((
                SELECT jsonb_agg(q.x ORDER BY q.tbl, q.idx)
                FROM (
                    SELECT a.tbl, a.idx,
                           jsonb_build_object(
                               'path', format('%s[%s].slug', a.tbl, a.idx),
                               'table', a.tbl,
                               'minted_slug', a.slug,
                               'minted_name', a.name,
                               -- Each candidate says WHY it is here. An
                               -- exact_name candidate is precisely the signal
                               -- 000033 substituted on and 000034 demoted: the
                               -- agent sees the name collision, sees how far
                               -- apart the slugs are, and decides -- instead of
                               -- the gate guessing on its behalf.
                               'candidates', jsonb_agg(jsonb_build_object(
                                   'slug', a.match_slug,
                                   'name', a.match_name,
                                   'match', CASE WHEN a.exact_name THEN 'exact_name' ELSE 'similarity' END,
                                   'slug_similarity', round(a.slug_sim, 3),
                                   'similarity', round(a.sim, 3)) ORDER BY a.rn)) AS x
                    FROM advisory a
                    GROUP BY a.tbl, a.idx, a.slug, a.name
                ) q), '[]'::jsonb)
        INTO v_subs, v_sub_map, v_cands;
    END IF;

    -- Apply the remaps. Only the `slug` key changes; the element keeps its own
    -- name and (for canonical_roles) its dimensions. The write loops upsert
    -- ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug, so the existing
    -- row's name is preserved and the proposed name is discarded -- which is
    -- the point: the existing row is the one being reused.
    --
    -- Array positions are not disturbed here, so every `path` reported above
    -- still indexes the payload as submitted.
    IF v_sub_map <> '{}'::jsonb THEN
        FOREACH v_array IN ARRAY c_tables
        LOOP
            IF v_sub_map ? v_array THEN
                SELECT coalesce(jsonb_agg(
                           CASE
                               WHEN v_sub_map -> v_array ? ((e.ord - 1)::text)
                               THEN e.value || jsonb_build_object(
                                        'slug', v_sub_map -> v_array -> ((e.ord - 1)::text) ->> 'slug')
                               ELSE e.value
                           END
                           ORDER BY e.ord), '[]'::jsonb)
                INTO v_out
                FROM jsonb_array_elements(coalesce(v_payload -> v_array, '[]'::jsonb))
                     WITH ORDINALITY AS e(value, ord);

                v_payload := jsonb_set(v_payload, ARRAY[v_array], v_out);
            END IF;
        END LOOP;
    END IF;

    -- ---- Phase C prep: existing usage counts for the surviving slugs. -----
    --
    -- Usage is the count of classification links a slug already has. It is the
    -- first tie-break for which member of a redundant pair survives: the tag
    -- the corpus already uses wins, so dedup pulls toward the established term
    -- rather than fragmenting further. A brand-new slug has no row and scores 0
    -- via the coalesce at the point of use.
    --
    -- Scoped to the payload's own slugs, so this is three index lookups' worth
    -- of work, not an aggregate over all 32k link rows.
    --
    -- Only worth computing when a redundant pair actually exists. This pre-check
    -- reads nothing but the payload itself -- at most a few dozen similarity()
    -- calls -- whereas the usage query below has to count link rows, and
    -- job_posting_skills is indexed on (classification_id, skill_id) with no
    -- index on skill_id alone, so counting by slug scans it. Most payloads carry
    -- no redundant pair and should not pay for that.
    SELECT EXISTS (
        SELECT 1
        FROM (VALUES ('canonical_roles'), ('specializations'), ('skills')) AS t(tbl)
        CROSS JOIN LATERAL jsonb_array_elements(coalesce(v_payload -> t.tbl, '[]'::jsonb))
             WITH ORDINALITY AS a(value, ord)
        CROSS JOIN LATERAL jsonb_array_elements(coalesce(v_payload -> t.tbl, '[]'::jsonb))
             WITH ORDINALITY AS b(value, ord)
        WHERE a.ord < b.ord
          AND greatest(
                public.similarity(a.value ->> 'slug', b.value ->> 'slug'),
                public.similarity(coalesce(a.value ->> 'name', ''),
                                  coalesce(b.value ->> 'name', '')))::numeric >= c_payload_dup_at
    )
    INTO v_needs_dedup;

    IF v_needs_dedup THEN
        WITH t(tbl) AS (
            VALUES ('canonical_roles'), ('specializations'), ('skills')
        ),
        p AS (
            SELECT t.tbl, e.value ->> 'slug' AS slug
            FROM t
            CROSS JOIN LATERAL jsonb_array_elements(coalesce(v_payload -> t.tbl, '[]'::jsonb)) AS e(value)
        ),
        u AS (
            SELECT 'canonical_roles'::text AS tbl, r.slug, count(j.classification_id) AS uses
            FROM public.canonical_roles r
            LEFT JOIN public.job_posting_roles j ON j.role_id = r.id
            WHERE r.slug IN (SELECT slug FROM p WHERE tbl = 'canonical_roles')
            GROUP BY r.slug
            UNION ALL
            SELECT 'specializations'::text, s.slug, count(j.classification_id)
            FROM public.specializations s
            LEFT JOIN public.job_posting_specializations j ON j.specialization_id = s.id
            WHERE s.slug IN (SELECT slug FROM p WHERE tbl = 'specializations')
            GROUP BY s.slug
            UNION ALL
            SELECT 'skills'::text, k.slug, count(j.classification_id)
            FROM public.skills k
            LEFT JOIN public.job_posting_skills j ON j.skill_id = k.id
            WHERE k.slug IN (SELECT slug FROM p WHERE tbl = 'skills')
            GROUP BY k.slug
        )
        SELECT coalesce(jsonb_object_agg(q.tbl, q.m), '{}'::jsonb)
        INTO v_uses_map
        FROM (
            SELECT u.tbl, jsonb_object_agg(u.slug, u.uses) AS m FROM u GROUP BY u.tbl
        ) q;
    END IF;

    -- ---- Phase C: within-payload redundancy. ------------------------------
    --
    -- Greedy over a total order: visit entries by descending existing usage,
    -- then shorter slug, then alphabetically, then submitted position. Keep the
    -- first, and drop any later entry that scores c_payload_dup_at or above
    -- against something already kept. Because the visit order is total and the
    -- comparison is symmetric, the survivor of any cluster is fully determined
    -- by the data -- there is no dependence on the order the agent happened to
    -- list its tags in.
    --
    -- Two proposals that Phase A remapped onto the same existing slug land here
    -- as an identical pair scoring 1.000, so this is also what guarantees each
    -- array holds distinct slugs before the write loops run. When that happens
    -- the dropped element's dimensions are merged into the survivor first, so a
    -- remap-into-collision cannot lose a canonical role's dimension mappings.
    FOREACH v_array IN ARRAY c_tables
    LOOP
        CONTINUE WHEN NOT v_needs_dedup;
        CONTINUE WHEN jsonb_array_length(coalesce(v_payload -> v_array, '[]'::jsonb)) < 2;

        SELECT coalesce(array_agg((e.ord - 1)::int ORDER BY
                   coalesce((v_uses_map -> v_array ->> (e.value ->> 'slug'))::bigint, 0) DESC,
                   length(e.value ->> 'slug') ASC,
                   (e.value ->> 'slug') ASC,
                   e.ord ASC), ARRAY[]::int[])
        INTO v_order
        FROM jsonb_array_elements(v_payload -> v_array) WITH ORDINALITY AS e(value, ord);

        v_keep_idx := ARRAY[]::int[];

        FOREACH v_i IN ARRAY v_order
        LOOP
            v_elem := v_payload -> v_array -> v_i;
            v_kept := NULL;
            v_best := 0;

            FOREACH v_j IN ARRAY v_keep_idx
            LOOP
                v_survivor := v_payload -> v_array -> v_j;
                v_sim := greatest(
                    public.similarity(v_elem ->> 'slug', v_survivor ->> 'slug'),
                    public.similarity(coalesce(v_elem ->> 'name', ''),
                                      coalesce(v_survivor ->> 'name', '')))::numeric;
                IF v_sim > v_best THEN
                    v_best := v_sim;
                    v_kept := v_j;
                END IF;
            END LOOP;

            IF v_kept IS NOT NULL AND v_best >= c_payload_dup_at THEN
                v_survivor := v_payload -> v_array -> v_kept;

                -- Same slug on both sides means Phase A remapped them together.
                -- Union the dimension lists so nothing the agent asserted about
                -- the surviving role is lost.
                IF (v_elem ->> 'slug') = (v_survivor ->> 'slug')
                   AND jsonb_array_length(coalesce(v_elem -> 'dimensions', '[]'::jsonb)) > 0 THEN
                    SELECT coalesce(jsonb_agg(DISTINCT d ORDER BY d), '[]'::jsonb)
                    INTO v_out
                    FROM jsonb_array_elements(
                        coalesce(v_survivor -> 'dimensions', '[]'::jsonb)
                        || coalesce(v_elem -> 'dimensions', '[]'::jsonb)) AS d;

                    v_payload := jsonb_set(v_payload,
                        ARRAY[v_array, v_kept::text, 'dimensions'], v_out);
                END IF;

                v_drops := v_drops || jsonb_build_object(
                    'path', format('%s[%s].slug', v_array, v_i),
                    'table', v_array,
                    'dropped_slug', v_elem ->> 'slug',
                    'dropped_name', v_elem ->> 'name',
                    'kept_slug', v_survivor ->> 'slug',
                    'similarity', round(v_best, 3),
                    'reason', CASE
                        WHEN (v_elem ->> 'slug') = (v_survivor ->> 'slug')
                        THEN 'substituted_onto_same_slug'
                        ELSE 'near_duplicate_in_payload'
                    END);
            ELSE
                v_keep_idx := v_keep_idx || v_i;
            END IF;
        END LOOP;

        IF array_length(v_keep_idx, 1) IS DISTINCT FROM jsonb_array_length(v_payload -> v_array) THEN
            SELECT coalesce(jsonb_agg(e.value ORDER BY e.ord), '[]'::jsonb)
            INTO v_out
            FROM jsonb_array_elements(v_payload -> v_array) WITH ORDINALITY AS e(value, ord)
            WHERE (e.ord - 1)::int = ANY (v_keep_idx);

            v_payload := jsonb_set(v_payload, ARRAY[v_array], v_out);
        END IF;
    END LOOP;

    -- Advisory candidates for a slug that Phase C then dropped are noise; the
    -- agent is not going to mint it at all. Keep only candidates whose subject
    -- survived into the write.
    IF jsonb_array_length(v_cands) > 0 AND jsonb_array_length(v_drops) > 0 THEN
        SELECT coalesce(jsonb_agg(c.value ORDER BY c.ord), '[]'::jsonb)
        INTO v_cands
        FROM jsonb_array_elements(v_cands) WITH ORDINALITY AS c(value, ord)
        WHERE EXISTS (
            SELECT 1
            FROM jsonb_array_elements(coalesce(v_payload -> (c.value ->> 'table'), '[]'::jsonb)) AS e
            WHERE e ->> 'slug' = c.value ->> 'minted_slug'
        );
    END IF;

    -- ---- Writes. Past this point any failure raises and rolls back the whole
    -- function call (single atomic statement from the caller's view).

    -- Insert exactly one new classification row. Append-only: never update/delete.
    INSERT INTO public.classifications (job_posting_id, model, prompt_version, seniority, notes)
    VALUES (
        v_posting_id,
        p_model,
        p_prompt_version,
        v_seniority,
        nullif(btrim(coalesce(v_notes, '')), '')
    )
    RETURNING id INTO v_class_id;

    -- Canonical roles: get-or-create, detect newly minted via xmax = 0 on the
    -- upserted row, attach dimensions, attach the role join row.
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(v_payload -> 'canonical_roles', '[]'::jsonb))
    LOOP
        v_slug := v_elem ->> 'slug';
        v_name := v_elem ->> 'name';

        -- xmax = 0 distinguishes a freshly inserted row from a conflict no-op.
        -- The no-op DO UPDATE SET slug = EXCLUDED.slug takes a row lock and always
        -- returns a row; created_at and name on the existing row are preserved.
        INSERT INTO public.canonical_roles (slug, name)
        VALUES (v_slug, v_name)
        ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
        RETURNING id, (xmax = 0) INTO v_id, v_inserted;

        IF v_inserted THEN
            v_new_roles := v_new_roles || jsonb_build_object('slug', v_slug, 'name', v_name);
        END IF;

        FOR v_dim IN SELECT jsonb_array_elements_text(coalesce(v_elem -> 'dimensions', '[]'::jsonb))
        LOOP
            INSERT INTO public.canonical_role_dimensions (canonical_role_id, dimension_id)
            SELECT v_id, rd.id FROM public.role_dimensions rd WHERE rd.slug = v_dim
            ON CONFLICT DO NOTHING;
        END LOOP;

        INSERT INTO public.job_posting_roles (classification_id, role_id)
        VALUES (v_class_id, v_id)
        ON CONFLICT DO NOTHING;
    END LOOP;

    -- Specializations.
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(v_payload -> 'specializations', '[]'::jsonb))
    LOOP
        v_slug := v_elem ->> 'slug';
        v_name := v_elem ->> 'name';

        INSERT INTO public.specializations (slug, name)
        VALUES (v_slug, v_name)
        ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
        RETURNING id, (xmax = 0) INTO v_id, v_inserted;

        IF v_inserted THEN
            v_new_specs := v_new_specs || jsonb_build_object('slug', v_slug, 'name', v_name);
        END IF;

        INSERT INTO public.job_posting_specializations (classification_id, specialization_id)
        VALUES (v_class_id, v_id)
        ON CONFLICT DO NOTHING;
    END LOOP;

    -- Skills.
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(v_payload -> 'skills', '[]'::jsonb))
    LOOP
        v_slug := v_elem ->> 'slug';
        v_name := v_elem ->> 'name';

        INSERT INTO public.skills (slug, name)
        VALUES (v_slug, v_name)
        ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
        RETURNING id, (xmax = 0) INTO v_id, v_inserted;

        IF v_inserted THEN
            v_new_skills := v_new_skills || jsonb_build_object('slug', v_slug, 'name', v_name);
        END IF;

        INSERT INTO public.job_posting_skills (classification_id, skill_id)
        VALUES (v_class_id, v_id)
        ON CONFLICT DO NOTHING;
    END LOOP;

    -- ---- Advisory audit (migration 000039). --------------------------------
    --
    -- Every near match the gate showed this call, and what the worker did with
    -- it, recorded before the envelope is built. Until this existed the
    -- `similarity_candidates` array was returned and then discarded, so the
    -- only record of an ignored advisory was the worker's own account of it --
    -- unfalsifiable, and on 2026-09-21 demonstrably wrong.
    --
    -- Two sources, one shape. An advisory candidate is a near match the gate
    -- SHOWED and left the worker free to ignore; a substitution is one the gate
    -- ACTED on. Both are "this proposal had a neighbour", and keeping them in
    -- one table is what makes the mint rate a single GROUP BY rather than a
    -- join between two tables with different columns.
    --
    -- `outcome` is read off the write that just happened, not predicted from
    -- the gate's decision: 'minted' iff the proposed slug is in the newly
    -- minted set the loops above accumulated. An advisory candidate therefore
    -- reads 'minted' because Phase A leaves advisory proposals alone, and a
    -- substitution reads 'reused' because the payload element was repointed
    -- onto a row that already existed. If either of those ever stops holding,
    -- this column says so instead of hiding it.
    --
    -- name_similarity is recomputed here rather than carried through the
    -- envelope. Phase A scores it (`name_sim`) but reports only `similarity`,
    -- the greater of the two axes, and widening the returned envelope is a
    -- contract change. public.similarity is IMMUTABLE and the inputs are the
    -- same two strings, so the recomputation is the same number.
    --
    -- Append-only, like every other write in this function: rows are inserted
    -- and never updated or deleted. A later call about the same term is
    -- another row, and the history is the sequence.
    IF jsonb_array_length(v_cands) > 0 OR jsonb_array_length(v_subs) > 0 THEN
        INSERT INTO public.taxonomy_similarity_advisories (
            classification_id, job_posting_id, table_name,
            proposed_slug, proposed_name, candidate_slug, candidate_name,
            candidate_rank, candidate_source, match_kind,
            slug_similarity, name_similarity, advisory_similarity,
            outcome, model, prompt_version)
        SELECT v_class_id, v_posting_id, a.tbl,
               a.proposed_slug, a.proposed_name, a.candidate_slug, a.candidate_name,
               a.candidate_rank, a.candidate_source, a.match_kind,
               a.slug_similarity, a.name_similarity, a.advisory_similarity,
               CASE WHEN EXISTS (
                        SELECT 1
                        FROM jsonb_array_elements(
                                 CASE a.tbl
                                     WHEN 'canonical_roles' THEN v_new_roles
                                     WHEN 'specializations' THEN v_new_specs
                                     ELSE v_new_skills
                                 END) AS n
                        WHERE n ->> 'slug' = a.proposed_slug)
                    THEN 'minted' ELSE 'reused'
               END,
               p_model, p_prompt_version
        FROM (
            -- Advisory candidates: one row per (proposal x candidate shown),
            -- `candidate_rank` being the order they were reported in, which is
            -- the order the worker read them.
            SELECT c.value ->> 'table'        AS tbl,
                   c.value ->> 'minted_slug'  AS proposed_slug,
                   c.value ->> 'minted_name'  AS proposed_name,
                   k.value ->> 'slug'         AS candidate_slug,
                   k.value ->> 'name'         AS candidate_name,
                   k.ord::int                 AS candidate_rank,
                   'advisory'::text           AS candidate_source,
                   k.value ->> 'match'        AS match_kind,
                   (k.value ->> 'slug_similarity')::numeric AS slug_similarity,
                   round(public.similarity(
                       coalesce(c.value ->> 'minted_name', ''),
                       coalesce(k.value ->> 'name', ''))::numeric, 3) AS name_similarity,
                   (k.value ->> 'similarity')::numeric AS advisory_similarity
            FROM jsonb_array_elements(v_cands) AS c(value)
            CROSS JOIN LATERAL jsonb_array_elements(c.value -> 'candidates')
                 WITH ORDINALITY AS k(value, ord)
            UNION ALL
            -- Substitutions: exactly one candidate, the row Phase A remapped
            -- onto, so the rank is always 1.
            SELECT s.value ->> 'table',
                   s.value ->> 'proposed_slug',
                   s.value ->> 'proposed_name',
                   s.value ->> 'substituted_slug',
                   s.value ->> 'substituted_name',
                   1,
                   'substitution',
                   s.value ->> 'match',
                   round(public.similarity(
                       coalesce(s.value ->> 'proposed_slug', ''),
                       coalesce(s.value ->> 'substituted_slug', ''))::numeric, 3),
                   round(public.similarity(
                       coalesce(s.value ->> 'proposed_name', ''),
                       coalesce(s.value ->> 'substituted_name', ''))::numeric, 3),
                   (s.value ->> 'similarity')::numeric
            FROM jsonb_array_elements(v_subs) AS s(value)
        ) a;
    END IF;

    -- The three gate arrays are added only when non-empty, so a payload the
    -- gate did not touch returns byte-identical to what 000031 returned.
    v_result := jsonb_build_object(
        'ok', true,
        'classification_id', v_class_id,
        'posting_id', v_posting_id,
        'new_taxonomy', jsonb_build_object(
            'canonical_roles', v_new_roles,
            'specializations', v_new_specs,
            'skills', v_new_skills
        )
    );

    IF jsonb_array_length(v_subs) > 0 THEN
        v_result := v_result || jsonb_build_object('substitutions', v_subs);
    END IF;
    IF jsonb_array_length(v_drops) > 0 THEN
        v_result := v_result || jsonb_build_object('dropped', v_drops);
    END IF;
    IF jsonb_array_length(v_cands) > 0 THEN
        v_result := v_result || jsonb_build_object('similarity_candidates', v_cands);
    END IF;

    RETURN v_result;
END;
$$;

-- CREATE OR REPLACE preserves the function's ACL, and the action role's EXECUTE
-- grant is on the mcp.save_enrichment wrapper, not on this implementation, so no
-- grant changes hands here. The REVOKEs below restate 000026's boundary so this
-- migration is self-sufficient if replayed against a cluster where the wrapper
-- was rebuilt.
REVOKE ALL ON FUNCTION mcp.save_enrichment_unlocked(jsonb, text, text) FROM PUBLIC;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'market_scout_actions') THEN
        REVOKE ALL ON FUNCTION mcp.save_enrichment_unlocked(jsonb, text, text)
            FROM market_scout_actions;
    END IF;
END;
$$;

-- ---------------------------------------------------------------------------
-- 7. Assert the premise the gate now rests on.
--
-- The rule above is only affordable while the vocabulary it guards is clean --
-- that is 000038's own argument for excluding roles, and it applies to the
-- wider key as much as to the narrower one. This asserts rather than assumes:
-- under the key the gate now uses, no two rows of any of the three taxonomy
-- tables share a normalized name. If sections 1-5 missed a cluster, the
-- migration fails here instead of shipping a gate that fires on existing
-- corruption.
--
-- The second assertion is the punctuation property, pinned so that a future
-- widening of this normalizer cannot quietly fold the three C-family languages
-- together. 'c', 'cpp' and 'csharp' are the canonical case; a database where
-- they do not all exist skips the check rather than failing it, because
-- nothing seeds the taxonomy from a migration.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    v_bad text;
    v_keys int;
BEGIN
    WITH all_rows AS (
        SELECT 'canonical_roles'::text AS tbl, slug, name FROM canonical_roles
        UNION ALL SELECT 'specializations', slug, name FROM specializations
        UNION ALL SELECT 'skills', slug, name FROM skills
    )
    SELECT string_agg(format('%s: %s (%s)', tbl, key, slugs), '; ') INTO v_bad
    FROM (
        SELECT tbl,
               lower(regexp_replace(name, '\s', '', 'g')) AS key,
               string_agg(slug, ', ' ORDER BY slug) AS slugs
        FROM all_rows
        GROUP BY 1, 2
        HAVING count(*) > 1
    ) q;
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'normalized-name collisions remain, gate would fire on existing rows: %', v_bad;
    END IF;

    SELECT count(DISTINCT lower(regexp_replace(name, '\s', '', 'g'))) INTO v_keys
    FROM skills WHERE slug IN ('c', 'cpp', 'csharp');
    IF v_keys <> 0 AND v_keys <> (SELECT count(*) FROM skills WHERE slug IN ('c', 'cpp', 'csharp')) THEN
        RAISE EXCEPTION 'the C-family languages no longer have distinct normalized names';
    END IF;
END
$$;
