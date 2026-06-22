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

GRANT USAGE ON SCHEMA public TO market_scout_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO market_scout_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO market_scout_readonly;
