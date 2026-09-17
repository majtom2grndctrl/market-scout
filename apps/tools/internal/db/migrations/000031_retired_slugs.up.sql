-- Retired-slug registry, enforced inside mcp.save_enrichment.
--
-- 000029 deleted 14 taxonomy slugs that violated the classifier contract.
-- 'japan-market' was re-minted as a specialization the next day, despite the
-- agent brief naming it explicitly as the negative example. Prose in a prompt
-- does not prevent recurrence; a constraint does. This is a category violation
-- (geography is never taxonomy, per project.md), not a near-duplicate, so a
-- similarity check at any threshold would never catch it -- it needs its own
-- mechanism.
--
-- retired_slugs is that mechanism's memory. The hardened save_enrichment body
-- (000012, still living under the name save_enrichment_unlocked since 000026)
-- consults it before minting -- never before reusing -- a slug.

-- ---------------------------------------------------------------------------
-- 1. The registry table.
--
-- table_name names the taxonomy table (canonical_roles, specializations,
-- skills) the slug may not be minted into; NULL means every table. The column
-- is nullable, so a slug COULD in principle be retired from one table while
-- staying free to live in another -- e.g. a term that is a legitimate skill
-- but should never be re-minted as a specialization.
--
-- That flexibility is not used by the seed data below. Every one of the 14
-- slugs 000029 removed is wrong for a reason that does not depend on which
-- table it lands in: a geography term is not taxonomy in any table, an
-- umbrella competency fails to discriminate in any table, a factually wrong
-- or bundled concept is wrong regardless of table. Scoping any of them to one
-- table would leave the other two tables open to the exact recurrence this
-- migration exists to prevent. The column stays nullable for a future case
-- that is genuinely table-specific; today every seed row is NULL.
--
-- The unique index treats NULL as a normal value (NULLS NOT DISTINCT, PG15+)
-- so re-running a seed insert can rely on ON CONFLICT rather than silently
-- accumulating duplicate blanket retirements of the same slug.
-- ---------------------------------------------------------------------------

CREATE TABLE retired_slugs (
    id                    bigserial PRIMARY KEY,
    slug                  text NOT NULL,
    table_name            text,
    retired_at            timestamptz NOT NULL DEFAULT now(),
    retired_by_migration  text NOT NULL,
    reason                text NOT NULL,
    CONSTRAINT retired_slugs_table_name_check
        CHECK (table_name IS NULL OR table_name IN ('canonical_roles', 'specializations', 'skills'))
);

CREATE UNIQUE INDEX retired_slugs_slug_table_uidx
    ON retired_slugs (slug, table_name) NULLS NOT DISTINCT;

-- ---------------------------------------------------------------------------
-- 2. Seed: the 14 slugs 000029 removed, with the reasons from its comments.
-- ---------------------------------------------------------------------------

INSERT INTO retired_slugs (slug, table_name, retired_by_migration, reason) VALUES
    ('japan-market', NULL, '000031_retired_slugs',
     'Geography is not taxonomy: creates a second, uncurated geography axis alongside the curated market dimension, which already recovers geography from posting location. A market grouping and a taxonomy grouping on the same term would answer the same question differently.'),
    ('south-korea-market', NULL, '000031_retired_slugs',
     'Geography is not taxonomy: creates a second, uncurated geography axis alongside the curated market dimension, which already recovers geography from posting location. A market grouping and a taxonomy grouping on the same term would answer the same question differently.'),
    ('emerging-technologies', NULL, '000031_retired_slugs',
     'Names no domain, industry, or product area. The specializations axis is domain; a term that means "things that are new" cannot be grouped on and does not narrow anything.'),
    ('vlm-inference', NULL, '000031_retired_slugs',
     'Factually wrong: the source description names vLLM, an inference server, not a vision-language model (VLM). Left in place this is wrong data a future agent would near-match against and legitimately reuse.'),
    ('geo-targeting', NULL, '000031_retired_slugs',
     'Factually wrong: minted from an "SEO/GEO Lead" title and read as geographic targeting, but in a search-marketing title GEO is Generative Engine Optimization. Left in place this is wrong data a future agent would near-match against and legitimately reuse.'),
    ('aws-gcp-azure', NULL, '000031_retired_slugs',
     'Bundled: three cloud platforms in one slug. Each already exists as its own skill; a posting naming all three should carry three tags, and one naming fewer should not be credited with the rest. A bundled slug cannot be filtered or counted honestly.'),
    ('icd-snomed-standards', NULL, '000031_retired_slugs',
     'Bundled: two terminology standards in one slug, overlapping the healthcare-terminology-standards specialization minted by the same run. A bundled slug cannot be filtered or counted honestly.'),
    ('technical-expertise', NULL, '000031_retired_slugs',
     'Umbrella skill: names a competency so broad it cannot discriminate between postings, which is the only thing a skill tag is for.'),
    ('technical-acumen', NULL, '000031_retired_slugs',
     'Umbrella skill: names a competency so broad it cannot discriminate between postings, which is the only thing a skill tag is for.'),
    ('systems-knowledge', NULL, '000031_retired_slugs',
     'Umbrella skill: names a competency so broad it cannot discriminate between postings, which is the only thing a skill tag is for.'),
    ('operations-management', NULL, '000031_retired_slugs',
     'Umbrella skill: names a competency so broad it cannot discriminate between postings, which is the only thing a skill tag is for.'),
    ('infrastructure', NULL, '000031_retired_slugs',
     'Umbrella skill: names a competency so broad it cannot discriminate between postings, which is the only thing a skill tag is for.'),
    ('cross-functional-alignment', NULL, '000031_retired_slugs',
     'Umbrella skill: names a competency so broad it cannot discriminate between postings, which is the only thing a skill tag is for.'),
    ('compliance', NULL, '000031_retired_slugs',
     'Umbrella skill: sits beside the pre-existing soc-2 and iso-27001 skills and the compliance-engineer role, where it adds no signal any of them lack.')
ON CONFLICT (slug, table_name) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Enforce the registry inside the hardened implementation.
--
-- CREATE OR REPLACE on save_enrichment_unlocked (the 000026 rename target),
-- never on the mcp.save_enrichment wrapper: the external function signature
-- and the advisory-lock wrapper from 000026 are untouched. The whole 000012
-- body is reproduced verbatim except for one new accumulating check, added
-- after the existing cross-table slug-ownership checks and before the
-- errors-so-far RETURN.
--
-- The new check only fires for a slug this call would MINT: it first confirms
-- the slug does not already exist in its own target table (reuse is never
-- blocked), then looks up retired_slugs for that slug scoped to this table or
-- to NULL (all tables). A match adds a `retired_slug` error whose message
-- carries the stored reason, so the agent that tripped it learns why instead
-- of just being blocked -- the same shape failure that let 'japan-market'
-- recur: a rule stated only in prose, never enforced.
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

    IF jsonb_array_length(v_errors) > 0 THEN
        RETURN jsonb_build_object('ok', false, 'errors', v_errors);
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
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(p_payload -> 'canonical_roles', '[]'::jsonb))
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
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(p_payload -> 'specializations', '[]'::jsonb))
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
    FOR v_elem IN SELECT * FROM jsonb_array_elements(coalesce(p_payload -> 'skills', '[]'::jsonb))
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

    RETURN jsonb_build_object(
        'ok', true,
        'classification_id', v_class_id,
        'posting_id', v_posting_id,
        'new_taxonomy', jsonb_build_object(
            'canonical_roles', v_new_roles,
            'specializations', v_new_specs,
            'skills', v_new_skills
        )
    );
END;
$$;
