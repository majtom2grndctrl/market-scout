# MCP Safe Actions

## Goal

Extend the local MCP server from read-only inspection to narrow, safe actions agents can request without receiving broad database write credentials.

The first actions cover watchlist growth and enrichment orchestration: add a verified company to the fetcher set, preview enrichment candidates, and launch bounded enrichment runs.

## Scope

### In scope

- Keep the generic `query` MCP tool read-only.
- Add an action-capable MCP database role with only the privileges needed by approved actions.
- Add curated MCP tools for company insertion and enrichment orchestration.
- Reuse existing validation and business logic where practical instead of creating a second ruleset inside `cmd/mcp`.
- Return structured JSON envelopes for every action so agents can verify what happened.
- Tests for role privilege boundaries, company action idempotence, and enrichment command construction.

### Out of scope

- General-purpose write SQL over MCP.
- Running migrations, changing schema, or editing taxonomy dimensions through MCP.
- Removing companies, deleting postings, or mutating historical snapshots/classifications.
- Agent UI or web app changes.
- Scheduler/cron setup for recurring MCP actions.
- Remote/multi-user authorization. This remains a local single-operator tool.

## Acceptance Criteria

- [ ] Existing `query` behavior remains read-only and still uses `DATABASE_URL_RO`.
- [ ] `cmd/mcp` starts when both read-only and action DSNs are set; it exits non-zero with a clear startup error when an action tool is enabled but the action DSN is absent.
- [ ] No MCP tool accepts arbitrary write SQL.
- [ ] `add_company` inserts or no-ops one company by `(ats, board_token)`, returning the canonical DB row and an `"inserted"` boolean.
- [ ] `add_company` rejects unsupported ATS values, empty names, empty board tokens, malformed Workday/Workable tokens, and values that violate existing company check constraints.
- [ ] `add_company` can optionally probe the ATS board before insertion; failed probes return a tool error and insert no row.
- [ ] Company insertion does not mutate `posting_snapshots`, `job_postings`, `classifications`, or taxonomy tables.
- [ ] The action role cannot run ad-hoc `INSERT`, `UPDATE`, `DELETE`, DDL, temp table creation, or writes outside the approved sqlc action path.
- [ ] `enrichment_preview` returns candidate counts and sample postings using the same selection rules as `batch-enrich`.
- [ ] `run_enrichment` launches a bounded `batch-enrich` invocation with validated flags, captures stdout/stderr, timeout, and exit code, and returns a structured report summary.
- [ ] `run_enrichment` enforces local safety caps for count, force, concurrency, and runtime before invoking the CLI.
- [ ] `go test ./cmd/mcp -count=1` and `go test ./...` pass.
- [ ] Integration tests under `//go:build integration` prove action-role write boundaries against Postgres.

## Tasks

### Task 1: Define MCP action boundary

Split MCP capabilities into two explicit DB connections: read-only inspection and approved actions. Keep `DATABASE_URL_RO` for the existing `query` and `fetch_status` tools. Add a separate `DATABASE_URL_ACTIONS` for curated tools that need writes.

The action connection is not exposed to the generic `query` tool. Every mutation must flow through named Go handlers and sqlc-generated statements. The MCP server should register action tools only after action DB startup checks pass. Startup errors should name the missing or misconfigured action DSN without printing credentials.

### Task 2: Provision a minimal action role

Add operational SQL beside the existing read-only setup script. The action role should start from no privileges, then receive only what approved action tools need.

Initial grants:

- `CONNECT` on the database.
- `USAGE` on `public`.
- `SELECT` on lookup/read tables needed by actions.
- `INSERT` on `companies` and `USAGE` on `companies_id_seq`.

Do not grant blanket table writes, schema create, database temporary privileges, function execute, or taxonomy writes. The script should revoke inherited/public paths first, normalize role flags, and fail if the role owns objects or belongs to other roles.

Future enrichment write privileges should be added only if the MCP server writes enrichment rows directly. The first enrichment action should launch the existing CLI, so it does not require taxonomy/classification grants for the MCP action role.

### Task 3: Add company action SQL

Add sqlc input under `apps/tools/internal/db/queries/` for the curated company write path:

- Find company by `(ats, board_token)`.
- Insert company with fields accepted by the tool.
- Return the canonical company row after insert/no-op.

Use `ON CONFLICT (ats, board_token) DO NOTHING` or an equivalent transaction shape that preserves existing rows. Do not update existing company metadata in this plan; stale merge remains human-owned.

Regenerate sqlc and commit SQL input plus generated output together.

### Task 4: Implement `add_company`

Add an MCP tool with explicit fields:

| Name | Go field | JSON key | SQL column |
|---|---|---|---|
| Company name | `Name` | `"name"` | `companies.name` |
| ATS | `ATS` | `"ats"` | `companies.ats` |
| Board token | `BoardToken` | `"board_token"` | `companies.board_token` |
| Industry | `Industry` | `"industry"` | `companies.industry` |
| Careers page URL | `CareersPageURL` | `"careers_page_url"` | `companies.careers_page_url` |
| Probe before insert | `Probe` | `"probe"` | — |

Validation:

- `ats` must be one of the live adapter keys.
- `name`, `ats`, and `board_token` are required after trimming.
- Workday token must be `{host}/{site}` and must not include URL scheme or locale segment.
- Workable token must be a lowercase bare slug.
- Optional metadata must satisfy existing database constraints before insert.

When `probe=true`, reuse the ATS adapter probe behavior. A probe failure aborts without insert. When `probe=false`, the tool still validates syntax and DB constraints, then inserts. Response shape should include `inserted`, `company`, and optional `probe_result`.

### Task 5: Keep seed-file drift explicit

Today `agent-context/lib/watchlist.md` says the seed file is canonical. Direct MCP insertion creates a local DB row that is not reflected in `apps/tools/internal/db/seeds/companies.sql`.

For v1, make this explicit in the tool response: include `"seed_file_updated": false` and a `"follow_up"` message naming the seed file. Do not have MCP edit source files as a side effect of a DB action.

Add a follow-up note to the plan's open questions: whether to later add a separate `stage_company_seed_patch` action that writes seed SQL for human review.

### Task 6: Add enrichment preview

Add an `enrichment_preview` MCP tool that reports what `batch-enrich` would select without spawning agents.

Inputs mirror safe selection flags:

- `count`
- `focus`
- `force`

Use the existing selection logic or extract it to a reusable package if needed. Response includes selected count, already-classified count when forced, and a capped sample of posting ids, company names, and titles. This lets an agent inspect scope before launching an expensive action.

### Task 7: Add bounded enrichment launcher

Add `run_enrichment` as a wrapper around the existing `cmd/batch-enrich` binary behavior.

Inputs:

- `count`
- `focus`
- `force`
- `report_format`
- `agent_timeout`
- `strip_timeout`

Safety caps:

- Maximum `count` defaults to 25.
- `force=false` by default; `force=true` requires an explicit input.
- `max_parallel`, `wave_size`, and `batch_size` are fixed conservative values in the MCP handler for v1.
- Overall subprocess timeout defaults to a bounded value.
- Progress output is disabled or slowed enough that stderr remains manageable.

The launcher should use `exec.CommandContext` from `apps/tools/`, pass flags explicitly, capture stdout and stderr separately, and return a JSON envelope with exit code, duration, parsed report when JSON report format is used, and truncated stderr.

### Task 8: Tests and integration coverage

Unit tests:

- Action DSN startup validation without printing secrets.
- `add_company` input validation.
- `add_company` idempotent response mapping with a fake DB/action interface.
- Enrichment flag validation and command construction.
- Subprocess timeout/cancellation mapping.

Integration tests:

- Action role can insert a company through the approved query.
- Action role cannot write to snapshots, postings, classifications, taxonomy tables, or schema.
- Read-only role remains unable to run writes.
- `add_company` writes exactly one company on repeated calls.

## Sequencing

**Phase 1 (sequential):** Task 1 — defines the boundary and server wiring.
**Phase 2 (concurrent):** Task 2, Task 3 — role setup and sqlc query input are independent.
**Phase 3 (sequential):** Task 4 — consumes Tasks 1–3.
**Phase 4 (sequential):** Task 5 — documents and surfaces seed drift after the DB action exists.
**Phase 5 (concurrent):** Task 6, Task 7 — preview and launcher share validation ideas but can be built independently after the action boundary exists.
**Phase 6 (sequential):** Task 8 — final test sweep across all action surfaces.

## Rough Sketch

`cmd/mcp` should grow small internal seams rather than become a monolith:

- DB opening: separate read-only and action pools.
- Tool registration: read-only tools always use the read-only pool; action tools use an action service.
- Company action service: validates input, optionally probes ATS, writes through sqlc, maps response DTOs.
- Enrichment service: validates MCP inputs, previews selection through existing selection logic, launches the CLI through a subprocess runner interface.

Favor package extraction only when needed to reuse existing command logic. `cmd/batch-enrich` is currently `package main`, so direct imports are not available. The likely clean split is to move selection/config logic that both CLI and MCP need into an internal package, while leaving process control in `cmd/batch-enrich`.

For company insertion, do not reuse `cmd/onboard` directly in v1. `onboard` is sidecar-and-seed-file oriented, while MCP needs one verified action with a structured response.

For enrichment, prefer launching the existing CLI in v1. Directly embedding classification orchestration in MCP would make the server own long-running agent subprocess state, writeback, progress reporting, and failure logs. The CLI already owns those contracts.

## Boundary Inventory

| Boundary | Input | Output | Owner |
|---|---|---|---|
| MCP `add_company` | JSON tool args | JSON action envelope | `cmd/mcp` |
| Company DB write | Validated action struct | `companies` row | sqlc query layer |
| ATS probe | ATS + board token | success/error | `internal/ats` adapters |
| MCP `enrichment_preview` | bounded selection args | JSON preview envelope | `cmd/mcp` + shared selection logic |
| MCP `run_enrichment` | bounded run args | JSON run envelope | `cmd/mcp` subprocess wrapper |
| Classification writeback | CLI-selected postings | DB classifications/taxonomy joins | `cmd/batch-enrich` |

## Open Questions

1. Should MCP insertion update `apps/tools/internal/db/seeds/companies.sql`, or should source-file mutation stay a separate human-reviewed step? This plan chooses no source-file side effect for v1.
2. Should `add_company` default `probe` to true? Safer, but slower and dependent on network availability. This plan accepts either if documented in implementation; default true is preferred for agent-initiated actions.
3. Should `run_enrichment` require a preview token from `enrichment_preview` before launching? Nice safety rail, but possibly too much ceremony for local use. Defer unless review finds the launcher too easy to misuse.
4. Should action tools be disabled unless `DATABASE_URL_ACTIONS` is set, while read-only tools still start? This plan chooses fail-fast for enabled action tools; implementation can expose a clear disabled-tools mode if MCP clients handle it well.
