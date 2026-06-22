# MCP Safe Actions

## Goal

Extend the local MCP server from read-only inspection to narrow, safe actions agents can request without receiving broad database write credentials.

The first actions cover watchlist growth and enrichment writeback: add a verified company to the fetcher set, preview enrichment candidates, and save an agent-produced enrichment payload.

## Scope

### In scope

- Keep the generic `query` MCP tool read-only.
- Add an action-capable MCP database role with only the privileges needed by approved actions.
- Add curated MCP tools for company insertion, enrichment preview, and enrichment writeback.
- Reuse existing validation and business logic where practical instead of creating a second ruleset inside `cmd/mcp`.
- Return structured JSON envelopes for every action so agents can verify what happened.
- Tests for role privilege boundaries, company action idempotence, and enrichment save validation.

### Out of scope

- General-purpose write SQL over MCP.
- Running migrations, changing schema, or editing taxonomy dimensions through MCP tools.
- Removing companies, deleting postings, updating historical snapshots, or updating/deleting historical classifications.
- Agent UI or web app changes.
- Scheduler/cron setup for recurring MCP actions.
- Remote/multi-user authorization. This remains a local single-operator tool.

## Acceptance Criteria

- [ ] Existing `query` behavior remains read-only and still uses `DATABASE_URL_RO`.
- [ ] Existing `fetch_status` behavior remains read-only and still uses `DATABASE_URL_RO`.
- [ ] `cmd/mcp` starts when both read-only and action DSNs are set; it exits non-zero with a clear startup error when `DATABASE_URL_ACTIONS` is absent.
- [ ] No MCP tool accepts arbitrary write SQL.
- [ ] `add_company` inserts or no-ops one company by `(ats, board_token)`, returning the canonical DB row and an `"inserted"` boolean.
- [ ] `add_company` rejects unsupported ATS values, empty names, empty board tokens, malformed Workday/Workable tokens, and malformed careers page URLs.
- [ ] `add_company` probes the ATS board by default; failed probes return a structured action error and insert no row.
- [ ] Company insertion does not mutate `posting_snapshots`, `job_postings`, `classifications`, or taxonomy tables.
- [ ] The action role cannot run ad-hoc `INSERT`, `UPDATE`, `DELETE`, DDL, temp table creation, or writes outside approved MCP functions.
- [ ] `enrichment_preview` returns candidate counts and sample postings using the same selection rules as `batch-enrich`.
- [ ] `save_enrichment` validates an agent-produced enrichment payload, inserts a new append-only classification, attaches taxonomy joins, and never updates or deletes existing classification history.
- [ ] `save_enrichment` rejects unknown role-dimension slugs, invalid seniority, invalid slugs, cross-table slug collisions, and payloads for nonexistent postings.
- [ ] `go test ./cmd/mcp -count=1` and `go test ./...` pass.
- [ ] Integration tests under `//go:build integration` prove action-role write boundaries against Postgres.

## Tasks

### Task 1: Define MCP action boundary

Split MCP capabilities into two explicit DB connections: read-only inspection and approved actions. Keep `DATABASE_URL_RO` for the existing `query`, `fetch_status`, and `enrichment_preview` tools. Add a separate `DATABASE_URL_ACTIONS` for curated tools that need writes.

The action connection is not exposed to the generic `query` tool. Every mutation must flow through named Go handlers and sqlc-generated calls to approved database functions. `DATABASE_URL_ACTIONS` is required in v1; if it is unset or fails startup checks, `cmd/mcp` exits non-zero before serving any tools. Startup errors should name the missing or misconfigured action DSN without printing credentials.

### Task 2: Provision a minimal action role

Add `apps/tools/internal/db/setup/action_role.sql` beside the existing read-only setup script. It creates and normalizes `market_scout_actions`, the role used by `DATABASE_URL_ACTIONS`. The action role should start from no privileges, then receive only what approved action tools need.

Initial grants:

- `CONNECT` on the database.
- `USAGE` on the dedicated MCP action schema.
- `EXECUTE` on approved MCP action functions.

Do not grant table writes, schema create, database temporary privileges, blanket function execute, or taxonomy writes directly to the role. The script should revoke inherited/public paths first, normalize role flags, and fail if the role owns objects or belongs to other roles.

Approved functions live in a dedicated `mcp` schema, are created by a numbered migration, are owned by the database owner, and use `SECURITY DEFINER` with a pinned `search_path`. The action role executes those functions; the functions perform the table writes with owner privileges. This is the enforceable boundary: a leaked `DATABASE_URL_ACTIONS` DSN can call only the approved functions, not arbitrary table writes.

### Task 3: Add company action SQL

Add a schema-level function in the `mcp` schema for the curated company write path, plus sqlc input under `apps/tools/internal/db/queries/` that calls it.

Function contract:

- Inputs: `name`, `ats`, `board_token`, `industry`, `careers_page_url`.
- Behavior: insert with `ON CONFLICT (ats, board_token) DO NOTHING`, then return the canonical row.
- Output: canonical company row plus an `inserted` boolean.

The function must not update existing company metadata. Stale merge remains human-owned. Regenerate sqlc and commit SQL input plus generated output together.

### Task 4: Implement `add_company`

Add an MCP tool with explicit request fields. These are MCP DTO fields, not sqlc model fields; response mapping should translate to the generated `db.Company` fields such as `Ats` and `CareersPageUrl`.

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
- `careers_page_url`, when present, must parse as an absolute HTTP(S) URL.
- `industry`, when present, is trimmed and stored verbatim.

`probe` defaults to true when omitted. Use a pointer/nullable bool in the request DTO so omitted and explicit false are distinguishable. When `probe=true`, construct the live ATS adapter and call `FetchPostings` with the board token; do not import `cmd/onboard` helpers because they are package-main internals. A probe failure aborts without insert. When `probe=false`, the tool still validates syntax, then inserts.

`probe_result` shape:

| Field | JSON key | Meaning |
|---|---|---|
| ATS | `"ats"` | ATS value probed |
| Board token | `"board_token"` | Board token probed |
| Attempted | `"attempted"` | false only when `probe=false` |
| Valid | `"valid"` | true when `FetchPostings` succeeds; empty boards are valid |
| Postings count | `"postings_count"` | Count returned by the adapter when valid |
| Error | `"error"` | Probe error string when invalid |

Response shape should include `ok`, `inserted`, `company`, `seed_file_updated`, `follow_up`, and `probe_result`. Validation, probe, and DB failures return `ok=false` in the JSON envelope via `mcp.NewToolResultText`, not an MCP transport error. Reserve MCP tool errors for malformed tool calls the server cannot decode.

### Task 5: Keep seed-file drift explicit

Today `agent-context/lib/watchlist.md` says the seed file is canonical. Direct MCP insertion creates a local DB row that is not reflected in `apps/tools/internal/db/seeds/companies.sql`.

For v1, make this explicit in the tool response: include `"seed_file_updated": false` and a `"follow_up"` message naming the seed file. Do not have MCP edit source files as a side effect of a DB action.

Leave a clear follow-up hook in the response for a future `stage_company_seed_patch` action that writes seed SQL for human review.

### Task 6: Add enrichment preview

Add an `enrichment_preview` MCP tool that reports what `batch-enrich` would select without spawning agents.

Inputs:

- `count`: integer, default 10, min 1, max 100.
- `focus`: string, default `""`; `%` and `_` keep the same SQL ILIKE wildcard behavior as `batch-enrich`.
- `force`: boolean, default false.

Use the existing selection logic or extract it to a reusable package if needed. Preview uses `DATABASE_URL_RO`, not `DATABASE_URL_ACTIONS`; the read-only role already has the needed table reads. Response includes `ok`, input echo, selected count, already-classified count when forced, and a sample capped at 20 rows.

The sample must include posting id, company id, company name, and title. Current `batch-enrich` selection returns posting id, company id, title, and description only, so the implementation must add a preview-specific join or extend the shared selection query/type to include company name.

### Task 7: Add enrichment save action

Add `save_enrichment`, an MCP tool that persists an enrichment payload produced by an agent or human-reviewed agent workflow. It does not spawn agents and does not invoke `cmd/batch-enrich`.

Input shape mirrors the per-posting classifier contract already used by `batch-enrich`:

- `posting_id`
- `classification.seniority`
- `classification.notes`
- `canonical_roles[]` with `slug`, `name`, and `dimensions[]`
- `specializations[]` with `slug` and `name`
- `skills[]` with `slug`, `name`, and optional `requirement`
- `summary` for response/reporting only; do not persist until a summary storage column exists

Validation should reuse the `batch-enrich` validation rules by extracting shared types/rules out of `cmd/batch-enrich` if needed. Do not import `cmd/batch-enrich` directly because it is `package main`. The MCP action may use the read-only pool to load taxonomy and confirm the posting exists. It must reject invalid seniority, invalid slugs, duplicate slugs, unknown role dimensions, cross-table slug collisions, null bytes, notes that exceed the existing cap, and nonexistent postings before calling the action function.

Persistence should call an approved `SECURITY DEFINER` function or a small set of approved functions through sqlc. The function path owns the transaction: get-or-create canonical roles, specializations, and skills; attach canonical role dimensions; insert one classifications row; attach role/specialization/skill join rows. The function must insert a new classification row every time rather than updating prior rows.

Response includes `ok`, `classification_id`, `posting_id`, `summary`, `new_taxonomy`, and any validation failure hints. `skills[].requirement` is accepted and echoed in the response but not persisted, matching current classifier behavior.

### Task 8: Tests and integration coverage

Unit tests:

- Action DSN startup validation without printing secrets.
- `add_company` input validation.
- `add_company` idempotent response mapping with a fake DB/action interface.
- `save_enrichment` payload validation and response mapping.
- Shared enrichment validation rejects the same cases as `batch-enrich`.

Integration tests:

- Action role can insert a company through the approved function.
- Action role can save one classification through the approved function path.
- Action role cannot write directly to snapshots, postings, classifications, taxonomy tables, or schema.
- Read-only role remains unable to run writes.
- `add_company` writes exactly one company on repeated calls.
- `save_enrichment` inserts a new classification on repeated calls without mutating previous classifications.

## Sequencing

**Phase 1 (sequential):** Task 1 — defines the boundary and server wiring.
**Phase 2 (concurrent):** Task 2, Task 3 — role setup and sqlc query input are independent.
**Phase 3 (sequential):** Task 4 — consumes Tasks 1–3.
**Phase 4 (sequential):** Task 5 — documents and surfaces seed drift after the DB action exists.
**Phase 5 (concurrent):** Task 6, Task 7 — preview is read-only; enrichment save consumes the action-function pattern from earlier phases.
**Phase 6 (sequential):** Task 8 — final test sweep across all action surfaces.

## Rough Sketch

`cmd/mcp` should grow small internal seams rather than become a monolith:

- DB opening: separate read-only and action pools.
- Tool registration: read-only tools always use the read-only pool; action tools use an action service.
- Company action service: validates input, optionally probes ATS, calls the approved company function through sqlc, maps response DTOs.
- Enrichment service: validates MCP inputs, previews selection through existing selection logic, saves enrichment payloads through approved function calls.

Favor package extraction only when needed to reuse existing command logic. `cmd/batch-enrich` is currently `package main`, so direct imports are not available. The likely clean split is to move selection/config logic that both CLI and MCP need into an internal package, while leaving process control in `cmd/batch-enrich`.

For company insertion, do not reuse `cmd/onboard` directly in v1. `onboard` is sidecar-and-seed-file oriented, while MCP needs one verified action with a structured response.

For enrichment, do not launch `cmd/batch-enrich` in v1. MCP owns only the save boundary. Agents may produce enrichment payloads through their own workflow, then call `save_enrichment` to persist them.

## Boundary Inventory

| Boundary | Input | Output | Owner |
|---|---|---|---|
| MCP `add_company` | JSON tool args | JSON action envelope | `cmd/mcp` |
| Company DB write | Validated action struct | `companies` row | sqlc query layer |
| ATS probe | ATS + board token | `probe_result` JSON | `cmd/mcp` using `internal/ats` adapters |
| MCP `enrichment_preview` | bounded selection args | JSON preview envelope | `cmd/mcp` + shared selection logic |
| MCP `save_enrichment` | classifier-shaped JSON payload | JSON save envelope | `cmd/mcp` + approved function |
| Classification writeback | validated enrichment payload | DB classifications/taxonomy joins | approved `SECURITY DEFINER` function path |

## Decisions

- MCP insertion does not update `apps/tools/internal/db/seeds/companies.sql` in v1. Source-file mutation stays a separate human-reviewed step.
- `save_enrichment` does not require a preview token in v1. The payload validator and append-only DB function are the safety rails.
- Approved action functions live in a dedicated `mcp` schema.
