# Agent Context Index

> **Read this first.** Entry point for all agent context in this repo.
> **Key invariant:** this file routes to the minimal docs needed for a task — not a summary of those docs.
> **Related:** `project.md` (full architecture), `style-guide.md` (writing conventions for this directory)

---

## Agent Router (Task → Minimal Docs)

Take your side, plus **Either side**. Skip the other.

**Frontend (`apps/web/`)**

- **Anything under `apps/web/`** → `agent-context/lib/web-guide.md` *(names which `developer-guide.md` sections still apply)*
- **Tailwind classes, custom utilities, design tokens, `cn()` silently dropping a class** → `agent-context/lib/web-guide.md` §Styling
- **Storybook stories — where they live, why the font decorator exists** → `agent-context/lib/web-guide.md` §Storybook
- **Build, typecheck, and dev commands** → `agent-context/lib/web-guide.md` §Commands

**Backend (`apps/tools/`)**

- **Dev setup, Go conventions, logging, comments** → `agent-context/lib/developer-guide.md`
- **Working directory for Go commands, `.env.local` symlink** → `agent-context/lib/developer-guide.md` §2 Development Setup
- **Test strategy, patterns, running tests** → `agent-context/lib/testing-guide.md` *(Go only)*
- **Go fetcher (structure, conventions)** → `agent-context/lib/project.md` §Settled architecture
- **ATS adapter (adding or modifying)** → `agent-context/lib/project.md` §ATS targets
- **Database schema / migrations** → `agent-context/lib/project.md` §Settled architecture
- **Snapshot storage model** → `agent-context/lib/project.md` §Settled architecture *(append-only, never upsert)*
- **pgvector / semantic search** → `agent-context/lib/project.md` §The database as AI agent knowledge store
- **Inspecting enrichment / classification data quality** → `agent-context/lib/developer-guide.md` §6.2
- **Company watchlist (active scrape run, candidates, onboarding)** → `agent-context/lib/watchlist.md`
- **Trust tiers for agent-written data** → `agent-context/lib/project.md` §Evidence trust tiers

**Either side**

- **Project purpose, goals, audience** → `agent-context/lib/project.md`
- **Repo layout (`apps/`, `research/`)** → `agent-context/lib/project.md` §Repo layout
- **Settled architecture decisions** → `agent-context/lib/project.md` §Settled architecture
- **Generated files — never hand-edit** → `agent-context/lib/developer-guide.md` §5.8 *(sqlc)*
- **When to skip the plan pipeline** → `agent-context/lib/developer-guide.md` §1.5 *(the light lane)*
- **Writing / editing `agent-context/` files** → `agent-context/lib/style-guide.md`
- **What's deferred / out of scope** → `agent-context/lib/project.md` §Non-goals

---

## Key Architectural Decisions

| Decision | Choice | Notes |
|---|---|---|
| Fetcher | Go binary, `apps/tools/cmd/fetcher` | Concurrent HTTP from ATS APIs, cron-scheduled |
| Database | Postgres in Docker | Option to point at Supabase later |
| DB client | `sqlc` + `database/sql` + `pgx/v5/stdlib` | Write SQL, get generated type-safe Go. No ORM. pgx is the registered `database/sql` driver. |
| Vector search | pgvector extension | Enabled from day one. Similarity queries in raw SQL. |
| Storage model | Append-only snapshots | Every fetch writes timestamped rows. Never upsert. Load-bearing for trend analysis. |
| App layer | Next.js → Postgres direct | No separate Go API server. `pg` or `postgres.js`. |
| ATS adapters | Interface in `apps/tools/cmd/fetcher`; implementations in `apps/tools/internal/ats/` | All adapters implement the same `FetchPostings` contract. |
