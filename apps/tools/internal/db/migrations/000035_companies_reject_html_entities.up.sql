-- Reject HTML character references in the two verbatim-stored display
-- columns on companies (name, industry).
--
-- Why: during the Built In Seattle discovery run an agent passed
-- 'DAT Freight &amp; Analytics' instead of 'DAT Freight & Analytics'. This is
-- not a scrape or storage defect -- the scrape decodes entities correctly and
-- a raw '&' is an ordinary character everywhere downstream (the seed file
-- already carries 75 of them; React escapes on output, so a raw '&' renders
-- correctly while '&amp;' renders as the literal text "&amp;"). The entity
-- was introduced at the call site and the insert silently accepted it. The
-- offending row was corrected by hand before this migration; no row in
-- companies matches the pattern, so the constraint validates cleanly.
--
-- This rejects rather than unescapes, for the same reason mcp.add_company
-- probes rather than trusts: a lossy transform would quietly "fix" caller
-- mistakes and hide them, and it cannot tell a mis-encoded name from one that
-- legitimately contains an entity-shaped substring. Failing loudly puts the
-- decision back on the caller.
--
-- A CHECK rather than a guard inside mcp.add_company, for three reasons:
--   - It covers every write path -- mcp.add_company, cmd/onboard, and the
--     seed file -- not just the MCP boundary.
--   - mcp.add_company is LANGUAGE sql and cannot RAISE. Converting it to
--     plpgsql to add a guard makes the RETURNS TABLE output names (id, name,
--     ats, ...) into plpgsql variables, which then collide with the table
--     columns in `ON CONFLICT (ats, board_token)`; the index-inference clause
--     cannot be table-qualified to disambiguate. The constraint avoids
--     touching that function -- and its SECURITY DEFINER hardening and ACL --
--     entirely.
--   - It matches the precedent set by 000030, which added name-integrity
--     CHECKs rather than validating inside mcp.save_enrichment.
--
-- Scope is the two human-readable columns. Deliberately NOT careers_page_url:
-- '&amp;' inside a URL query string is a legitimate encoding, so the same
-- check there would produce false rejections. ats and board_token are
-- constrained identifiers that cannot carry display text.
--
-- Pattern covers named (&amp;), decimal (&#38;) and hex (&#x26;) references.
-- NULL industry passes: a NULL ~ anything is NULL, and a CHECK only fails on
-- an explicit false.

ALTER TABLE companies
    ADD CONSTRAINT companies_name_no_html_entity
    CHECK (name !~ '&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]{1,30});');

ALTER TABLE companies
    ADD CONSTRAINT companies_industry_no_html_entity
    CHECK (industry !~ '&(#[0-9]+|#[xX][0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]{1,30});');

COMMENT ON CONSTRAINT companies_name_no_html_entity ON companies IS
    'Rejects HTML character references such as &amp;. Pass the decoded character instead.';

COMMENT ON CONSTRAINT companies_industry_no_html_entity ON companies IS
    'Rejects HTML character references such as &amp;. Pass the decoded character instead.';
