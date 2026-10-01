-- One profile per install, in a private schema, and the owner-only functions
-- that merge, retire, and undo taxonomy terms without orphaning it.
--
-- The two halves ship together because the profile is the first taxonomy
-- reference that is not posting history. Repairs so far were hand-written per
-- migration (000025, 000029, 000037, 000040), each naming slugs from the
-- author's own taxonomy. Every install runs every repair, but each install
-- grows its own taxonomy, so a profile clause written by hand into each repair
-- would only ever run against real rows on the author's database -- and on a
-- fork whose owner pinned the merged role it would hit the RESTRICT foreign key
-- below and leave a dirty migration. One set of functions carries every
-- reference a term has, on every install, and is tested once.
--
-- See: agent-context/lib/project.md §Settled architecture (profile and pins,
-- taxonomy repair) and agent-context/plans/done/profile-and-pins.

-- ---------------------------------------------------------------------------
-- 1. The profile.
--
-- A separate schema is the privacy boundary. market_scout_readonly serves both
-- the web app and the MCP read gateway, and readonly_role.sql grants it every
-- table in `public` by default privilege, so a profile table in `public` would
-- reach every agent session and its model provider. A new schema receives
-- nothing from that script; setup/app_role.sql grants market_scout_app these
-- tables one at a time.
--
-- Mutable, unlike posting history: these rows hold what the person claims now.
-- Install-scoped, so no profile key.
--
-- Every taxonomy reference is ON DELETE RESTRICT. A repair that skips the
-- functions below fails instead of orphaning a pin.
-- ---------------------------------------------------------------------------

CREATE SCHEMA app;
REVOKE ALL ON SCHEMA app FROM PUBLIC;

-- A title the person held. The rank vocabulary is the title-stated one
-- (000036), because a résumé title is a title. 'unstated' is the grouping value
-- for a title that states no rank; here absence is NULL, so it is refused.
-- No natural key: two stints under one title are real.
CREATE TABLE app.past_titles (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title_text  text        NOT NULL,
    role_id     bigint      REFERENCES public.canonical_roles (id) ON DELETE RESTRICT,
    seniority   text        REFERENCES public.title_seniority_seeds (slug) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT past_titles_title_text_present CHECK (btrim(title_text) <> ''),
    CONSTRAINT past_titles_seniority_stated CHECK (seniority <> 'unstated')
);

-- A skill the person claims, in their words, optionally matched to a term.
-- One claim per matched skill, and one per text once trimmed and case-folded.
CREATE TABLE app.claimed_skills (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    skill_text  text        NOT NULL,
    skill_id    bigint      REFERENCES public.skills (id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT claimed_skills_skill_text_present CHECK (btrim(skill_text) <> ''),
    CONSTRAINT claimed_skills_skill_unique UNIQUE (skill_id)
);

CREATE UNIQUE INDEX claimed_skills_text_unique
    ON app.claimed_skills (lower(btrim(skill_text)));

-- A canonical role the person watches. pinned_name keeps the role's name at pin
-- time, so a pin whose role is retired still says what it was.
CREATE TABLE app.pins (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    role_id      bigint      REFERENCES public.canonical_roles (id) ON DELETE RESTRICT,
    pinned_name  text        NOT NULL,
    pinned_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pins_pinned_name_present CHECK (btrim(pinned_name) <> ''),
    CONSTRAINT pins_role_unique UNIQUE (role_id)
);

-- ---------------------------------------------------------------------------
-- 2. The repair archive.
--
-- One row per repair call, and below it exactly what that call removed, so
-- undo restores from the repair's own record and nothing else. It lives in
-- `public`, readable by the read-only role, because it holds no profile text:
-- undo leaves profile rows where the repair put them, so nothing about them
-- needs keeping.
--
-- Not 000037's retired_slug_links: that table's own down migration drops it.
-- ---------------------------------------------------------------------------

CREATE TABLE taxonomy_repairs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operation     text        NOT NULL,
    table_name    text        NOT NULL,
    -- Who asked: the migration that called the function, or an operator label.
    -- Copied into retired_slugs.retired_by_migration.
    retired_by    text        NOT NULL,
    performed_at  timestamptz NOT NULL DEFAULT now(),
    undone_at     timestamptz,
    CONSTRAINT taxonomy_repairs_operation_check CHECK (operation IN ('merge', 'retire')),
    CONSTRAINT taxonomy_repairs_table_name_check
        CHECK (table_name IN ('canonical_roles', 'specializations', 'skills')),
    CONSTRAINT taxonomy_repairs_retired_by_present CHECK (btrim(retired_by) <> '')
);

-- One row per term the call named. `applied` terms were deleted, and their row
-- is what undo puts back. `absent` terms were not on this install; only the
-- retired-slug record was written. `survivor_absent` is a merge pair skipped
-- because its survivor is not on this install; nothing was written for it.
CREATE TABLE taxonomy_repair_terms (
    repair_id      bigint      NOT NULL REFERENCES taxonomy_repairs (id) ON DELETE RESTRICT,
    slug           text        NOT NULL,
    outcome        text        NOT NULL,
    term_id        bigint,
    term_name      text,
    term_created   timestamptz,
    survivor_slug  text,
    survivor_id    bigint,
    reason         text        NOT NULL,
    PRIMARY KEY (repair_id, slug),
    CONSTRAINT taxonomy_repair_terms_outcome_check
        CHECK (outcome IN ('applied', 'absent', 'survivor_absent')),
    CONSTRAINT taxonomy_repair_terms_applied_has_row
        CHECK ((outcome = 'applied') = (term_id IS NOT NULL
                                        AND term_name IS NOT NULL
                                        AND term_created IS NOT NULL))
);

-- One row per classification link the call moved or deleted. `collided` marks
-- a merge link whose survivor pair already existed: the move was a no-op for
-- it, so undo must not delete that survivor link -- it predates the repair.
-- No foreign key to classifications: the archive must never block a delete.
CREATE TABLE taxonomy_repair_links (
    repair_id          bigint  NOT NULL REFERENCES taxonomy_repairs (id) ON DELETE RESTRICT,
    term_id            bigint  NOT NULL,
    classification_id  bigint  NOT NULL,
    collided           boolean NOT NULL,
    PRIMARY KEY (repair_id, term_id, classification_id)
);

-- Role dimensions cascade on role delete (000001), so a repair that did not
-- archive them would lose them silently.
CREATE TABLE taxonomy_repair_role_dimensions (
    repair_id     bigint  NOT NULL REFERENCES taxonomy_repairs (id) ON DELETE RESTRICT,
    role_id       bigint  NOT NULL,
    dimension_id  bigint  NOT NULL,
    collided      boolean NOT NULL,
    PRIMARY KEY (repair_id, role_id, dimension_id)
);

-- Which repair wrote a retired-slug record, so undo removes exactly its own and
-- leaves an older record for the same slug standing. NULL for every record a
-- migration wrote by hand.
ALTER TABLE retired_slugs
    ADD COLUMN retired_by_repair bigint REFERENCES taxonomy_repairs (id) ON DELETE RESTRICT;

-- ---------------------------------------------------------------------------
-- 3. The reference census.
--
-- The functions are complete only while they know every reference a term has:
-- merge, retire, and undo each handle a reference, and this list is what makes
-- forgetting one loud.
-- This lists every foreign key into the three taxonomy tables that they do not
-- handle; merge and retire refuse to run while it returns a row. A migration
-- that adds a taxonomy reference therefore extends the functions and this list
-- in the same file, or the next repair fails loudly instead of leaving a row
-- pointing at nothing.
-- ---------------------------------------------------------------------------

CREATE FUNCTION public.taxonomy_repair_unhandled_references()
RETURNS TABLE (constraint_name text, referencing text, referenced_table text)
LANGUAGE sql
STABLE
SET search_path = pg_catalog
AS $$
    SELECT c.conname::text,
           format('%I.%I', n.nspname, cl.relname),
           rc.relname::text
    FROM pg_constraint c
    JOIN pg_class cl ON cl.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = cl.relnamespace
    JOIN pg_class rc ON rc.oid = c.confrelid
    JOIN pg_namespace rn ON rn.oid = rc.relnamespace
    WHERE c.contype = 'f'
      AND rn.nspname = 'public'
      AND rc.relname IN ('canonical_roles', 'specializations', 'skills')
      AND NOT (
          array_length(c.conkey, 1) = 1
          AND (n.nspname::text, cl.relname::text,
               (SELECT a.attname::text FROM pg_attribute a
                WHERE a.attrelid = c.conrelid AND a.attnum = c.conkey[1]),
               rc.relname::text)
              IN (VALUES
                  ('public', 'job_posting_roles',           'role_id',           'canonical_roles'),
                  ('public', 'canonical_role_dimensions',   'canonical_role_id', 'canonical_roles'),
                  ('public', 'job_posting_specializations', 'specialization_id', 'specializations'),
                  ('public', 'job_posting_skills',          'skill_id',          'skills'),
                  ('app',    'past_titles',                 'role_id',           'canonical_roles'),
                  ('app',    'pins',                        'role_id',           'canonical_roles'),
                  ('app',    'claimed_skills',              'skill_id',          'skills'))
      )
    ORDER BY 1;
$$;

-- Shared preamble for merge and retire: argument checks, the census, and the
-- locks. Returns the link table and column for the taxonomy table.
--
-- The advisory lock is mcp.save_enrichment's own (000026), so a repair and a
-- save never interleave: a save in flight finishes before the repair reads its
-- links. The precedent repairs copied links in one statement and deleted them
-- in the next, so a link committed between the two was deleted unrecorded.
--
-- A writer that skips that lock -- cmd/batch-enrich writes directly -- meets
-- the FOR UPDATE each repair takes on the terms it names, which conflicts with
-- the FOR KEY SHARE a foreign-key check takes on the referenced row. A link it
-- has in flight commits before the repair moves anything; one it starts later
-- waits, then fails the foreign key. Only rows that exist can be locked, and
-- that writer skips the retired-slug gate too, so it can still mint a slug a
-- repair has just retired. It stays unused until it writes through
-- mcp.save_enrichment.
CREATE FUNCTION public.taxonomy_repair_begin(
    p_table      text,
    p_retired_by text,
    OUT link_table  text,
    OUT link_column text
)
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    v_unhandled text;
BEGIN
    IF p_table IS NULL OR p_table NOT IN ('canonical_roles', 'specializations', 'skills') THEN
        RAISE EXCEPTION 'taxonomy repair: unknown table %; expected canonical_roles, specializations, or skills', p_table;
    END IF;
    IF p_retired_by IS NULL OR btrim(p_retired_by) = '' THEN
        RAISE EXCEPTION 'taxonomy repair: retired_by is required; name the migration or operator making the repair';
    END IF;

    SELECT string_agg(format('%s on %s -> %s', u.constraint_name, u.referencing, u.referenced_table), '; ')
    INTO v_unhandled
    FROM public.taxonomy_repair_unhandled_references() u;
    IF v_unhandled IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy repair: foreign keys the repair functions do not handle: %. Extend the functions and taxonomy_repair_unhandled_references() in the migration that added them.', v_unhandled;
    END IF;

    PERFORM pg_advisory_xact_lock(734771, 26);

    link_table := CASE p_table
        WHEN 'canonical_roles' THEN 'job_posting_roles'
        WHEN 'specializations' THEN 'job_posting_specializations'
        ELSE 'job_posting_skills'
    END;
    link_column := CASE p_table
        WHEN 'canonical_roles' THEN 'role_id'
        WHEN 'specializations' THEN 'specialization_id'
        ELSE 'skill_id'
    END;
END;
$$;

-- ---------------------------------------------------------------------------
-- 4. Merge.
--
--   SELECT public.taxonomy_merge('skills',
--       '[{"slug": "golang", "into": "go", "reason": "alias"}]',
--       '000050_merge_go_aliases');
--
-- Moves every reference from each merged term to its survivor, deletes the
-- merged term, and records its slug as retired. Returns the repair id, which
-- taxonomy_undo takes.
--
--   posting links, role dimensions  move; the survivor's row stands on collision
--   past titles                     re-point
--   pins, claimed skills            re-point; deleted when the survivor already holds one
--
-- A merged term absent from this install records the slug as retired and
-- changes nothing else. A survivor absent from this install skips the pair
-- with a notice. Neither present: the merged slug is still recorded, since a
-- fork may mint it later. A map is refused whole when it chains -- a survivor
-- merged elsewhere in the same map -- merges a term into itself, names a slug
-- twice, or names something that is not a slug.
--
-- Write the reason as a fragment; the stored text adds its own punctuation.
-- ---------------------------------------------------------------------------

CREATE FUNCTION public.taxonomy_merge(p_table text, p_map jsonb, p_retired_by text)
RETURNS bigint
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    v_link_table  text;
    v_link_column text;
    v_repair_id   bigint;
    v_bad         text;
    v_pair          record;
    v_term_id       bigint;
    v_term_name     text;
    v_term_created  timestamptz;
    v_survivor_id   bigint;
    v_reason        text;
BEGIN
    IF jsonb_typeof(p_map) IS DISTINCT FROM 'array' OR jsonb_array_length(p_map) = 0 THEN
        RAISE EXCEPTION 'taxonomy_merge: map must be a non-empty JSON array of {"slug", "into"} objects, each with an optional "reason"';
    END IF;

    -- ON COMMIT DROP cleans up after an error; the explicit DROP at the end
    -- lets a second call in the same transaction create the table again.
    CREATE TEMP TABLE taxonomy_merge_map ON COMMIT DROP AS
    SELECT e.ord, btrim(e.value ->> 'slug') AS slug, btrim(e.value ->> 'into') AS into_slug,
           nullif(btrim(e.value ->> 'reason'), '') AS reason
    FROM jsonb_array_elements(p_map) WITH ORDINALITY AS e(value, ord);

    SELECT string_agg(ord::text, ', ') INTO v_bad
    FROM taxonomy_merge_map WHERE coalesce(slug, '') = '' OR coalesce(into_slug, '') = '';
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_merge: entries % need both "slug" and "into"', v_bad;
    END IF;

    SELECT string_agg(slug, ', ') INTO v_bad FROM taxonomy_merge_map WHERE slug = into_slug;
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_merge: a term cannot merge into itself: %', v_bad;
    END IF;

    SELECT string_agg(DISTINCT slug, ', ') INTO v_bad
    FROM taxonomy_merge_map m
    WHERE (SELECT count(*) FROM taxonomy_merge_map x WHERE x.slug = m.slug) > 1;
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_merge: slug named more than once in the map: %', v_bad;
    END IF;

    SELECT string_agg(m.into_slug, ', ') INTO v_bad
    FROM taxonomy_merge_map m
    WHERE EXISTS (SELECT 1 FROM taxonomy_merge_map x WHERE x.slug = m.into_slug);
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_merge: map chains; survivor is itself merged in the same map: %', v_bad;
    END IF;

    SELECT string_agg(x.s, ', ') INTO v_bad
    FROM (SELECT slug AS s FROM taxonomy_merge_map UNION SELECT into_slug FROM taxonomy_merge_map) x
    WHERE x.s !~ '^[a-z0-9]+(-[a-z0-9]+)*$';
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_merge: not a slug (lowercase letters and digits, single hyphens): %', v_bad;
    END IF;

    SELECT b.link_table, b.link_column INTO v_link_table, v_link_column
    FROM public.taxonomy_repair_begin(p_table, p_retired_by) b;

    INSERT INTO public.taxonomy_repairs (operation, table_name, retired_by)
    VALUES ('merge', p_table, p_retired_by)
    RETURNING id INTO v_repair_id;

    -- Every named term, locked in one statement in slug order, so a repair and
    -- any other writer that locks terms always take them the same way round.
    EXECUTE format(
        'SELECT 1 FROM public.%I
         WHERE slug IN (SELECT slug FROM taxonomy_merge_map UNION SELECT into_slug FROM taxonomy_merge_map)
         ORDER BY slug FOR UPDATE', p_table);

    FOR v_pair IN SELECT * FROM taxonomy_merge_map ORDER BY ord LOOP
        v_reason := CASE
            WHEN v_pair.reason IS NULL THEN format('Merged into %L. Use %L.', v_pair.into_slug, v_pair.into_slug)
            ELSE format('Merged into %L: %s. Use %L.', v_pair.into_slug, rtrim(v_pair.reason, '.'), v_pair.into_slug)
        END;

        EXECUTE format('SELECT id, name, created_at FROM public.%I WHERE slug = $1', p_table)
        INTO v_term_id, v_term_name, v_term_created USING v_pair.slug;
        EXECUTE format('SELECT id FROM public.%I WHERE slug = $1', p_table)
        INTO v_survivor_id USING v_pair.into_slug;

        IF v_term_id IS NULL THEN
            RAISE NOTICE 'taxonomy_merge: % % is not on this install; slug recorded as retired, nothing else changed',
                p_table, v_pair.slug;
            INSERT INTO public.taxonomy_repair_terms (repair_id, slug, outcome, survivor_slug, reason)
            VALUES (v_repair_id, v_pair.slug, 'absent', v_pair.into_slug, v_reason);
            INSERT INTO public.retired_slugs (slug, table_name, retired_by_migration, reason, retired_by_repair)
            VALUES (v_pair.slug, p_table, p_retired_by, v_reason, v_repair_id)
            ON CONFLICT (slug, table_name) DO NOTHING;
            CONTINUE;
        END IF;

        IF v_survivor_id IS NULL THEN
            RAISE NOTICE 'taxonomy_merge: % % has no survivor % on this install; pair skipped',
                p_table, v_pair.slug, v_pair.into_slug;
            INSERT INTO public.taxonomy_repair_terms (repair_id, slug, outcome, survivor_slug, reason)
            VALUES (v_repair_id, v_pair.slug, 'survivor_absent', v_pair.into_slug, v_reason);
            CONTINUE;
        END IF;

        -- Posting links: archive, copy to the survivor, then remove. The term
        -- row locks above keep a new link from landing between the three.
        EXECUTE format(
            'INSERT INTO public.taxonomy_repair_links (repair_id, term_id, classification_id, collided)
             SELECT $1, $2, j.classification_id,
                    EXISTS (SELECT 1 FROM public.%1$I x
                            WHERE x.classification_id = j.classification_id AND x.%2$I = $3)
             FROM public.%1$I j WHERE j.%2$I = $2', v_link_table, v_link_column)
        USING v_repair_id, v_term_id, v_survivor_id;
        EXECUTE format(
            'INSERT INTO public.%1$I (classification_id, %2$I)
             SELECT classification_id, $2 FROM public.%1$I WHERE %2$I = $1
             ON CONFLICT DO NOTHING', v_link_table, v_link_column)
        USING v_term_id, v_survivor_id;
        EXECUTE format('DELETE FROM public.%1$I WHERE %2$I = $1', v_link_table, v_link_column)
        USING v_term_id;

        IF p_table = 'canonical_roles' THEN
            INSERT INTO public.taxonomy_repair_role_dimensions (repair_id, role_id, dimension_id, collided)
            SELECT v_repair_id, crd.canonical_role_id, crd.dimension_id,
                   EXISTS (SELECT 1 FROM public.canonical_role_dimensions x
                           WHERE x.canonical_role_id = v_survivor_id AND x.dimension_id = crd.dimension_id)
            FROM public.canonical_role_dimensions crd
            WHERE crd.canonical_role_id = v_term_id;
            INSERT INTO public.canonical_role_dimensions (canonical_role_id, dimension_id)
            SELECT v_survivor_id, dimension_id
            FROM public.canonical_role_dimensions WHERE canonical_role_id = v_term_id
            ON CONFLICT DO NOTHING;
            DELETE FROM public.canonical_role_dimensions WHERE canonical_role_id = v_term_id;

            UPDATE app.past_titles SET role_id = v_survivor_id WHERE role_id = v_term_id;

            IF EXISTS (SELECT 1 FROM app.pins WHERE role_id = v_survivor_id) THEN
                DELETE FROM app.pins WHERE role_id = v_term_id;
            ELSE
                UPDATE app.pins SET role_id = v_survivor_id WHERE role_id = v_term_id;
            END IF;
        ELSIF p_table = 'skills' THEN
            IF EXISTS (SELECT 1 FROM app.claimed_skills WHERE skill_id = v_survivor_id) THEN
                DELETE FROM app.claimed_skills WHERE skill_id = v_term_id;
            ELSE
                UPDATE app.claimed_skills SET skill_id = v_survivor_id WHERE skill_id = v_term_id;
            END IF;
        END IF;

        INSERT INTO public.taxonomy_repair_terms (
            repair_id, slug, outcome, term_id, term_name, term_created,
            survivor_slug, survivor_id, reason)
        VALUES (v_repair_id, v_pair.slug, 'applied', v_term_id, v_term_name, v_term_created,
                v_pair.into_slug, v_survivor_id, v_reason);

        EXECUTE format('DELETE FROM public.%I WHERE id = $1', p_table) USING v_term_id;

        INSERT INTO public.retired_slugs (slug, table_name, retired_by_migration, reason, retired_by_repair)
        VALUES (v_pair.slug, p_table, p_retired_by, v_reason, v_repair_id)
        ON CONFLICT (slug, table_name) DO NOTHING;
    END LOOP;

    DROP TABLE taxonomy_merge_map;
    RETURN v_repair_id;
END;
$$;

-- ---------------------------------------------------------------------------
-- 5. Retire.
--
--   SELECT public.taxonomy_retire('skills',
--       '[{"slug": "technical-acumen", "reason": "Umbrella skill: cannot discriminate."}]',
--       '000051_retire_umbrella_skills');
--
-- Deletes each term with its posting links and role dimensions, and records
-- its slug as retired. Profile rows stay visible: a past title or claimed skill
-- loses its reference and keeps its text, and a pin loses its role and keeps
-- its pinned-at name. A term absent from this install records the slug and
-- changes nothing else.
-- ---------------------------------------------------------------------------

CREATE FUNCTION public.taxonomy_retire(p_table text, p_terms jsonb, p_retired_by text)
RETURNS bigint
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    v_link_table  text;
    v_link_column text;
    v_repair_id   bigint;
    v_bad         text;
    v_entry         record;
    v_term_id       bigint;
    v_term_name     text;
    v_term_created  timestamptz;
BEGIN
    IF jsonb_typeof(p_terms) IS DISTINCT FROM 'array' OR jsonb_array_length(p_terms) = 0 THEN
        RAISE EXCEPTION 'taxonomy_retire: terms must be a non-empty JSON array of {"slug", "reason"} objects';
    END IF;

    -- Same temp-table lifecycle as taxonomy_merge.
    CREATE TEMP TABLE taxonomy_retire_list ON COMMIT DROP AS
    SELECT e.ord, btrim(e.value ->> 'slug') AS slug, nullif(btrim(e.value ->> 'reason'), '') AS reason
    FROM jsonb_array_elements(p_terms) WITH ORDINALITY AS e(value, ord);

    SELECT string_agg(ord::text, ', ') INTO v_bad
    FROM taxonomy_retire_list WHERE coalesce(slug, '') = '' OR reason IS NULL;
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_retire: entries % need both "slug" and "reason"; the reason is what a blocked mint reads back', v_bad;
    END IF;

    SELECT string_agg(DISTINCT slug, ', ') INTO v_bad
    FROM taxonomy_retire_list r
    WHERE (SELECT count(*) FROM taxonomy_retire_list x WHERE x.slug = r.slug) > 1;
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_retire: slug named more than once: %', v_bad;
    END IF;

    SELECT string_agg(slug, ', ') INTO v_bad
    FROM taxonomy_retire_list WHERE slug !~ '^[a-z0-9]+(-[a-z0-9]+)*$';
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_retire: not a slug (lowercase letters and digits, single hyphens): %', v_bad;
    END IF;

    SELECT b.link_table, b.link_column INTO v_link_table, v_link_column
    FROM public.taxonomy_repair_begin(p_table, p_retired_by) b;

    INSERT INTO public.taxonomy_repairs (operation, table_name, retired_by)
    VALUES ('retire', p_table, p_retired_by)
    RETURNING id INTO v_repair_id;

    EXECUTE format(
        'SELECT 1 FROM public.%I WHERE slug IN (SELECT slug FROM taxonomy_retire_list)
         ORDER BY slug FOR UPDATE', p_table);

    FOR v_entry IN SELECT * FROM taxonomy_retire_list ORDER BY slug LOOP
        EXECUTE format('SELECT id, name, created_at FROM public.%I WHERE slug = $1', p_table)
        INTO v_term_id, v_term_name, v_term_created USING v_entry.slug;

        IF v_term_id IS NULL THEN
            -- Said aloud: a mistyped slug and a term this install never had
            -- look the same in the record.
            RAISE NOTICE 'taxonomy_retire: % % is not on this install; slug recorded as retired, nothing else changed',
                p_table, v_entry.slug;
            INSERT INTO public.taxonomy_repair_terms (repair_id, slug, outcome, reason)
            VALUES (v_repair_id, v_entry.slug, 'absent', v_entry.reason);
        ELSE
            EXECUTE format(
                'INSERT INTO public.taxonomy_repair_links (repair_id, term_id, classification_id, collided)
                 SELECT $1, $2, j.classification_id, false
                 FROM public.%1$I j WHERE j.%2$I = $2', v_link_table, v_link_column)
            USING v_repair_id, v_term_id;
            EXECUTE format('DELETE FROM public.%1$I WHERE %2$I = $1', v_link_table, v_link_column)
            USING v_term_id;

            IF p_table = 'canonical_roles' THEN
                INSERT INTO public.taxonomy_repair_role_dimensions (repair_id, role_id, dimension_id, collided)
                SELECT v_repair_id, canonical_role_id, dimension_id, false
                FROM public.canonical_role_dimensions WHERE canonical_role_id = v_term_id;
                DELETE FROM public.canonical_role_dimensions WHERE canonical_role_id = v_term_id;

                UPDATE app.past_titles SET role_id = NULL WHERE role_id = v_term_id;
                UPDATE app.pins SET role_id = NULL WHERE role_id = v_term_id;
            ELSIF p_table = 'skills' THEN
                UPDATE app.claimed_skills SET skill_id = NULL WHERE skill_id = v_term_id;
            END IF;

            INSERT INTO public.taxonomy_repair_terms (
                repair_id, slug, outcome, term_id, term_name, term_created, reason)
            VALUES (v_repair_id, v_entry.slug, 'applied', v_term_id, v_term_name, v_term_created,
                    v_entry.reason);

            EXECUTE format('DELETE FROM public.%I WHERE id = $1', p_table) USING v_term_id;
        END IF;

        INSERT INTO public.retired_slugs (slug, table_name, retired_by_migration, reason, retired_by_repair)
        VALUES (v_entry.slug, p_table, p_retired_by, v_entry.reason, v_repair_id)
        ON CONFLICT (slug, table_name) DO NOTHING;
    END LOOP;

    DROP TABLE taxonomy_retire_list;
    RETURN v_repair_id;
END;
$$;

-- ---------------------------------------------------------------------------
-- 6. Undo.
--
-- Restores the term rows, posting links, and role dimensions a repair
-- recorded, removes the survivor links and dimensions it created, and removes
-- the retired-slug records it wrote. Restores only that: a posting linked to
-- the survivor after the merge stays linked, and a retired-slug record older
-- than the repair stays.
--
-- Profile rows stay where the repair put them. A re-pointed pin stays on the
-- survivor; a retired pin stays retired and can be pinned again.
--
-- Refuses rather than guesses: when a later repair that has not been undone
-- names any of the same terms, or when a deleted slug has been minted again.
-- A second undo of the same repair raises a notice and changes nothing.
-- ---------------------------------------------------------------------------

CREATE FUNCTION public.taxonomy_undo(p_repair_id bigint)
RETURNS void
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    v_repair      record;
    v_link_table  text;
    v_link_column text;
    v_bad         text;
BEGIN
    PERFORM pg_advisory_xact_lock(734771, 26);

    SELECT * INTO v_repair FROM public.taxonomy_repairs WHERE id = p_repair_id FOR UPDATE;
    IF v_repair.id IS NULL THEN
        RAISE EXCEPTION 'taxonomy_undo: no repair %', p_repair_id;
    END IF;
    IF v_repair.undone_at IS NOT NULL THEN
        RAISE NOTICE 'taxonomy_undo: repair % was already undone at %; nothing changed',
            p_repair_id, v_repair.undone_at;
        RETURN;
    END IF;

    SELECT string_agg(DISTINCT l.id::text, ', ') INTO v_bad
    FROM public.taxonomy_repairs l
    JOIN public.taxonomy_repair_terms lt ON lt.repair_id = l.id
    WHERE l.id > p_repair_id
      AND l.undone_at IS NULL
      AND l.table_name = v_repair.table_name
      AND EXISTS (
          SELECT 1 FROM public.taxonomy_repair_terms t
          WHERE t.repair_id = p_repair_id
            AND (t.slug IN (lt.slug, lt.survivor_slug)
                 OR t.survivor_slug IN (lt.slug, lt.survivor_slug)));
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_undo: later repairs % touch the same terms; undo them first', v_bad;
    END IF;

    -- Any of the three tables: mcp.save_enrichment keeps a slug in one table
    -- only, so restoring one that now lives elsewhere would break that rule.
    SELECT string_agg(t.slug, ', ') INTO v_bad
    FROM public.taxonomy_repair_terms t
    WHERE t.repair_id = p_repair_id AND t.outcome = 'applied'
      AND (EXISTS (SELECT 1 FROM public.canonical_roles x WHERE x.slug = t.slug)
           OR EXISTS (SELECT 1 FROM public.specializations x WHERE x.slug = t.slug)
           OR EXISTS (SELECT 1 FROM public.skills x WHERE x.slug = t.slug));
    IF v_bad IS NOT NULL THEN
        RAISE EXCEPTION 'taxonomy_undo: slugs minted again since repair %: %', p_repair_id, v_bad;
    END IF;

    v_link_table := CASE v_repair.table_name
        WHEN 'canonical_roles' THEN 'job_posting_roles'
        WHEN 'specializations' THEN 'job_posting_specializations'
        ELSE 'job_posting_skills'
    END;
    v_link_column := CASE v_repair.table_name
        WHEN 'canonical_roles' THEN 'role_id'
        WHEN 'specializations' THEN 'specialization_id'
        ELSE 'skill_id'
    END;

    -- Original ids, not regenerated: the sequences have advanced past them, so
    -- a restored id cannot collide with a row minted since.
    EXECUTE format(
        'INSERT INTO public.%I (id, slug, name, created_at)
         SELECT term_id, slug, term_name, term_created FROM public.taxonomy_repair_terms
         WHERE repair_id = $1 AND outcome = ''applied''', v_repair.table_name)
    USING p_repair_id;

    -- A classification deleted since -- its posting removed -- has nothing to
    -- link back to.
    EXECUTE format(
        'INSERT INTO public.%1$I (classification_id, %2$I)
         SELECT l.classification_id, l.term_id FROM public.taxonomy_repair_links l
         WHERE l.repair_id = $1
           AND EXISTS (SELECT 1 FROM public.classifications c WHERE c.id = l.classification_id)
         ON CONFLICT DO NOTHING', v_link_table, v_link_column)
    USING p_repair_id;

    EXECUTE format(
        'DELETE FROM public.%1$I j
         USING public.taxonomy_repair_links l, public.taxonomy_repair_terms t
         WHERE l.repair_id = $1 AND NOT l.collided
           AND t.repair_id = $1 AND t.term_id = l.term_id AND t.survivor_id IS NOT NULL
           AND j.classification_id = l.classification_id AND j.%2$I = t.survivor_id',
        v_link_table, v_link_column)
    USING p_repair_id;

    IF v_repair.table_name = 'canonical_roles' THEN
        -- A dimension a later migration removed has nothing to come back to.
        INSERT INTO public.canonical_role_dimensions (canonical_role_id, dimension_id)
        SELECT d.role_id, d.dimension_id FROM public.taxonomy_repair_role_dimensions d
        WHERE d.repair_id = p_repair_id
          AND EXISTS (SELECT 1 FROM public.role_dimensions rd WHERE rd.id = d.dimension_id)
        ON CONFLICT DO NOTHING;

        DELETE FROM public.canonical_role_dimensions crd
        USING public.taxonomy_repair_role_dimensions d, public.taxonomy_repair_terms t
        WHERE d.repair_id = p_repair_id AND NOT d.collided
          AND t.repair_id = p_repair_id AND t.term_id = d.role_id AND t.survivor_id IS NOT NULL
          AND crd.canonical_role_id = t.survivor_id AND crd.dimension_id = d.dimension_id;
    END IF;

    DELETE FROM public.retired_slugs WHERE retired_by_repair = p_repair_id;

    UPDATE public.taxonomy_repairs SET undone_at = now() WHERE id = p_repair_id;
END;
$$;

-- Undoes every standing repair a label made, newest first -- the order the
-- later-repair refusal requires. A repair migration's down calls this with the
-- same label its up passed as retired_by:
--
--   SELECT public.taxonomy_undo_label('000050_merge_go_aliases');
--
-- A label that names nothing raises a notice and changes nothing.
CREATE FUNCTION public.taxonomy_undo_label(p_retired_by text)
RETURNS void
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
DECLARE
    v_id bigint;
    v_found boolean := false;
BEGIN
    PERFORM pg_advisory_xact_lock(734771, 26);
    FOR v_id IN
        SELECT id FROM public.taxonomy_repairs
        WHERE retired_by = p_retired_by AND undone_at IS NULL
        ORDER BY id DESC
    LOOP
        v_found := true;
        PERFORM public.taxonomy_undo(v_id);
    END LOOP;
    IF NOT v_found THEN
        RAISE NOTICE 'taxonomy_undo_label: no standing repair labelled %; nothing changed', p_retired_by;
    END IF;
END;
$$;

-- ---------------------------------------------------------------------------
-- 7. Owner only.
--
-- Postgres grants EXECUTE to PUBLIC on every new function, and every
-- application role belongs to PUBLIC. readonly_role.sql's default privilege
-- removes that grant only once the script has run; revoking here keeps a fresh
-- install closed before it does. The functions run with invoker rights, so
-- EXECUTE alone hands a caller nothing its own grants lack -- but they read and
-- write `app` and the core taxonomy, and keeping them owner-only means that
-- stays true whatever a later grant changes.
-- ---------------------------------------------------------------------------

REVOKE ALL ON FUNCTION public.taxonomy_repair_unhandled_references() FROM PUBLIC;
REVOKE ALL ON FUNCTION public.taxonomy_repair_begin(text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.taxonomy_merge(text, jsonb, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.taxonomy_retire(text, jsonb, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.taxonomy_undo(bigint) FROM PUBLIC;
REVOKE ALL ON FUNCTION public.taxonomy_undo_label(text) FROM PUBLIC;
