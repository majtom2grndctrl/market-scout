# Database Infrastructure

## Goal

Stand up local Postgres with pgvector, define the initial schema, and wire golang-migrate and sqlc. Provides the data layer that Plan 2 (Go Fetcher) writes into.

## Scope

### In scope

- `.env.example` documenting required connection variables
- `cmd/migrate/main.go` — golang-migrate runner
- Initial migration: `companies`, `job_postings`, `posting_snapshots`, plus taxonomy tables (`canonical_roles`, `specializations`, `skills` and their join tables)
- `sqlc.yaml` config pointing at schema and queries directories
- `go.mod` / `go.sum` initialized with required dependencies

### Out of scope

- Go fetcher binary and ATS adapters (Plan 2)
- Next.js app layer
- Seed data, fixture scripts, or a CLI to insert companies
- pgvector similarity queries (schema supports them; queries come later)
- Scheduler or cron setup
- Enrichment workflow that maps descriptions to roles, specializations, and skills
- `description_embedding` column on `job_postings` — embedding dimension lands with the enrichment plan once the model is chosen

## Acceptance criteria

- [ ] `docker compose up -d` starts Postgres; container is healthy and survives restart
- [ ] `go run ./cmd/migrate up` applies all migrations cleanly against the running container
- [ ] `go run ./cmd/migrate down` reverses migrations cleanly
- [ ] `psql` confirms `companies`, `job_postings`, `posting_snapshots` tables exist
- [ ] `psql` confirms taxonomy tables (`canonical_roles`, `specializations`, `skills`) and their join tables exist
- [ ] `\dx` in psql confirms pgvector extension is active
- [ ] `psql` confirms composite index on `posting_snapshots(job_posting_id, fetched_at DESC)` and GIN index on `job_postings(cities)` exist
- [ ] `sqlc generate` completes without errors
- [ ] `go build ./...` succeeds with no compile errors
- [ ] `.env.example` exists and documents `DATABASE_URL`, `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_PORT`

## Tasks

### Task 1: Docker Compose + environment

Update `docker-compose.yml` to load env vars via `env_file: .env.local` rather than hardcoded values. Create `.env.local` (gitignored) with dev values for `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_PORT`, and `DATABASE_URL`. Create `.env.example` documenting all five variables with safe placeholder values. `DATABASE_URL` format: `postgres://<user>:<password>@localhost:<port>/<db>?sslmode=disable`.

### Task 2: Go module + migrate runner

Initialize the Go module (`go mod init github.com/majtom2grndctrl/market-scout`). Add dependencies: `golang-migrate/migrate/v4` and `jackc/pgx/v5`. Include blank imports for the migrate drivers: `_ "github.com/golang-migrate/migrate/v4/database/postgres"` and `_ "github.com/golang-migrate/migrate/v4/source/file"`. Write `cmd/migrate/main.go`: reads `DATABASE_URL` from env, applies up/down migrations from `internal/db/migrations/`. The binary is invoked as `go run ./cmd/migrate <direction>` where `<direction>` is `up` or `down` (first positional arg). Use `m.Up()` for `up` and `m.Down()` for `down` (reverses all applied steps). Treat `migrate.ErrNoChange` as success; exit non-zero for all other errors.

### Task 3: Schema migration

Write the initial numbered migration in `internal/db/migrations/` (e.g. `000001_initial_schema.up.sql` and `000001_initial_schema.down.sql`). All tables use `bigserial` primary keys. All foreign keys are `ON DELETE RESTRICT` except the taxonomy join tables, which are `ON DELETE CASCADE`.

`CREATE EXTENSION IF NOT EXISTS vector` — enables pgvector for future similarity queries.

**`companies`** — one row per company to monitor:

| Column | Type | Constraints |
|---|---|---|
| `id` | `bigserial` | PK |
| `name` | `text` | NOT NULL |
| `ats` | `text` | NULL — one of `greenhouse`, `lever`, `ashby`; NULL for web-scrape or agent-sourced companies |
| `board_token` | `text` | NOT NULL — ATS-specific slug or token used in API URLs |
| `created_at` | `timestamptz` | NOT NULL DEFAULT now() |

Unique constraint: `(ats, board_token)`.

No CHECK constraint on `ats` — the ATS vendor list is expected to grow; values are validated at the application boundary.

Enrichment-populated fields (all nullable; set by research runs, not the ATS fetch):

| Column | Type | Notes |
|---|---|---|
| `employee_count_range` | `text` | CHECK IN (`1-10`, `11-50`, `51-200`, `201-500`, `501-1000`, `1001-5000`, `5001+`) |
| `founded_year` | `integer` | 4-digit year |
| `funding_stage` | `text` | CHECK IN (`bootstrapped`, `seed`, `series_a`, `series_b`, `series_c`, `series_d_plus`, `public`, `acquired`) |
| `total_funding_usd` | `bigint` | Lifetime raised; NULL for bootstrapped or unknown |
| `industry` | `text` | Business vertical — free text, no CHECK; examples: `fintech`, `healthtech`, `b2b_saas` |
| `company_type` | `text` | CHECK IN (`product`, `in_house`, `agency`, `consultancy`) |
| `careers_page_url` | `text` | Direct link to the company's careers page |
| `enriched_at` | `timestamptz` | Timestamp of last enrichment run; NULL means not yet enriched |

**`job_postings`** — identity record per unique posting:

| Column | Type | Constraints |
|---|---|---|
| `id` | `bigserial` | PK |
| `company_id` | `bigint` | NOT NULL, FK → `companies(id)` ON DELETE RESTRICT |
| `source_type` | `text` | NOT NULL CHECK IN (`ats`, `web`, `agent`) |
| `source_url` | `text` | NOT NULL — canonical URL to re-check this posting; constructed from company board token + job ID for ATS sources, scraped URL for web/agent |
| `source_id` | `text` | NULL — ATS-native job ID (Greenhouse returns integers, Lever/Ashby return UUIDs — normalize to text); NULL for web and agent sources |
| `cities` | `text[]` | NULL — normalized city list; populated by enrichment, not the initial fetch |
| `first_seen_at` | `timestamptz` | NOT NULL DEFAULT now() |

Unique constraint: `(company_id, source_url)`. ATS is derivable via the `companies` join — not stored again here. Consistency between `source_type = 'ats'` and `companies.ats IS NOT NULL` is enforced in application code, not at the DB level.

**`posting_snapshots`** — append-only time-series; one row per posting per fetch:

| Column | Type | Nullable | Notes |
|---|---|---|---|
| `id` | `bigserial` | NOT NULL | PK |
| `job_posting_id` | `bigint` | NOT NULL | FK → `job_postings(id)` ON DELETE RESTRICT |
| `fetched_at` | `timestamptz` | NOT NULL | Anchor timestamp for this snapshot |
| `title` | `text` | NULL | |
| `location_text` | `text` | NULL | Raw location string from ATS — captured as-is |
| `department` | `text` | NULL | |
| `team` | `text` | NULL | |
| `employment_type` | `text` | NULL | CHECK IN (`full_time`, `part_time`, `contract`, `intern`, `temporary`); Greenhouse doesn't expose this |
| `workplace_type` | `text` | NULL | CHECK IN (`remote`, `hybrid`, `onsite`); Lever is lowercase, Ashby is PascalCase — both normalize to lowercase |
| `posted_at` | `timestamptz` | NULL | Not all ATSs expose it |
| `job_url` | `text` | NULL | |
| `raw_data` | `jsonb` | NOT NULL | Full ATS response; holds any fields not in the normalized set |

No update path exists for `posting_snapshots`. Every fetch appends new rows; it never modifies existing ones.

**Taxonomy tables** — controlled vocabularies populated by a future enrichment step. Each lookup table holds canonical entries; each join table associates them with `job_postings`:

- `canonical_roles` — `id bigserial PK`, `slug text NOT NULL UNIQUE`, `name text NOT NULL`, `category text NOT NULL CHECK IN ('design', 'engineering', 'research')`
- `specializations` — `id bigserial PK`, `slug text NOT NULL UNIQUE`, `name text NOT NULL`
- `skills` — `id bigserial PK`, `slug text NOT NULL UNIQUE`, `name text NOT NULL`
- `job_posting_roles` — `(job_posting_id bigint, role_id bigint)`, composite PK, both FKs ON DELETE CASCADE
- `job_posting_specializations` — `(job_posting_id bigint, specialization_id bigint)`, composite PK, both FKs ON DELETE CASCADE
- `job_posting_skills` — `(job_posting_id bigint, skill_id bigint)`, composite PK, both FKs ON DELETE CASCADE

Joins reference `job_postings` (identity) rather than `posting_snapshots` (time-series). Roles, specializations, and skills are stable across a posting's lifetime; if a description changes materially, enrichment re-runs for that posting rather than re-tagging every snapshot.

Taxonomy tables ship empty — population belongs to the enrichment plan.

**Indexes**: `posting_snapshots(job_posting_id, fetched_at DESC)` for trend queries that read the latest snapshot per posting. GIN index on `job_postings(cities)` for `ANY()` containment queries by market. Unique indexes on `slug` for all lookup tables (covered by UNIQUE constraints above).

### Task 4: sqlc config

Write `sqlc.yaml`. Point it at the migrations directory for schema and at `internal/db/queries/` for hand-written SQL. Configure output to `internal/db/` with package name `db`. Create a placeholder `.sql` file in `internal/db/queries/` so `sqlc generate` has something to process. The file must contain at least one annotated query, e.g.:
```sql
-- name: Ping :one
SELECT 1;
```
The generated files (`*.sql.go`, `models.go`) are checked in.

## Sequencing

**Phase 1 (sequential):** Task 1 — Docker environment must be up before migrations can run.

**Phase 2 (concurrent):** Task 2 (Go module + migrate runner) and Task 3 (schema SQL) can be authored without a running container. Verification of Task 2 (running migrations) requires Phase 1 complete.

**Phase 3:** Task 4 (`sqlc.yaml` authoring) can proceed in parallel with Task 3; `sqlc generate` runs sequentially after Task 3 schema is finalized.

## Rough sketch

**Three taxonomy axes describe a posting:**

| Axis | Vocabulary | Examples | Cardinality |
|---|---|---|---|
| Role | `canonical_roles` | `product-designer`, `frontend-engineer` | usually 1, sometimes 2 (hybrid roles) |
| Specialization | `specializations` | `design-systems`, `prototyping`, `growth` | 0–N |
| Skills | `skills` | `react`, `figma`, `storybook` | 0–N |

Illustrative query (not the canonical implementation — dedupe logic should use `DISTINCT ON` or a lateral join in practice):

```sql
SELECT jp.id, c.name, ps.title
FROM job_postings jp
JOIN companies c              ON c.id = jp.company_id
JOIN posting_snapshots ps     ON ps.job_posting_id = jp.id
JOIN job_posting_roles jpr    ON jpr.job_posting_id = jp.id
JOIN canonical_roles cr       ON cr.id = jpr.role_id
JOIN job_posting_specializations jps ON jps.job_posting_id = jp.id
JOIN specializations sp       ON sp.id = jps.specialization_id
JOIN job_posting_skills jpk   ON jpk.job_posting_id = jp.id
JOIN skills sk                ON sk.id = jpk.skill_id
WHERE cr.category = 'engineering'
  AND sp.slug = 'design-systems'
  AND sk.slug = 'react'
  AND ps.fetched_at = (
    SELECT MAX(fetched_at) FROM posting_snapshots WHERE job_posting_id = jp.id
  );
```

sqlc.yaml:
```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "internal/db/queries"
    schema: "internal/db/migrations"
    gen:
      go:
        package: "db"
        out: "internal/db"
```

Migration filename convention: `000001_initial_schema.up.sql` / `000001_initial_schema.down.sql`.

## Open questions

- **Fetch run grouping**: Should `posting_snapshots` include a `fetch_run_id` to group all rows from one company fetch? Useful for gap detection (postings that disappeared between fetches). Deferred — `fetched_at` is a coarse proxy for now. Revisit when building trend queries.
