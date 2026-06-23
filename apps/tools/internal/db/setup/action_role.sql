-- Operational setup for the MCP action database role (market_scout_actions),
-- used by DATABASE_URL_ACTIONS. Like readonly_role.sql, this is not a numbered
-- migration: roles are cluster-level, and credentials must be chosen outside
-- source control.
--
-- Security model: this role starts from no privileges and receives only CONNECT
-- on the database and USAGE on the `mcp` schema. It never gets table-write
-- privileges. All MCP writes flow through approved SECURITY DEFINER functions in
-- the `mcp` schema (created by numbered migrations, owned by the database owner),
-- granted to this role one EXECUTE at a time in the section below. A leaked
-- DATABASE_URL_ACTIONS DSN can therefore call only the approved functions, never
-- run arbitrary table writes or DDL.
--
-- Run this script with the same owner role used for migrations, after applying
-- migrations. Rerun it after any migration that adds a new approved MCP function,
-- so the new EXECUTE grant attaches.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_roles
        WHERE rolname = 'market_scout_actions'
    ) THEN
        CREATE ROLE market_scout_actions LOGIN;
    ELSE
        ALTER ROLE market_scout_actions
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

-- Strip database-level privileges to a clean baseline, then grant only CONNECT.
-- TEMPORARY is revoked from PUBLIC too: the action role would otherwise inherit
-- temp-table creation, a write path outside the approved functions.
DO $$
BEGIN
    EXECUTE format('REVOKE TEMPORARY ON DATABASE %I FROM PUBLIC', current_database());
    EXECUTE format('REVOKE CREATE, TEMPORARY ON DATABASE %I FROM market_scout_actions', current_database());
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO market_scout_actions', current_database());
END
$$;

-- Memberships or owned objects can preserve write paths beyond explicit revokes.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_auth_members
        WHERE member = 'market_scout_actions'::regrole
    ) THEN
        RAISE EXCEPTION 'market_scout_actions must not be a member of any other role; revoke memberships before applying action grants';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relowner = 'market_scout_actions'::regrole
    ) THEN
        RAISE EXCEPTION 'market_scout_actions owns database objects; transfer ownership before applying action grants';
    END IF;
END
$$;

ALTER ROLE market_scout_actions
    LOGIN
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOINHERIT
    NOREPLICATION
    NOBYPASSRLS;

-- The action role gets NO privileges on schema public or its objects. It must
-- not read or write application tables directly; the approved mcp functions do
-- that with owner privileges. Revoke any leftover grants to be safe.
REVOKE ALL PRIVILEGES ON SCHEMA public FROM market_scout_actions;
REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM market_scout_actions;
REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM market_scout_actions;
REVOKE ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public FROM market_scout_actions;

-- The only schema privilege the role gets: USAGE on `mcp`, so it can resolve and
-- call the approved functions granted below. No CREATE: the role cannot add
-- objects to the schema.
REVOKE ALL PRIVILEGES ON SCHEMA mcp FROM market_scout_actions;
GRANT USAGE ON SCHEMA mcp TO market_scout_actions;

-- Defense in depth: deny the implicit PUBLIC EXECUTE on FUTURE functions the
-- owner creates. Postgres hard-wires EXECUTE-to-PUBLIC on every new function.
-- Each approved mcp function's migration REVOKEs that, but a future mcp.*
-- migration could forget to — and since market_scout_actions is a member of
-- PUBLIC, that lapse would silently hand it EXECUTE on an unreviewed function.
-- This default-privilege rule strips that implicit PUBLIC EXECUTE, so the only
-- way the action role gets EXECUTE is an explicit per-function GRANT below.
--
-- IMPORTANT — this is intentionally NOT scoped `IN SCHEMA mcp`. Postgres builds a
-- schema-scoped default-privilege ACL by merging the GRANT/REVOKE against an
-- EMPTY acl, not against the hard-wired default (acldefault). A schema-scoped
-- `REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC` therefore has nothing to subtract and
-- is a verified no-op: a new mcp function still gets the hard-wired PUBLIC grant.
-- Only the global (no-schema) form, which merges against acldefault, actually
-- removes it. Verified on the live PG 17 cluster via has_function_privilege on a
-- throwaway mcp function. See aclchk.c SetDefaultACL (def_acl = make_empty_acl()
-- when a schema is given).
--
-- The global FOR ROLE form applies to every schema, but the only functions this
-- owner creates are the mcp.* approved functions (each then gets its explicit
-- GRANT below) plus the pgvector extension's functions in public — which already
-- exist with their PUBLIC grants intact, since default privileges affect only
-- functions created AFTER this runs, never existing ones. ALTER DEFAULT
-- PRIVILEGES is idempotent.
ALTER DEFAULT PRIVILEGES FOR ROLE market_scout
    REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;

-- ---------------------------------------------------------------------------
-- Approved MCP function EXECUTE grants.
--
-- One block per approved function. Never use GRANT EXECUTE ON ALL FUNCTIONS:
-- that would grant execute on functions added later that have not been reviewed
-- for this role. Each function must first be created by a numbered migration.
-- For each, REVOKE EXECUTE FROM PUBLIC, then GRANT EXECUTE to the action role.
--
-- Operators rerun this whole script after applying a migration that adds an
-- approved function, so its EXECUTE grant attaches.
-- ---------------------------------------------------------------------------

-- mcp.add_company (migration 000010): narrow company insert. REVOKE-from-PUBLIC
-- is also in the migration; repeated here so this script is self-sufficient when
-- rerun against an existing function.
REVOKE ALL ON FUNCTION mcp.add_company(text, text, text, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mcp.add_company(text, text, text, text, text) TO market_scout_actions;

-- mcp.save_enrichment (migration 000011): full classifier writeback (get-or-create
-- taxonomy, insert one classification row, attach join rows) behind the SECURITY
-- DEFINER boundary. Append-only: it never updates or deletes classification history.
-- REVOKE-from-PUBLIC is also in the migration; repeated here for self-sufficiency.
REVOKE ALL ON FUNCTION mcp.save_enrichment(jsonb, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mcp.save_enrichment(jsonb, text, text) TO market_scout_actions;
