-- Operational setup for the MCP read-only database role.
-- This is not a numbered migration: roles are cluster-level, and credentials
-- must be chosen outside source control.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = 'market_scout_readonly'
    ) THEN
        CREATE ROLE market_scout_readonly LOGIN;
    ELSE
        ALTER ROLE market_scout_readonly
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
    EXECUTE format('REVOKE CREATE, TEMPORARY ON DATABASE %I FROM market_scout_readonly', current_database());
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO market_scout_readonly', current_database());
END
$$;

REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Memberships or owned objects can preserve write paths beyond explicit revokes.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_auth_members
        WHERE member = 'market_scout_readonly'::regrole
    ) THEN
        RAISE EXCEPTION 'market_scout_readonly must not be a member of any other role; revoke memberships before applying read-only grants';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relowner = 'market_scout_readonly'::regrole
    ) THEN
        RAISE EXCEPTION 'market_scout_readonly owns database objects; transfer ownership before applying read-only grants';
    END IF;
END
$$;

-- The ALTER DEFAULT PRIVILEGES below carries no FOR ROLE clause, so it binds to
-- whoever runs this script and raises nothing if that is the wrong role. On
-- managed Postgres or in CI the admin login is often not the migration owner;
-- the script would then write a rule covering nothing that owner creates, and
-- the first later migration to add a function would hand it to PUBLIC unnoticed.
-- Derived from the tables and views migrations created rather than a hardcoded
-- role name. Deliberately not derived from pg_proc: an operator may install
-- extensions under a different role, which would make functions a false signal.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'v')
          AND c.relowner <> current_user::text::regrole
    ) THEN
        RAISE EXCEPTION 'readonly_role.sql must run as the role that owns the migrated objects in schema public; % does not', current_user;
    END IF;
END
$$;

ALTER ROLE market_scout_readonly
    LOGIN
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    NOREPLICATION
    NOBYPASSRLS;

REVOKE ALL PRIVILEGES ON SCHEMA public FROM market_scout_readonly;
REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM market_scout_readonly;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM market_scout_readonly;
REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM market_scout_readonly;

-- Postgres hard-wires an EXECUTE grant to PUBLIC into every routine it creates.
-- The role-scoped revoke above does subtract grants held under the role's own
-- name, but it cannot touch PUBLIC's -- and on a default ACL PUBLIC's is the
-- only one present. Without the two statements below the role can execute every
-- pg_trgm and pgvector function in the schema, and the explicit grants at the
-- bottom of this file describe a boundary that does not exist. It also makes the
-- boundary an accident of history rather than of this script: a database
-- hardened out of band and a database built from this file end up with different
-- function privileges.
--
-- The membership guard above cannot catch this exposure. PUBLIC is an implicit
-- grantee, not a role, and never appears in pg_auth_members.
--
-- ROUTINES, not FUNCTIONS: `ALL FUNCTIONS` silently skips procedures. Verified
-- on PG 17 -- a procedure keeps its PUBLIC grant through any number of
-- `ALL FUNCTIONS` revokes, while aggregates are stripped. `ALL ROUTINES` covers
-- functions, procedures, and aggregates alike.
REVOKE EXECUTE ON ALL ROUTINES IN SCHEMA public FROM PUBLIC;

-- Covers routines created later, by a migration or a new extension -- a related
-- boundary, not the same one. The revoke above covers every routine already in
-- `public` whatever created it; this rule covers only routines created by the
-- role that runs this script, in any schema. The guard above is what makes that
-- role the migration owner.
--
-- Deliberately NOT scoped `IN SCHEMA public`: Postgres builds a schema-scoped
-- default-privilege ACL by merging the REVOKE against an EMPTY acl rather than
-- against the hard-wired default, so a schema-scoped revoke has nothing to
-- subtract and is a verified no-op. Only the global form removes the implicit
-- PUBLIC grant. FUNCTIONS here already means routines -- a procedure created
-- under this rule gets no PUBLIC grant, and the ROUTINES spelling is a synonym
-- that writes the identical catalog row -- so there is no ROUTINES variant to
-- reach for as there is above.
--
-- Global also means every schema this owner creates functions in, `mcp`
-- included. Not a regression: `action_role.sql` already writes the identical
-- catalog row, and the approved mcp functions get their EXECUTE from the
-- explicit per-function grants there, which neither statement here disturbs.
-- That copy names the owner with FOR ROLE, which is what makes it
-- owner-independent; this one is not, and the guard above is what stands in.
-- ALTER DEFAULT PRIVILEGES is idempotent, so a database that gets both scripts
-- ends up with one row either way.
ALTER DEFAULT PRIVILEGES
    REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;

GRANT USAGE ON SCHEMA public TO market_scout_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO market_scout_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO market_scout_readonly;

-- pg_trgm's similarity() is used by the read-only dedup lookup. Grant only the
-- required extension function; the role retains no general function access.
--
-- The grant is on the function, not on pg_trgm's `%` shorthand: `a % b` resolves
-- to similarity_op, which is not granted, so it fails with "permission denied
-- for function similarity_op" -- naming a function the caller never typed. The
-- MCP query tool hands agents arbitrary SQL over DATABASE_URL_RO, so spell out
-- similarity() there.
GRANT EXECUTE ON FUNCTION public.similarity(text, text) TO market_scout_readonly;

-- Delta reads the historical open cohort through this narrowly granted
-- read-model function; every other public function remains unavailable.
GRANT EXECUTE ON FUNCTION public.open_postings_as_of(timestamptz) TO market_scout_readonly;
