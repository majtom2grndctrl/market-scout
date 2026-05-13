-- Initial schema: companies, job_postings, posting_snapshots, classification taxonomy
-- (canonical_roles, specializations, skills, role_dimensions, classifications, join tables),
-- and seed data. Enables pgvector for future similarity search; no vector columns yet.
-- See: agent-context/lib/project.md
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE companies (
    id                   bigserial PRIMARY KEY,
    name                 text NOT NULL,
    ats                  text,
    board_token          text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    employee_count_range text CHECK (employee_count_range IN ('1-10','11-50','51-200','201-500','501-1000','1001-5000','5001+')),
    founded_year         integer,
    funding_stage        text CHECK (funding_stage IN ('bootstrapped','seed','series_a','series_b','series_c','series_d_plus','public','acquired')),
    total_funding_usd    bigint,
    industry             text,
    company_type         text CHECK (company_type IN ('product','in_house','agency','consultancy')),
    careers_page_url     text,
    enriched_at          timestamptz,
    CONSTRAINT uq_companies_ats_board_token UNIQUE (ats, board_token)
);

CREATE TABLE job_postings (
    id            bigserial PRIMARY KEY,
    company_id    bigint NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    source_type   text NOT NULL CHECK (source_type IN ('ats','web','agent')),
    source_url    text NOT NULL,
    source_id     text,
    cities        text[],
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_job_postings_company_source_url UNIQUE (company_id, source_url)
);

CREATE TABLE posting_snapshots (
    id              bigserial PRIMARY KEY,
    job_posting_id  bigint NOT NULL REFERENCES job_postings(id) ON DELETE RESTRICT,
    fetched_at      timestamptz NOT NULL,
    title           text,
    location_text   text,
    department      text,
    team            text,
    employment_type text CHECK (employment_type IN ('full_time','part_time','contract','intern','temporary')),
    workplace_type  text CHECK (workplace_type IN ('remote','hybrid','onsite')),
    posted_at       timestamptz,
    job_url         text,
    raw_data        jsonb NOT NULL
);

CREATE TABLE canonical_roles (
    id         bigserial PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE specializations (
    id         bigserial PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE skills (
    id         bigserial PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Closed seed-only set. Agents may not extend this table at runtime — unknown slugs abort writeback.
-- Enforcement: batch-enrich preflight resolves every emitted dimension slug against the
-- in-memory taxonomy map loaded at invocation start; any unknown slug aborts that posting's transaction.
-- No created_at because membership is governed by migrations, not runtime inserts.
-- The seed is distributed across migrations; read all up scripts to enumerate the full set.
CREATE TABLE role_dimensions (
    id   bigserial PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL
);

CREATE TABLE canonical_role_dimensions (
    canonical_role_id bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE CASCADE,
    dimension_id      bigint NOT NULL REFERENCES role_dimensions(id) ON DELETE RESTRICT,
    PRIMARY KEY (canonical_role_id, dimension_id)
);

CREATE INDEX idx_canonical_role_dimensions_dimension ON canonical_role_dimensions (dimension_id);

-- FK asymmetry (intentional): CASCADE from job_postings — deleting a posting takes its full
-- classification history with it. RESTRICT on taxonomy FKs (canonical_roles, specializations,
-- skills) — taxonomy deletion must confront existing classification history; provenance is the goal,
-- so the DB blocks the delete rather than silently orphaning historical records.
--
-- A classification is one model run against one posting. Join tables hang off classifications so we
-- keep provenance per run. Append-only: never delete or update classification rows. "Current" is
-- derived per query (ORDER BY classified_at DESC LIMIT 1), backed by idx_classifications_posting_classified.
-- Orchestrator idempotency (--force inserts a new row per run; non-force skips already-classified postings) is the enforcement boundary against unintended duplicates.
CREATE TABLE classifications (
    id             bigserial PRIMARY KEY,
    job_posting_id bigint NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    model          text NOT NULL,
    prompt_version text NOT NULL,
    classified_at  timestamptz NOT NULL DEFAULT now(),
    seniority      text NOT NULL CHECK (seniority IN ('intern','junior','mid','senior','staff','principal','lead','director','unknown')),
    notes          text
);

CREATE INDEX idx_classifications_posting_classified ON classifications (job_posting_id, classified_at DESC);

CREATE TABLE job_posting_roles (
    classification_id bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE,
    role_id           bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE RESTRICT,
    PRIMARY KEY (classification_id, role_id)
);

CREATE TABLE job_posting_specializations (
    classification_id bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE,
    specialization_id bigint NOT NULL REFERENCES specializations(id) ON DELETE RESTRICT,
    PRIMARY KEY (classification_id, specialization_id)
);

CREATE TABLE job_posting_skills (
    classification_id bigint NOT NULL REFERENCES classifications(id) ON DELETE CASCADE,
    skill_id          bigint NOT NULL REFERENCES skills(id) ON DELETE RESTRICT,
    PRIMARY KEY (classification_id, skill_id)
);

CREATE INDEX idx_posting_snapshots_posting_fetched ON posting_snapshots (job_posting_id, fetched_at DESC);
CREATE INDEX idx_job_postings_cities ON job_postings USING GIN (cities);

-- Initial seed data: base role dimensions (full set spans all migrations; see all up.sql files for the complete list), plus the General Application canonical role mapped to other.
-- These rows live here because they must exist before any enrichment run; company and watchlist seeding belongs in internal/db/seeds/.
INSERT INTO role_dimensions (slug, name) VALUES
    ('design', 'Design'),
    ('engineering', 'Engineering'),
    ('research', 'Research'),
    ('product', 'Product'),
    ('data', 'Data'),
    ('operations', 'Operations'),
    ('other', 'Other')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO canonical_roles (slug, name) VALUES ('general-application', 'General Application')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO canonical_role_dimensions (canonical_role_id, dimension_id)
VALUES (
    (SELECT id FROM canonical_roles WHERE slug = 'general-application'),
    (SELECT id FROM role_dimensions WHERE slug = 'other')
)
ON CONFLICT DO NOTHING;
