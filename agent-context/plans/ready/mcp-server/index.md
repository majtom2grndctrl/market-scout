# Read-Only MCP Server (`cmd/mcp`)

## Goal
A local MCP server exposing read-only access to the market-scout Postgres DB, so Claude Code agents can verify pipeline output — did the fetcher and enricher store data correctly? — without writing SQL against a read-write connection. Replaces the current ad-hoc "agent writes SQL and runs it directly" workflow with a safe, sandboxed equivalent.

## Scope

### In scope
- New Go binary `cmd/mcp`, built on `github.com/mark3labs/mcp-go`, stdio transport.
- A `query` tool: accepts a SQL string, executes it read-only, returns rows as JSON. Deliberate, sanctioned exception to the repo's sqlc-only rule (no string-built SQL in business logic) — the read-only role, not Go-side SQL parsing, is the safety boundary.
- Safety enforced by a dedicated read-only Postgres role (SELECT-only), not by parsing SQL strings in Go.
- Defense in depth: read-only transaction, per-statement timeout, result row cap.
- One curated tool, `fetch_status`: latest `fetch_run` per company — directly answers "did the fetcher run correctly."
- Read-only role provisioning script + setup docs.
- Claude Code wiring via a repo-root `.mcp.json` `mcpServers` entry.

### Out of scope
- Any write/mutation tools (re-queue, re-fetch, re-classify). Reads only.
- Curated tools beyond `fetch_status` (classification coverage, snapshot field coverage, seed/DB drift). Add once usage shows which checks recur; agents express these through `query` meanwhile.
- Reading `failures.jsonl` or other file-based surfaces — the DB is the surface here.
- Market-analysis / product features (the Next.js app and its agent are a separate workstream).
- Auth / multi-user. Single local admin user only.

## Acceptance criteria
- [ ] `go run ./cmd/mcp` (from `apps/tools/`) starts an stdio MCP server; a Claude Code session configured per the repo-root `.mcp.json` entry lists its tools.
- [ ] `query` with a valid `SELECT` returns an envelope object `{ "rows": [...], "row_count": N, "truncated": bool }`, where each element of `rows` is an object keyed by column name (column name → value; SQL NULL → JSON null). `row_count` equals `len(rows)` — the number of rows returned after any cap, not the total matching rows (the cap+1 scan strategy cannot know the true total).
- [ ] `query` with an `INSERT`/`UPDATE`/`DELETE`/DDL statement returns an error and mutates no data (row counts unchanged afterward).
- [ ] A result set larger than the row cap is truncated to the cap, and the response indicates truncation.
- [ ] A query exceeding the statement timeout is aborted and returns a timeout error rather than hanging.
- [ ] The binary exits non-zero with a clear message when `DATABASE_URL_RO` is unset.
- [ ] `fetch_status` returns, per company, the company name, the most recent run's status, started/completed times, postings count, and error message (null when none).

## Tasks

### Task 1: MCP server scaffold + read-only `query` tool
Create `cmd/mcp/main.go` following the `cmd/onboard` structure: a testable `run` body, `slog` to stderr, `godotenv.Load(".env.local")` (errors are non-fatal — `.env.local` may be absent in CI or production), `signal.NotifyContext` for shutdown. Add `mark3labs/mcp-go` to `go.mod`. Read the read-only DSN from `DATABASE_URL_RO`; fail fast if unset — no fallback to `DATABASE_URL`, since the fallback would silently defeat the safety boundary. Open with `sql.Open("pgx", dsn)`, ping with a timeout; a failed startup ping (DB unreachable) also exits non-zero with a clear message, mirroring the unset-`DATABASE_URL_RO` failure path (AC #6). Register a `query` tool with one required string param `sql`. Handler runs the SQL inside a read-only transaction (`sql.TxOptions{ReadOnly: true}`) issuing `SET LOCAL statement_timeout` as the first statement in the tx, scans dynamic columns into a JSON-friendly shape, caps rows, and returns `mcp.NewToolResultText(json)` where `json` is the envelope object (`{rows, row_count, truncated}`), not a bare array — matching AC #2 and the Boundary inventory. SQL execution failures (write rejected, statement timeout, invalid SQL) return via `mcp.NewToolResultError(...)` (a tool result with `is_error`), satisfying AC #3 and AC #5; the handler's `error` return value is reserved for infrastructure failures (server cannot send a response). Row cap (1000) and statement timeout are `const` literals (`statementTimeout = 5 * time.Second`) in the binary — no env/config knobs; add config only if a real need appears. `SET LOCAL statement_timeout` takes a string literal: `SET LOCAL statement_timeout = '5s'` — a bare integer would mean milliseconds (5ms), not 5 seconds. Serve over stdio with `server.ServeStdio`.

### Task 2: Read-only role provisioning + setup docs
Provide an idempotent SQL script (e.g. `apps/tools/internal/db/setup/readonly_role.sql`) that creates the read-only role and grants `SELECT` on all current **and future** tables in `public` (`GRANT SELECT ON ALL TABLES IN SCHEMA public` plus `ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES`). Document the one-time setup step and `DATABASE_URL_RO` (read-only DSN, sits next to `DATABASE_URL` in `.env.local`) in `developer-guide.md`. This is operational setup, not a numbered schema migration — roles are cluster-level, and a migration would bake a credential into source. Note: `apps/tools/internal/db/setup/readonly_role.sql` is operational SQL, not sqlc input or output — sqlc scans only `internal/db/queries/` (and migrations), so this file is not picked up by `sqlc generate` and is not subject to the developer-guide §5.8 "never hand-edit db/" generated-file rule. **Credential handling:** the script must not commit a secret. It creates the role with `LOGIN` but leaves the password to the operator, set out-of-band (`\password <role>` in psql, or a `psql -v` variable substituted at apply time). The setup doc instructs the operator to choose a password and write the matching read-only DSN into `.env.local` as `DATABASE_URL_RO`.

### Task 3: `fetch_status` curated tool
Add a sqlc query in `apps/tools/internal/db/queries/` returning the latest `fetch_run` per company (`DISTINCT ON (company_id) ... ORDER BY company_id, started_at DESC`), joined to `companies.name`. Run `sqlc generate`; commit SQL input and generated output together. Register a `fetch_status` tool (no params) that calls the query through `db.New(conn)` (where `conn` is the `*sql.DB` from Task 1; `db.New` accepts any `DBTX`, satisfied by `*sql.DB` or `*sql.Tx`) and returns the rows as JSON in the same envelope shape as `query`. Do not marshal the sqlc row struct directly: `sqlc.yaml` has no `emit_json_tags`, so generated structs have no json tags (fields marshal as PascalCase `Name`/`Status`/`StartedAt`) and `sql.NullTime`/`sql.NullString`/`sql.NullInt32` marshal as `{"Time":...,"Valid":...}` objects rather than the lowercase keys and JSON null the Boundary inventory specifies. Instead, define a dedicated response DTO struct with explicit json tags matching the Boundary inventory keys (`json:"company"`, `json:"status"`, `json:"started_at"`, `json:"completed_at"`, `json:"postings_count"`, `json:"error_message"`); map `StartedAt` to an RFC3339 string; map each nullable field to `*string` / `*int32` / `*time.Time` (or an `any` set to `nil`) so it marshals as JSON null when not valid. Map each sqlc row to the DTO before marshaling.

### Task 4: Claude Code wiring + tests
Add an `mcpServers` entry for the `cmd/mcp` server to the repo-root `.mcp.json` (Claude Code project MCP config; `.claude/settings.local.json` holds only `permissions`). The entry is `{"command": "go", "args": ["run", "-C", "apps/tools", "./cmd/mcp"], "env": {"DATABASE_URL_RO": "${DATABASE_URL_RO}"}}`. `DATABASE_URL_RO` is supplied through the entry's `env` map (Claude Code expands `${DATABASE_URL_RO}` from its own environment), so the server does **not** depend on the launched process's working directory to locate config — `godotenv.Load(".env.local")` stays a convenience fallback for direct `go run` invocation only. `go run -C apps/tools` resolves the `./cmd/mcp` package path against the module; the `-C` is a build-path concern, not a config-loading one. Unit-test the row→JSON encoding (type mapping, NULL handling, truncation) with no DB. Add an integration test (`//go:build integration`) that, against a real Postgres connected as the read-only role, confirms a `SELECT` returns rows and a write statement is rejected. Also assert (a) `run` exits non-zero with a clear message when `DATABASE_URL_RO` is unset, and (b) a query exceeding `statement_timeout` returns a timeout error and a result above the row cap comes back with `truncated: true`. The integration test skips when `DATABASE_URL_RO` is unset (matching the repo's skip-on-unset convention) and requires the read-only role provisioned in Task 2.

## Sequencing
**Phase 1 (sequential):** Task 1 — establishes server, tool registration, and query execution that everything builds on.
**Phase 2 (concurrent):** Task 2, Task 3 — independent of each other.
**Phase 3 (sequential):** Task 4 — wires `.mcp.json` and tests the finished surface; consumes Tasks 1–3.

## Rough sketch
mcp-go API (verified against current docs): `server.NewMCPServer(name, version, server.WithToolCapabilities(true), server.WithRecovery())`; `mcp.NewTool("query", mcp.WithDescription(...), mcp.WithString("sql", mcp.Required(), mcp.Description(...)))`; `s.AddTool(tool, handler)` with handler `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`; `req.RequireString("sql")`; results via `mcp.NewToolResultText(...)` / errors via `mcp.NewToolResultError(...)`; `server.ServeStdio(s)` blocks. Imports `github.com/mark3labs/mcp-go/mcp` and `.../server`.

DB safety layers, outermost first: (1) read-only role — the real boundary; (2) `BeginTx(ctx, &sql.TxOptions{ReadOnly: true})`; (3) `SET LOCAL statement_timeout` inside the tx; (4) app-side row cap — scan at most cap+1 rows, truncate, set a flag.

Result encoding contract: the top-level result is the envelope (`{rows, row_count, truncated}` — see the Boundary inventory and AC #2); `rows` is the array of per-row objects keyed by column name. Duplicate column names in a result set collapse last-wins under `map[string]any`; an agent that needs both must alias them in its query. Accepted, agent's responsibility. Scan strategy: call `rows.Columns()` to get the column name slice; for each row, scan into a `[]any` of `len(cols)` (pgx stdlib driver returns concrete Go types), then zip names and values into `map[string]any{col: val}` — this is the bridge from positional slice to the `[]map[string]any` shape in the Boundary inventory. The "driver returns concrete Go types" assumption is load-bearing: for an `*any` destination, `database/sql` hands back whatever the driver's `Value()` yields, which is not guaranteed to be `time.Time`/`int64`. The unit/integration test must assert the actual driver return types for timestamp, integer, and text columns early; any column that arrives as unexpected `[]byte` falls through to the already-documented base64 behavior (cast to `text` in the query to recover). Convert `time.Time` to an RFC3339 string, and leave every other scanned value to default `encoding/json` marshaling. SQL NULL → JSON null. Known v1 limitation: `jsonb` and `numeric` arrive from the driver as `[]byte`, which `encoding/json` base64-encodes — correct for `bytea`, but `jsonb`/`numeric` surface as base64 strings rather than JSON/number. An agent that needs them as JSON or number casts to `text` in its query. Accepted v1 simplification, not a defect — no per-type decoding machinery. No dedicated schema tool needed — the agent introspects via `query` against `information_schema`.

## Boundary inventory
| Name | Go | JSON key | SQL |
|---|---|---|---|
| SQL input | `req.RequireString("sql")` | `"sql"` | — |
| row count | `RowCount int` | `"row_count"` | — |
| truncated flag | `Truncated bool` | `"truncated"` | — |
| rows array (`query`) | `Rows []map[string]any` | `"rows"` | — |
| company name (`fetch_status`) | `Name string` | `"company"` | `companies.name` |
| run status | `Status string` | `"status"` | `fetch_runs.status` |
| postings count (`fetch_status`) | `PostingsCount sql.NullInt32` | `"postings_count"` (null when absent) | `fetch_runs.postings_count` |
| started at (`fetch_status`) | `StartedAt time.Time` | `"started_at"` (RFC3339) | `fetch_runs.started_at` |
| completed at (`fetch_status`) | `CompletedAt sql.NullTime` | `"completed_at"` (null when in_progress) | `fetch_runs.completed_at` |
| error message (`fetch_status`) | `ErrorMessage sql.NullString` | `"error_message"` (null when none) | `fetch_runs.error_message` |

## Decisions
- **Row cap / statement timeout:** 1000 rows / 5s, `const` literals in the binary. No env/config knobs — add config only if a real need appears.
- **Read-only DSN env var:** `DATABASE_URL_RO` — read-only-ness is the salient property; sits next to `DATABASE_URL` in `.env.local`.
- **Invocation:** `go run -C apps/tools ./cmd/mcp`, no build step. The server starts once per Claude Code session over stdio, so startup cost amortizes; matches the repo's `go run ./cmd/X` convention.
- **Config delivery:** `DATABASE_URL_RO` is injected via the `.mcp.json` entry's `env` map (`${DATABASE_URL_RO}` expansion), not loaded from the launched process's CWD. Removes the dependence on the spawned binary's working directory; `godotenv` remains a fallback for direct `go run`.
- **`mark3labs/mcp-go` over the official `github.com/modelcontextprotocol/go-sdk`:** the binding is a handful of API calls in one `main.go`, so switching later is a contained change — low-consequence and reversible.
