-- Restore the hardened implementation as the one approved external endpoint.
-- The original function's OID (and its PUBLIC revocation) is preserved by the
-- rename; dropping only the wrapper avoids copying a large, security-sensitive
-- implementation body into this migration.

DROP FUNCTION mcp.save_enrichment(jsonb, text, text);

ALTER FUNCTION mcp.save_enrichment_unlocked(jsonb, text, text)
    RENAME TO save_enrichment;

REVOKE ALL ON FUNCTION mcp.save_enrichment(jsonb, text, text) FROM PUBLIC;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'market_scout_actions') THEN
        GRANT EXECUTE ON FUNCTION mcp.save_enrichment(jsonb, text, text)
            TO market_scout_actions;
    END IF;
END;
$$;
