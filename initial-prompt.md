Here's a draft prompt. Treat it as a starting point - tweak the voice and any details that don't match how you actually think about it.

---

I'm starting a new project tonight and I want to give you full context before we begin.

**What I'm building:**

A local-first market intelligence tool that tracks job postings across companies I'm interested in, so I can answer questions about role trends, hiring patterns, and market dynamics over time. Think "personal Crunchbase for hiring data, scoped to my niche."

The audience is initially me, but I'm building it open-source for other people who build products with AI - designers, PMs, hybrid roles - to fork and run their own instance with their own company list.

**Why I'm building it:**

Two reasons that should shape your suggestions:

1. I want to develop credible backend skills as a generalist who can work at small companies. I've been a designer-engineer hybrid for years and need to fill in real backend fundamentals.
2. I want actionable market intel from day one, with the data compounding into trend analysis over weeks and months.

I'm not trying to build a product to sell. I'm building a personal tool that doubles as a portfolio piece and a learning vehicle.

**Architecture decisions I've made:**

- **Go** for the fetcher binary (concurrent HTTP fetching from ATS APIs, scheduled via cron)
- **Postgres** in Docker for local dev, with the option to point at Supabase later for "production"
- **Next.js** as the full-stack app layer, with Server Actions / API routes connecting directly to Postgres via `pg` or `postgres.js` (no separate Go API server)
- **pgvector** extension enabled from day one for future semantic search
- **Snapshot-based storage** - every fetch writes timestamped rows, never destructive updates. This is architecturally load-bearing for trend analysis. Don't suggest upsert patterns.
- **`sqlc`** in Go — write SQL, get generated type-safe Go code. Underlying driver is `pgx`. No magic ORM; SQL stays legible and learnable. pgvector similarity queries written as raw SQL (ORMs don't map cleanly to vector ops anyway). DB doubles as knowledge store for an AI agent.
- **`init` CLI** eventually, for scaffolding new users' setups

**ATS targets:**

Starting with Greenhouse (cleanest API). Lever and Ashby coming later. Want adapter pattern that makes adding new ATS sources straightforward.

**My background:**

I'm a UX/Design Engineer with deep React/TypeScript experience, comfortable in Next.js, Electron, and most modern frontend tooling. I'm new to Go and want idiomatic patterns explained, not just working code. When you make architectural choices in Go (channels vs mutexes, interface design, error handling patterns), tell me why so I'm learning and not just copy-pasting.

**Today's session goals:**

1. `docker-compose.yml` running Postgres locally
2. Initial schema migration - `companies`, `job_postings`, `posting_snapshots` tables, plus `CREATE EXTENSION vector`
3. Go project scaffold - `cmd/fetcher`, `internal/ats`, `internal/db`
4. One Greenhouse adapter
5. A manual run that fetches one real company, stores snapshots, prints what it found

**Definition of done:** I can run the Go binary, it hits Greenhouse for one company, and I can `psql` into Postgres to see timestamped rows.

Things to defer to future sessions: scheduler, additional ATS adapters, the Next.js app, the enrichment workflow, the agent UI.

**Working agreement:**

- Save key architectural decisions and project conventions to the appropriate file in `agent-context/lib` as we make them
- Ask me clarifying questions before making non-obvious choices
- When you're about to do something Go-idiomatic that I might not recognize, explain it briefly
