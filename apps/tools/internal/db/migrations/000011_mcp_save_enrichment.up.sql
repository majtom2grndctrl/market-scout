-- Approved MCP action function: mcp.save_enrichment.
-- Second write function the action role (market_scout_actions) may call. It owns
-- the full classifier writeback path that cmd/batch-enrich performs in Go, but
-- behind the SECURITY DEFINER boundary so the action role needs no table-write
-- privileges. The MCP server validates the payload Go-side first; this function
-- re-checks the cross-table and dimension invariants the database also owns, and
-- returns those as structured errors rather than raising, so Go never has to
-- parse exception text to tell a validation failure from a real DB fault.
--
-- Security definer hardening (same model as mcp.add_company, migration 000010):
--   - Owned by the migration-running role (the database owner); runs with owner
--     privileges regardless of caller.
--   - SET search_path = pg_catalog pins resolution; every app object is fully
--     qualified as public.<table>.
--   - EXECUTE is revoked from PUBLIC here; the explicit grant to
--     market_scout_actions lives in internal/db/setup/action_role.sql, rerun
--     after this migration applies.
--
-- Append-only discipline (load-bearing): the function INSERTs exactly one new
-- public.classifications row per call and NEVER updates or deletes prior history.
-- "Current" classification is derived per query (latest classified_at). See the
-- classifications table comment in 000001 and developer-guide §6.2.
--
-- p_payload shape (provenance is passed separately as p_model / p_prompt_version,
-- never inside the payload; skills[].requirement is stripped Go-side and absent
-- here):
--   {
--     "posting_id": 123,
--     "classification": { "seniority": "senior", "notes": "text" | null },
--     "canonical_roles": [ { "slug": "...", "name": "...", "dimensions": ["..."] } ],
--     "specializations": [ { "slug": "...", "name": "..." } ],
--     "skills":          [ { "slug": "...", "name": "..." } ]
--   }
--
-- Return shape on success:
--   { "ok": true, "classification_id": <id>, "posting_id": <id>,
--     "new_taxonomy": { "canonical_roles": [{slug,name}...],
--                       "specializations": [{slug,name}...],
--                       "skills": [{slug,name}...] } }
-- Only entries this call newly minted appear in new_taxonomy.
--
-- Return shape on SQL-level invariant violation (no rows written; the function
-- returns before any insert):
--   { "ok": false, "errors": [ { "path": "...", "code": "...", "message": "..." } ] }
--
-- See: agent-context/lib/developer-guide.md §2 (Action MCP role), §6.2.
CREATE FUNCTION mcp.save_enrichment(
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

-- Deny the implicit PUBLIC EXECUTE grant. The action role's grant is applied
-- explicitly by internal/db/setup/action_role.sql.
REVOKE ALL ON FUNCTION mcp.save_enrichment(jsonb, text, text) FROM PUBLIC;
