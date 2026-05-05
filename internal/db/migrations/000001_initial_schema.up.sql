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
    id       bigserial PRIMARY KEY,
    slug     text NOT NULL UNIQUE,
    name     text NOT NULL,
    category text NOT NULL CHECK (category IN ('design','engineering','research'))
);

CREATE TABLE specializations (
    id   bigserial PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL
);

CREATE TABLE skills (
    id   bigserial PRIMARY KEY,
    slug text NOT NULL UNIQUE,
    name text NOT NULL
);

CREATE TABLE job_posting_roles (
    job_posting_id bigint NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    role_id        bigint NOT NULL REFERENCES canonical_roles(id) ON DELETE CASCADE,
    PRIMARY KEY (job_posting_id, role_id)
);

CREATE TABLE job_posting_specializations (
    job_posting_id    bigint NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    specialization_id bigint NOT NULL REFERENCES specializations(id) ON DELETE CASCADE,
    PRIMARY KEY (job_posting_id, specialization_id)
);

CREATE TABLE job_posting_skills (
    job_posting_id bigint NOT NULL REFERENCES job_postings(id) ON DELETE CASCADE,
    skill_id       bigint NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    PRIMARY KEY (job_posting_id, skill_id)
);

CREATE INDEX idx_posting_snapshots_posting_fetched ON posting_snapshots (job_posting_id, fetched_at DESC);
CREATE INDEX idx_job_postings_cities ON job_postings USING GIN (cities);
