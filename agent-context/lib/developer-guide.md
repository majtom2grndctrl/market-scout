# market-scout Development Guide

> **Read this when:** setting up the repo, adding a feature, or onboarding. Covers dev setup, conventions, and coding standards.
> **Key invariant:** snapshots are append-only — every fetch writes new timestamped rows; never upsert. Validate at every external API and DB boundary.
> **Related:** [Project Overview](./project.md) · [Web App Guide](./web-guide.md) · [Testing Guide](./testing-guide.md) · [Style Guide](./style-guide.md)

---

## Agent TL;DR

- Optimize for **readability over cleverness**; prefer small, explicit changes.
- Respect **package boundaries**: `apps/tools/cmd/fetcher` orchestrates and owns the DB lifecycle. `apps/tools/internal/ats` adapts ATS APIs (HTTP only — no DB access). `apps/tools/internal/db` holds sqlc output. Adapters and the fetcher exchange `domain.Posting` from `apps/tools/internal/domain`.
- `posting_snapshots` is **append-only**. Every fetch writes new timestamped rows. Never upsert. This is load-bearing for trend analysis.
- Define **Go interfaces at package boundaries** (`apps/tools/internal/ats/` exposes the ATS adapter interface; each ATS is a separate file).
- Validate ATS API responses at the adapter boundary; return typed errors. No `panic` in library code.
- Database access goes through `sqlc`-generated functions; no ad-hoc SQL strings in business logic. Raw SQL lives in `apps/tools/internal/db/queries/`.
- This guide is **Go-first**. For `apps/web/`, read [`web-guide.md`](./web-guide.md) — it names which sections here still apply.
- **Deliver the impact defined in docs and tickets.** Specs define what and why; use judgment on how. When the plan doesn't survive contact with the code, adapt — but surface deviations and update the docs. See §1.

---

## 1) Implementation Quality

Docs and tickets define the **impact** to deliver — the *what* and *why*. The recommended approach is the starting plan, not a mandate. Deliver the intended impact cleanly; use judgment when the plan doesn't survive contact with the code.

### 1.1 Deliver the impact

Read the spec and ticket before writing code. They tell you what outcome matters and what constraints apply. Start with the recommended approach, but treat it as a plan, not a contract.

**Do:**
- Handle the error states and edge cases that fall within the defined scope (HTTP failures, malformed JSON, rate limits, partial fetches).
- Write tests now, while you have full context on what the code should do.
- When the approach works as specced, deliver it without embellishment.

**Don't:**
- Add capabilities the ticket didn't ask for ("while I'm here, I'll also add Lever support...").
- Skip work that's clearly within scope and justify it with "TODO" or version labels.
- Invent abstractions, helpers, or config options for hypothetical future ATS integrations.

### 1.2 When the plan doesn't work

Sometimes the spec's approach hits a wall — an ATS API doesn't expose the field the spec assumes, schema migration conflicts, an interface doesn't compose. The response depends on scale:

**Small adjustment** (same impact, minor approach change):
1. Ask the user: explain what you found and propose the alternative.
2. On confirmation, implement the alternative.
3. Update the spec/docs to reflect the actual approach taken.

**Significant change** (different contracts, shifted scope, new trade-offs):
1. Stop and surface the issue to the user with enough detail to decide.
2. Propose options with trade-offs.
3. On resolution, update docs/specs *before* resuming implementation.

The key principle: **specs are working documents — update them during implementation, never silently deviate.** After the feature ships, durable knowledge lives in architecture docs and code comments; the spec moves to `done/` as a historical record. See §1.5.

### 1.3 Clean, not clever

Over-engineering is as costly as under-delivering. Both create surface area that has to be understood, tested, and maintained.

| | Under-delivering | Over-engineering |
|---|---|---|
| **What** | Shipping scope with missing validation, error handling, or tests | Adding scope, abstractions, or infrastructure the ticket didn't request |
| **Cost** | Broken states, follow-up tickets, lost context | Unnecessary complexity, harder reviews, maintenance burden |
| **Example** | "Greenhouse fetch works but doesn't handle 429" | "Added a generic ATS plugin framework with config-driven field mapping" |

### 1.4 When to file a follow-up instead

Adjacent work discovered during implementation gets a follow-up ticket, not a scope expansion.

- **Robustness gaps** outside the ticket's scope → file a ticket with `[Robustness]` prefix.
- **Optimizations** with no current performance problem → file a ticket, don't add caching or batching preemptively.
- **Abstractions** without multiple concrete consumers → three similar lines beat a premature interface. Wait until you have at least two ATS adapters before refactoring shared logic.

When you defer, create a ticket with enough context for the next agent. Never leave a bare `// TODO: fix later`.

### 1.5 Documentation lifecycle

Normative home: [Style Guide](./style-guide.md) §Documentation Lifecycle. Summary:

- **Specs live in `agent-context/plans/`**, moving `drafts/` → `ready/` → `in-progress/` → `done/`. Acceptance criteria are part of the spec — vague "done" conditions push ambiguity to the implementer.
- **After the feature ships:** the plan moves to `done/` and stays as a historical record. Don't maintain it; the implementation is the source of truth.
- **Architecture docs** (`agent-context/lib/`) capture what's durable — design principles, package boundaries, contracts, snapshot model, schema invariants. Content an agent can't derive by opening the relevant file.
- **Code comments** capture implementation-level "why" decisions. Rationale a reader can't derive from the code alone. See §7.

**The light lane.** The plan pipeline (draft → review → promote → orchestrate) is institutional memory for stateless agents, not quality control. A change touching no schema, no agent-facing contract, and no data write path skips it: implement directly, then `/review-panel`. When in doubt, the cost of a wrong guess decides — code is rewritable; data and contracts are not.

**What doesn't belong in `agent-context/lib/`:**

- Specs for specific features or epics (use `agent-context/plans/`)
- Implementation plans or task breakdowns (use `agent-context/plans/`)
- Content that names specific functions, types, or file paths as load-bearing detail (see [Style Guide](./style-guide.md))

---

## 2) Development Setup

### Working directory

Go commands run from `apps/tools/`, not the repo root. The module root, `godotenv.Load(".env.local")` in every binary, and two CWD-relative path literals (the onboard seed path and the batch-enrich failures-log path) all assume this. `docker compose` runs from repo root.

### Initial Setup

After cloning, start the database, link env, and apply migrations:

```bash
docker compose up -d                        # Postgres + pgvector (repo root)
ln -s ../../.env.local apps/tools/.env.local # Go tools; root .env.local is canonical
ln -s ../../.env.local apps/web/.env.local   # Next app reads its own directory
cd apps/tools
go run ./cmd/migrate up                     # Apply schema migrations
```

`.env.local` is canonical at repo root (docker-compose reads it). `apps/tools/.env.local` and `apps/web/.env.local` point to it, so both apps read the same credentials. All three are gitignored; the symlinks are local-setup steps, not checked in.

Generate sqlc code if any `.sql` files in `apps/tools/internal/db/queries/` have changed:

```bash
sqlc generate
```

The generated `.go` files in `apps/tools/internal/db/` are checked in — regenerate after changing schema or queries, then commit the diff.

### Running the Fetcher

```bash
# Run the fetcher once (one-shot fetch across configured companies)
go run ./cmd/fetcher

# Build the binary
go build -o bin/fetcher ./cmd/fetcher

# Run with a specific ATS / company filter (flags TBD per ticket)
go run ./cmd/fetcher --ats greenhouse --company stripe
```

The fetcher is intended to run on a cron schedule. For local development, invoke it directly; the cron wrapper is a thin shell around the same binary.

### Database Access

The connection string lives in `.env.local` as `DATABASE_URL`. The fetcher and migration runner load it via `godotenv` at startup; no separate shell session is needed.

### Read-only MCP role

The MCP server uses `DATABASE_URL_RO`. It must be a read-only DSN and never falls back to `DATABASE_URL`.

Provision after migrations, with the same owner role used for migrations. The script enforces this: it aborts, naming the role, unless the running role owns the migrated tables and views in `public`. The role itself is cluster-level and created once; its grants are per-database, so re-run the script against every database in the cluster the agent reads:

```bash
psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -f internal/db/setup/readonly_role.sql
psql "$DATABASE_URL" -c '\password market_scout_readonly'
```

`ON_ERROR_STOP` is not optional: the script's guards abort on a role that owns objects, inherits another role, or is not the migration owner, and without the flag `psql` reports the abort and keeps applying grants anyway.

Future table grants attach to the owner that ran the script, and so does its rule that functions created later are not executable by `PUBLIC`. Postgres stores that rule per owner, so it covers only functions the script's owner creates — which is how `migrate up` creates them. A function created by any other role, an extension installed as superuser being the usual case, still arrives with the EXECUTE grant Postgres hands `PUBLIC` on every new function, and every role belongs to `PUBLIC`.

Re-run the script after any migration or `CREATE EXTENSION` that adds a routine — function, procedure, or aggregate — to `public`. For owner-created routines the re-run reconfirms a boundary that already holds; for the rest it is the only thing that revokes the `PUBLIC` grant. The routine itself does not say which case you are in, so re-run either way.

Choose the password out of band. Then add the matching DSN to root `.env.local`, next to `DATABASE_URL`:

```bash
DATABASE_URL_RO=postgres://market_scout_readonly:<password>@localhost:5432/market_scout?sslmode=disable
```

`DATABASE_URL` is the admin/read-write DSN for migrations, setup, and writer binaries. `DATABASE_URL_RO` is the agent-safe DSN for MCP verification. Human operators may keep both in `.env.local`; the MCP server reads only `DATABASE_URL_RO`.

The role's function access is deny-by-default: the explicit `GRANT EXECUTE` statements at the end of the script are its entire surface. Anything new on the read-only path needs a grant of that same shape — when vector search lands, that means the functions backing whichever distance operators the queries use.

pg_trgm's `%` similarity shorthand is not part of that surface: `a % b` resolves to `similarity_op`, ungranted, so it fails with a permission-denied error naming a function no caller typed. The MCP `query` tool hands agents arbitrary SQL over `DATABASE_URL_RO` — write `similarity()` there, never `%`.

`internal/db/setup/readonly_role.sql` is operational SQL, not a numbered migration and not sqlc output. Roles are cluster-level, and credentials do not belong in source. This setup file may be hand-edited; sqlc input stays in `internal/db/queries/` and numbered migrations.

### Action MCP role

The MCP server's write tools use `DATABASE_URL_ACTIONS`. Its role, `market_scout_actions`, can call only approved functions in the `mcp` schema — never run arbitrary writes. Approved functions are `SECURITY DEFINER`, owned by the database owner, added by numbered migrations. The role gets CONNECT, USAGE on `mcp`, and one EXECUTE grant per approved function. A leaked DSN can call only those functions.

Provision after migrations, with the same owner role used for migrations:

```bash
psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -f internal/db/setup/action_role.sql
psql "$DATABASE_URL" -c '\password market_scout_actions'
```

Rerun `action_role.sql` after any migration that adds an approved `mcp` function, so its EXECUTE grant attaches. The script's grant section lists one block per function; never use `GRANT EXECUTE ON ALL FUNCTIONS`. Choose the password out of band, then add the DSN to root `.env.local`:

```bash
DATABASE_URL_ACTIONS=postgres://market_scout_actions:<password>@localhost:5432/market_scout?sslmode=disable
```

`action_role.sql` is operational SQL, hand-editable, not a numbered migration or sqlc input — same as `readonly_role.sql`.

Primary debugging surfaces:

- **TablePlus** — GUI client. Connect using `DATABASE_URL`. Inspect tables, run ad-hoc queries, browse schema.
- **`psql`** — `docker exec -it market-scout-db psql` drops into a shell using the container's credentials. Port 5432 is also exposed to localhost, so a host-side `psql $DATABASE_URL` works if `psql` is installed.
- **Ad-hoc Go scripts** — write a `.go` file to `apps/tools/` (the Go module root), run with `go run <script>.go`, then delete it. Scripts must live alongside `go.mod`: Go modules require `go.mod` to resolve imports (`github.com/jackc/pgx/v5/stdlib`, `github.com/joho/godotenv`, etc.). Running from `/tmp` or any directory without `go.mod` fails with "no required module provides package."

### Test database

The `.db.test.ts` suites in `apps/web/` write fixtures to a dedicated database, `market_scout_test`, never to `market_scout`. The status and postings suites date their fixtures from `now()`, so in the development database they land inside the live `now() - interval` windows the read model queries, and they survive any teardown failure. The boundary is the separate database, not teardown hygiene.

Both DSNs resolve through `apps/web/lib/db/test-dsn.ts`, which enforces that boundary instead of trusting it. A DSN naming a database whose name does not end in `_test` throws before any connection opens, naming the database it found; unset stays a skip. The suffix is the contract, so a per-worktree or CI test database needs no code change — and a `DATABASE_URL_TEST` copied from `DATABASE_URL` fails every DSN-dependent test rather than writing fixtures into the development database.

Add both DSNs to root `.env.local` first — the provisioning commands below read them:

```bash
DATABASE_URL_TEST=postgres://market_scout:<password>@localhost:5432/market_scout_test?sslmode=disable
DATABASE_URL_TEST_RO=postgres://market_scout_readonly:<password>@localhost:5432/market_scout_test?sslmode=disable
```

Both name `market_scout_test`. `market_scout_readonly` already exists cluster-wide, so reuse the password `DATABASE_URL_RO` already carries; there is no `\password` step here.

`.env.local` is the only file the suites read. `apps/web/vitest.setup.ts` forces `NODE_ENV` to a non-`test` value before calling `loadEnvConfig(process.cwd())`: `.env.local` is in every file set except the test-mode one, so forcing a non-`test` mode is what keeps it loaded. `.env.test` is in no other set and is never read.

One Postgres cluster serves both databases. Source root `.env.local` into the shell (`set -a; source .env.local; set +a`), then create the database as the migration owner, apply migrations, and run the read-only grants against it:

```bash
psql "$DATABASE_URL" -c 'CREATE DATABASE market_scout_test'
DATABASE_URL="$DATABASE_URL_TEST" go run ./cmd/migrate up
psql -v ON_ERROR_STOP=1 "$DATABASE_URL_TEST" -f internal/db/setup/readonly_role.sql
```

The middle line hands the test DSN to the migration runner under the name the runner reads; written the other way round it migrates the development database instead. Sourcing first is load-bearing for it: `godotenv.Load` never overwrites a key already present in the environment, so an unsourced `DATABASE_URL="$DATABASE_URL_TEST"` expands to empty, still counts as set, and suppresses the `.env.local` fallback — the migration then finds no DSN.

`action_role.sql` is not needed here. No `.db.test.ts` suite calls an `mcp.` function, and `readonly_role.sql` now revokes `PUBLIC` EXECUTE itself — on the routines already in the schema, and by default privilege on the ones its owner creates later — so the function boundary no longer depends on `action_role.sql` to survive later migrations.

Parity between the two databases is an action you repeat, not a property you assert. `migrate up` and `readonly_role.sql` each target one database; run each against both after any migration, and re-run `readonly_role.sql` after any `CREATE EXTENSION` as well. Two checks read the result. `migrate version` must report the same version with the dirty flag clear in each. And this must return the same rows in each — one line per function, so two databases granting different functions read as different instead of as equal counts:

```sql
SELECT p.oid::regprocedure::text AS executable_by_readonly
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'public'
  AND has_function_privilege('market_scout_readonly', p.oid, 'EXECUTE')
ORDER BY 1;
```

The expected list is the per-function `GRANT EXECUTE` statements at the end of `readonly_role.sql`; read it rather than memorizing a count, and a commit that grants another function updates both sides at once. Signatures print in full, so a `timestamptz` argument in the script reads as `timestamp with time zone` here.

The count only exposes an ownership mismatch once a later migration adds a function and the count drifts; the script's own guard, not this check, catches the mismatch at provisioning time.

Vitest fixture rows that leaked into `market_scout` before the suites moved are removed by two operational scripts, run counts → purge → counts against a quiet database: `internal/db/setup/purge_test_fixtures_counts.sql`, then `internal/db/setup/purge_test_fixtures.sql`, then the counts script again. Their headers own the procedure — the dump that has to precede it, the reconciliation, and what the purge deliberately leaves behind.

### Schema Migrations

Migrations live in `apps/tools/internal/db/migrations/` as numbered SQL files. Apply with `go run ./cmd/migrate up` (from `apps/tools/`). Never edit a migration after it has run against any environment; add a new one.

`migrate` reads `DATABASE_URL` only, so one run lands in `market_scout` and leaves `market_scout_test` behind. Nothing gates on the gap — `pnpm test:db` never compares versions. Apply every migration to both:

```bash
go run ./cmd/migrate up
DATABASE_URL="$DATABASE_URL_TEST" go run ./cmd/migrate up
```

When the migration adds a routine to `public`, re-run `readonly_role.sql` against both afterward. See §2 Test database.

`migrate` supports four verbs: `up`, `down`, `force <version>`, `version`. `version` prints the current version and the dirty flag (or reports no migrations on a fresh DB). `force <version>` pins the recorded version and clears the dirty flag.

After a schema change, regenerate sqlc:

```bash
sqlc generate
```

#### Teardown and Recovery

`migrate down` is a full teardown — it reverts every migration, not a single step. It blocks at the RESTRICT foreign keys in migrations 000007 and 000008 when enrichment history exists, leaving the DB `dirty`. That block is a feature: it protects enrichment provenance from being silently dropped.

To recover from a stuck teardown:

1. `migrate version` — read the stuck version and confirm the dirty flag.
2. Delete the blocking rows the down migration names in its own comments: `canonical_role_dimensions` rows for the stuck dimension (000007), or `job_posting_roles` rows for the stuck role (000008). The down `DELETE` can then proceed.
3. `migrate force <N>` — clear the dirty flag at the stuck version `N`.
4. `migrate up` — re-apply forward to a clean state.

Each down migration spells out the prerequisite child-row `DELETE` in its comments — the `WHERE` clause to run against `canonical_role_dimensions` (`000007_add_sales_role_dimension.down.sql`) or `job_posting_roles` (`000008_add_legal_engineer_role.down.sql`). Run that first. The file's own runnable `DELETE` is the parent-row delete the FK blocks until those child rows are gone, so don't just copy that one.

### Reload Behavior

Go has no hot reload. Rebuild and re-run after every change. For a tight loop:

```bash
go run ./cmd/fetcher
```

`go run` recompiles each invocation; for the fetcher's startup time this is fine.

### Cost map

Builds and tests are free — run them liberally. Live-surface commands spend money, hit third-party APIs, or churn provenance — run them deliberately.

| Operation | Cost | Rule |
|---|---|---|
| `go build`, `go vet`, `go test ./...` | Free | The primary verification loop. Run freely. |
| `go test -tags=integration ./...` | Cheap; needs Postgres | Run when touching queries or migrations. |
| `go run ./cmd/fetcher` | Live ATS traffic | One manual run to verify a change is fine. Never in a loop — rate limits and politeness are real. |
| `go run ./cmd/batch-enrich` | Legacy/automation runner; may consume model/API budget | Never run as a test. Exercise the pipeline with unit tests and fakes; a live run is an operator decision. |
| `/batch-enrich` skill | Session tokens; worker usage is model-dependent | Primary human-operated bulk path. It reads through the project MCP server and writes only through `mcp.save_enrichment`; still an operator decision, never a test. |
| `batch-enrich --force` (either path) | Paid re-classification, provenance churn | Operator-only, after a contract fix. Never to "re-verify." |
| `go run ./cmd/migrate down` | Full teardown; blocks on enrichment history | See §2 Teardown and Recovery. Never a casual reset. |

Task agents run the free tier only. Live-surface commands belong to the coordinator or the human.

---

## 3) Build System Architecture

### Build Pipeline

From `apps/tools/`:

```bash
go build ./cmd/fetcher       # Compile the fetcher binary
go build ./...               # Compile every package (verifies the tree)
go vet ./...                 # Static checks
staticcheck ./...            # Stricter linter
go test ./...                # Run the test suite
```

Configured commands are also built into `bin/` via `make` (from `apps/tools/`):

```bash
make build       # Build all configured commands into bin/
make fetcher     # Build a single command (any name in CMDS)
make check       # go build ./... + go vet ./... + make build (pre-commit gate)
make clean       # Remove the built binaries
```

Targets always shell out to `go build` and let Go's build cache decide what to recompile — a Makefile prerequisite list can't track transitive Go deps and would risk stale binaries. `bin/` is gitignored; the binaries are local artifacts (native Mac arch). Cron and `cmd/batch-enrich`'s subprocess call (`./bin/strip-boilerplate`) both expect the prebuilt binaries, so rebuild after pulling changes. Add a new command by appending its `cmd/` directory name to `CMDS` in the Makefile.

`sqlc generate` is a separate step — it reads `apps/tools/internal/db/queries/*.sql` and schema from `apps/tools/internal/db/migrations/` and writes typed Go query functions into `apps/tools/internal/db/`. Run it after editing SQL or migrations.

### Output Structure

```
market-scout/
├── apps/
│   └── tools/                       # Go module (binaries + shared packages)
│       ├── go.mod
│       ├── cmd/
│       │   ├── fetcher/             # Main fetcher entry point
│       │   │   └── main.go
│       │   ├── mcp/                 # MCP server (read-only query gateway + action tools)
│       │   ├── batch-enrich/        # Batch classification runner
│       │   ├── onboard/             # Company onboarding tool (seed + probe)
│       │   ├── migrate/             # Migration runner
│       │   │   └── main.go
│       │   └── strip-boilerplate/   # Per-company boilerplate stripper (classification preprocessor)
│       │       └── main.go
│       └── internal/
│           ├── ats/                 # ATS adapter implementations (interface lives in apps/tools/cmd/fetcher per §5.3)
│           │   ├── greenhouse.go    # Greenhouse implementation
│           │   ├── lever.go         # Lever implementation
│           │   ├── ashby.go         # Ashby implementation
│           │   ├── workday.go       # Workday implementation
│           │   └── httpfetch.go     # Shared HTTP helpers (GET and POST)
│           ├── enrich/              # Enrichment pipeline packages
│           │   ├── boilerplate/     # Per-company boilerplate detection and stripping
│           │   ├── classify/        # Classifier contract, validation, taxonomy loading
│           │   └── selection/       # Posting selection logic for batch runs
│           └── db/
│               ├── migrations/      # Numbered migration files (source of truth for schema)
│               ├── queries/         # Hand-written SQL for sqlc
│               ├── setup/           # Hand-run operational SQL: role grants and one-time data repair (not migrations, not sqlc input)
│               ├── *.sql.go         # sqlc-generated query functions
│               └── models.go        # sqlc-generated row types
├── agent-context/
│   ├── lib/                         # Durable architecture docs
│   └── plans/                       # Ephemeral session plans
└── docker-compose.yml               # Postgres + pgvector
```

The Next.js app lives at `apps/web/`. Its setup and conventions are in [`web-guide.md`](./web-guide.md).

---

## 4) File & Directory Organization

**Rule: split by responsibility, not by line count.** A file earns a split when it serves distinct jobs. Line count alone is not a trigger.

### 4.1 File size guidance

- **~400–500 lines** (source, non-test): yellow flag. Consider splitting on next significant addition.
- **~600+ lines**: split before adding more code.
- **Test files**: exempt. Test suites are flat and linear; large is fine.
- **Generated files** (sqlc and any future codegen): exempt from size guidance, and never hand-edited. See §5.8.

Existing files above these thresholds are not immediate refactoring targets. Apply when adding significant new code.

### 4.2 Valid seams for splitting

Split along natural boundaries:

1. **Responsibility** — file serves two distinct jobs. Extract each into its own file (e.g. HTTP transport vs. response parsing).
2. **Consumer** — different importers use different subsets of exports. Each subset becomes a file.
3. **Change frequency** — stable plumbing vs. actively-evolving logic. Separate to reduce churn.

In Go, prefer **multiple files in the same package** over new packages. New packages are only justified when there's a real interface boundary (e.g. `apps/tools/internal/ats/` is a package because adapters share an interface; individual adapter files are not sub-packages).

### 4.3 Splits to avoid

- Arbitrary line-count splits with no conceptual boundary.
- No grab-bag `util.go` / `helpers.go` files. Co-locate single-caller helpers with their caller.
- Don't separate types from the code that uses them. Struct definitions live in the file that owns the behavior, unless the struct is a cross-package contract.

### 4.4 Directory density

- **Uniform directories** (all ATS adapters, all migrations): flat-and-many is fine. 20+ files OK.
- **Mixed-concern directories**: introduce subdirectories — but in Go this means a new package, so weigh the cost. Usually it's better to split files within the package.

### 4.5 When to split

- **Proactively**: when adding significant new functionality to an already-large file. You have full context; the split is cheapest now.
- **Not retroactively** just to meet a number. Only when the file actively causes pain (hard to navigate, merge conflicts, too many responsibilities).
- **Never during a bugfix.** Don't mix structural refactoring with behavior changes in one changeset.

---

## 5) Go Conventions

### 5.1 Readability-first

- Prefer **plain Go** over clever generics or reflection. Generics are appropriate when they remove real duplication across types; otherwise pick a concrete type.
- Prefer **descriptive names** and early returns over deeply nested logic. Guard clauses with `if err != nil { return ... }` are idiomatic — embrace them.
- Keep functions small; extract helpers only when it reduces duplication or clarifies a step.
- Receiver names are short (1–2 letters) and consistent across methods on the same type. Idiomatic Go style.

### 5.2 Error handling

- **No `panic` in library code.** `panic` is reserved for genuinely unrecoverable conditions (programmer error, corrupted invariant). API failures, missing fields, parse errors all return `error`.
- Wrap errors with `fmt.Errorf("fetching %s postings: %w", company, err)` to preserve the chain. Use `errors.Is` / `errors.As` to inspect.
- Define **typed errors** (`var ErrRateLimited = errors.New(...)` or a struct implementing `Error()`) when callers need to branch on failure mode. Plain string errors are fine for terminal failures.
- Return errors at the boundary where context is richest. Don't log-and-return — pick one. The fetcher entry point is the right place to log; library code returns.

### 5.3 Interfaces at boundaries

- Define interfaces in the package that **consumes** them, not the package that implements them. Idiomatic Go: `apps/tools/cmd/fetcher` declares what it needs from an ATS adapter; implementations in `apps/tools/internal/ats/` satisfy it implicitly.
- Keep interfaces small. The ATS adapter interface should be the minimum the fetcher needs to call (e.g. `FetchPostings(ctx, company) ([]domain.Posting, error)`, where `domain.Posting` lives in `apps/tools/internal/domain` so producers and consumers share it without depending on each other).
- Validate external responses at the adapter boundary. Decode JSON into typed structs; reject malformed payloads with a wrapped error rather than passing `map[string]any` upward.

### 5.4 Structs over maps for typed data

- Anything with a known shape gets a `struct`. Maps are for genuinely dynamic key sets.
- ATS API responses → decode into structs that mirror the response, then translate into domain types (`apps/tools/internal/domain.Posting`) before returning. The wire shape and the domain shape are separate concerns.
- Use struct tags (`json:"job_id"`) at the wire-shape layer only. Domain types should not carry transport tags.

### 5.5 Type switches over stringly-typed branching

When you have a set of related variants (e.g. fetch outcome: success, rate-limited, not-modified, error), prefer a sealed interface + type switch over a `string` discriminator field. The compiler enforces exhaustiveness at the call site, and adding a new variant surfaces every place that needs to handle it.

### 5.6 Concurrency

Concurrency exists to overlap waits — outbound HTTP, mostly. It is not a performance lever for CPU or DB throughput. Apply the minimum that meets the requirement.

- **Concurrency at the unit-of-work boundary.** One company, one goroutine. Sequential within. Goroutines inside a unit of work are a smell — the bottleneck is the upstream API, not local code.
- **Bounded fan-out.** Cap simultaneous outbound requests with a semaphore. Unbounded fan-out is rude to ATS providers and trades politeness for marginal speed.
- **Atomicity per unit.** Each unit's writes land in a single transaction or not at all. Snapshot semantics depend on `fetched_at` representing a complete view of one board.
- **Context-driven shutdown.** Plumb `context.Context` from `signal.NotifyContext` through every blocking call. Acquiring a worker slot is itself blocking — `select` against `ctx.Done()` so SIGTERM doesn't strand undispatched work. In-flight work finishes within its own timeout or surfaces `context.Canceled`; classify and report, don't retry.
- **Stdlib patterns.** `sync.WaitGroup`, buffered channels as semaphores, `errgroup` when error propagation matters. Avoid hand-rolled pools or channel pipelines — they obscure intent.

### 5.7 Database access

- All SQL goes through `sqlc`-generated query functions. No string-built queries in business logic.
- Exception: `apps/tools/cmd/mcp` may accept caller-provided SQL only inside the MCP read-only query gateway. It must use the read-only role, a read-only transaction, statement timeout, and row cap.
- Exception: `apps/tools/cmd/mcp` action tools call approved `mcp.*` SECURITY DEFINER functions through the action pool; callers never supply SQL. `add_company` issues a fixed, fully-parameterized `QueryRowContext` call (constant statement, all values bound) rather than a sqlc-generated function — sqlc's offline parser cannot expand a multi-column `RETURNS TABLE` without a live database. `save_enrichment` uses a sqlc-generated function — `mcp.save_enrichment` returns a scalar `jsonb`, which the offline parser handles fine.
- Hand-written SQL lives in `apps/tools/internal/db/queries/*.sql` with `-- name: FunctionName :many` annotations.
- sqlc targets the standard `database/sql` interface (no `sql_package` override). pgx v5 is registered as the `pgx` driver via a blank import of `github.com/jackc/pgx/v5/stdlib`. Open with `sql.Open("pgx", dsn)`; pass the resulting `*sql.DB` — or a `*sql.Tx` inside a transaction — into `db.New`.
- Generated row types use `sql.NullString` and `sql.NullTime`. Translate to and from `*string` / `*time.Time` at the DB boundary so `apps/tools/internal/domain` stays free of `database/sql` symbols.
- `apps/tools/cmd/fetcher` owns DB lifecycle: it opens the pool, begins per-company transactions, and invokes the sqlc functions. ATS adapters are HTTP-only and never receive a `*sql.DB`.
- Snapshot writes are **append-only inserts**. There is no update path for `posting_snapshots`. If you find yourself reaching for `ON CONFLICT ... DO UPDATE`, stop and re-read the snapshot model.

### 5.8 Generated files

Hard rule: **never hand-edit a generated file.** Humans, agents, no exceptions. The next codegen run silently strips the change.

Recognize one by the header `Code generated by ... DO NOT EDIT.` at the top of the file, in whatever comment syntax that file type uses.

| File / glob | Generator | Committed? | Regenerate with |
|---|---|---|---|
| `apps/tools/internal/db/*.sql.go` | sqlc | yes | `sqlc generate` |
| `apps/tools/internal/db/db.go`, `apps/tools/internal/db/models.go` | sqlc | yes | `sqlc generate` |
| `apps/tools/internal/db/migrations_embed.go` | hand-written (uses `//go:embed`) | yes — not generated | n/a |

sqlc is the only generator in the tree, and nothing in `apps/web` is generated. There are no `go:generate` directives; sqlc runs by hand, and its output diff is committed alongside the source change that caused it.

If generated output is wrong, **fix the source, not the output:**

- sqlc type wrong for a column? Edit the `.sql` query in `apps/tools/internal/db/queries/`, or add an `overrides` entry in `sqlc.yaml`. Then regenerate.
- Migrations: never edit a shipped migration; add a new one (see §2 Schema Migrations).

Workflow: change the source → `sqlc generate` → review the diff → commit input and output together. The generated diff is part of the change.

---

## 6) Logging, Errors, and Debuggability

### 6.1 Logging rules

- Prefix logs with a subsystem tag: `[fetcher]`, `[greenhouse]`, `[db]`, `[migrate]`.
- Use structured logging (`log/slog`). Key-value pairs over interpolated strings: `slog.Info("fetched postings", "company", c, "count", n)`.
- Log actionable failures once at the boundary; avoid spamming logs in hot paths (e.g. don't log per posting during a 5,000-row insert).
- Make the "happy path" easy to follow in code; keep failure branches explicit.

### 6.2 Inspecting state

- `go test -v ./...` — verbose test output, the primary first-line debugging tool.
- **TablePlus** or an ad-hoc Go script (see §2 Database Access) — inspect snapshot rows, vector embeddings, and migration state directly.
- `slog` output goes to stderr by default. Pipe through `jq` if you switch the handler to JSON for richer filtering.

**Enrichment / classification output.** The `classifications` table is the primary data quality surface. Current classification per posting is the latest row by `classified_at` — the `DISTINCT ON` pattern in `apps/tools/internal/db/queries/classifications.sql` is the canonical query shape; read it before writing ad-hoc inspection queries.

Key invariants:

- `created_at` on `canonical_roles`, `specializations`, and `skills` distinguishes emergent (agent-minted at runtime) from seeded (migration-installed) taxonomy entries.
- Slug collisions across `specializations` and `skills` are expected — the taxonomy deduplicates by slug at load time, first-owner wins. Logged as warnings at run start, not errors.
- `prompt_version` on each `classifications` row identifies which classifier contract produced it — the primary audit key across contract changes. See *Prompt version lineages* below.
- `failures.jsonl` at `agent-output/batch-enrich/failures.jsonl` is append-only history for operator review. The enrichment coordinator records terminal worker failures there after reconciling them with persisted classifications; it does not drive post-failure behavior.
- `--force` re-classifies postings that already have a `classifications` row. Without it, the enrichment workflow skips them. Use it only for a corrected contract.

**Prompt version lineages.** `prompt_version` answers one question: which
classifier contract produced this row. It never encodes the model — `model` is
its own column — and it never encodes the harness.

One writer, one pin. Each pin is the only place its value is written; nothing
else restates it as a literal.

| Writer | Pin lives in | Lineage |
|---|---|---|
| Codex skill (live path) | `classification-pins` block, `.agents/skills/batch-enrich/SKILL.md` | `batch-enrich-v<n>` |
| Claude skill | `classification-pins` block, `.claude/skills/batch-enrich/SKILL.md` | `batch-enrich-v<n>` |
| Go runner (legacy/automation) | `PromptVersion` constant, `apps/tools/cmd/batch-enrich/config.go` | `batch-enrich-go-v<n>` |

The two skills share one lineage and bump together: they run the same
classification contract under different harnesses and models. The Go runner
does not — different contract, different transport, and it can run the same
Haiku model as the Claude skill, so a shared lineage would make its rows
indistinguishable.

`mcp.save_enrichment` requires `provenance.model` and
`provenance.prompt_version` and rejects a call omitting either. It does not
check the value against a list of known versions: an allowlist in the server
would force a rebuild and restart before a bumped pin could be written, which
is the surest way to stop pins being bumped.

**Historical rows are not relabelled.** Storage is append-only and these rows
are the only evidence the drift happened. Four cohorts written before 2026-09-21
cannot be attributed to a single writer, and queries that split by
`prompt_version` must treat them as unresolved:

| `prompt_version` | Model | Rows | Ambiguity |
|---|---|---:|---|
| `batch-enrich-v4` | `claude-haiku-4-5-20251001` | 4,474 | Claude skill or Go runner `--runner claude` — same pin, same model, overlapping window. |
| `batch-enrich-v5` | `claude-haiku-4-5-20251001` | 561 | v5 was specified for the Codex path; these Haiku rows carry it too. Same label, different contract from the rows below. |
| `batch-enrich-v6` | `claude-sonnet-5` | 66 | No pin in any writer names this model. Origin unidentified. |
| `mcp-save-enrichment-v1` | `mcp-agent` | 231 | The removed `save_enrichment` default. Records only that provenance was omitted; the real contract and model are unrecoverable. |

### 6.3 Where logs come from

In a single-binary app, all logs surface in the terminal where you ran the fetcher. The subsystem tag tells you which package emitted the line. Filter with `grep '\[greenhouse\]'` or similar.

If a fetch fails silently, check (1) the database — did snapshots actually write? (2) the exit code of the fetcher binary, (3) the structured error chain (look for the wrapped `%w` context).

### 6.4 Console subsystem tags

Logs are prefixed with subsystem tags to identify the source package. Common tags include `[fetcher]`, `[greenhouse]`, `[lever]`, `[ashby]`, `[db]`, `[migrate]`. Filter by tag to isolate traffic during debugging.

---

## 7) Code Comments

### 7.1 Comments that earn their keep

- **Why, not what.** Explain rationale. The code shows behavior.
- **Non-obvious context.** Why a field is denormalized, why a query is written this way for a specific Postgres planner quirk, why an ATS adapter handles a quirky response shape. Things a reader can't derive from the code alone.
- **Spec pointers.** Brief link to the governing doc or contract when code implements a specific spec. File path or doc name — not an inline summary.

### 7.2 File headers (package doc comments)

In Go, the file-level comment on the `package` declaration becomes the package's godoc. Keep it short — what the package owns, then which doc governs it.

**Good:**
```go
// Package ats defines the adapter interface for applicant tracking systems
// and hosts per-vendor implementations.
// See: agent-context/lib/project.md
package ats
```

**Too much:**
```go
// Package ats provides a unified abstraction over multiple ATS vendors.
// Each adapter implements the FetchPostings method, returning a normalized
// []Posting slice. The package handles retries, rate limiting, and response
// validation. The adapter pattern allows new vendors to be added by ...
```

The header's job is to tell you *which doc to load*, not to summarize that doc. Behavioral descriptions and architectural context belong in the spec.

### 7.3 Comments to avoid

- **Restating code.** If the code is unclear, improve the code — don't narrate it.
- **Changelog annotations.** `// Added in PR #42`, `// Refactored from old approach`. Git handles provenance.
- **Orphan TODOs.** `// TODO: fix later` without a ticket or actionable context. File a ticket and reference it, or fix it now.
- **Duplicating docs.** Don't restate architecture docs in comments. Two sources of truth, both eventually wrong.

### 7.4 Revising on encounter

When you find a misleading, stale, or code-restating comment in a file you're already changing:

- Fix or remove it in the same changeset. A missing comment beats a lying one.
- Scope to code you're touching. Don't sweep unrelated files for comment cleanup.

---

## 8) Testing

For detailed patterns, test strategy, and commands, see [Testing Guide](./testing-guide.md).
