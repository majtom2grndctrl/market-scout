# Web App Read Model

## Goal

Give `apps/web` a Postgres access layer, and give the whole system one shared definition of "currently open posting." The append-only schema answers every product question by derivation, so the derivation lands in SQL views that the Next.js app and the agent's MCP `query` tool both read. Proves the read path end to end; writes come later.

## Scope

### In scope

- Four read-model views: latest successful fetch run, open postings, a display row, and long-form current taxonomy.
- `apps/web` Postgres client on the existing read-only DSN.
- Typed read queries under `apps/web/lib/db/`, and one unstyled page proving them.
- Vitest as the first JavaScript test runner in `apps/web`.

### Out of scope

- Preferences and saved searches. The filter vocabulary is unknowable until a screen names what it filters on, and product UI is still deferred. Deferring keeps this plan read-only end to end.
- A `market_scout_app` role. Without a write path its grants are identical to `market_scout_readonly`; a second role with the same privileges is ceremony. The app role earns its existence alongside the first write.
- Designed product screens. This plan proves the data layer; UI work follows in its own brief.
- Materialized views. A sub-100ms live query does not justify a refresh story.
- Facet counts and trend aggregates. The taxonomy view makes them possible; no screen needs them yet.
- pgvector semantic search from the web app. Embedding storage is still deferred per `project.md`.
- Storybook-integrated testing. `@storybook/addon-vitest` needs a Vite-based framework, so it would mean migrating `.storybook/main.ts` from `@storybook/nextjs` to `@storybook/nextjs-vite` plus a Playwright install. Choosing Vitest keeps that additive.
- Component tests of the proof page. It is an `async` Server Component, which no unit runner renders. Next's guidance is E2E; there is no E2E harness here.

## Acceptance criteria

Each item is a test, a **review gate** (a human or agent reads code or output; nothing to automate), or both. Gates are labelled.

- [ ] A posting counts as open only when it appeared in the most recent successful fetch run for its company. A failed or in-progress run never causes a posting to read as closed.
- [ ] `open_postings_display` exposes the `started_at` of the successful run it derived from, so a caller can distinguish a posting confirmed today from one last confirmed 53 days ago.
- [ ] Postings with no classification appear in `open_postings_display` with null classification columns rather than being dropped.
- [ ] A posting with more than one classification row resolves to its most recent one.
- [ ] A snapshot whose `fetch_run_id` is null never causes its posting to be reported as open. Task 4 seeds the row; live data contains none.
- [ ] `open_posting_taxonomy` returns one row per posting per term across all four term kinds, drawn only from each posting's most recent classification.
- [ ] `SELECT * FROM open_postings_display` returns every open posting in under 250ms, measured with `EXPLAIN (ANALYZE, TIMING OFF)` on a warm cache.
- [ ] `SELECT … LIMIT 5` against each of the four views returns rows through the agent's MCP `query` tool. The tool caps results at 1000 rows, so an unbounded select reports `truncated: true` — that is the cap, not a failure.
- [ ] `sqlc generate` exits 0 with the new migration in place. **Review gate:** confirm in the `models.go` diff that `classification_id` and `seniority` came out `sql.Null*`. sqlc ignores an `overrides` entry whose column matches nothing, silently and with exit 0, so a typo cannot be caught by the exit code.
- [ ] **Review gate:** applying `000017`'s down file inside a single `BEGIN; … ROLLBACK;` session drops all four views and leaves the rest of the schema untouched. Never apply it outside a rolled-back transaction — `schema_migrations` would still read 17, so `migrate up` reports no change and the views stay gone.
- [ ] Running `pnpm dev` with `DATABASE_URL_RO=` set empty makes the first request to `/postings` fail with an error naming that variable. **Review gate:** no fallback DSN appears in the client module.
- [ ] `pnpm build` in `apps/web` completes with the database stopped. Owned by Task 4.
- [ ] After six edit-and-reload cycles against `/postings`, connections with `application_name = 'market-scout-web'` stay at or below 5.
- [ ] **Review gate:** `/postings` lists open postings with company name, title, location, seniority, and the run timestamp behind "open." Unclassified postings render with their classification fields visibly empty.
- [ ] `pnpm test` passes with the database stopped and runs at least one assertion. `pnpm test:db` asserts that a posting whose company's most recent run failed still reads as open, sourced from the prior successful run. Both owned by Task 4.
- [ ] `pnpm typecheck`, `pnpm build`, and `pnpm test` pass in `apps/web`; `go test ./...` passes from `apps/tools`.

## Tasks

### Task 1: Read-model views

Add migration `000017_read_model_views`, creating four views in dependency order: `latest_successful_fetch_runs`, `open_postings`, `open_postings_display`, `open_posting_taxonomy`.

- Confirm the highest existing migration number before naming the file. `000016` is applied but untracked in git, so `ls` and `git ls-files` disagree.
- Filter `fetch_runs` to `status = 'success'` before taking the latest per company, via `DISTINCT ON (company_id)`.
- Derive openness from a snapshot whose `fetch_run_id` matches that latest successful run. Absence from a *failed* run means nothing — conflating the two makes a network blip look like a wave of closed roles.
- Expose the run's `started_at` as `run_started_at`, and expose it raw. The oldest latest-success on live data is 52.9 days old, so "open" alone overstates freshness; a staleness threshold is a product decision that would force a migration each time it changed.
- Give `open_postings_display` these columns: `job_posting_id`, `company_id`, `company_name`, `run_started_at`, `title`, `location_text`, `workplace_type`, `compensation_min`, `compensation_max`, `compensation_currency`, `classification_id`, `seniority`.
- Give `open_posting_taxonomy` these columns: `job_posting_id`, `term_kind`, `slug`, `name`. `term_kind` values are `role`, `specialization`, `skill`, and `dimension`.
- Resolve current snapshot and current classification with one `LEFT JOIN LATERAL` each, ordered descending, limited to one. Both are index-served by `idx_posting_snapshots_posting_fetched` and `idx_classifications_posting_classified`.
- Keep the laterals. The `LEFT JOIN (DISTINCT ON …)` alternative sorts and materializes all 61,642 snapshot rows, spilling a 5160kB external merge to disk, where the laterals stay on index scans. Timing does not separate the two shapes — the plan does.
- Carry both `slug` and `name` on every taxonomy branch. The agent reads this view over MCP to answer questions in prose, and slug-only would make it re-join four taxonomy tables to render one term.
- Route the taxonomy view through the current classification. `job_posting_roles`, `job_posting_specializations`, and `job_posting_skills` key off `classification_id`, not `job_posting_id` — their names mislead, and joining them directly leaks superseded classifications.
- Reach dimensions through `canonical_role_dimensions` off the role — one hop further than the other three terms — taking slug and name from `role_dimensions`.
- `DISTINCT` the dimension branch; two roles can share a dimension.
- Comment in the migration that `market_scout_readonly` reads these views through two existing mechanisms: default privileges on the live cluster, and `GRANT SELECT ON ALL TABLES IN SCHEMA public` in `setup/readonly_role.sql` on a fresh one.
- Ship a `.down.sql` dropping all four views in reverse dependency order. Every existing migration ships one, and `migrate down` is documented recovery.

Then regenerate sqlc and pin the types it gets wrong.

- Run `sqlc generate` and commit its output with the migration. sqlc parses `migrations/` as schema input and emits one struct per view, so `models.go` changes.
- Add `overrides` entries in `sqlc.yaml` forcing `open_postings_display.classification_id` to `database/sql.NullInt64` and `.seniority` to `database/sql.NullString`. sqlc infers nullability correctly for a LEFT JOIN against a *table* but not against a derived table, so both come out non-nullable and would panic on the first Go read.
- Use `sql.Null*` in the overrides, not pointers, matching the convention in `project.md` — generated models carry `sql.Null*` and translate at the DB boundary.
- Check the `models.go` diff by eye. A mistyped column name in an override is ignored silently, with exit 0.

Then evaluate the index, as exploration rather than a gate.

- Measure your own baseline with `EXPLAIN (ANALYZE, TIMING OFF)` on a warm cache and record the range in a migration comment. Ours landed between 69 and 115ms across runs — a spread wide enough to swallow any small gain, so treat a single number as meaningless.
- The plan sequentially scans all 61,642 rows of `posting_snapshots`: the openness test flattens into `HashAggregate Group Key: ps.job_posting_id, ps.fetch_run_id` feeding a hash join on `ps.fetch_run_id = fetch_runs.id`. `idx_posting_snapshots_fetch_run_id` covers `fetch_run_id` alone and cannot serve it.
- Try an index on `posting_snapshots (fetch_run_id, job_posting_id)`. The existing partial predicate `WHERE fetch_run_id IS NOT NULL` currently excludes nothing — zero rows are null — so copying it is optional and costs nothing either way.
- Judge the index on plan shape — whether the sequential scan is gone — not on milliseconds. Record the outcome either way.
- The index is upside, not a requirement. The unindexed query already clears the 250ms budget with room to spare.

Do not:
- Add a `GRANT` to any role in the migration. No migration in the tree does; roles are cluster-level, and `developer-guide.md` §2 applies migrations *before* provisioning them, so a grant would fail on a fresh clone and leave the schema dirty.
- Add materialized views or a refresh mechanism.
- Hand-edit sqlc's outputs (`models.go`, `*.sql.go`, `db.go`) — see `developer-guide.md` §5.8. `sqlc.yaml` is config, not output, and is meant to be edited.
- Join the taxonomy tables directly to `job_postings`.
- Drop `idx_posting_snapshots_fetch_run_id`.

### Task 2: Postgres client in `apps/web`

Add the `postgres` package and `apps/web/lib/db/client.ts`.

- Install `postgres`, not `postgres.js`. The latter is the project's display name; npm 404s on it.
- Connect with `DATABASE_URL_RO`. The web app is read-only for now, so the process serving the browser cannot write, enforced by Postgres.
- Fail with a message naming `DATABASE_URL_RO` when it is unset or empty. `cmd/mcp` fails fast rather than falling back to the owner DSN; a fallback silently defeats the boundary.
- Export one async accessor — `getSql()` — that awaits `connection()` from `next/server` and then returns the client. Callers cannot reach the client without it, which is the point: `next build` otherwise prerenders DB-reading pages and runs their queries at build time, failing whenever the database is down.
- Cache the client on `globalThis`. Next dev re-evaluates modules on edit, so a bare module-scope client leaks a pool per reload until Postgres refuses connections.
- Set `max: 5`, and set `connection: { application_name: 'market-scout-web' }`. Three MCP connections already run as the same role with an empty `application_name`, so without the tag the web app's connections cannot be counted separately.
- Symlink the env file: `ln -s ../../.env.local apps/web/.env.local`. Next loads `.env.local` from its own directory, and `apps/tools/.env.local` is already the same symlink. It is local setup, not checked in.
- Document the symlink in `developer-guide.md` §2 beside the existing `ln -s`, and note it in `web-guide.md`'s What-applies table — that table tells web readers to skip §2 today.

Do not:
- Export the raw client alongside `getSql()`. A second door lets a caller skip `connection()`.
- Add `pg`.
- Add a codegen step for row types. `project.md` already declined codegen-for-sync-only when it rejected a token pipeline.
- Use the `transform` option. Column names should match the views exactly; a transform is a second place for them to drift.
- Import the client into a Client Component.

### Task 3: Read queries and proof page

Add query functions under `apps/web/lib/db/`, and a page at `/postings` rendering open postings as an unstyled list.

- Read the view and column names from `apps/tools/internal/db/migrations/000017_read_model_views.up.sql`. That migration is the contract; nothing under `apps/web` describes the views.
- One exported async function per question the page asks, each with its own `snake_case` row interface matching the view columns exactly.
- Reach the client through `getSql()` from `lib/db/client.ts`. It awaits `connection()`, without which `pnpm build` runs these queries at build time.
- Keep SQL in `lib/db/`, never inline in a component, so it has one home.
- Render in a Server Component querying directly. No route handler, no client-side fetch.
- Show unclassified postings with empty classification fields rather than filtering them out. They are 3,258 of the 4,466 open postings.
- Render all open postings unbounded. A long list is fine for a correctness check.
- Add a `lib/db/` row to the Layout block in `web-guide.md`. Task 4 edits the same file's §Commands table and test-runner line concurrently — edit only the Layout block.

The page is a correctness check, not a design artifact. Styling comes later in its own brief.

Do not:
- Stop or restart Postgres. Task 4 owns the database-stopped criteria; stopping the container fails this task's own page check and Task 4 may be running concurrently.
- Add route handlers or API routes.
- Filter out unclassified postings.
- Add a filter, sort, or search control. No acceptance criterion needs one.

### Task 4: Vitest setup and coverage

Add Vitest to `apps/web` with two scripts: `test` for DB-free tests, `test:db` for tests needing Postgres. This task owns every database-stopped criterion.

- Give Vitest its own `vitest.config.ts` with the `node` environment. Next does not build with Vite, so there is no config to extend, and nothing under test renders.
- Split the two suites by filename glob and keep `test:db` out of `test`. Gating on an unset env var would not work: the DSN stays set in `.env.local` while the server is down.
- Write at least one DB-free test, covering `getSql()` failing with a message naming `DATABASE_URL_RO` when that variable is empty. Vitest exits 1 on "no test files found", so `pnpm test` is otherwise an empty gate — and this assertion is what makes that criterion runnable rather than a review gate.
- Load `.env.local` in `setupFiles` via `loadEnvConfig` from `@next/env`, already present transitively through `next`. Vitest does not read it the way Next does, and Vite's own handling exposes only `VITE_`-prefixed vars on `import.meta.env`, never `process.env`. Without this the DB suite sees no DSN and skips while appearing to pass.
- Import `describe`, `it`, and `expect` from `vitest` rather than enabling globals. `tsconfig.json` typechecks every `.ts` in the project, so globals would need a `types` entry to keep `pnpm typecheck` passing.
- Query the views directly in `test:db`, reading their names from `apps/tools/internal/db/migrations/000017_read_model_views.up.sql`. Do not import Task 3's query functions; it may be running concurrently.
- Seed this fixture shape, which is the only arrangement that exercises the openness rule: one company, a `success` run with the earlier `started_at`, a `failed` run with the later one, and a snapshot carrying the successful run's `fetch_run_id`. The posting must still read as open.
- Seed a second snapshot with a null `fetch_run_id` and assert its posting is not reported as open. Live data has zero such rows, so this criterion is unexercisable without the fixture.
- Read `000001_initial_schema.up.sql` for the constraints the seed must satisfy: `companies.ats` NOT NULL and unique with `board_token`, `job_postings.source_type` CHECK, `posting_snapshots.raw_data` NOT NULL, `classifications.seniority` CHECK.
- Seed through `DATABASE_URL` and read back through `DATABASE_URL_RO`, mirroring `action_integration_test.go`'s owner-seeds-then-role-reads shape. The read-only role cannot write, and the two pools are separate connections, so fixtures must be committed rather than held in a rolled-back transaction.
- Delete fixtures in cleanup, in FK order: snapshots, classifications, postings, fetch runs, company. `job_postings` is `ON DELETE RESTRICT` from `posting_snapshots`, so the reverse order fails.
- Skip `test:db` when either DSN is unset, matching the repo's skip-on-unset convention.
- Stop and start the database with `docker compose stop db` and `docker compose start db` when verifying the database-stopped criteria.
- Correct `web-guide.md`: the "no JavaScript test runner" line and the §Commands table, which omits `pnpm test` and `pnpm test:db`. Correct `testing-guide.md`'s Go-only framing. Task 3 edits the same file's Layout block concurrently — leave it alone.
- Leave `pnpm typecheck` as the type gate. Vitest transpiles without typechecking.

| Mirror | Don't mirror |
|---|---|
| `action_integration_test.go` — owner seeds, restricted role reads | The Go `//go:build integration` tag; Vitest has no build tags |

Do not:
- Run `docker compose down -v`. The named volume `market-scout-pgdata` holds 61,642 snapshots and 52 days of append-only history that cannot be refetched.
- Add `@storybook/addon-vitest`, Playwright, or JSDOM.
- Change the Storybook framework.
- Write tests that render the proof page.
- Let `pnpm test` touch Postgres.

## Sequencing

**Phase 1 (sequential):** Task 1 — every later task reads its views.
**Phase 2 (sequential):** Task 2 — Tasks 3 and 4 both import its client.
**Phase 3 (concurrent):** Task 3, Task 4 — separate code files, disjoint sections of `web-guide.md`. Task 4 owns Postgres up/down state; Task 3 is told not to touch it.

## Boundary inventory

SQL is the source of truth. TypeScript row interfaces mirror the view columns verbatim — no transform layer. Aliases are applied in the view definition so both consumers see one name.

| Name | Source column | View column | TS field |
|---|---|---|---|
| Posting id | `job_postings.id` | `job_posting_id` | `job_posting_id` |
| Company id | `companies.id` | `company_id` | `company_id` |
| Company name | `companies.name` | `company_name` | `company_name` |
| Run timestamp | `fetch_runs.started_at` | `run_started_at` | `run_started_at` |
| Title | `posting_snapshots.title` | `title` | `title` |
| Location | `posting_snapshots.location_text` | `location_text` | `location_text` |
| Classification id | `classifications.id` | `classification_id` | `classification_id` |
| Seniority | `classifications.seniority` | `seniority` | `seniority` |
| Term kind | — (literal) | `term_kind` | `term_kind` |
| Term slug | `canonical_roles.slug`, `specializations.slug`, `skills.slug`, `role_dimensions.slug` | `slug` | `slug` |
| Term name | the same four tables' `name` | `name` | `name` |

## Rough sketch

Views build on each other rather than repeating the openness derivation: `latest_successful_fetch_runs` → `open_postings` → `open_postings_display` → `open_posting_taxonomy`. Anything needing "open" selects downstream of the second, so the failed-run rule has exactly one definition.

`connection()` is Next's documented way to mark work as request-time only, and it resolves immediately on a real request. Routing every query through `getSql()` means a page cannot accidentally omit it and get its queries executed during `next build`.

Filter composition in `postgres` reduces an array of fragments — `conditions.reduce((a, b) => sql`${a} AND ${b}`)` — with `sql(ids)` expanding an array into a parameterized `IN` list. Not needed by this plan; recorded for the first screen that filters.

## Open questions

None. Migration `000016_fix_scrambled_taxonomy_names` is committed as of this plan's landing.
