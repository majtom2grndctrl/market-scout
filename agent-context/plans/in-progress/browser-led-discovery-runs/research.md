# Research anchors — browser-led-discovery-runs

Code-grounded facts the spec relies on. Confirmed against source 2026-07-02. Not durable — delete when the epic ships.

## MCP server (`apps/tools/cmd/mcp/`)

- Library: `github.com/mark3labs/mcp-go v0.55.0` (`.../mcp`, `.../server`). Tools built with `mcp.NewTool(name, opts...)`, registered via `s.AddTool(tool, handler)` in `newMCPServer`; handler type `server.ToolHandlerFunc`. Args decoded with `mcpReq.BindArguments(&req)`. Results via `mcp.NewToolResultText(string(payload))`. `mcp.NewToolResultError` reserved for undecodable calls.
- Pools: `type dbPools struct { readOnly *sql.DB; action *sql.DB }`. Read-only tools bind `pools.readOnly`. `dedup_candidates` is read-only → `pools.readOnly`.
- Action-error shape (`main.actionError`, in `add_company.go`): `Path`/`Code`/`Message` with json tags `path`/`code`/`message`. Reuse for the empty-`name` envelope.
- Per-tool seams are inline `pool*`-backed struct literals (e.g. `poolSelector{pool}`), not `NewXService` constructors. `xHandler(pool)` wires production deps into `xHandlerWithDeps(...)` for tests. `enrichment_preview` and `detect_ats` are the closest read-only templates.

## Existing dedup query

`apps/tools/internal/db/queries/onboard.sql` → `FindCompanyDedupStatus :one`. Args: `ats`, `board_token`, `recency_days int`. Returns `company_id bigint`, `has_recent_snapshot bool`. Zero rows when no `(ats, board_token)` match. The 30-day window is a caller-passed parameter, not hardcoded. No name-based match query exists — the spec adds one.

## Schema (dedup chain)

- `companies`: `id bigserial`, `name text NOT NULL`, `ats text NOT NULL`, `board_token text NOT NULL`. Unique `uq_companies_ats_board_token (ats, board_token)`.
- Recency chain: `posting_snapshots.job_posting_id → job_postings.id`, `job_postings.company_id → companies.id`. Recency column `posting_snapshots.fetched_at timestamptz NOT NULL`.

## Watchlist rules reused (not re-derived)

`agent-context/lib/watchlist.md` §Dedup: normalize both sides (strip punctuation + whitespace, lowercase, equality; substring is not a match). `(ats, board_token)` + recent snapshot within 30 days → `duplicate`. Match with no recent snapshot, or name match with non-matching/absent ATS → `stale` (surfaced for inspection, never a silent duplicate). Flag uncertain matches; never silently skip.
