-- Serialize direct enrichment writes without widening the action-role surface.
--
-- The hardened implementation checks cross-table slug ownership before it mints
-- taxonomy. Concurrent calls could both observe an absent slug, then insert it
-- into different taxonomy tables. Keep the established external function name
-- as the approved action endpoint, but make it a transaction-scoped advisory
-- lock wrapper around the existing hardened body.
--
-- A session-level lock would survive an errored call on a pooled connection;
-- pg_advisory_xact_lock releases automatically when this function's statement
-- transaction ends. The two fixed int4 keys form this function's private lock
-- namespace and intentionally serialize every enrichment save globally.

ALTER FUNCTION mcp.save_enrichment(jsonb, text, text)
    RENAME TO save_enrichment_unlocked;

-- CREATE OR REPLACE is equivalent to CREATE after the rename at runtime. sqlc's
-- offline migration parser does not model ALTER FUNCTION ... RENAME, so this
-- spelling also lets it resolve the established function signature.
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
BEGIN
    PERFORM pg_catalog.pg_advisory_xact_lock(734771, 26);
    RETURN mcp.save_enrichment_unlocked(p_payload, p_model, p_prompt_version);
END;
$$;

-- New functions otherwise receive an implicit PUBLIC EXECUTE grant. The old
-- action-role grant followed the renamed implementation's OID, so remove it
-- when the optional operational role exists. action_role.sql grants the wrapper
-- after migrations, leaving this implementation unreachable to action callers.
REVOKE ALL ON FUNCTION mcp.save_enrichment(jsonb, text, text) FROM PUBLIC;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'market_scout_actions') THEN
        REVOKE ALL ON FUNCTION mcp.save_enrichment_unlocked(jsonb, text, text)
            FROM market_scout_actions;
    END IF;
END;
$$;
