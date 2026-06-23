-- Creates the dedicated `mcp` schema that will hold approved SECURITY DEFINER
-- action functions invoked by the MCP server's action role (market_scout_actions).
-- This is the security boundary for MCP writes: later migrations add narrow
-- functions here owned by the database owner; the action role gets only USAGE on
-- this schema plus EXECUTE on each named function, never table-write privileges.
-- See: agent-context/lib/developer-guide.md §2 (Action MCP role, Schema Migrations)
--
-- Baseline only. This migration creates no functions; add_company and
-- save_enrichment land in later numbered migrations (Tasks 3 and 7).
--
-- The schema is owned by the migration-running role (the database owner), so the
-- SECURITY DEFINER functions added later run with owner privileges.
CREATE SCHEMA IF NOT EXISTS mcp;

-- Deny the implicit PUBLIC grants on the schema. Without this, any role
-- (including a leaked action DSN) would inherit USAGE. The action role is
-- granted USAGE explicitly in internal/db/setup/action_role.sql.
REVOKE ALL ON SCHEMA mcp FROM PUBLIC;
