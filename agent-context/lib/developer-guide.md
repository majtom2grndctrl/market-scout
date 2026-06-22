# market-scout Development Guide

> **Read this when:** setting up the repo, adding a feature, or onboarding. Covers dev setup, conventions, and coding standards.
> **Key invariant:** snapshots are append-only — every fetch writes new timestamped rows; never upsert. Validate at every external API and DB boundary.
> **Related:** [Project Overview](./project.md) · [Testing Guide](./testing-guide.md) · [Style Guide](./style-guide.md)

---

## Agent TL;DR

- Optimize for **readability over cleverness**; prefer small, explicit changes.
- Respect **package boundaries**: `apps/tools/cmd/fetcher` orchestrates and owns the DB lifecycle. `apps/tools/internal/ats` adapts ATS APIs (HTTP only — no DB access). `apps/tools/internal/db` holds sqlc output. Adapters and the fetcher exchange `domain.Posting` from `apps/tools/internal/domain`.
- `posting_snapshots` is **append-only**. Every fetch writes new timestamped rows. Never upsert. This is load-bearing for trend analysis.
- Define **Go interfaces at package boundaries** (`apps/tools/internal/ats/` exposes the ATS adapter interface; each ATS is a separate file).
- Validate ATS API responses at the adapter boundary; return typed errors. No `panic` in library code.
- Database access goes through `sqlc`-generated functions; no ad-hoc SQL strings in business logic. Raw SQL lives in `apps/tools/internal/db/queries/`.
- Next.js app layer is **deferred**. See [`project.md` §Non-goals](./project.md).
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

The key principle: **specs are working documents — update them during implementation, never silently deviate.** After the feature ships, durable knowledge lives in architecture docs and code comments; the spec is consumed and removed. See §1.5.

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

Specs are working documents — they align on what to build and why, then get consumed during implementation.

**Where specs live:**

- **Default: ticket.** Spec content lives in the parent ticket alongside orchestration guidance (child task breakdown, dependencies, sequencing). One focused space for everything about the epic. Tickets include acceptance criteria — the conditions that must hold for the work to be complete. Vague "done" conditions push ambiguity to the implementer.
- **Escape hatch: `agent-context/plans/`.** When the spec outgrows ticket format — complex schemas needing cross-referencing, extensive tables, content multiple implementers need open simultaneously — extract to a temporary doc. The parent ticket keeps orchestration and links to the temp doc. Use judgment on when to extract.

**After the feature ships:**

- **Delete the spec.** Temp docs in `agent-context/plans/` are removed when the epic closes. Ticket specs close naturally with the ticket.
- **Architecture docs** (`agent-context/lib/`) capture what's durable — design principles, package boundaries, contracts, snapshot model, schema invariants. Content an agent can't derive by opening the relevant file.
- **Code comments** capture implementation-level "why" decisions. Rationale a reader can't derive from the code alone. See §7.

**What doesn't belong in `agent-context/lib/`:**

- Specs for specific features or epics (use tickets or `agent-context/plans/`)
- Implementation plans or task breakdowns (use tickets)
- Content that names specific functions, types, or file paths as load-bearing detail (see [Style Guide](./style-guide.md))

---

## 2) Development Setup

### Working directory

Go commands run from `apps/tools/`, not the repo root. The module root, `godotenv.Load(".env.local")` in every binary, and two CWD-relative path literals (the onboard seed path and the batch-enrich failures-log path) all assume this. `docker compose` runs from repo root.

### Initial Setup

After cloning, start the database, link env, and apply migrations:

```bash
docker compose up -d                        # Postgres + pgvector (repo root)
ln -s ../../.env.local apps/tools/.env.local # Fresh-clone setup; root .env.local is canonical
cd apps/tools
go run ./cmd/migrate up                     # Apply schema migrations
```

`.env.local` is canonical at repo root (docker-compose reads it). `apps/tools/.env.local` is a symlink so the Go tools see the same file — no DB-credential drift. Both are gitignored; the symlink is a local-setup step, not checked in.

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

Provision the role once per Postgres cluster, after migrations:

```bash
psql "$DATABASE_URL" -f internal/db/setup/readonly_role.sql
psql "$DATABASE_URL" -c '\password market_scout_readonly'
```

Run the script with the same owner role used for migrations, so future table grants attach to that owner.

Choose the password out of band. Then add the matching DSN to root `.env.local`, next to `DATABASE_URL`:

```bash
DATABASE_URL_RO=postgres://market_scout_readonly:<password>@localhost:5432/market_scout?sslmode=disable
```

`DATABASE_URL` is the admin/read-write DSN for migrations, setup, and writer binaries. `DATABASE_URL_RO` is the agent-safe DSN for MCP verification. Human operators may keep both in `.env.local`; the MCP server reads only `DATABASE_URL_RO`.

`internal/db/setup/readonly_role.sql` is operational SQL, not a numbered migration and not sqlc output. Roles are cluster-level, and credentials do not belong in source. This setup file may be hand-edited; sqlc input stays in `internal/db/queries/` and numbered migrations.

Primary debugging surfaces:

- **TablePlus** — GUI client. Connect using `DATABASE_URL`. Inspect tables, run ad-hoc queries, browse schema.
- **`psql`** — `docker exec -it market-scout-db psql` drops into a shell using the container's credentials. Port 5432 is also exposed to localhost, so a host-side `psql $DATABASE_URL` works if `psql` is installed.
- **Ad-hoc Go scripts** — write a `.go` file to `apps/tools/` (the Go module root), run with `go run <script>.go`, then delete it. Scripts must live alongside `go.mod`: Go modules require `go.mod` to resolve imports (`github.com/jackc/pgx/v5/stdlib`, `github.com/joho/godotenv`, etc.). Running from `/tmp` or any directory without `go.mod` fails with "no required module provides package."

### Schema Migrations

Migrations live in `apps/tools/internal/db/migrations/` as numbered SQL files. Apply with `go run ./cmd/migrate up` (from `apps/tools/`). Never edit a migration after it has run against any environment; add a new one.

After a schema change, regenerate sqlc:

```bash
sqlc generate
```

### Reload Behavior

Go has no hot reload. Rebuild and re-run after every change. For a tight loop:

```bash
go run ./cmd/fetcher
```

`go run` recompiles each invocation; for the fetcher's startup time this is fine.

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
│           └── db/
│               ├── migrations/      # Numbered migration files (source of truth for schema)
│               ├── queries/         # Hand-written SQL for sqlc
│               ├── *.sql.go         # sqlc-generated query functions
│               └── models.go        # sqlc-generated row types
├── agent-context/
│   ├── lib/                         # Durable architecture docs
│   └── plans/                       # Ephemeral session plans
└── docker-compose.yml               # Postgres + pgvector
```

The Next.js app layer lands at `apps/web/` later; see [`project.md` §Non-goals](./project.md).

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
- Hand-written SQL lives in `apps/tools/internal/db/queries/*.sql` with `-- name: FunctionName :many` annotations.
- sqlc targets the standard `database/sql` interface (no `sql_package` override). pgx v5 is registered as the `pgx` driver via a blank import of `github.com/jackc/pgx/v5/stdlib`. Open with `sql.Open("pgx", dsn)`; pass the resulting `*sql.DB` — or a `*sql.Tx` inside a transaction — into `db.New`.
- Generated row types use `sql.NullString` and `sql.NullTime`. Translate to and from `*string` / `*time.Time` at the DB boundary so `apps/tools/internal/domain` stays free of `database/sql` symbols.
- `apps/tools/cmd/fetcher` owns DB lifecycle: it opens the pool, begins per-company transactions, and invokes the sqlc functions. ATS adapters are HTTP-only and never receive a `*sql.DB`.
- Snapshot writes are **append-only inserts**. There is no update path for `posting_snapshots`. If you find yourself reaching for `ON CONFLICT ... DO UPDATE`, stop and re-read the snapshot model.

### 5.8 Generated files

Hard rule: **never hand-edit a generated file.** Humans, agents, no exceptions. The next codegen run silently strips the change.

Recognize one by the header `// Code generated by ... DO NOT EDIT.` at the top of the file.

| File / glob | Generator | Committed? | Regenerate with |
|---|---|---|---|
| `apps/tools/internal/db/*.sql.go` | sqlc | yes | `sqlc generate` |
| `apps/tools/internal/db/db.go`, `apps/tools/internal/db/models.go` | sqlc | yes | `sqlc generate` |
| `apps/tools/internal/db/migrations_embed.go` | hand-written (uses `//go:embed`) | yes — not generated | n/a |

There are no `go:generate` directives in the tree; sqlc is invoked by hand and the diff is committed alongside the SQL change.

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
- `prompt_version` on each `classifications` row identifies which agent contract produced it — the primary audit key across contract changes.
- `failures.jsonl` at `agent-output/batch-enrich/failures.jsonl` is append-only history for operator review. The binary never reads it; it does not drive post-failure behavior.
- `--force` re-classifies postings that already have a `classifications` row. Without it, the binary skips them. Useful for re-running a corrected contract against already-processed postings.

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
