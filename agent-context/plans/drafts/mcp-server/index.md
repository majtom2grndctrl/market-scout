# Read-Only MCP Server (`cmd/mcp`)

## Goal
A local MCP server exposing read-only access to the market-scout Postgres DB, so Claude Code agents can verify pipeline output — did the fetcher and enricher store data correctly? — without writing SQL against a read-write connection. Replaces the current ad-hoc "agent writes SQL and runs it directly" workflow with a safe, sandboxed equivalent.

## Scope

### In scope
- New Go binary `cmd/mcp`, built on `github.com/mark3labs/mcp-go`, stdio transport.
- A `query` tool: accepts a SQL string, executes it read-only, returns rows as JSON.
- Safety enforced by a dedicated read-only Postgres role (SELECT-only), not by parsing SQL strings in Go.
- Defense in depth: read-only transaction, per-statement timeout, result row cap.
- One curated tool, `fetch_status`: latest `fetch_run` per company — directly answers "did the fetcher run correctly."
- Read-only role provisioning script + setup docs.
- Claude Code wiring via `.claude/settings.json`.

### Out of scope
- Any write/mutation tools (re-queue, re-fetch, re-classify). Reads only.
- Curated tools beyond `fetch_status` (classification coverage, snapshot field coverage, seed/DB drift). Add once usage shows which checks recur; agents express these through `query` meanwhile.
- Reading `failures.jsonl` or other file-based surfaces — the DB is the surface here.
- Market-analysis / product features (the Next.js app and its agent are a separate workstream).
- Auth / multi-user. Single local admin user only.

## Acceptance criteria
- [ ] `go run ./cmd/mcp` (from `apps/tools/`) starts an stdio MCP server; a Claude Code session configured per the settings entry lists its tools.
- [ ] `query` with a valid `SELECT` returns matching rows as a JSON array of objects (column name → value; SQL NULL → JSON null).
- [ ] `query` with an `INSERT`/`UPDATE`/`DELETE`/DDL statement returns an error and mutates no data (row counts unchanged afterward).
- [ ] A result set larger than the row cap is truncated to the cap, and the response indicates truncation.
- [ ] A query exceeding the statement timeout is aborted and returns a timeout error rather than hanging.
- [ ] The binary exits non-zero with a clear message when the read-only DSN env var is unset.
- [ ] `fetch_status` returns, per company, the most recent run's status, started/completed times, postings count, and error message (null when none).

## Tasks

### Task 1: MCP server scaffold + read-only `query` tool
Create `cmd/mcp/main.go` following the `cmd/onboard` structure: a testable `run` body, `slog` to stderr, `godotenv.Load(".env.local")`, `signal.NotifyContext` for shutdown. Add `mark3labs/mcp-go` to `go.mod`. Read the read-only DSN from its env var; fail fast if unset — no fallback to `DATABASE_URL`, since the fallback would silently defeat the safety boundary. Open with `sql.Open("pgx", dsn)`, ping with a timeout. Register a `query` tool with one required string param `sql`. Handler runs the SQL inside a read-only transaction (`sql.TxOptions{ReadOnly: true}`) with a per-transaction `statement_timeout`, scans dynamic columns into a JSON-friendly shape, caps rows, and returns `mcp.NewToolResultText(json)`. Serve over stdio with `server.ServeStdio`.

### Task 2: Read-only role provisioning + setup docs
Provide an idempotent SQL script (e.g. `apps/tools/internal/db/setup/readonly_role.sql`) that creates the read-only role and grants `SELECT` on all current **and future** tables in `public` (`GRANT SELECT ON ALL TABLES IN SCHEMA public` plus `ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES`). Document the one-time setup step and the read-only DSN env var in `developer-guide.md`. This is operational setup, not a numbered schema migration — roles are cluster-level, and a migration would bake a credential into source.

### Task 3: `fetch_status` curated tool
Add a sqlc query in `apps/tools/internal/db/queries/` returning the latest `fetch_run` per company (`DISTINCT ON (company_id) ... ORDER BY company_id, started_at DESC`), joined to `companies.name`. Run `sqlc generate`; commit SQL input and generated output together. Register a `fetch_status` tool (no params) that calls the query through `db.New(pool)` and returns the rows as JSON.

### Task 4: Claude Code wiring + tests
Add the `cmd/mcp` server to `.claude/settings.json`. Unit-test the row→JSON encoding (type mapping, NULL handling, truncation) with no DB. Add an integration test (`//go:build integration`) that, against a real Postgres connected as the read-only role, confirms a `SELECT` returns rows and a write statement is rejected.

## Sequencing
**Phase 1 (sequential):** Task 1 — establishes server, tool registration, and query execution that everything builds on.
**Phase 2 (concurrent):** Task 2, Task 3 — independent of each other.
**Phase 3 (sequential):** Task 4 — wires settings and tests the finished surface; consumes Tasks 1–3.

## Rough sketch
mcp-go API (verified against current docs): `server.NewMCPServer(name, version, server.WithToolCapabilities(true), server.WithRecovery())`; `mcp.NewTool("query", mcp.WithDescription(...), mcp.WithString("sql", mcp.Required(), mcp.Description(...)))`; `s.AddTool(tool, handler)` with handler `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`; `req.RequireString("sql")`; results via `mcp.NewToolResultText(...)` / errors via `mcp.NewToolResultError(...)`; `server.ServeStdio(s)` blocks. Imports `github.com/mark3labs/mcp-go/mcp` and `.../server`.

DB safety layers, outermost first: (1) read-only role — the real boundary; (2) `BeginTx(ctx, &sql.TxOptions{ReadOnly: true})`; (3) `SET LOCAL statement_timeout` inside the tx; (4) app-side row cap — scan at most cap+1 rows, truncate, set a flag.

Result encoding contract: JSON array of objects keyed by column name. NULL → JSON null; numeric → JSON number; `timestamptz` → RFC3339 string; text → string; `bytea` → string/base64. Envelope carries `row_count` and `truncated`. No dedicated schema tool needed — the agent introspects via `query` against `information_schema`.

## Boundary inventory
| Name | Go | JSON key | SQL |
|---|---|---|---|
| SQL input | `req.RequireString("sql")` | `"sql"` | — |
| row count | `RowCount int` | `"row_count"` | — |
| truncated flag | `Truncated bool` | `"truncated"` | — |
| company name (`fetch_status`) | `Name string` | `"company"` | `companies.name` |
| run status | `Status string` | `"status"` | `fetch_runs.status` |
| postings count | `PostingsCount *int32` | `"postings_count"` | `fetch_runs.postings_count` |

## Open questions
- Row cap and statement-timeout defaults — propose 1000 rows / 5s.
- Read-only DSN env var name — propose `MCP_DATABASE_URL` (alt: `DATABASE_URL_RO`).
- Settings invocation — `go run ./cmd/mcp` (from `apps/tools/`, no build step, slower start) vs. a prebuilt `apps/tools/bin/mcp` (faster, needs build). Propose `go run` for dev simplicity.
- `mark3labs/mcp-go` vs. the official `github.com/modelcontextprotocol/go-sdk` (now exists, High source reputation). You chose mark3labs after research; flagged only because the official SDK is a long-term alternative. Default: proceed with mark3labs.
