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
- Product schema/model changes beyond the dedicated MCP action schema and approved action functions. Adding those functions by migration is in scope.
- Running migrations or editing taxonomy dimensions through MCP tools.
- Removing companies, deleting postings, updating historical snapshots, or updating/deleting historical classifications.
- Agent UI or web app changes.
- Scheduler/cron setup for recurring MCP actions.
- Remote/multi-user authorization. This remains a local single-operator tool.

## Acceptance Criteria

- [ ] Existing `query` behavior remains read-only and still uses `DATABASE_URL_RO`.
- [ ] Existing `fetch_status` behavior remains read-only and still uses `DATABASE_URL_RO`.
- [ ] `cmd/mcp` starts when both read-only and action DSNs are set; it exits non-zero with a clear startup error when `DATABASE_URL_ACTIONS` is absent.
- [ ] No MCP tool accepts arbitrary write SQL. Review verifies only `query` accepts a freeform SQL argument, and it is bound to the read-only pool and read-only transaction.
- [ ] `add_company` inserts or no-ops one company by `(ats, board_token)`, returning the canonical DB row and an `"inserted"` boolean.
- [ ] `add_company` rejects unsupported ATS values, empty names, empty board tokens, malformed Workday/Workable tokens, and malformed careers page URLs.
- [ ] `add_company` probes the ATS board by default; failed probes return a structured action error and insert no row.
- [ ] Company insertion does not mutate `posting_snapshots`, `job_postings`, `classifications`, or taxonomy tables. Integration tests assert scoped before/after row counts for test data; review verifies `mcp.add_company` writes only `companies`.
- [ ] The action role cannot run ad-hoc `INSERT`, `UPDATE`, `DELETE`, DDL, temp table creation, or writes outside approved MCP functions.
- [ ] `enrichment_preview` returns `selected_count`, `sample_count`, `already_classified_count`, and sample postings using the same selection rules as `batch-enrich`.
- [ ] `save_enrichment` validates an agent-produced enrichment payload, inserts a new append-only classification, attaches taxonomy joins, and never updates or deletes existing classification history. Integration tests assert repeated calls append rows and seeded prior classifications remain unchanged; review verifies `mcp.save_enrichment` does not update or delete classification history.
- [ ] `save_enrichment` rejects the full stable validation-code table in Task 7, including unknown role dimensions, missing or invalid seniority, invalid slugs, duplicate slugs, cross-table collisions, null bytes, notes length, invalid provenance, empty dimensions, and nonexistent postings.
- [ ] From `apps/tools`: `go test ./cmd/mcp -count=1` and `go test ./...` pass.
- [ ] From `apps/tools`: `go test -tags=integration ./cmd/mcp -count=1` proves action-role write boundaries against Postgres when `DATABASE_URL`, `DATABASE_URL_RO`, and `DATABASE_URL_ACTIONS` are set.

## Tasks

### Task 1: Define MCP action boundary

Split MCP capabilities into two explicit DB connections: read-only inspection and approved actions. Keep `DATABASE_URL_RO` for the existing `query`, `fetch_status`, and `enrichment_preview` tools. Add a separate `DATABASE_URL_ACTIONS` for curated tools that need writes.

The action connection is not exposed to the generic `query` tool. Every mutation must flow through named Go handlers and sqlc-generated calls to approved database functions. `DATABASE_URL_ACTIONS` is required in v1; if it is unset or fails startup checks, `cmd/mcp` exits non-zero before serving any tools. Startup errors should name the missing or misconfigured action DSN without printing credentials.

### Task 2: Provision a minimal action role

Add `apps/tools/internal/db/setup/action_role.sql` beside the existing read-only setup script. It creates and normalizes `market_scout_actions`, the role used by `DATABASE_URL_ACTIONS`. Add the numbered migration that creates the `mcp` schema baseline. The action role should start from no privileges, then receive only what approved action tools need.

Initial grants:

- `CONNECT` on the database.
- `USAGE` on the dedicated MCP action schema.
- `EXECUTE` on approved MCP action functions that already exist.

Do not grant table writes, schema create, database temporary privileges, blanket function execute, or taxonomy writes directly to the role. The script should revoke inherited/public paths first, normalize role flags, and fail if the role owns objects or belongs to other roles.

Approved functions live in a dedicated `mcp` schema, are created by a numbered migration, are owned by the database owner, and use `SECURITY DEFINER SET search_path = pg_catalog`. Fully qualify every application table and function inside them: `public.<table>` and `mcp.<function>`. Revoke public paths explicitly: `REVOKE ALL ON SCHEMA mcp FROM PUBLIC` and `REVOKE ALL ON FUNCTION ... FROM PUBLIC` for each approved function before granting `market_scout_actions`. The action role executes those functions; the functions perform the table writes with owner privileges. This is the enforceable boundary: a leaked `DATABASE_URL_ACTIONS` DSN can call only the approved functions, not arbitrary table writes.

Grant ownership:

- Numbered migrations create the `mcp` schema and approved functions.
- Task 2 creates the role script and `mcp` schema baseline. It may grant only functions that already exist.
- Tasks 3 and 7 update `action_role.sql` with explicit `EXECUTE` grants after adding each approved function.
- `action_role.sql` creates/normalizes `market_scout_actions` and grants `USAGE` plus explicit `EXECUTE` on each approved function.
- Add every new approved function to `action_role.sql`; do not use blanket `GRANT EXECUTE ON ALL FUNCTIONS`.
- Operators rerun `action_role.sql` after migrations that add approved MCP functions.

### Task 3: Add company action SQL

Add `mcp.add_company` in the `mcp` schema for the curated company write path, plus sqlc input under `apps/tools/internal/db/queries/` that calls it.

Approved-function requirements:

- Add the function in a numbered migration.
- Use `SECURITY DEFINER SET search_path = pg_catalog`.
- Fully qualify application objects as `public.<table>` and `mcp.<function>`.
- Revoke `EXECUTE` from `PUBLIC` before granting the action role.
- Update `apps/tools/internal/db/setup/action_role.sql` with an explicit `EXECUTE` grant for `mcp.add_company`.

Function contract:

- Signature: `mcp.add_company(p_name text, p_ats text, p_board_token text, p_industry text, p_careers_page_url text)`.
- Behavior: insert with `ON CONFLICT (ats, board_token) DO NOTHING`, then return the canonical row.
- Output columns: `id bigint`, `name text`, `ats text`, `board_token text`, `created_at timestamptz`, `industry text`, `careers_page_url text`, `inserted boolean`.

The function must not update existing company metadata. Stale merge remains human-owned. The sqlc call query must pass nullable arguments for `p_industry` and `p_careers_page_url` so omitted values become SQL NULL, not empty strings. Regenerate sqlc and commit SQL input plus generated output together.

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

- `ats` must be one of: `greenhouse`, `lever`, `ashby`, `workday`, `workable`.
- `name`, `ats`, and `board_token` are required after trimming.
- Workday token must have exactly two slash-separated components: `{host}/{site}`. `host` must match `^[a-z0-9.-]+\.myworkdayjobs\.com$`; `site` must be non-empty and contain no slash, query, or fragment. Do not rely on the current Workday adapter parser for this stricter validation.
- Workable token must match `^[a-z0-9]+(-[a-z0-9]+)*$`. Do not rely on the current Workable adapter parser for this stricter lowercase validation.
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

Response shape should include `ok`, `inserted`, `company`, `seed_file_updated`, `follow_up`, `probe_result`, and `errors`. Validation, probe, and DB failures return `ok=false` in the JSON envelope via `mcp.NewToolResultText`, not an MCP transport error. Reserve MCP tool errors for malformed tool calls the server cannot decode.

Failure defaults:

| Field | Default |
|---|---|
| `ok` | `false` |
| `inserted` | `false` |
| `company` | `null` |
| `seed_file_updated` | `false` |
| `follow_up` | `null` |
| `probe_result` | `null` unless the probe ran; probe failures include the failed probe result |
| `errors` | non-empty array |

Failure errors use the shared action error shape: `{ "path": string, "code": string, "message": string }`.

`add_company` error codes:

| Code | Path |
|---|---|
| `missing_required` | missing field |
| `unsupported_ats` | `ats` |
| `invalid_board_token` | `board_token` |
| `invalid_url` | `careers_page_url` |
| `probe_failed` | `probe` |
| `db_error` | `db` |

`company` response shape:

| Field | JSON key | Null handling |
|---|---|---|
| ID | `"id"` | never null |
| Name | `"name"` | never null |
| ATS | `"ats"` | never null |
| Board token | `"board_token"` | never null |
| Created at | `"created_at"` | RFC3339 string, never null |
| Industry | `"industry"` | null when absent |
| Careers page URL | `"careers_page_url"` | null when absent |

### Task 5: Keep seed-file drift explicit

Today `agent-context/lib/watchlist.md` says the seed file is canonical. Direct MCP insertion creates a local DB row that is not reflected in `apps/tools/internal/db/seeds/companies.sql`.

For v1, make this explicit in the tool response: always include `"seed_file_updated": false`. On successful insert or no-op company responses, include a `"follow_up"` message naming the seed file. Failure responses keep `follow_up` null. Do not have MCP edit source files as a side effect of a DB action.

Leave a clear follow-up hook in the response for a future `stage_company_seed_patch` action that writes seed SQL for human review.

### Task 6: Add enrichment preview

Add an `enrichment_preview` MCP tool that reports what `batch-enrich` would select without spawning agents.

Inputs:

- `count`: integer, default 10, min 1, max 100.
- `focus`: string, default `""`; `%` and `_` keep the same SQL ILIKE wildcard behavior as `batch-enrich`.
- `force`: boolean, default false.

Extract selection/config logic that MCP and `cmd/batch-enrich` both need into a dedicated internal package before adding the tool. Task 6 owns selection extraction; Task 7 must not move the same files. Preview uses `DATABASE_URL_RO`, not `DATABASE_URL_ACTIONS`; the read-only role already has the needed table reads. MCP validates `count` against its own 1..100 bound before calling shared selection logic.

Response includes `ok`, input echo, `selected_count`, `sample_count`, `already_classified_count`, and a sample capped at 20 rows. `selected_count` is the number of rows selected after the requested `count` limit, not a pre-limit total. `sample_count` is the number of rows in the returned sample. Do not add `total_candidate_count` unless the implementation adds a separate count query. `already_classified_count` is always present: when `force=true`, it counts selected postings that already have at least one classification row; when `force=false`, it is `0` because classified postings are excluded from selection.

The sample must include posting id, company id, company name, and title. Current `batch-enrich` selection returns posting id, company id, title, and description only, so the implementation must add a preview-specific join or extend the shared selection query/type to include company name.

### Task 7: Add enrichment save action

Add `save_enrichment`, an MCP tool that persists an enrichment payload produced by an agent or human-reviewed agent workflow. It does not spawn agents and does not invoke `cmd/batch-enrich`.

Approved-function requirements:

- Add the function in a numbered migration.
- Use `SECURITY DEFINER SET search_path = pg_catalog`.
- Fully qualify application objects as `public.<table>` and `mcp.<function>`.
- Revoke `EXECUTE` from `PUBLIC` before granting the action role.
- Update `apps/tools/internal/db/setup/action_role.sql` with an explicit `EXECUTE` grant for `mcp.save_enrichment`.

Input shape mirrors the per-posting classifier contract already used by `batch-enrich`:

- `posting_id`
- `provenance.model`
- `provenance.prompt_version`
- `classification.seniority`
- `classification.notes`
- `canonical_roles[]` with `slug`, `name`, and `dimensions[]`
- `specializations[]` with `slug` and `name`
- `skills[]` with `slug`, `name`, and optional `requirement`
- `summary` for response/reporting only; do not persist until a summary storage column exists

`provenance.model` defaults to `"mcp-agent"` when omitted. `provenance.prompt_version` defaults to `"mcp-save-enrichment-v1"` when omitted. Both must match `^[A-Za-z0-9._-]+$` after defaults are applied.

Validation should reuse the `batch-enrich` validation rules by extracting shared types/rules out of `cmd/batch-enrich` if needed. Do not import `cmd/batch-enrich` directly because it is `package main`. Shared validation must expose structured failures; add an adapter that converts them back to existing `batch-enrich` retry hints. The MCP action may use the read-only pool to load taxonomy and confirm the posting exists. It must reject invalid seniority, invalid slugs, duplicate slugs, unknown role dimensions, cross-table slug collisions, null bytes, notes that exceed the existing cap, and nonexistent postings before calling the action function.

Task 7 owns validation/type extraction only. It must not move the selection/config files extracted by Task 6.

Slug rules:

- Taxonomy slugs must match `^[a-z0-9]+(-[a-z0-9]+)*$` and be at most 64 characters.
- Duplicate slugs within a payload array are invalid.
- A slug emitted in two different payload arrays is invalid.
- Cross-table collision checks run against existing taxonomy across canonical roles, specializations, skills, and role dimensions, and against the payload itself.

Stable validation codes:

| Code | Path |
|---|---|
| `missing_seniority` | `classification.seniority` |
| `invalid_seniority` | `classification.seniority` |
| `invalid_slug` | `<array>[i].slug` |
| `slug_too_long` | `<array>[i].slug` |
| `duplicate_slug` | duplicate `<array>[i].slug` |
| `slug_collision` | colliding `<array>[i].slug` |
| `empty_dimensions` | `canonical_roles[i].dimensions` |
| `unknown_dimension` | `canonical_roles[i].dimensions[j]` |
| `null_byte` | offending string field |
| `notes_too_long` | `classification.notes` |
| `invalid_provenance` | `provenance.model` or `provenance.prompt_version` |
| `posting_not_found` | `posting_id` |

Persistence should call one approved function through sqlc: `mcp.save_enrichment(p_payload jsonb, p_model text, p_prompt_version text) RETURNS jsonb`. The function runs as one SQL statement; PostgreSQL rolls back all writes from the statement on error. It owns the full write path: get-or-create canonical roles, specializations, and skills; attach canonical role dimensions; insert one classifications row; attach role/specialization/skill join rows. The function must insert a new classification row every time rather than updating prior rows.

The MCP request wraps the current `batch-enrich` AgentResponse shape and adds MCP-only provenance. Pass provenance separately as `p_model` and `p_prompt_version` after defaults. `summary` is echoed only by Go. `skills[].requirement` is retained in the request and response, stripped from `p_payload`, and not persisted.

Exact `p_payload` shape:

```json
{
  "posting_id": 123,
  "classification": {
    "seniority": "senior",
    "notes": "text"
  },
  "canonical_roles": [
    {
      "slug": "backend-engineer",
      "name": "Backend Engineer",
      "dimensions": ["api-platform"]
    }
  ],
  "specializations": [
    {
      "slug": "distributed-systems",
      "name": "Distributed Systems"
    }
  ],
  "skills": [
    {
      "slug": "go",
      "name": "Go"
    }
  ]
}
```

Inside `mcp.save_enrichment`, re-check cross-table slug ownership and role-dimension existence before inserts. SQL-level invariant violations return JSON with `ok=false` and `errors[]` using the shared action error shape; do not require Go to parse arbitrary exception text for validation errors. Unexpected DB failures may still surface as `db_error`.

`mcp.save_enrichment` returns JSON with `classification_id`, `posting_id`, and `new_taxonomy`. The function must distinguish newly inserted taxonomy from existing taxonomy; do not rely on current `GetOrCreate*` sqlc queries, which return only IDs.

Response includes `ok`, `classification_id`, `posting_id`, `summary`, `new_taxonomy`, and `errors`. `skills[].requirement` is accepted and echoed in the response but not persisted, matching current classifier behavior.

`new_taxonomy` shape:

| Field | JSON key | Value |
|---|---|---|
| Canonical roles | `"canonical_roles"` | array of `{ "slug": string, "name": string }` |
| Specializations | `"specializations"` | array of `{ "slug": string, "name": string }` |
| Skills | `"skills"` | array of `{ "slug": string, "name": string }` |

Validation errors use this shape:

| Field | JSON key | Value |
|---|---|---|
| Path | `"path"` | dotted/indexed field path, e.g. `canonical_roles[0].slug` |
| Code | `"code"` | stable snake_case error code |
| Message | `"message"` | human-readable hint |

### Task 8: Tests and integration coverage

Unit tests:

- Action DSN startup validation without printing secrets.
- `add_company` input validation.
- `add_company` idempotent response mapping with a fake DB/action interface.
- `save_enrichment` payload validation and response mapping.
- Shared enrichment validation rejects the same cases as `batch-enrich`.

Integration tests:

Integration tests require `DATABASE_URL`, `DATABASE_URL_RO`, and `DATABASE_URL_ACTIONS`. Skip integration tests when any required DSN is absent. Use `DATABASE_URL` for setup and cleanup, `DATABASE_URL_RO` for read-only assertions, and `DATABASE_URL_ACTIONS` for action-role boundary assertions.

- Action role can insert a company through the approved function.
- Action role can save one classification through the approved function path.
- Action role cannot write directly to snapshots, postings, classifications, taxonomy tables, or schema.
- Action role cannot directly insert, update, or delete `companies`.
- Action role cannot `CREATE TEMP TABLE`.
- Read-only role remains unable to run writes.
- `add_company` writes exactly one company on repeated calls.
- `save_enrichment` inserts a new classification on repeated calls without mutating previous classifications.

## Sequencing

**Phase 1 (sequential):** Task 1 — defines the boundary and server wiring.
**Phase 2 (sequential):** Task 2 — creates the role/setup framework and `mcp` schema convention.
**Phase 3 (sequential):** Task 3 — creates the company function/query and consumes Task 2's schema convention.
**Phase 4 (sequential):** Task 4 — consumes Tasks 1–3.
**Phase 5 (sequential):** Task 5 — documents and surfaces seed drift after the DB action exists.
**Phase 6 (sequential):** Task 6 — extracts shared selection/config logic and adds preview.
**Phase 7 (sequential):** Task 7 — extracts validation/type logic and adds enrichment save without moving Task 6's files.
**Phase 8 (sequential):** Task 8 — final test sweep across all action surfaces.

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
