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
- Materialized views. A 73–115ms live query does not justify a refresh story.
- Facet counts and trend aggregates. The taxonomy view makes them possible; no screen needs them yet.
- pgvector semantic search from the web app. Embedding storage is still deferred per `project.md`.
- Storybook-integrated testing. `@storybook/addon-vitest` needs a Vite-based framework, so it would mean migrating `.storybook/main.ts` from `@storybook/nextjs` to `@storybook/nextjs-vite` plus a Playwright install. Choosing Vitest keeps that additive.
- Component tests of the proof page. It is an `async` Server Component, which no unit runner renders. Next's guidance is E2E; there is no E2E harness here.

## Acceptance criteria

- [ ] A posting counts as open only when it appeared in the most recent successful fetch run for its company. A failed or in-progress run never causes a posting to read as closed.
- [ ] `open_postings_display` exposes the `started_at` of the successful run it derived from, so a caller can distinguish a posting confirmed today from one last confirmed 53 days ago.
- [ ] Postings with no classification appear in `open_postings_display` with null classification columns rather than being dropped.
- [ ] A posting with more than one classification row resolves to its most recent one.
- [ ] Snapshots with a null `fetch_run_id` never cause their posting to be reported as open.
- [ ] `open_posting_taxonomy` returns one row per posting per term across all four term kinds, drawn only from each posting's most recent classification.
- [ ] Querying `open_postings_display` for all open postings completes in under 250ms against the current dataset.
- [ ] The agent's MCP `query` tool can select from all four views.
- [ ] `sqlc generate` runs clean, and every column of `open_postings_display` reachable through an outer join is typed as a `sql.Null*` variant.
- [ ] Applying `000017`'s down migration removes all four views and leaves the preceding schema intact. `cmd/migrate`'s `down` verb is a full teardown, so verify with the down file directly.
- [ ] The first request to a DB-reading route with `DATABASE_URL_RO` unset fails with an error naming that variable, and no fallback DSN is attempted.
- [ ] `pnpm build` in `apps/web` completes with the database stopped.
- [ ] Editing a file during `pnpm dev` never takes the web app above 5 Postgres connections.
- [ ] `/postings` lists open postings with company name, title, location, seniority, and the run timestamp behind "open." Unclassified postings render with their classification fields visibly empty.
- [ ] `pnpm test` passes with the database stopped. `pnpm test:db` asserts that a posting whose company's most recent run failed still reads as open, sourced from the prior successful run.
- [ ] `pnpm typecheck`, `pnpm build`, and `pnpm test` pass in `apps/web`; `go test ./...` passes from `apps/tools`.

## Tasks

### Task 1: Read-model views

Add migration `000017_read_model_views`, creating four views in dependency order: `latest_successful_fetch_runs`, `open_postings`, `open_postings_display`, `open_posting_taxonomy`.

- Confirm the highest existing migration number before naming the file. `000016` is applied but untracked in git, so `ls` and `git ls-files` disagree.
- Filter `fetch_runs` to `status = 'success'` before taking the latest per company, via `DISTINCT ON (company_id)`.
- Derive openness from a snapshot whose `fetch_run_id` matches that latest successful run. Absence from a *failed* run means nothing — conflating the two makes a network blip look like a wave of closed roles.
- Carry the run's `started_at` into `open_postings_display` as `run_started_at`. The oldest latest-success on live data is 52.9 days old, so "open" alone overstates freshness.
- Expose the raw timestamp only. A staleness threshold is a product decision that will change, and a column encoding it forces a migration each time it does.
- Resolve current snapshot and current classification with one `LEFT JOIN LATERAL` each, ordered descending, limited to one. Both are index-served by `idx_posting_snapshots_posting_fetched` and `idx_classifications_posting_classified`.
- Keep the laterals. The `LEFT JOIN (DISTINCT ON …)` alternative sorts and materializes all 61,642 snapshot rows, spilling a 5160kB external merge to disk, where the laterals stay on index scans. Timing does not separate the two shapes — the plan does.
- Give `open_posting_taxonomy` a `term_kind` column over `UNION ALL` branches, values `role`, `specialization`, `skill`, and `dimension`.
- Carry both `slug` and `name` on every branch. The agent reads this view over MCP to answer questions in prose, and slug-only would make it re-join four taxonomy tables to render one term.
- Route the taxonomy view through the current classification. `job_posting_roles`, `job_posting_specializations`, and `job_posting_skills` key off `classification_id`, not `job_posting_id` — their names mislead, and joining them directly leaks superseded classifications.
- Reach dimensions through `canonical_role_dimensions` off the role — one hop further than the other three terms — taking the slug from `role_dimensions`.
- `DISTINCT` the dimension branch; two roles can share a dimension.
- Grant `SELECT` on each view to `market_scout_readonly` in the migration. Default privileges already cover it (`pg_default_acl` objtype `r` includes views), so this is for legibility — a reader of the migration sees who can read the view.
- Ship a `.down.sql` dropping all four views in reverse dependency order. Every existing migration ships one, and `migrate down` is documented recovery.
- Record the measured baseline in a comment in the migration, not in the commit message. The comment is inspectable later; a task agent does not own the commit.

Then regenerate sqlc and pin the types it gets wrong.

- Run `sqlc generate` and commit its output with the migration. sqlc parses `migrations/` as schema input and emits one struct per view, so `models.go` changes.
- Add `overrides` entries in `sqlc.yaml` forcing `open_postings_display.classification_id` to `database/sql.NullInt64` and `.seniority` to `database/sql.NullString`. sqlc infers nullability correctly for a LEFT JOIN against a *table* but not against a derived table, so both come out non-nullable and would panic on the first Go read.
- Use `sql.Null*` in the overrides, not pointers, matching the convention in `project.md` — generated models carry `sql.Null*` and translate at the DB boundary.

Then evaluate the index, as exploration rather than a gate.

- Baseline is 73–115ms across runs on a warm cache, via `EXPLAIN (ANALYZE, TIMING OFF)`. Record the range, not a single number — the spread is wide enough to swallow any small gain.
- The plan sequentially scans all 61,642 rows of `posting_snapshots`, because the openness `EXISTS` flattens into a hash aggregate. `idx_posting_snapshots_fetch_run_id` covers `fetch_run_id` alone and cannot serve it.
- Try an index on `posting_snapshots (fetch_run_id, job_posting_id)`, matching the existing partial predicate `WHERE fetch_run_id IS NOT NULL`.
- Judge it on plan shape — whether the sequential scan is gone — not on milliseconds. Record the outcome either way in the migration comment.
- The index is upside, not a requirement. The unindexed range already clears the 250ms budget.

Do not:
- Add materialized views or a refresh mechanism.
- Hand-edit sqlc's outputs (`models.go`, `*.sql.go`, `db.go`) — see `developer-guide.md` §5.8. `sqlc.yaml` is config, not output, and is meant to be edited.
- Join the taxonomy tables directly to `job_postings`.
- Drop `idx_posting_snapshots_fetch_run_id`; the partial index still serves other lookups.

### Task 2: Postgres client in `apps/web`

Add the `postgres` package and a client module under `apps/web/lib/db/`.

- Install `postgres`, not `postgres.js`. The latter is the project's display name; npm 404s on it.
- Connect with `DATABASE_URL_RO`. The web app is read-only for now, so the process serving the browser cannot write, enforced by Postgres.
- Fail with a message naming `DATABASE_URL_RO` when it is unset. `cmd/mcp` fails fast rather than falling back to the owner DSN; a fallback silently defeats the boundary.
- Cache the client on `globalThis`. Next dev re-evaluates modules on edit, so a bare module-scope client leaks a pool per reload until Postgres refuses connections.
- Set `max: 5`. A pinned ceiling makes the leak above observable instead of gradual.
- Call `connection()` from `next/server` before the first query in every module that queries. Without it `next build` prerenders DB-reading pages and runs their queries at build time, which fails whenever the database is down.
- Symlink the env file: `ln -s ../../.env.local apps/web/.env.local`. Next loads `.env.local` from its own directory, and `apps/tools/.env.local` is already the same symlink. It is local setup, not checked in.
- Document the symlink in `developer-guide.md` §2 beside the existing `ln -s`, and note it in `web-guide.md`'s What-applies table — that table tells web readers to skip §2 today.
- Update the App layer row in `project.md` and the matching row in `index.md`: `postgres`, and Server Components plus Server Actions rather than API routes.

Do not:
- Add `pg`.
- Add a codegen step for row types. `project.md` already declined codegen-for-sync-only when it rejected a token pipeline.
- Use `postgres.js`'s `transform` option. Column names should match the views exactly; a transform is a second place for them to drift.
- Import the client into a Client Component.

### Task 3: Read queries and proof page

Add query functions under `apps/web/lib/db/`, and a page at `/postings` rendering open postings as an unstyled list.

- One exported async function per question the page asks, each with its own `snake_case` row interface matching the view columns.
- Keep SQL in `lib/db/`, never inline in a component, so it has one home.
- Compose any optional filter as a nested `sql` fragment rather than string concatenation.
- Render in a Server Component querying directly. No route handler, no client-side fetch.
- Show unclassified postings with empty classification fields rather than filtering them out. They are the majority of the open set.
- Add a `lib/db/` row to the Layout block in `web-guide.md`.

The page is a correctness check, not a design artifact. Styling comes later in its own brief.

Do not:
- Add route handlers or API routes.
- Filter out unclassified postings.

### Task 4: Vitest setup and coverage

Add Vitest to `apps/web` with two scripts: `test` for DB-free tests, `test:db` for tests needing Postgres.

- Give Vitest its own `vitest.config.ts` with the `node` environment. Next does not build with Vite, so there is no config to extend, and nothing under test renders.
- Split the two suites by filename glob, and keep `test:db` out of `test`. `test` must pass with Docker stopped — gating on an unset env var would not work, because the DSN stays set in `.env.local` while the server is down.
- Load `.env.local` explicitly via `setupFiles`. Vitest does not read it the way Next does, and Vite's own env handling exposes only `VITE_`-prefixed vars on `import.meta.env`, never on `process.env`. Without this the DB suite sees no DSN and skips while appearing to pass.
- Import `describe`, `it`, and `expect` from `vitest` rather than enabling globals. `tsconfig.json` typechecks every `.ts` in the project, so globals would need a `types` entry to keep `pnpm typecheck` passing.
- Assert the openness rule in `test:db`: a posting whose company's most recent run failed still reads as open from the prior successful run.
- Seed fixtures through `DATABASE_URL` and read them back through `DATABASE_URL_RO`, mirroring `action_integration_test.go`'s owner-seeds-then-role-reads shape. The read-only role cannot write, and the two pools are separate connections, so fixtures must be committed rather than held in a rolled-back transaction.
- Delete fixtures in cleanup, in FK order: snapshots, classifications, postings, fetch runs, company. `job_postings` is `ON DELETE RESTRICT` from `posting_snapshots`, so the reverse order fails.
- Skip `test:db` when either DSN is unset, matching the repo's skip-on-unset convention.
- Correct `web-guide.md`: the "no JavaScript test runner" line and the §Commands table, which omits `pnpm test`. Correct `testing-guide.md`'s Go-only framing.
- Leave `pnpm typecheck` as the type gate. Vitest transpiles without typechecking.

| Mirror | Don't mirror |
|---|---|
| `action_integration_test.go` — owner seeds, restricted role reads | The Go `//go:build integration` tag; Vitest has no build tags |

Do not:
- Add `@storybook/addon-vitest`, Playwright, or JSDOM.
- Change the Storybook framework.
- Write tests that render the proof page.
- Let `pnpm test` touch Postgres.

## Sequencing

**Phase 1 (sequential):** Task 1 — every later task reads its views.
**Phase 2 (sequential):** Task 2 — Tasks 3 and 4 both import its client.
**Phase 3 (concurrent):** Task 3, Task 4 — separate code files, disjoint sections of `web-guide.md`; Task 4 tests the views directly, not Task 3's page.

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

`connection()` returns a hanging promise during prerender and resolves immediately on a real request. That is why it is the guard rather than `export const dynamic = 'force-dynamic'`, which throws `DynamicServerError` at build time instead of deferring.

Filter composition in `postgres.js` reduces an array of fragments — `conditions.reduce((a, b) => sql`${a} AND ${b}`)` — with `sql(ids)` expanding an array into a parameterized `IN` list.

## Open questions

- Migration `000016_fix_scrambled_taxonomy_names` is applied to the live database but untracked in git. Until it is committed, a fresh clone applies 15 then 17 and silently skips it. Worth resolving before this plan lands, though it is not this plan's work.
