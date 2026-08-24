-- Derive curated markets from run-scoped snapshot locations without changing
-- append-only snapshots. Local performance check (2026-08-21, 4,595 open
-- postings): after applying this migration in a dev database, record
-- SELECT count(*) FROM open_posting_markets and the median of five warm
-- EXPLAIN (ANALYZE, TIMING OFF) runs before expanding the dictionary.

CREATE TABLE market_seeds (
    slug     text PRIMARY KEY,
    name     text NOT NULL,
    kind     text NOT NULL CHECK (kind IN ('hub', 'region', 'remote')),
    patterns text[] NOT NULL DEFAULT '{}'
);

-- Every pattern below was observed in the 2026-08-21 open-cohort location
-- harvest. Patterns omit their outer word boundaries; the view applies \y to
-- every match so, for example, remote cannot match inside Vermont.
INSERT INTO market_seeds (slug, name, kind, patterns) VALUES
    ('sf-bay-area', 'SF Bay Area', 'hub', ARRAY[
        'san francisco', 'sf', 'bay area', 'mountain view', 'fremont',
        'milpitas', 'oakland'
    ]),
    ('new-york', 'New York', 'hub', ARRAY['new york', 'new york city', 'nyc']),
    ('seattle', 'Seattle', 'hub', ARRAY[
        'seattle', 'everett', 'bellevue', 'kirkland', 'kent', 'bellingham'
    ]),
    ('london', 'London', 'hub', ARRAY['london']),
    ('singapore', 'Singapore', 'hub', ARRAY['singapore']),
    ('india', 'India', 'hub', ARRAY[
        'india', 'bengaluru', 'bangalore', 'hyderabad', 'gurgaon', 'delhi', 'mumbai', 'pune'
    ]),
    ('dublin', 'Dublin', 'hub', ARRAY['dublin']),
    ('tokyo', 'Tokyo', 'hub', ARRAY['tokyo']),
    ('washington-dc', 'Washington, DC', 'hub', ARRAY[
        'washington,? (d\.?c\.?|district of columbia)'
    ]),
    ('chicago', 'Chicago', 'hub', ARRAY['chicago']),
    ('toronto', 'Toronto', 'hub', ARRAY['toronto']),
    ('mexico-city', 'Mexico City', 'hub', ARRAY['mexico city']),
    ('paris', 'Paris', 'hub', ARRAY['paris']),
    ('berlin', 'Berlin', 'hub', ARRAY['berlin']),
    ('sydney', 'Sydney', 'hub', ARRAY['sydney']),
    ('sao-paulo', 'São Paulo', 'hub', ARRAY['são paulo', 'sao paulo', 'sao paolo']),
    ('tel-aviv', 'Tel Aviv', 'hub', ARRAY['tel aviv']),
    ('seoul', 'Seoul', 'hub', ARRAY['seoul']),
    ('taipei', 'Taipei', 'hub', ARRAY['taipei']),
    ('doha', 'Doha', 'hub', ARRAY['doha']),
    ('manila', 'Manila', 'hub', ARRAY['manila']),
    ('warsaw', 'Warsaw', 'hub', ARRAY['warsaw']),
    ('dubai', 'Dubai', 'hub', ARRAY['dubai']),
    ('munich', 'Munich', 'hub', ARRAY['munich']),
    ('us', 'United States', 'region', ARRAY[
        'united states', 'usa', 'us', 'north america', 'namer', 'americas'
    ]),
    ('europe', 'Europe', 'region', ARRAY['europe', 'european union']),
    ('united-kingdom', 'United Kingdom', 'region', ARRAY['united kingdom', 'uk']),
    ('emea', 'EMEA', 'region', ARRAY['emea']),
    ('remote-us', 'Remote — United States', 'remote', ARRAY[
        '(remote.*(us|usa|united states)|(us|usa|united states).*remote)'
    ]),
    ('unmapped', 'Unmapped', 'region', ARRAY[]::text[]);

CREATE VIEW open_posting_markets AS
WITH matched_markets AS (
    SELECT DISTINCT
        open_postings.job_posting_id,
        market_seeds.slug,
        market_seeds.name,
        market_seeds.kind
    FROM open_postings
    JOIN LATERAL (
        SELECT posting_snapshots.location_texts
        FROM posting_snapshots
        WHERE posting_snapshots.job_posting_id = open_postings.job_posting_id
            AND posting_snapshots.fetch_run_id = open_postings.fetch_run_id
        ORDER BY posting_snapshots.fetched_at DESC, posting_snapshots.id DESC
        LIMIT 1
    ) AS current_snapshot ON TRUE
    CROSS JOIN LATERAL unnest(current_snapshot.location_texts) AS location(location_text)
    JOIN market_seeds
        ON market_seeds.slug <> 'unmapped'
    CROSS JOIN LATERAL unnest(market_seeds.patterns) AS pattern(value)
    WHERE location.location_text ~* ('\y' || pattern.value || '\y')
)
SELECT job_posting_id, slug, name, kind
FROM matched_markets

UNION ALL

SELECT
    open_postings.job_posting_id,
    unmapped.slug,
    unmapped.name,
    unmapped.kind
FROM open_postings
JOIN market_seeds AS unmapped ON unmapped.slug = 'unmapped'
WHERE NOT EXISTS (
    SELECT 1
    FROM matched_markets
    WHERE matched_markets.job_posting_id = open_postings.job_posting_id
);

CREATE OR REPLACE VIEW open_posting_taxonomy AS
SELECT
    open_postings_display.job_posting_id,
    'role'::text AS term_kind,
    canonical_roles.slug,
    canonical_roles.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_roles ON canonical_roles.id = job_posting_roles.role_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'specialization'::text AS term_kind,
    specializations.slug,
    specializations.name
FROM open_postings_display
JOIN job_posting_specializations
    ON job_posting_specializations.classification_id = open_postings_display.classification_id
JOIN specializations ON specializations.id = job_posting_specializations.specialization_id

UNION ALL

SELECT
    open_postings_display.job_posting_id,
    'skill'::text AS term_kind,
    skills.slug,
    skills.name
FROM open_postings_display
JOIN job_posting_skills
    ON job_posting_skills.classification_id = open_postings_display.classification_id
JOIN skills ON skills.id = job_posting_skills.skill_id

UNION ALL

SELECT DISTINCT
    open_postings_display.job_posting_id,
    'dimension'::text AS term_kind,
    role_dimensions.slug,
    role_dimensions.name
FROM open_postings_display
JOIN job_posting_roles
    ON job_posting_roles.classification_id = open_postings_display.classification_id
JOIN canonical_role_dimensions
    ON canonical_role_dimensions.canonical_role_id = job_posting_roles.role_id
JOIN role_dimensions ON role_dimensions.id = canonical_role_dimensions.dimension_id

UNION ALL

SELECT
    open_posting_markets.job_posting_id,
    'market'::text AS term_kind,
    open_posting_markets.slug,
    open_posting_markets.name
FROM open_posting_markets;
