# Test Data Isolation

## Goal

Give the DB-backed Vitest suites their own database, and purge the fixture rows that leaked into the shared development database, `market_scout`, from the suites' current practice of writing there.

Today `pnpm test:db`, run from `apps/web/`, writes fixtures into the same database the fetcher writes to. Four of the five suites clean up in a `finally` block, best-effort; `status.db.test.ts` rolls its fixtures back inside a transaction and never leaked. One suite pins fixture time in 2099; the rest write near-present or `now()`-dated rows, which land inside live `now() - interval` windows and are worse — no future-timestamp check catches them. Cleanup has failed at least three times. The residue is 36 fixture companies plus their postings, snapshots, classifications, and taxonomy rows — and every fixture company became a permanent fetcher target that 404s against Greenhouse on each run. The fetch list is filtered only on `ats IS NOT NULL`, and every fixture insert sets `ats = 'greenhouse'`.

The durable fix is a boundary, not better hygiene: fixtures written to a separate database cannot reach real rows, however teardown fails.

## Scope

### In scope
- One-time purge of the leaked fixture rows from the development database.
- A `market_scout_test` database, provisioned from the same numbered migrations and `readonly_role.sql` the development database uses.
- `DATABASE_URL_TEST` / `DATABASE_URL_TEST_RO` env vars, resolved through a shared helper and consumed by the five suites, in place of the development DSNs.
- Setup documentation in `developer-guide.md` §2, the test-database contract in `web-testing-guide.md` §Database Tests, and the integration-test note in `testing-guide.md` §1.

### Out of scope
- **No `companies` schema change.** A fetch-list filter column (`active`, `fixture`) was considered and rejected: it treats the symptom, leaves fixture rows inflating every status metric, and adds a development schema column to work around a test problem.
- **No teardown rewrite.** Both teardown shapes — the four `finally` blocks and `status.db.test.ts`'s rollback — stay as-is. Once it runs against a disposable database, a failed cleanup costs nothing. Hardening it is a follow-up if the test database proves noisy in practice.
- **The 5 orphaned `in_progress` fetch_runs from 2026-05-28** (Anthropic, Avante, Boulder Care, Brinc, Carbon Direct). Real companies, a crashed run, unrelated to fixture leakage. Separate ticket.
- **The Go `//go:build integration` tests.** They also write to `DATABASE_URL`, but left no residue. Pointing them at the test database is a follow-up that inherits this plan's infrastructure.
- No CI wiring. The test database is a local-development contract for now.

## Acceptance criteria

Automated by `pnpm test:db` (both test DSNs set): AC 7. Everything else is an operator or review gate, not a default test run: AC 1, 2, 3, 4, 5, 6, and 10 are operator SQL run directly against the databases; AC 8 is a deliberate manual run of `pnpm test:db` with the test DSNs unset; AC 9 is a grep check; AC 11, 12, and 13 are documentation reviewed against the cited files.

- [ ] The development database contains no company whose `board_token` matches `%vitest%`, and no `canonical_roles`, `role_dimensions`, `skills`, or `specializations` row whose `slug` matches `%vitest%`.
- [ ] After the purge, no `fetch_runs.started_at` or `completed_at`, `posting_snapshots.fetched_at`, or `classifications.classified_at` value is a future timestamp. This verifies the assumption that every future-dated row is reachable from a fixture company; if any survive, stop and report rather than widening the predicate.
- [ ] After the purge, each of `companies` and `job_postings` holds a post-purge count equal to its pre-purge count (captured in Task 1) minus the script's reported delete count for that table. `open_postings` is a view, not a deleted table: its count is measured before and after the purge in the same session, and the drop equals the number of fixture postings that were open. (Drafting-time reference only, measured 2026-09-10: `companies` 155 → 119, `job_postings` 10,039 → 9,919, `open_postings` 4,808 → 4,734.) The counts are evaluated in the same session as the purge, before any fetcher run.
- [ ] The purge script is idempotent: running it a second time deletes zero rows and exits successfully.
- [ ] The purge script deletes only rows reachable from a fixture company or carrying a fixture slug. Every table outside the delete order holds an identical pre- and post-purge count, except `job_posting_roles`, `job_posting_specializations`, and `job_posting_skills` — these cascade from the `classifications` delete, and each must reconcile to the number of link rows the deleted classifications owned. Every table inside the delete order reconciles to the script's reported delete count.
- [ ] `market_scout_test` exists and carries the same migration version as the development database (per Task 2's migration-parity check). An ordered diff of `information_schema.role_table_grants` for `market_scout_readonly` across both databases is empty, and `has_function_privilege('market_scout_readonly', 'public.open_postings_as_of(timestamptz)', 'EXECUTE')` and `has_function_privilege('market_scout_readonly', 'public.similarity(text,text)', 'EXECUTE')` both return true in both databases.
- [ ] `pnpm test:db`, run from `apps/web/`, passes against `market_scout_test` with `DATABASE_URL_TEST` and `DATABASE_URL_TEST_RO` set.
- [ ] Each of the five `.db.test.ts` suites skips — rather than falling back to `DATABASE_URL` — when its test DSN is unset. With both `_TEST` vars unset, `pnpm test:db` run from `apps/web/` reports 7 skipped and 2 passed; the two passing are the pure-validation tests at `measure-engine.db.test.ts:628` and `:647`, which carry no DSN guard. This is a deliberate manual gate, not part of a default run.
- [ ] All five suites resolve their DSNs through `apps/web/lib/db/test-dsn.ts`. This is a grep gate: `grep -l process.env apps/web/lib/db/*.db.test.ts` returns nothing. `apps/web/lib/db/client.ts` legitimately reads `process.env.DATABASE_URL_RO` and is outside this predicate's scope.
- [ ] After the purge, `ListCompaniesWithATS` — the query the fetcher builds its fetch list from (`apps/tools/internal/db/queries/fetcher.sql:1`) — returns no row whose `board_token` matches `%vitest%`. Checked by running that query, not by running the fetcher: adapter HTTP behavior is already covered by `TestAdaptersIndependentlyOverHTTP` (`apps/tools/cmd/fetcher/main_test.go:323`) over `httptest`, and the adapters' parsing by the recorded fixtures in `apps/tools/internal/ats/*_test.go`. A live run would add third-party traffic without adding coverage.
- [ ] `developer-guide.md` §2 documents test-database provisioning alongside the existing role-provisioning steps.
- [ ] `web-testing-guide.md` §Database Tests names `DATABASE_URL_TEST` / `DATABASE_URL_TEST_RO` in place of `DATABASE_URL` / `DATABASE_URL_RO`, keeps the owner/application role distinction, states the no-fallback rule and its reason, and no longer claims tests share a development database or implies the old DSN pair at `:38` and `:61`.
- [ ] `testing-guide.md` §1 notes the Go `//go:build integration` tests still use `DATABASE_URL`, and that pointing them at the test database is a follow-up.

## Tasks

Order: Task 2 → Task 3 → Task 1 → Task 4. The purge runs last; running it before the suites are re-pointed lets the next `pnpm test:db` re-create the rows it just deleted.

### Task 1: Purge script

Add `apps/tools/internal/db/setup/purge_test_fixtures.sql`. Operational SQL, hand-editable — same class as `readonly_role.sql` and `action_role.sql` in that directory.

- Make it a single transaction. A partial purge is what created this mess; all-or-nothing is the property that matters.
- Scope every delete to fixture companies: `board_token LIKE '%vitest%'`.
- Scope every delete to fixture taxonomy rows: `slug LIKE '%vitest%'`.
- Delete in this order:

  1. `classifications`
  2. `posting_snapshots`
  3. `job_postings`
  4. `fetch_runs`
  5. `companies`
  6. `canonical_role_dimensions`
  7. `canonical_roles`
  8. `specializations`
  9. `skills`
  10. `role_dimensions`

- Steps 7-9 are each blocked by a link-table foreign key until step 1's `classifications` delete cascades those rows away — `job_posting_roles.role_id`, `job_posting_specializations.specialization_id`, and `job_posting_skills.skill_id` are all `ON DELETE RESTRICT`. An FK abort here means a fixture taxonomy row is referenced by real classification history — stop and report rather than widening the predicate.
- `canonical_role_dimensions` carries neither a `board_token` nor a `slug` column. Delete rows whose `canonical_role_id` is a fixture role or whose `dimension_id` is a fixture dimension — both sides, because `dimension_id` is `ON DELETE RESTRICT` and a fixture dimension linked to a real role would block the `role_dimensions` delete.
- `posting_snapshots` and `classifications` carry no `board_token`; each reaches a fixture company only via `job_posting_id → job_postings.company_id`. Scope `posting_snapshots` on `job_posting_id IN (fixture postings) OR fetch_run_id IN (fixture runs)` — `posting_snapshots.fetch_run_id` has no `ON DELETE` clause (`NO ACTION`), so a snapshot on a real posting pointing at a fixture run would otherwise abort the `fetch_runs` delete.
- Report per-table deleted counts so the operator can run the AC 5 comparison. Each table is its own statement — `WITH d AS (DELETE ... RETURNING 1) SELECT '<table>', count(*) FROM d` — sequenced inside one explicit `BEGIN`/`COMMIT`. Chaining all ten as data-modifying CTEs in a single statement puts them on one snapshot, where step 1's cascade is invisible to steps 7-9 and they abort on the RESTRICT links.
- Add a companion `apps/tools/internal/db/setup/purge_test_fixtures_counts.sql` that reports counts for every table in the delete order, the three cascading link tables (`job_posting_roles`, `job_posting_specializations`, `job_posting_skills`), and `open_postings`. The operator runs it immediately before and immediately after the purge, in the same session.
- Every delete is predicate-driven, so a second run matches zero rows. Idempotency is what makes a retry after a partial failure safe.
- Run both from `apps/tools/`: `psql "$DATABASE_URL" -f internal/db/setup/purge_test_fixtures_counts.sql` before and after, and `psql "$DATABASE_URL" -f internal/db/setup/purge_test_fixtures.sql` between them.
- The implementing agent writes and reviews the script; a human runs it — a destructive write against the live database is an operator action, per `developer-guide.md` §2's closing line, "Task agents run the free tier only."

This order was validated against the live database inside a rolled-back transaction on 2026-09-10. The dry run produced the per-table delete counts; the post-purge totals in the acceptance criteria follow from subtracting them from the pre-purge counts.

Do not:
- Write this as a numbered migration. Migrations version schema across environments; this is a one-time data repair on one environment, and a fresh database has nothing to purge.
- Delete by timestamp.

### Task 2: Provision the test database

- Create a second database in the running `market-scout-db` cluster: `docker exec market-scout-db psql -U market_scout -d market_scout -c 'CREATE DATABASE market_scout_test'`. `market_scout` is the migration owner, so the new database is owned by the role `readonly_role.sql` expects.
- Source the env file first, from the repo root: `set -a; source .env.local; set +a`. `godotenv.Load` skips keys already in the environment, and an unsourced `DATABASE_URL="$DATABASE_URL_TEST"` prefix expands to empty — which counts as present and suppresses the `.env.local` fallback.
- Run the migrate steps from `apps/tools/`. `godotenv.Load(".env.local")` in `cmd/migrate/main.go` is CWD-relative.
- Apply migrations with `DATABASE_URL="$DATABASE_URL_TEST" go run ./cmd/migrate up`.
- Run `readonly_role.sql` against the test database with the same owner role used for migrations, so future table grants attach to that owner. `market_scout_readonly` is cluster-level and already exists; the script's grants are per-database and must be re-run here.
- Add `DATABASE_URL_TEST` to root `.env.local`, beside the existing DSNs — the owner DSN: `postgres://market_scout:<password>@localhost:5432/market_scout_test?sslmode=disable`.
- Add `DATABASE_URL_TEST_RO` to root `.env.local` — the application DSN: `postgres://market_scout_readonly:<password>@localhost:5432/market_scout_test?sslmode=disable`. Reuse the existing `market_scout_readonly` password — the role is cluster-level and already has one.
- All three env files are gitignored; the symlinks already carry them to both apps.
- Verify parity from `apps/tools/`: `DATABASE_URL="$DATABASE_URL_TEST" go run ./cmd/migrate version` and `go run ./cmd/migrate version` print the same version with the dirty flag clear.

`market_seeds` needs no seeding step — all 30 rows ship in `000020_market_dimension.up.sql`, so a migrated test database is fully equivalent for the market-derivation suite.

Do not:
- Start a second Postgres container. The cluster is already running, and a second container doubles the port and volume surface for no isolation gain.
- Run `action_role.sql` against the test database. No `.db.test.ts` suite calls an `mcp.` function.

### Task 3: Point the DB suites at the test database

Task 2 provisions `market_scout_test` and adds `DATABASE_URL_TEST` / `DATABASE_URL_TEST_RO` to root `.env.local`. `apps/web/.env.local` symlinks to it, and `vitest.setup.ts` loads it through `@next/env`, so the suites see both vars with no further wiring.

Five files, seven DSN guard sites across thirteen `process.env` reads — one site RO-only:
- `apps/web/lib/db/market-derivation.db.test.ts` — reads at `:8-9`
- `apps/web/lib/db/measure-engine.db.test.ts` — reads at `:15-16`
- `apps/web/lib/db/postings.db.test.ts` — reads at `:8-9`
- `apps/web/lib/db/read-model-views.db.test.ts` — reads at `:8-9` and again at `:289-290`
- `apps/web/lib/db/status.db.test.ts` — reads at `:28-29` and at `:156`, where it reads `DATABASE_URL_RO` alone with no owner DSN

- Replace each `process.env` DSN read with a `testDsns()` call, at every site listed above.
- Keep the existing `context.skip()` behavior when a DSN is unset. `context` is a per-test argument and cannot cross the helper boundary, so each suite keeps its own guard.
- Add `apps/web/lib/db/test-dsn.ts` exporting `export function testDsns(): { ownerDsn?: string; readOnlyDsn?: string }`, reading `DATABASE_URL_TEST` and `DATABASE_URL_TEST_RO`.
- Verify from `apps/web/`: `pnpm test:db` runs all five suites against `market_scout_test` with zero skips.

Do not:
- Fall back to `DATABASE_URL` when the test DSN is unset. The fallback is the bug this plan removes.
- Change fixture content, the fixture timestamps, or the teardown blocks. Out of scope, and harmless once the target database is disposable.
- Touch `apps/web/lib/db/client.ts`. It reads `DATABASE_URL_RO` because that is the running app's DSN, not a test DSN.
- Rewrite the dynamic-import comments at `status.db.test.ts:35-36` and `postings.db.test.ts:15`. The `DATABASE_URL_RO` they name is the app DSN read by `client.ts`, not a test DSN. They are stale for an unrelated reason and are tracked separately.

### Task 4: Document

- `developer-guide.md` §2: add test-database provisioning after the "Primary debugging surfaces" list at the end of the Database Access material, immediately before `### Schema Migrations`, in the same shell-block style.
- `developer-guide.md` §2 Read-only MCP role: correct "Provision the role once per Postgres cluster, after migrations." The role is cluster-level, but its grants are per-database and must be re-run for each new database — Task 2 re-runs them for `market_scout_test`.
- `developer-guide.md` §2: note that `vitest.setup.ts` forces `NODE_ENV=development` before calling `loadEnvConfig(process.cwd())`, so a `.env.test` file is never loaded. The new `DATABASE_URL_TEST` / `DATABASE_URL_TEST_RO` vars must live in `.env.local`.
- `web-testing-guide.md` §Database Tests: replace `DATABASE_URL` / `DATABASE_URL_RO` with `DATABASE_URL_TEST` / `DATABASE_URL_TEST_RO` throughout that section, keep the owner/application role distinction, and add the no-fallback rule with its reason — fixture rows land inside live `now() - interval` windows and poison `max(started_at)` reads if they survive teardown. That rule supersedes `:46` ("Without either DSN, individual DB tests call `context.skip()`") directly. Also fix two lines the scoped edit otherwise leaves false: `:38` ("Tests share a development database, so assertions must not depend on absolute totals"), stale once the suites run against `market_scout_test` and stated as the marker convention's own rationale; and `:61` in §Running Tests ("when both DSNs are available"), which names the old DSN pair by implication.
- `testing-guide.md` §1: one sentence noting the Go `//go:build integration` tests still use `DATABASE_URL`, and that pointing them at the test database is a follow-up.

All three are durable architectural constraints, so they land before this plan moves out of `drafts/`.
