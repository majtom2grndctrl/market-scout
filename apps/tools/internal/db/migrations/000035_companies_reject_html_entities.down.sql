-- Reverse 000035: drop the HTML-character-reference CHECKs on companies.
--
-- Fully reversible. The up migration added two CHECK constraints and two
-- constraint comments; it altered no data, no column, and no function. The
-- constraints only ever rejected writes, never rewrote a value, so no stored
-- name or industry was modified and there is nothing to restore.
--
-- Dropping a constraint also drops its COMMENT, so the comments need no
-- separate statement here.

ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_industry_no_html_entity;
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_name_no_html_entity;
