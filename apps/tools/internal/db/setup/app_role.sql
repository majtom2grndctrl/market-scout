-- Operational setup for the web app's profile role (market_scout_app), used by
-- DATABASE_URL_APP. Like readonly_role.sql and action_role.sql, this is not a
-- numbered migration: roles are cluster-level, and credentials must be chosen
-- outside source control.
--
-- Security model: the profile lives in the private `app` schema, which the
-- read-only role -- shared by the web app and the MCP read gateway -- cannot
-- reach. This role is the only application role that can. It reads and writes
-- the profile tables by explicit per-table grant and reads the taxonomy tables
-- the profile names. It never writes outside `app`.
--
-- Plain grants, not SECURITY DEFINER functions: the `mcp` schema needs those
-- because the agent writes core tables and the write *shape* needed
-- constraining. The profile has one writer and no provenance requirement.
--
-- Run with the same owner role used for migrations, after applying them.
-- Rerun after any migration that adds a table to `app`: no default privilege
-- grants this role anything, so a later table stays closed until its grant is
-- written below.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = 'market_scout_app'
    ) THEN
        CREATE ROLE market_scout_app LOGIN;
    ELSE
        ALTER ROLE market_scout_app
            LOGIN
            NOSUPERUSER
            NOCREATEDB
            NOCREATEROLE
            NOINHERIT
            NOREPLICATION
            NOBYPASSRLS;
    END IF;
END
$$;

DO $$
BEGIN
    EXECUTE format('REVOKE TEMPORARY ON DATABASE %I FROM PUBLIC', current_database());
    EXECUTE format('REVOKE CREATE, TEMPORARY ON DATABASE %I FROM market_scout_app', current_database());
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO market_scout_app', current_database());
END
$$;

-- Memberships or owned objects can preserve write paths beyond explicit
-- revokes. Ownership is checked across relations, routines, schemas, types, and
-- databases: owning any of them carries privileges no revoke below can remove.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_auth_members
        WHERE member = 'market_scout_app'::regrole
    ) THEN
        RAISE EXCEPTION 'market_scout_app must not be a member of any other role; revoke memberships before applying app grants';
    END IF;

    IF EXISTS (SELECT 1 FROM pg_class WHERE relowner = 'market_scout_app'::regrole)
       OR EXISTS (SELECT 1 FROM pg_proc WHERE proowner = 'market_scout_app'::regrole)
       OR EXISTS (SELECT 1 FROM pg_namespace WHERE nspowner = 'market_scout_app'::regrole)
       OR EXISTS (SELECT 1 FROM pg_type WHERE typowner = 'market_scout_app'::regrole)
       OR EXISTS (SELECT 1 FROM pg_database WHERE datdba = 'market_scout_app'::regrole)
    THEN
        RAISE EXCEPTION 'market_scout_app owns database objects; transfer ownership before applying app grants';
    END IF;
END
$$;

-- Same reason as readonly_role.sql: the default-privilege revoke below binds to
-- whoever runs this script, so the script must run as the migration owner.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_namespace
        WHERE nspname = 'app' AND nspowner = current_user::text::regrole
    ) THEN
        RAISE EXCEPTION 'app_role.sql must run as the role that owns schema app (the migration owner); % does not', current_user;
    END IF;
END
$$;

ALTER ROLE market_scout_app
    LOGIN
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    NOREPLICATION
    NOBYPASSRLS;

-- Clean baseline: nothing in `public` or `mcp`, nothing in `app` until the
-- grants below.
REVOKE ALL PRIVILEGES ON SCHEMA public FROM market_scout_app;
REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM market_scout_app;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM market_scout_app;
REVOKE ALL PRIVILEGES ON ALL ROUTINES IN SCHEMA public FROM market_scout_app;
REVOKE ALL PRIVILEGES ON SCHEMA mcp FROM market_scout_app;
REVOKE ALL PRIVILEGES ON SCHEMA app FROM market_scout_app;
REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA app FROM market_scout_app;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA app FROM market_scout_app;

-- This role belongs to PUBLIC, so a routine that kept Postgres's implicit
-- PUBLIC EXECUTE would be callable by it. Both statements repeat
-- readonly_role.sql's, so "executes no function" does not depend on that
-- script having run: the first strips the grant from routines already in
-- `public` (pg_trgm's and pgvector's included), the second from routines the
-- owner creates later. Both are idempotent.
REVOKE EXECUTE ON ALL ROUTINES IN SCHEMA public FROM PUBLIC;
ALTER DEFAULT PRIVILEGES
    REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;

-- ---------------------------------------------------------------------------
-- Profile tables, one grant per table. Never ON ALL TABLES IN SCHEMA app and
-- never a default privilege: a table added later must not be writable by
-- accident.
-- ---------------------------------------------------------------------------

GRANT USAGE ON SCHEMA app TO market_scout_app;

GRANT SELECT, INSERT, UPDATE, DELETE ON app.past_titles TO market_scout_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.claimed_skills TO market_scout_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.pins TO market_scout_app;

-- ---------------------------------------------------------------------------
-- Taxonomy the profile names: the role to label a pin or past title, the skill
-- a claim matches, and the rank vocabulary a past title takes. Read only.
-- Taxonomy *search* stays on the read-only client; these grants serve the
-- profile read and the pin write, which copies the role's name at pin time.
-- ---------------------------------------------------------------------------

GRANT USAGE ON SCHEMA public TO market_scout_app;

GRANT SELECT ON public.canonical_roles TO market_scout_app;
GRANT SELECT ON public.skills TO market_scout_app;
GRANT SELECT ON public.title_seniority_seeds TO market_scout_app;
