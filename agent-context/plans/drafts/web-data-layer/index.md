# Web App Data Layer

## Goal

Give `apps/web` a Postgres access layer, and give the whole system one shared definition of "currently open posting." Every product screen asks questions the append-only schema answers only by derivation, so the derivation lands in SQL views that the Next.js app and the agent's MCP `query` tool both read. Adds mutable storage for user preferences and saved searches — the first write path the web app owns.

## Scope

### In scope

- Read-model views deriving open postings, current snapshot state, and current classification.
- Mutable tables for user preferences and saved searches.
- A `market_scout_app` database role and `DATABASE_URL_APP` DSN.
- `apps/web` Postgres client: `postgres.js`, connection singleton, `.env.local` link.
- Typed read queries under `apps/web/lib/db/`, and Server Actions for saved-search writes.
- One unstyled proof page listing open postings, plus saved-search create/list/delete.
- Integration coverage asserting the app role cannot write market-intel tables.

### Out of scope

- Designed product screens. This plan proves the data layer; UI work follows in its own brief.
- Facet counts and aggregate trend queries. The views make them possible; no screen needs them yet.
- Materialized views. Measured cost does not justify a refresh story (see Task 1).
- pgvector semantic search from the web app. Embedding storage is still deferred per `project.md`.
- Pagination beyond a `LIMIT`. 4,466 open postings render acceptably in one query.
- Auth and multi-user support. One instance per fork; no `user_id` anywhere.
- HTTP route handlers. Server Components and Server Actions cover both directions.

## Acceptance criteria

- [ ] A posting counts as open only when it appeared in the most recent **successful** fetch run for its company; a failed or in-progress run never causes a posting to read as closed.
- [ ] The open-posting view exposes the timestamp of the successful run it derived from, so a caller can distinguish a posting confirmed today from one last confirmed 53 days ago.
- [ ] Postings with no classification appear in the read model with null classification fields rather than being dropped.
- [ ] A posting with more than one classification row resolves to its most recent one, and its prior classifications remain in the table.
- [ ] Snapshots with a null `fetch_run_id` do not cause their posting to be reported as open.
- [ ] Current roles, specializations, and skills are retrievable per posting from the read model, one row per posting per term, and reflect only that posting's most recent classification.
- [ ] Querying the full open-posting read model completes in under 250ms against the current dataset, and its query plan contains no sequential scan over `posting_snapshots`.
- [ ] The agent's MCP `query` tool can select from every new view without a grant change beyond what the migrations apply.
- [ ] Connecting as `market_scout_app` and attempting to insert, update, or delete any row in `companies`, `job_postings`, `posting_snapshots`, `classifications`, or `fetch_runs` is rejected by Postgres.
- [ ] Connecting as `market_scout_app` and inserting, updating, and deleting rows in the preferences and saved-search tables succeeds.
- [ ] A stored saved search holds structured filter terms that name slugs and ids; no stored value contains SQL text.
- [ ] Starting `apps/web` with `DATABASE_URL_APP` unset fails with an error naming that variable, and does not fall back to another DSN.
- [ ] `pnpm build` in `apps/web` completes with the database stopped.
- [ ] Editing a file during `pnpm dev` does not increase the Postgres connection count for the web app beyond its configured pool maximum.
- [ ] The proof page lists open postings with company name, title, location, seniority, and the timestamp of the run that confirmed each one, and unclassified postings appear with their classification fields empty.
- [ ] Submitting saved-search criteria containing an unrecognized key is rejected, and the rejection renders on screen rather than surfacing as an unhandled error.
- [ ] Creating a saved search through the UI, reloading the page, and deleting it all reflect immediately without a manual refresh.
- [ ] `pnpm typecheck` and `pnpm build` pass in `apps/web`; `go test ./...` passes from `apps/tools`.

## Tasks

### Task 1: Read-model views

Add migration `000016_read_model_views`. Three derived relations: the latest successful fetch run per company, the set of open postings, and a display row joining each open posting to its current snapshot state and current classification.

- Filter fetch runs to `status = 'success'` before taking the latest per company. `ListLatestFetchRunsByCompany` in `queries/fetch_runs.sql` takes the latest run of *any* status — mirror its `DISTINCT ON` shape, not its filter.
- Derive openness from a posting having a snapshot whose `fetch_run_id` matches that latest successful run. Absence from a *failed* run means nothing; conflating the two makes a network blip look like a wave of closed roles.
- Carry the successful run's `started_at` through to the display view. The oldest latest-success in the current data is 52.9 days old, so "open" alone would overstate freshness.
- Resolve current snapshot and current classification with one lateral per posting, ordered descending and limited to one. Both are already index-served: `idx_posting_snapshots_posting_fetched` and `idx_classifications_posting_classified`.
- Join classification with an outer join. Only 2,862 of 7,906 postings are classified, and the open subset skews further unclassified — an inner join would hide most of the data.
- Expose current taxonomy (roles, specializations, skills) keyed by posting in long form, one row per posting per term. Saved-search filters and future facet counts both read it; an aggregated array column serves display but not counting.
- Grant `SELECT` on each new view to `market_scout_readonly` explicitly in the migration. The `pg_default_acl` entry for `market_scout` already covers new relations in `public`, but `action_role.sql` documents a case where default-privilege semantics silently no-op'd — an explicit grant costs one line and does not depend on ACL merge subtleties.

Then measure. Baseline for the full display query is 94.9ms, all buffers cached, and its plan contains a sequential scan over all 61,642 rows of `posting_snapshots` because the openness `EXISTS` flattens into a hash aggregate. `idx_posting_snapshots_fetch_run_id` covers `fetch_run_id` alone and cannot serve it. Evaluate an index on `posting_snapshots (fetch_run_id, job_posting_id)`, keep it only if `EXPLAIN (ANALYZE, BUFFERS)` shows the sequential scan gone, and record the before and after numbers in the commit message.

Do not:
- Reuse `ListLatestFetchRunsByCompany` as-is; it does not filter by status.
- Add materialized views or a refresh mechanism.
- Hand-edit anything under `apps/tools/internal/db/` outside `migrations/` and `setup/` — see `developer-guide.md` §5.8.

### Task 2: Preferences and saved-search tables

Add migration `000017_user_preferences_and_saved_searches`. Two mutable tables.

- Store preferences as key-value with a `jsonb` value, keyed by a text primary key. The user expects to keep adding preference values; typed columns would mean a migration per addition.
- Give saved searches a unique human-supplied name, a `jsonb` criteria object, and created and updated timestamps.
- Store criteria as structured filter terms — role slugs, dimension slugs, company ids, date bounds — never SQL. Persisted SQL is an arbitrary-execution path from stored data, and it silently rots when the views underneath it change shape.
- Write a comment in the migration recording that these two tables are deliberately mutable, and that the append-only rule governs snapshot and classification history rather than the whole schema. Without it the next reader either "fixes" these tables into append-only or reads their mutability as licence to upsert snapshots.

Do not:
- Add a `user_id` column or a users table.
- Add `SECURITY DEFINER` wrapper functions. The `mcp` schema uses them because the agent writes to core tables and the write *shape* needed constraining; these tables have one writer and no provenance requirement, so table grants suffice.

### Task 3: `market_scout_app` role

Add `apps/tools/internal/db/setup/app_role.sql`, following `readonly_role.sql` and `action_role.sql`: idempotent, no committed credential, password set out of band by the operator.

- Grant `SELECT` on all current and future tables in `public`, plus `INSERT`, `UPDATE`, and `DELETE` on the two tables from Task 2 by name and their sequences.
- Grant nothing else. The property worth keeping is the one already true of the other two roles: the process serving the browser *cannot* write market-intel tables, enforced by Postgres rather than by discipline.
- Reject membership in other roles and ownership of objects, mirroring the guard blocks both existing scripts open with.
- Document the role and `DATABASE_URL_APP` in `developer-guide.md` §2 next to the existing DSNs, including that `app_role.sql` must be rerun after any migration that adds a table the web app writes.

| Mirror | Don't mirror |
|---|---|
| `readonly_role.sql` — table-level grants, default privileges | `action_role.sql` — per-function `EXECUTE` grants, `SECURITY DEFINER` boundary |

Do not use `GRANT ALL` or grant write on any table not named in Task 2.

### Task 4: Postgres client in `apps/web`

Add `postgres.js` as a dependency and a client module under `apps/web/lib/db/`.

- Cache the client on `globalThis`. Next dev re-evaluates modules on edit, so a bare module-scope client leaks a pool per reload until Postgres refuses connections.
- Set an explicit `max`. The default pool size against a local single-user Postgres is unnecessary headroom and makes the leak above harder to spot.
- Read `DATABASE_URL_APP`, and fail with a message naming the variable when it is unset. `cmd/mcp` fails fast on a missing `DATABASE_URL_RO` rather than falling back to the owner DSN; same reasoning — a fallback silently defeats the boundary.
- Symlink the root env file: `ln -s ../../.env.local apps/web/.env.local`. Next loads `.env.local` from its own directory, and `apps/tools/.env.local` is already the same symlink. Add it to the setup steps in `developer-guide.md` §2 beside the existing `ln -s`; it is a local step, not checked in.
- Call `connection()` from `next/server` before the first query in every module that queries. Without it `next build` prerenders DB-reading pages and runs the queries at build time, which fails whenever the database is not up.
- Keep column names as they come back from Postgres and write `snake_case` fields in the row interfaces. A `transform` option is a second place for names to drift from the views.

Do not:
- Add a codegen step for row types. Hand-written interfaces on a handful of queries; `project.md` already declined codegen-for-sync-only when it rejected a token pipeline.
- Add `pg` alongside `postgres.js`.
- Import the client into any Client Component.

### Task 5: Read queries and proof page

Add query functions under `apps/web/lib/db/` returning typed rows from the Task 1 views, and a page that renders open postings as an unstyled list — company, title, location, seniority, and the run timestamp behind "open."

- One exported async function per question the UI asks, each with its own row interface. Queries belong in `lib/db/`, not inline in components, so the SQL has one home.
- Compose optional filters as nested `sql` fragments rather than string concatenation, since saved-search criteria arrive as an open-ended object.
- Render in a Server Component and query directly. There is no HTTP hop and no client-side fetch.
- Show unclassified postings with their classification fields visibly empty rather than filtering them out — they are the majority of the open set.

The page is a correctness check, not a design artifact. Styling comes later in its own brief.

### Task 6: Saved-search Server Actions

Add `'use server'` actions for create, rename, delete, and preference read/write, backed by the Task 2 tables.

- Validate the criteria object's shape before writing, and reject unknown keys. It is read back as query input, so an unvalidated write becomes a malformed query later, far from its cause.
- Call `revalidatePath` for the affected route after each mutation so the list reflects the change without a manual reload.
- Return a typed result the caller can render on failure. A thrown error in an action surfaces as an unhandled rejection with nothing useful on screen.

Do not add route handlers for these; Server Actions are the mutation path.

### Task 7: Role boundary integration test

Extend the integration tests in `apps/tools/cmd/mcp/` — or a sibling package if they prove a poor fit — to cover the app role, following `action_integration_test.go`, which already opens pools for three separate DSNs and asserts what each role cannot do.

- Assert writes to `companies`, `job_postings`, `posting_snapshots`, `classifications`, and `fetch_runs` are all rejected as `market_scout_app`.
- Assert insert, update, and delete succeed on the two Task 2 tables as the same role.
- Skip when `DATABASE_URL_APP` is unset, matching the repo's skip-on-unset convention.
- Keep the `//go:build integration` tag so the default `go test ./...` stays DB-free.

## Sequencing

**Phase 1 (concurrent):** Task 1, Task 2 — independent migrations; numbers pre-assigned above so they cannot collide.
**Phase 2 (sequential):** Task 3 — grants name Task 2's tables.
**Phase 3 (sequential):** Task 4 — needs `DATABASE_URL_APP` from Task 3.
**Phase 4 (concurrent):** Task 5, Task 6 — both consume Task 4's client; separate files.
**Phase 5 (sequential):** Task 7 — asserts the boundary Tasks 2 and 3 establish.

## Boundary inventory

SQL is the source of truth for names; TypeScript mirrors it verbatim. No transform layer.

| Name | SQL column | TS field | Saved-search criteria key |
|---|---|---|---|
| Posting id | `job_posting_id` | `job_posting_id` | `company_ids` (array of company id) |
| Company name | `name` | `company_name` (aliased in the view) | — |
| Run timestamp | `started_at` | `run_started_at` (aliased in the view) | — |
| Seniority | `seniority` | `seniority` | `seniority` |
| Role slug | `slug` | `role_slug` | `role_slugs` |
| Dimension slug | `slug` | `dimension_slug` | `dimension_slugs` |

Aliases are applied in the view definition, not in the TypeScript layer, so both consumers see one name. `jsonb` arrives already parsed — `postgres.js` registers `JSON.parse` for OIDs 114 and 3802.

## Rough sketch

Views build on each other rather than repeating the openness derivation: latest successful run per company → open postings → display row. Anything needing "open" selects from the second, so the failed-run rule has exactly one definition.

`connection()` returns a hanging promise during prerender and resolves immediately on a real request, which is why it is the guard rather than `export const dynamic = 'force-dynamic'` — the latter throws `DynamicServerError` at build time instead of deferring.

Filter composition in `postgres.js` reduces an array of fragments: `conditions.reduce((a, b) => sql`${a} AND ${b}`)`, with `sql(ids)` expanding an array into a parameterized `IN` list.

## Open questions

- Should the display view carry a coarse `is_stale` flag, or leave freshness judgment to the caller given the run timestamp is exposed? Leaning caller — the threshold is a product decision and 4 of 119 companies currently exceed 7 days.
- `apps/web` has no JS test runner (`web-guide.md`), so Tasks 5 and 6 verify by typecheck, build, and inspection. Worth adding one now, or does the Go integration test in Task 7 cover the part that matters?
