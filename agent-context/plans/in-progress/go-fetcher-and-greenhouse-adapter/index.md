# Go Fetcher and Greenhouse Adapter

## Goal

Build the Go fetcher binary and Greenhouse ATS adapter — the first complete data ingestion path. On each run: load companies from the DB, fetch all open Greenhouse job postings, and write append-only snapshots. Provides the working data pipeline on top of the DB infrastructure.

## Scope

### In scope

- `internal/domain/posting.go` — shared `Posting` type produced by ATS adapters and consumed by the fetcher
- `internal/ats/greenhouse.go` — Greenhouse implementation: HTTP fetch, JSON decode, normalization
- `internal/db/queries/fetcher.sql` — three queries: list companies, upsert job posting, insert snapshot
- `cmd/fetcher/main.go` — entry point: load companies, concurrent dispatch, per-company transaction. Owns the `atsAdapter` interface (consumer-defined per developer-guide §5.3).

### Out of scope

- Lever and Ashby adapters (deferred per project.md)
- Scheduler / cron wrapper (deferred). A typed HTTP error (e.g. `type httpError struct { Status int; Body string }`) distinguishing 4xx from 5xx is deferred to the scheduler plan, where retry semantics live.
- Next.js app layer
- Enrichment: city normalization, taxonomy tagging, embeddings
- Seed data or CLI tooling for inserting companies
- Postings removed from a board between runs — absence detection is deferred

## Acceptance criteria

- [ ] `go build ./cmd/fetcher` succeeds
- [ ] `go vet ./...` and `staticcheck ./...` report no issues
- [ ] Running the fetcher against a real Greenhouse board writes rows to both `job_postings` and `posting_snapshots`; a second run adds new snapshot rows without duplicating `job_postings` rows
- [ ] A 4xx or 5xx HTTP response from Greenhouse results in a logged error for that company; other companies continue to completion
- [ ] An HTTP timeout results in a logged error for that company; other companies continue
- [ ] When Greenhouse returns a non-empty value for `title`, `location.name`, `departments[0].name`, or `absolute_url`, the corresponding snapshot column is non-null; fields Greenhouse doesn't expose (`team`, `employment_type`, `workplace_type`, `posted_at`) are NULL
- [ ] `raw_data` on every snapshot contains the full Greenhouse job object as valid JSONB
- [ ] `go test ./internal/ats/...` passes, covering response parsing and field normalization
- [ ] `source_url` on each `job_postings` row is the stable identity key for that posting, constructed as `https://boards-api.greenhouse.io/v1/boards/{boardToken}/jobs/{id}` with `boardToken` URL-path-escaped
- [ ] Adapter rejects a Greenhouse job with `id == 0` (missing/null in response) as a wrapped error rather than emitting `SourceID="0"`
- [ ] Adapter URL-escapes `boardToken` (via `url.PathEscape`) in both the fetch URL and the constructed `SourceURL`
- [ ] If any posting returned by an adapter has empty `RawData`, the fetcher aborts that company's transaction with an error (not a silent skip) — preserves the "`fetched_at` represents a complete view of the board" invariant
- [ ] Fetcher exits 0 when all attempted companies succeed; exits 1 if any company failed for a reason other than shutdown
- [ ] Final summary log distinguishes `success`, `failed` (real fetch/write failures), `aborted_shutdown` (cancelled by signal), and `skipped_unsupported`. Invariant: `success + failed + aborted_shutdown = total_attempted`, where `total_attempted = len(supported)`.

## Tasks

### Task 1: Shared Posting type

Write `internal/domain/posting.go`. Define `Posting`, the normalized output type all ATS adapters return. Fields mirror the normalized columns on `posting_snapshots` plus `SourceID` and `SourceURL` (used to populate `job_postings`).

Field shape rules:
- `SourceID`, `SourceURL` — plain `string`. Always populated; an adapter that can't produce them MUST return an error.
- `Title`, `JobURL`, `LocationText`, `Department`, `Team`, `EmploymentType`, `WorkplaceType` — `*string`. Pointer so absent values reach the DB as NULL without zero-value ambiguity.
- `PostedAt` — `*time.Time`. Same reason.
- `RawData` — `json.RawMessage`. Always populated.

Add a godoc comment on `Posting` explaining (a) it is the producer/consumer contract between ATS adapters and the fetcher, (b) the asymmetry: identity fields are required strings, all other ATS-shaped fields are nullable pointers because the wire format does not guarantee them.

The `Adapter` interface is **not** defined here. Per developer-guide §5.3, interfaces live in the consuming package. The fetcher (`cmd/fetcher`) defines an unexported `atsAdapter` interface that adapter constructors satisfy implicitly.

Include package-level doc comment pointing to `agent-context/lib/project.md`.

### Task 2: Greenhouse adapter

Write `internal/ats/greenhouse.go`. Implement an adapter that satisfies the `cmd/fetcher.atsAdapter` interface (one method: `FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error)`).

The fetch URL is `https://boards-api.greenhouse.io/v1/boards/{boardToken}/jobs?content=true`. No auth required. Response is `{"jobs": [...]}` — one request, no pagination. JSON decode into private wire structs; translate into `[]domain.Posting` before returning. Never return wire structs to callers.

**URL construction:** apply `url.PathEscape` to `boardToken` in both the fetch URL and the constructed `SourceURL`. The `boardToken` comes from a DB row and must be treated as untrusted input at the HTTP boundary.

**ID validation:** if `job.id == 0` after JSON decode (i.e. missing or null in the wire response), return a wrapped error naming the offending entry's index. A null ID would otherwise produce `SourceID="0"` and a `SourceURL` ending in `/jobs/0`, silently corrupting identity.

**Field normalization:**

| Posting field | Greenhouse source | Notes |
|---|---|---|
| `SourceID` | `id` as `string` | Integer in wire (decode as `int64`); convert with `strconv.FormatInt(id, 10)`. Reject `id == 0`. |
| `SourceURL` | Constructed | `https://boards-api.greenhouse.io/v1/boards/{url.PathEscape(boardToken)}/jobs/{id}` |
| `Title` | `title` | Direct |
| `LocationText` | `location.name` | Empty string → NULL (`nil`) |
| `Department` | `departments[0].name` | First entry only; absent → NULL |
| `Team` | — | Always NULL — Greenhouse doesn't expose team |
| `EmploymentType` | — | Always NULL |
| `WorkplaceType` | — | Always NULL |
| `PostedAt` | — | **Always NULL.** Do NOT parse `first_published` or `updated_at`. Greenhouse only exposes last-modified semantics; mapping that to `posted_at` would silently corrupt the meaning of the column. |
| `JobURL` | `absolute_url` | Direct |
| `RawData` | Full job object | Decode jobs array as `[]json.RawMessage` first; store the raw bytes per job. Do not re-marshal the typed wire struct — that silently drops undeclared fields. |

**Error policy:** field-level absence (empty string, missing key, empty array) maps to `nil` per the table. Schema-level mismatch (entry that fails JSON unmarshal into the wire struct) returns a wrapped error including the index — aborts the whole board fetch. There is no in-between: with `PostedAt` always-NULL there is no field whose parsing can partially fail.

**HTTP errors** (non-2xx) and network failures return a wrapped error; the adapter does not retry. Timeouts honor the passed `context.Context`.

**Body cap:** read the response with `io.LimitReader(body, maxResponseBytes+1)` and treat exactly `maxResponseBytes+1` bytes as truncation. (`==N` is a false positive at exact-N bodies.)

Provide a constructor `New(client *http.Client) *Greenhouse`. If `client` is nil, use `http.DefaultClient`. No client-level timeout — timeouts come from the context only.

Write unit tests in `internal/ats/greenhouse_test.go` covering: (a) successful parse with all fields populated, (b) missing optional fields map to nil, (c) non-2xx HTTP response returns a wrapped error, (d) malformed JSON returns a wrapped error, (e) `id == 0` (missing in wire) returns a wrapped error.

### Task 3: Fetcher SQL queries

Write `internal/db/queries/fetcher.sql` with three named queries for sqlc.

**ListCompaniesWithATS** — returns all companies where `ats IS NOT NULL`, ordered by name. The fetcher iterates this to know which boards to fetch.

**UpsertJobPosting** — `INSERT INTO job_postings (company_id, source_type, source_url, source_id) VALUES ($1, $2, $3, $4) ON CONFLICT (company_id, source_url) DO UPDATE SET source_id = EXCLUDED.source_id RETURNING id`. Add an inline SQL comment above this query explaining: `DO UPDATE` (no-op in practice — `source_id` is stable) is required so `RETURNING id` fires on conflict; `DO NOTHING` returns no rows on conflict. The append-only rule from developer-guide §5.6 applies to `posting_snapshots`, not this table.

**InsertPostingSnapshot** — `INSERT INTO posting_snapshots (job_posting_id, fetched_at, title, location_text, department, team, employment_type, workplace_type, posted_at, job_url, raw_data) VALUES (...)`. No `RETURNING` needed.

After writing the SQL, run `sqlc generate` and commit the generated `*.sql.go` and `models.go` diffs.

### Task 4: Fetcher entry point

Write `cmd/fetcher/main.go`. Define an unexported `atsAdapter` interface here:

```go
type atsAdapter interface {
    FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error)
}
```

Execution flow:

1. Read `DATABASE_URL` from env. Fatal if absent.
2. Open the DB connection via `database/sql` with the `pgx/v5/stdlib` driver (`sql.Open("pgx", databaseURL)`). Fatal if unreachable.
3. Build the adapter dispatch map: `adapters := map[string]atsAdapter{"greenhouse": greenhouse.New(nil)}`. Keep dispatch explicit and testable — do not import concrete adapters from helper packages.
4. Set up `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` so SIGINT/SIGTERM cancels the parent context.
5. Load all ATS companies via `ListCompaniesWithATS`. Partition into `supported` (ATS name has a registered adapter) and `unsupported` (ATS value with no adapter — log once per distinct value, count toward `skipped_unsupported`).
6. Dispatch one goroutine per supported company, bounded by a buffered-channel semaphore of size 5. (Chosen to parallelize across a typical watchlist without overwhelming any single ATS endpoint.) Acquire the semaphore via `select` on `<-sem` vs. `<-ctx.Done()` so post-cancellation acquisition never deadlocks. Each goroutine derives a 45s context from the parent (`context.WithTimeout(ctx, 45*time.Second)`), looks up its adapter, calls `FetchPostings`, then writes snapshots in one transaction. Pass `source_type = "ats"` to `UpsertJobPosting` for all ATS-sourced companies.
7. Per-company errors do NOT abort other fetches. Log each error inline. Classify each error as either `failed` (genuine HTTP/DB/parse failure) or `aborted_shutdown` (when `errors.Is(err, context.Canceled)` AND the parent context is also done — meaning the cause is signal, not per-company timeout).
8. After `wg.Wait()`, log a final summary with `total_attempted`, `success`, `failed`, `aborted_shutdown`, and `skipped_unsupported`. Maintain the invariant `success + failed + aborted_shutdown = total_attempted`. Exit 1 if `failed > 0`; exit 0 otherwise (shutdown alone is exit 0 — graceful).

**Per-company timeout: single 45s budget** derived from the parent context. Covers fetch + DB write together. Splitting fetch and write timeouts is permitted only with explicit justification (e.g. measured evidence that a slow DB write starves an otherwise-completed fetch). Default to one budget.

**Snapshot write (per company, in one transaction):** For each posting returned by the adapter, call `UpsertJobPosting` to get (or create) the `job_postings` row, then call `InsertPostingSnapshot` with `fetched_at` set to the moment the goroutine began its fetch for that company (not `time.Now()` per row). All writes for a company go in a single transaction — rolled back if any insert fails. This preserves the invariant that each `fetched_at` timestamp represents a complete view of a company's board; partial fetches would corrupt trend gap detection.

**Empty `RawData` is fatal for the company's transaction.** If any posting returned by the adapter has `len(p.RawData) == 0`, return a wrapped error and let the deferred rollback discard the company's writes. Do not skip-with-warn — that would log success while persisting a partial board, violating the invariant above.

Use `log/slog` throughout. Subsystem tag: `[fetcher]`. Log the company name and posting count on success; log company name and error on failure.

Unknown `ats` values (values added to the DB before a matching adapter exists) are partitioned out in step 5 — counted as `skipped_unsupported`, logged once per distinct value, never dispatched.

## Sequencing

**Phase 0 (prerequisite, sequential):** db-infrastructure plan must be complete — provides `go.mod`, DB schema, and the sqlc toolchain. `sqlc.yaml` uses the default `database/sql` target (no `sql_package` override); the fetcher imports `github.com/jackc/pgx/v5/stdlib` for its driver registration.

**Phase 1:** Task 1 — `domain.Posting`. Blocks Phases 2 and 3.

**Phase 2 (concurrent):** Task 2 (Greenhouse adapter) and Task 3 (SQL queries + sqlc generate) — both depend on Task 1 and are independent of each other.

**Phase 3 (sequential):** Task 4 (fetcher entry point) — consumes the adapter from Task 2 and the generated query functions from Task 3.

## Rough sketch

```go
// internal/domain/posting.go — Proposed design; remove after implementation
package domain

type Posting struct {
    SourceID       string          // required
    SourceURL      string          // required
    Title          *string
    LocationText   *string
    Department     *string
    Team           *string
    EmploymentType *string
    WorkplaceType  *string
    PostedAt       *time.Time
    JobURL         *string
    RawData        json.RawMessage // required
}
```

```go
// cmd/fetcher/main.go — interface lives with the consumer
type atsAdapter interface {
    FetchPostings(ctx context.Context, boardToken string) ([]domain.Posting, error)
}

adapters := map[string]atsAdapter{"greenhouse": greenhouse.New(nil)}
```

Use a plain semaphore (buffered channel capped at 5) plus `sync.WaitGroup` from the standard library — direct expression of "bounded concurrency, collect all errors," no extra package. `errgroup` from `golang.org/x/sync` cancels the shared context on first failure, which is the opposite of the fetcher's continue-on-error semantics.

Each company goroutine gets a 45s context timeout, derived from the parent context. This bounds worst-case runtime per company without setting a global wall-clock limit on the full run.

## Resolved decisions

- **DB driver: `database/sql` + `pgx/v5/stdlib`.** Chosen over native `pgxpool` to keep the codebase on the standard-library SQL interface while the primary author builds Go fluency. Trade-off accepted: `sql.NullString`/`sql.NullTime` appear in sqlc-generated models and must be translated to/from `domain.Posting`'s `*string`/`*time.Time` fields at the adapter→DB boundary in Task 4. `pgx`-native affordances (batch, listen/notify, native JSONB scan) are not used by this fetcher; revisit if a future workload needs them. Update `agent-context/lib/project.md` to reflect this.

## Open questions

- _None blocking. Add new questions here as they arise._

- **`fetch_run_id`**: Deferred. `fetched_at` is a coarse proxy for now, scoped per-company (not per-run). If gap detection later requires grouping all postings from one run, a fetch run table will need a migration and a fetcher change. Revisit when building trend queries.
