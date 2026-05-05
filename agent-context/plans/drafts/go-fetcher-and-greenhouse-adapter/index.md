# Go Fetcher and Greenhouse Adapter

## Goal

Build the Go fetcher binary and Greenhouse ATS adapter — the first complete data ingestion path. On each run: load companies from the DB, fetch all open Greenhouse job postings, and write append-only snapshots. Provides the working data pipeline on top of the DB infrastructure.

## Scope

### In scope

- `internal/ats/ats.go` — `Adapter` interface and shared `Posting` type
- `internal/ats/greenhouse.go` — Greenhouse implementation: HTTP fetch, JSON decode, normalization
- `internal/db/queries/fetcher.sql` — three queries: list companies, upsert job posting, insert snapshot
- `cmd/fetcher/main.go` — entry point: load companies, concurrent dispatch, per-company transaction

### Out of scope

- Lever and Ashby adapters (deferred per project.md)
- Scheduler / cron wrapper (deferred)
- Next.js app layer
- Enrichment: city normalization, taxonomy tagging, embeddings
- Seed data or CLI tooling for inserting companies

## Acceptance criteria

- [ ] `go build ./cmd/fetcher` succeeds
- [ ] `go vet ./...` and `staticcheck ./...` report no issues
- [ ] Running the fetcher against a real Greenhouse board writes rows to both `job_postings` and `posting_snapshots`; a second run adds new snapshot rows without duplicating `job_postings` rows
- [ ] A 4xx or 5xx HTTP response from Greenhouse results in a logged error for that company; other companies continue to completion
- [ ] An HTTP timeout results in a logged error for that company; other companies continue
- [ ] All normalized fields present in the Greenhouse response (`title`, `location_text`, `department`, `job_url`) are non-null in the snapshot row; fields Greenhouse doesn't expose (`team`, `employment_type`, `workplace_type`, `posted_at`) are NULL
- [ ] `raw_data` on every snapshot contains the full Greenhouse job object as valid JSONB
- [ ] `go test ./internal/ats/...` passes, covering response parsing and field normalization
- [ ] `source_url` on each `job_postings` row is the stable API URL for that posting (board token + job ID)

## Tasks

### Task 1: ATS adapter interface and shared types

Write `internal/ats/ats.go`. Define the `Adapter` interface — one method: `FetchPostings(ctx context.Context, boardToken string) ([]Posting, error)`. Define `Posting`, the normalized output type all adapters return. Fields mirror the normalized columns on `posting_snapshots` plus `SourceID` and `SourceURL` (used to populate `job_postings`). Nullable ATS fields use pointer types (`*string`, `*time.Time`) so absent values reach the DB as NULL without zero-value ambiguity.

Include package-level doc comment pointing to `agent-context/lib/project.md`.

### Task 2: Greenhouse adapter

Write `internal/ats/greenhouse.go`. Implement `Adapter` against the public Greenhouse board API. The fetch URL is `https://boards-api.greenhouse.io/v1/boards/{boardToken}/jobs?content=true`. No auth required. Response is `{"jobs": [...]}` — one request, no pagination. JSON decode into private wire structs; translate into `[]Posting` before returning. Never return wire structs to callers.

**Field normalization:**

| Posting field | Greenhouse source | Notes |
|---|---|---|
| `SourceID` | `id` as `string` | Integer in wire; convert with `strconv.Itoa` |
| `SourceURL` | Constructed | `https://boards-api.greenhouse.io/v1/boards/{boardToken}/jobs/{id}` |
| `Title` | `title` | Direct |
| `LocationText` | `location.name` | Empty string → NULL (`nil`) |
| `Department` | `departments[0].name` | First entry only; absent → NULL |
| `Team` | — | Always NULL — Greenhouse doesn't expose team |
| `EmploymentType` | — | Always NULL |
| `WorkplaceType` | — | Always NULL |
| `PostedAt` | — | Always NULL — `updated_at` is last-modified, not post date |
| `JobURL` | `absolute_url` | Direct |
| `RawData` | Full job object | Marshal the wire struct back to JSON |

HTTP errors (non-2xx) and network failures return a wrapped error; the adapter does not retry. Timeouts honor the passed `context.Context`.

### Task 3: Fetcher SQL queries

Write `internal/db/queries/fetcher.sql` with three named queries for sqlc.

**ListCompaniesWithATS** — returns all companies where `ats IS NOT NULL`, ordered by name. The fetcher iterates this to know which boards to fetch.

**UpsertJobPosting** — `INSERT INTO job_postings (company_id, source_type, source_url, source_id) VALUES ($1, $2, $3, $4) ON CONFLICT (company_id, source_url) DO UPDATE SET source_id = EXCLUDED.source_id RETURNING id`. The `DO UPDATE` is a no-op in practice (source_id is stable) but is required to trigger `RETURNING` on conflict — `DO NOTHING` would return no rows on conflict.

**InsertPostingSnapshot** — `INSERT INTO posting_snapshots (job_posting_id, fetched_at, title, location_text, department, team, employment_type, workplace_type, posted_at, job_url, raw_data) VALUES (...)`. No `RETURNING` needed.

After writing the SQL, run `sqlc generate` and commit the generated `*.sql.go` and `models.go` diffs.

### Task 4: Fetcher entry point

Write `cmd/fetcher/main.go`. Execution flow:

1. Read `DATABASE_URL` from env. Fatal if absent.
2. Open a `pgxpool.Pool`. Fatal if unreachable.
3. Load all ATS companies via `ListCompaniesWithATS`.
4. Dispatch one goroutine per company, bounded by a semaphore (buffered channel) with a concurrency limit of 5. Each goroutine selects an adapter based on `company.Ats`, calls `FetchPostings`, then calls the snapshot writer. Log per-company errors and continue — a single company failure must not abort other fetches.
5. After all goroutines complete, exit 0 if no errors occurred; exit 1 if any company failed, logging the full set of failures.

**Snapshot write (per company, in one transaction):** For each posting returned by the adapter, call `UpsertJobPosting` to get (or create) the `job_postings` row, then call `InsertPostingSnapshot` with `fetched_at` set to the moment the goroutine began its fetch for that company (not `time.Now()` per row). All writes for a company go in a single `pgx` transaction — the transaction is rolled back if any insert fails. This preserves the invariant that each `fetched_at` timestamp represents a complete view of a company's board; partial fetches would corrupt trend gap detection.

Use `log/slog` throughout. Subsystem tag: `[fetcher]`. Log the company name and posting count on success; log company name and error on failure.

Unknown `ats` values (values added to the DB before a matching adapter exists) return an error — log and skip the company.

## Sequencing

**Phase 0 (prerequisite, sequential):** db-infrastructure plan must be complete — provides `go.mod`, DB schema, and the sqlc toolchain.

**Phase 1 (sequential):** Task 1 — adapter interface and Posting type. Tasks 2, 3, and 4 all depend on the Posting struct.

**Phase 2 (concurrent):** Task 2 (Greenhouse adapter) and Task 3 (SQL queries + sqlc generate) — both depend on Task 1 types but are independent of each other.

**Phase 3 (sequential):** Task 4 (fetcher entry point) — consumes the adapter from Task 2 and the generated query functions from Task 3.

## Rough sketch

```go
// internal/ats/ats.go — Proposed design; remove after implementation
type Adapter interface {
    FetchPostings(ctx context.Context, boardToken string) ([]Posting, error)
}

type Posting struct {
    SourceID       string
    SourceURL      string
    Title          string
    LocationText   *string
    Department     *string
    Team           *string
    EmploymentType *string
    WorkplaceType  *string
    PostedAt       *time.Time
    JobURL         *string
    RawData        json.RawMessage
}
```

The fetcher does not import the `Adapter` interface from `internal/ats` directly — it accepts a map of `string → ats.Adapter` keyed by ATS name. This keeps the dispatch table explicit and testable without requiring the fetcher to know all adapter implementations at compile time.

`go.mod` already includes `pgx/v5` from db-infrastructure. No additional dependencies needed. Use a plain semaphore (buffered channel capped at 5) plus `sync.WaitGroup` from the standard library — direct expression of "bounded concurrency, collect all errors," no extra package. `errgroup` from `golang.org/x/sync` cancels the shared context on first failure, which is the opposite of the fetcher's continue-on-error semantics.

Each company goroutine gets a 45s context timeout, derived from the parent context. This bounds worst-case runtime per company without setting a global wall-clock limit on the full run.

## Open questions

- **`fetch_run_id`**: Deferred. `fetched_at` is a coarse proxy for now. If gap detection later requires grouping all postings from one run, a fetch run table will need a migration and a fetcher change. Revisit when building trend queries.
