# Testing Guide

> **Read this when:** writing new tests, deciding what to test, or setting up test infrastructure.
> **Key invariant:** tests document market-scout-specific behavior and cross-boundary interactions — not language or framework basics.
> **Related:** [Developer Guide](./developer-guide.md), [Project](./project.md) (architecture and ATS adapter contract)

---

## 1. Test Infrastructure

Standard Go `testing` package. Tests live next to the code they exercise (`foo.go` ↔ `foo_test.go`, package `foo`). Cross-package integration tests use package `foo_test` (black-box) in the same directory.

| Layer | Scope | Location |
|-------|-------|----------|
| Unit | Pure logic, parsers, response decoding | Alongside source (`apps/tools/internal/ats/greenhouse_test.go`) |
| Adapter HTTP | ATS adapter against `httptest.Server` with recorded fixtures | `apps/tools/internal/ats/*_test.go` |
| DB integration | `sqlc` queries against a real Postgres (testcontainers-go or a shared dev container) | `apps/tools/internal/db/*_integration_test.go`, build tag `//go:build integration` |
| End-to-end | `cmd/fetcher` run against fixture ATS server + real Postgres | `apps/tools/cmd/fetcher/*_e2e_test.go`, build tag `//go:build e2e` |

Default `go test ./...` runs unit + adapter HTTP tests. Integration and E2E tests require their build tags (`go test -tags=integration ./...`) and a running Postgres.

Fixtures (recorded ATS JSON responses) live in `apps/tools/internal/ats/testdata/<adapter>/`. The `testdata/` directory name is recognized by the Go toolchain and excluded from build.

---

## 2. What to Test

### Priority targets

| Category | Examples |
|----------|----------|
| ATS adapter HTTP boundary | Real HTTP roundtrip against `httptest.Server`, pagination, rate-limit/retry behavior, non-200 handling |
| ATS response parsing | Decoding recorded JSON fixtures into adapter types; missing fields, nullable fields, schema drift |
| Snapshot write correctness | One fetch produces N rows in `posting_snapshots` with correct timestamp, fetch_id, and payload — never upserts existing rows |
| DB query behavior | `sqlc`-generated queries return expected rows under realistic data shapes (multiple snapshots per posting, deleted postings, etc.) |
| pgvector queries | Similarity ordering correctness; distance operator (`<=>`, `<->`) matches intent; index usage where it matters |
| Fetch → parse → store flow | End-to-end: adapter fetches from fake ATS, fetcher writes snapshots, query reads them back |
| Concurrency | Concurrent fetches across companies don't corrupt rows or leak goroutines |

### Decision criteria

Test it if **all** of these hold:
- Market-scout-specific behavior (not a language feature or library API)
- Crosses a boundary or shows how the system behaves at a seam (HTTP, DB, adapter interface)
- Captures a real fetch/parse/store/query scenario or documents a workflow for future readers

Skip it otherwise.

---

## 3. What Not to Test

- Standard library or third-party basics (`json.Unmarshal`, `pgx` connection plumbing, `sqlc`-generated boilerplate)
- Language features (struct field assignment, slice append, channel send semantics)
- HTTP client internals (`net/http` redirect handling, transport pooling)
- SQL the database itself defines (don't test that `INSERT` inserts — test that the `INSERT` shape matches the snapshot model)
- Logging output text. Test what gets written to the DB or returned to the caller, not what shows up in logs. Logs are observation, not contract.

---

## 4. Test Patterns

### Behavior over implementation

Assert observable outcomes: rows written to the DB, results returned by a query, HTTP requests issued by an adapter. Avoid asserting on private struct fields or intermediate values that could change without affecting behavior.

### Real interaction flows

Model the actual lifecycle: HTTP fetch → JSON decode → snapshot row construction → DB insert → query read. Over-mocking the DB or HTTP layer hides the interactions tests exist to document. Prefer `httptest.Server` with fixture JSON over mocking the adapter interface.

Reference: `apps/tools/internal/ats/greenhouse_test.go` (adapter against `httptest.Server` serving recorded JSON).

### Seam-crossing tests

When testing code that bridges two systems, derive mock inputs from the source system's actual output and assertions from the destination system's contract. For an ATS adapter: inputs are real ATS JSON (recorded into `testdata/`), assertions are the snapshot row shape the DB expects. If both come from the same mental model — e.g. you hand-write JSON that mirrors your Go struct — the test proves the decoder is a passthrough, not that the passthrough is correct.

### Test naming

Go convention: `TestSubject_Behavior` or `TestSubject_Condition_Expected`. Names describe the exact behavior and boundary under test.

```go
func TestGreenhouseAdapter_PaginatesUntilEmptyPage(t *testing.T) { ... }
func TestSnapshotWriter_AppendsRowEvenWhenPostingUnchanged(t *testing.T) { ... }
```

Use table-driven subtests (`t.Run`) when several inputs exercise the same behavior. Each subtest name should describe the case, not the index.

### Stable test harnesses

| Rule | Rationale |
|------|-----------|
| Each integration test gets a fresh schema or transaction-wrapped DB | Cross-test row leakage produces flaky, order-dependent failures. |
| Use `t.Cleanup` for teardown | Runs even on `t.Fatal`. Prefer over `defer` in helpers. |
| Use `context.Context` with `t.Context()` (Go 1.24+) or a per-test `context.WithCancel` | Prevents goroutine leaks when a test fails mid-fetch. |
| Never share a `*sql.DB` between tests that mutate schema | Use one pool per package, isolate state via transactions or schemas. |
| Record real ATS responses into `testdata/` rather than hand-writing JSON | Hand-written fixtures encode your assumptions, not the API's actual shape. |
| Use `testing.Short()` to gate slow tests | `go test -short ./...` stays fast for the inner loop. |

### Build tags for environment-gated tests

Integration and E2E tests that require Postgres or network:

```go
//go:build integration

package db_test
```

Place at the top of the file, before the package declaration, with a blank line after.

---

## 5. Test Organization

Tests co-locate with source (`*_test.go`). Shared infrastructure:

| Directory | Purpose |
|-----------|---------|
| `apps/tools/internal/testutil/` | Reusable helpers (`NewTestDB`, fixture loaders, fake clock) |
| `apps/tools/internal/ats/testdata/<adapter>/` | Recorded ATS JSON responses |
| `apps/tools/internal/db/testdata/` | SQL seed scripts for integration tests |

Test files are exempt from source file size guidance. Test suites are flat and linear — large is fine. A 600-line `_test.go` with 30 table cases is healthier than three files split by aesthetic.

Helpers go in `apps/tools/internal/testutil/` only when reused across packages. Package-local helpers stay in `helpers_test.go` next to the tests that use them.

---

## 6. Running Tests

From `apps/tools/`:

```bash
go test ./...                              # All unit + adapter tests
go test ./internal/ats/...                 # One package tree
go test -run TestGreenhouseAdapter ./...   # By name (regex)
go test -v ./internal/ats                  # Verbose: show each subtest
go test -race ./...                        # Race detector (run before merging)
go test -cover ./...                       # Coverage summary
go test -coverprofile=cover.out ./... && go tool cover -html=cover.out
go test -tags=integration ./...            # Integration tests (need Postgres)
go test -tags=e2e ./...                    # End-to-end tests
go test -short ./...                       # Skip slow tests
go vet ./...                               # Static checks (catches struct tag typos, shadowing)
```

`go test` caches results per package; pass `-count=1` to force re-run.

---

## 7. Non-Goals

- 100% coverage targets (coverage is a tool, not a goal — but adapter parsing and snapshot write paths should approach it)
- Testing third-party library behavior (`pgx`, `sqlc` output, `net/http`)
- Mocking the DB. Use a real Postgres for anything past unit-level decode logic. The append-only snapshot model is load-bearing — exercise it against the actual database, not a fake.
- Asserting on generated `sqlc` code. Test the SQL by exercising the generated function against real data.
- E2E coverage for every fetch scenario. One end-to-end happy path per adapter is enough; unit and integration tests cover the variants.
