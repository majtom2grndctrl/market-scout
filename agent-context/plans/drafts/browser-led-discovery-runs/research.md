# Research anchors — browser-led-discovery-runs

Code-grounded facts the spec relies on. Confirmed against source 2026-07-02. Not durable — delete when the epic ships.

## MCP server (`apps/tools/cmd/mcp/`)

- Library: `github.com/mark3labs/mcp-go v0.55.0` (`.../mcp`, `.../server`). Tools built with `mcp.NewTool(name, opts...)`, registered via `s.AddTool(tool, handler)`; handler type `server.ToolHandlerFunc`. Args decoded with `mcpReq.BindArguments(&req)`. Results via `mcp.NewToolResultText(string(payload))`. `mcp.NewToolResultError` reserved for undecodable calls.
- Pools: `type dbPools struct { readOnly *sql.DB; action *sql.DB }`. Both required at startup (`openVerifiedPool` on `DATABASE_URL_RO` / `DATABASE_URL_ACTIONS`). Read-only tools bind `pools.readOnly`; action tools bind `pools.action`; `save_enrichment` takes both.
- Action-error shape appears twice with identical json tags (`path`/`code`/`message`): `main.actionError` (in `add_company.go`) and `atsdetect.ActionError` (in `internal/atsdetect/detect.go`).
- `add_company` calls its approved function with a fixed parameterized `QueryRowContext` on `poolExecutor{pool}` (constant `addCompanySQL`, `RETURNS TABLE` bypasses sqlc). `save_enrichment` uses a sqlc-generated call (`mcp.save_enrichment` returns scalar `jsonb`). New multi-column `RETURNS TABLE` functions follow the `add_company` raw-QueryRow pattern; scalar `jsonb`-returning functions can use sqlc.
- Per-tool seams are inline struct literals (`poolExecutor{pool}`, `poolSelector{pool}`), not `NewXService` constructors. `xHandler(pool)` wires production deps into `xHandlerWithDeps(...)` for tests.

## Existing dedup query

`apps/tools/internal/db/queries/onboard.sql` → `FindCompanyDedupStatus :one`. Args: `ats`, `board_token`, `recency_days int`. Returns `company_id bigint`, `has_recent_snapshot bool`. Zero rows when no `(ats, board_token)` match. The 30-day window is a caller-passed parameter, not hardcoded. No name-based match query exists.

## Schema

- Migrations run through `000012`. Next is `000013`.
- `companies`: `id bigserial PK`, `name text NOT NULL`, `ats text NOT NULL`, `board_token text NOT NULL`, `industry text`, `careers_page_url text`, `created_at timestamptz`. Unique constraint `uq_companies_ats_board_token UNIQUE (ats, board_token)`.
- Dedup recency chain: `posting_snapshots.job_posting_id → job_postings.id`, `job_postings.company_id → companies.id`. Recency column `posting_snapshots.fetched_at timestamptz NOT NULL`.
- `mcp` schema created in `000009`; `REVOKE ALL ON SCHEMA mcp FROM PUBLIC`. Owned by migration role `market_scout`.
- Approved-function pattern (`000010`/`000011`/`000012`): `CREATE FUNCTION mcp.<name>(...) ... SECURITY DEFINER SET search_path = pg_catalog`, all app objects fully qualified `public.<t>`/`mcp.<fn>`, trailing `REVOKE ALL ON FUNCTION mcp.<name>(<argtypes>) FROM PUBLIC`. Explicit `GRANT EXECUTE ... TO market_scout_actions` lives in `internal/db/setup/action_role.sql` (one REVOKE+GRANT block per function), reran after each such migration.
- Seed file `apps/tools/internal/db/seeds/companies.sql`: `INSERT INTO companies (name, ats, board_token, industry) VALUES ...`, `ON CONFLICT (ats, board_token) DO NOTHING`.

## Watchlist rules reused (not re-derived)

`agent-context/lib/watchlist.md` §Dedup: normalize both sides (strip punctuation + whitespace, lowercase, equality; substring is not a match). `(ats, board_token)` + recent snapshot within 30 days → `duplicate`. Match with no recent snapshot, or name match with non-matching ATS → `stale-needs-merge`. Flag uncertain matches; never silently skip. Status taxonomy: `unsupported-ats`, `no-careers`, `dead`, `duplicate`, `stale-needs-merge`, `invalid-token`.
