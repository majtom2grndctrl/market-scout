# Research Notes — MCP Server

Grounding facts gathered during drafting. Not decisions — those live in `index.md`.

## mcp-go API (verified via Context7, `/mark3labs/mcp-go`)

- Server: `server.NewMCPServer(name, version string, opts ...server.ServerOption)`. Useful opts: `server.WithToolCapabilities(true)`, `server.WithRecovery()`.
- Tool def: `mcp.NewTool(name, mcp.WithDescription(...), mcp.WithString("sql", mcp.Required(), mcp.Description(...)))`. Also `mcp.WithNumber`, `mcp.Enum`.
- Register: `s.AddTool(tool, handler)`.
- Handler signature: `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)`.
- Typed arg access: `req.RequireString("sql")`, `req.RequireFloat(...)` — return an error to convert into `mcp.NewToolResultError(err.Error())`.
- Results: `mcp.NewToolResultText(string)`, `mcp.NewToolResultError(string)`. Structured I/O alternative exists (`mcp.WithInputSchema[T]`, `mcp.WithOutputSchema[T]`, `mcp.NewStructuredToolHandler`) — not needed for v1; JSON-as-text is simpler and agents parse it fine.
- Transport: `server.ServeStdio(s)` blocks until terminated. Right transport for a local CLI-integrated tool.
- Imports: `github.com/mark3labs/mcp-go/mcp`, `github.com/mark3labs/mcp-go/server`.

## Codebase grounding (verified against source)

- Module path: `github.com/majtom2grndctrl/market-scout/apps/tools` (`go.mod:1`).
- `db.New(db DBTX) *Queries` — `apps/tools/internal/db/db.go:19`. `DBTX` satisfied by `*sql.DB` and `*sql.Tx`.
- DB wiring (both `cmd/fetcher` and `cmd/onboard`): `godotenv.Load(".env.local")` → `os.Getenv("DATABASE_URL")` (fatal if empty) → `sql.Open("pgx", dsn)` → `pool.PingContext` (10s timeout) → `db.New(pool)`. pgx registered via blank import `_ "github.com/jackc/pgx/v5/stdlib"`.
- main/run split: `cmd/onboard` uses `func run(args, stdout, stderr) int` returning an exit code (`exitOK=0`, `exitGenericError=1`, `exitPreconditionMissing=2`); `main` just `os.Exit(run(...))`. `cmd/fetcher` uses `func run() error`. The onboard pattern fits the MCP binary.
- Signal handling: `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.
- slog: `slog.New(slog.NewTextHandler(stderr, nil))` then `slog.SetDefault`. Subsystem tag convention: `[mcp]`.

## fetch_runs (for Task 3)

`fetch_runs` columns (migration `000005`): `id`, `company_id`, `started_at`, `completed_at` (nullable), `status` CHECK in (`in_progress`,`success`,`failed`), `error_message` (nullable), `postings_count` (nullable int). Existing queries are write-only (`InsertFetchRun`, `MarkFetchRunSuccess`, `MarkFetchRunFailed`) — no read query yet, so Task 3 adds the first one. Index `idx_fetch_runs_company_id_started_at (company_id, started_at DESC)` (added in migration `000006`) supports the `DISTINCT ON` shape.

## Existing read queries (context for what `query` replaces)

The 9 sqlc read queries are oriented to the fetcher/enrichment pipelines, not verification: `ListCompaniesWithATS`, `GetCurrentClassificationForPosting`, `ListCurrentClassifications`, taxonomy lists (`ListCanonicalRoles/Specializations/Skills/RoleDimensions`), `ListLatestDescriptionsByCompany`, `FindCompanyDedupStatus`. No query lists job_postings or posting_snapshots. The generic `query` tool covers all of these cases and arbitrary verification slices without adding sqlc queries per question.

## Test conventions

Unit tests alongside source; integration tests gated `//go:build integration`, skip when `DATABASE_URL` unset, open a real pool. `cmd/fetcher/main_test.go` uses `httptest` + fake interfaces; `apps/tools/cmd/fetcher/fetch_runs_integration_test.go` shows the DB-integration shape.
