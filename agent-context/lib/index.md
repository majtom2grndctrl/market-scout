# Agent Context Index

> **Read this first.** Entry point for all agent context in this repo.
> **Key invariant:** this file routes to the minimal docs needed for a task — not a summary of those docs.
> **Related:** `project.md` (full architecture), `style-guide.md` (writing conventions for this directory)

---

## Agent Router (Task → Minimal Docs)

- **Project purpose, goals, audience** → `agent-context/lib/project.md`
- **Settled architecture decisions** → `agent-context/lib/project.md` §Settled architecture
- **Writing / editing `agent-context/` files** → `agent-context/lib/style-guide.md`
- **Dev setup, Go conventions, logging, comments** → `agent-context/lib/developer-guide.md`
- **Test strategy, patterns, running tests** → `agent-context/lib/testing-guide.md`
- **Go fetcher (structure, conventions)** → `agent-context/lib/project.md` §Settled architecture
- **ATS adapter (adding or modifying)** → `agent-context/lib/project.md` §ATS targets
- **Database schema / migrations** → `agent-context/lib/project.md` §Settled architecture
- **Generated files (sqlc, codegen) — never hand-edit** → `agent-context/lib/developer-guide.md` §5.8
- **Snapshot storage model** → `agent-context/lib/project.md` §Settled architecture *(append-only, never upsert)*
- **pgvector / semantic search** → `agent-context/lib/project.md` §The database as AI agent knowledge store
- **Next.js app layer** → `agent-context/lib/project.md` §Settled architecture
- **What's deferred / out of scope** → `agent-context/lib/project.md` §Non-goals

---

## Key Architectural Decisions

| Decision | Choice | Notes |
|---|---|---|
| Fetcher | Go binary, `cmd/fetcher` | Concurrent HTTP from ATS APIs, cron-scheduled |
| Database | Postgres in Docker | Option to point at Supabase later |
| DB client | `sqlc` + `database/sql` + `pgx/v5/stdlib` | Write SQL, get generated type-safe Go. No ORM. pgx is the registered `database/sql` driver. |
| Vector search | pgvector extension | Enabled from day one. Similarity queries in raw SQL. |
| Storage model | Append-only snapshots | Every fetch writes timestamped rows. Never upsert. Load-bearing for trend analysis. |
| App layer | Next.js → Postgres direct | No separate Go API server. `pg` or `postgres.js`. |
| ATS adapters | Interface in `cmd/fetcher`; implementations in `internal/ats/` | All adapters implement the same `FetchPostings` contract. |
