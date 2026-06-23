-- Approved MCP action function: mcp.add_company.
-- First write function the action role (market_scout_actions) may call. It is the
-- security boundary for MCP company inserts: the action role holds no table-write
-- privileges; this SECURITY DEFINER function performs the INSERT with owner
-- privileges and exposes only this narrow, audited behavior.
--
-- Security definer hardening:
--   - Owned by the migration-running role (the database owner), so it runs with
--     owner privileges regardless of caller.
--   - SET search_path = pg_catalog pins resolution so a caller cannot shadow
--     application objects with same-named objects on their own search_path. Every
--     app object is fully qualified (public.companies, mcp.add_company).
--   - EXECUTE is revoked from PUBLIC here. The explicit grant to
--     market_scout_actions lives in internal/db/setup/action_role.sql, rerun after
--     this migration applies.
--
-- Behavior: append-only insert with conflict no-op. On (ats, board_token)
-- conflict the existing row is returned unchanged and inserted=false. The
-- function NEVER updates existing company metadata — stale-row merge stays a
-- human-owned decision (no DO UPDATE).
--
-- See: agent-context/lib/developer-guide.md §2 (Action MCP role)
CREATE FUNCTION mcp.add_company(
    p_name             text,
    p_ats              text,
    p_board_token      text,
    p_industry         text,
    p_careers_page_url text
)
RETURNS TABLE (
    id               bigint,
    name             text,
    ats              text,
    board_token      text,
    created_at       timestamptz,
    industry         text,
    careers_page_url text,
    inserted         boolean
)
LANGUAGE sql
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
    WITH ins AS (
        INSERT INTO public.companies (name, ats, board_token, industry, careers_page_url)
        VALUES (p_name, p_ats, p_board_token, p_industry, p_careers_page_url)
        ON CONFLICT (ats, board_token) DO NOTHING
        RETURNING
            companies.id,
            companies.name,
            companies.ats,
            companies.board_token,
            companies.created_at,
            companies.industry,
            companies.careers_page_url,
            true AS inserted
    )
    SELECT id, name, ats, board_token, created_at, industry, careers_page_url, inserted
    FROM ins
    UNION ALL
    -- Conflict path: ins is empty, so return the pre-existing canonical row.
    SELECT
        c.id,
        c.name,
        c.ats,
        c.board_token,
        c.created_at,
        c.industry,
        c.careers_page_url,
        false AS inserted
    FROM public.companies c
    WHERE c.ats = p_ats
      AND c.board_token = p_board_token
      AND NOT EXISTS (SELECT 1 FROM ins);
$$;

-- Deny the implicit PUBLIC EXECUTE grant. The action role's grant is applied
-- explicitly by internal/db/setup/action_role.sql.
REVOKE ALL ON FUNCTION mcp.add_company(text, text, text, text, text) FROM PUBLIC;
