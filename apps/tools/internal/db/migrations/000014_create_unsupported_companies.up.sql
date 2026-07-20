-- Registry of companies whose careers surface is currently unsupported or absent.
-- This is informational discovery metadata, separate from fetchable companies.
CREATE TABLE unsupported_companies (
    id                bigserial PRIMARY KEY,
    name              text NOT NULL,
    url               text,
    detected_platform text,
    reason            text NOT NULL CHECK (reason IN ('unsupported_ats', 'no_careers')),
    first_seen_at     timestamptz NOT NULL DEFAULT now(),
    last_checked_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_unsupported_companies_normalized_name
    ON unsupported_companies (lower(regexp_replace(name, '[^[:alnum:]]', '', 'g')));
