-- Approved MCP action function: mcp.record_unsupported_company.
-- It is the narrow SECURITY DEFINER write path for unsupported-companies
-- registry metadata. The action role has no direct table privileges.
-- See: agent-context/lib/developer-guide.md §2 (Action MCP role)
CREATE FUNCTION mcp.record_unsupported_company(
    p_name              text,
    p_url               text,
    p_detected_platform text,
    p_reason            text
)
RETURNS TABLE (
    id                bigint,
    name              text,
    url               text,
    detected_platform text,
    reason            text,
    first_seen_at     timestamptz,
    last_checked_at   timestamptz
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog
AS $$
BEGIN
    IF p_name IS NULL OR p_name !~ '[^[:space:]]' THEN
        RAISE EXCEPTION USING
            ERRCODE = '22023',
            MESSAGE = 'name must not be blank';
    END IF;

    IF p_reason IS NULL OR p_reason NOT IN ('unsupported_ats', 'no_careers') THEN
        RAISE EXCEPTION USING
            ERRCODE = '22023',
            MESSAGE = 'reason must be unsupported_ats or no_careers';
    END IF;

    IF p_reason = 'unsupported_ats'
       AND (p_url IS NULL OR p_url !~ '[^[:space:]]') THEN
        RAISE EXCEPTION USING
            ERRCODE = '22023',
            MESSAGE = 'url is required when reason is unsupported_ats';
    END IF;

    IF p_url IS NOT NULL
       AND (
           p_url !~* '^https?://([^/?#@[:space:]]+(:[^/?#@[:space:]]*)?@)?(\[[0-9A-Fa-f:.]+\]|[^/?#:[:space:]]+)(:[0-9]+)?([/?#][^[:space:]]*)?$'
           OR p_url ~ '%($|[^0-9A-Fa-f]|[0-9A-Fa-f]($|[^0-9A-Fa-f]))'
       ) THEN
        RAISE EXCEPTION USING
            ERRCODE = '22023',
            MESSAGE = 'url must be an absolute http(s) URL';
    END IF;

    RETURN QUERY
    INSERT INTO public.unsupported_companies AS unsupported_company
        (name, url, detected_platform, reason)
    VALUES (p_name, p_url, p_detected_platform, p_reason)
    ON CONFLICT (
        pg_catalog.lower(
            pg_catalog.regexp_replace(
                unsupported_company.name,
                '[^[:alnum:]]',
                '',
                'g'
            )
        )
    )
    DO UPDATE SET
        url               = EXCLUDED.url,
        detected_platform = EXCLUDED.detected_platform,
        reason            = EXCLUDED.reason,
        last_checked_at   = pg_catalog.now()
    RETURNING
        unsupported_company.id,
        unsupported_company.name,
        unsupported_company.url,
        unsupported_company.detected_platform,
        unsupported_company.reason,
        unsupported_company.first_seen_at,
        unsupported_company.last_checked_at;
END;
$$;

-- Deny the implicit PUBLIC EXECUTE grant. The action role's grant is applied
-- explicitly by internal/db/setup/action_role.sql.
REVOKE ALL ON FUNCTION mcp.record_unsupported_company(text, text, text, text) FROM PUBLIC;
