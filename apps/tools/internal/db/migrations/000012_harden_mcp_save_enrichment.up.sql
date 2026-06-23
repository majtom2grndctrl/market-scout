-- Harden mcp.save_enrichment.
--
-- The function's documented guarantee is that a payload bypassing Go validation
-- cannot corrupt the taxonomy. The 000011 body re-checked only posting existence,
-- empty/unknown dimensions, and cross-table slug ownership against EXISTING rows.
-- It still trusted Go for slug shape, slug length, within-array duplicates,
-- within-payload cross-array collisions, and seniority — so a payload that
-- bypassed Go (a leaked action DSN calling the function directly) could write a
-- malformed slug, or rely on the table CHECK to RAISE on a bad seniority (which
-- the caller cannot distinguish from a real DB fault: it surfaces as db_error).
--
-- This migration CREATE OR REPLACEs the function with a body that ALSO re-checks
-- those rules BEFORE any write, accumulating into the same structured
-- {ok:false, errors:[{path,code,message}]} result rather than raising. The codes
-- and `<array>[i].slug` path convention match internal/enrich/classify/validate.go
-- exactly, so the two layers agree. All existing behavior is preserved:
-- append-only insert, get-or-create taxonomy, xmax=0 new-taxonomy detection, and
-- the existing posting / dimension / cross-table-against-existing re-checks.
--
-- CREATE OR REPLACE keeps the same (jsonb, text, text) signature, so the existing
-- EXECUTE grant to market_scout_actions is preserved (verified after apply).
CREATE OR REPLACE FUNCTION mcp.save_enrichment(
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
    v_posting_id   bigint;
    v_seniority    text;
    v_notes        text;
    v_errors       jsonb := '[]'::jsonb;
    v_class_id     bigint;
    v_elem         jsonb;
    v_dim          text;
    v_slug         text;
    v_name         text;
    v_id           bigint;
    v_inserted     boolean;
    v_new_roles    jsonb := '[]'::jsonb;
    v_new_specs    jsonb := '[]'::jsonb;
    v_new_skills   jsonb := '[]'::jsonb;
    v_idx          int;
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

-- CREATE OR REPLACE preserves the existing EXECUTE grant to market_scout_actions
-- (same signature). Re-asserted in internal/db/setup/action_role.sql; the REVOKE
-- below keeps the implicit PUBLIC grant denied, matching 000011.
REVOKE ALL ON FUNCTION mcp.save_enrichment(jsonb, text, text) FROM PUBLIC;
