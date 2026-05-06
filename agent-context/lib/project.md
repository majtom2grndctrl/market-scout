# Project Overview

> **Read this when:** starting any session or onboarding to this codebase. Covers purpose, audience, and settled architectural decisions.
> **Key invariant:** decisions here are load-bearing — don't relitigate without explicit discussion.
> **Related:** `index.md` (decision table), `style-guide.md` (writing conventions)

---

## What it is

Local-first market intelligence tool. Tracks job postings across companies of interest. Surfaces role trends, hiring patterns, and market dynamics over time. "Personal Crunchbase for hiring data, scoped to a niche."

## Audience

Initially the author. Open-source so other AI-product builders (designers, PMs, hybrid roles) can fork and run their own instance with their own company list.

## Why it exists

Two goals that should shape every suggestion:

1. Developing credible backend skills as a generalist who works at small companies.
2. Actionable market intel from day one, compounding into trend analysis over weeks and months.

Not a product to sell. A personal tool that doubles as a portfolio piece and learning vehicle.

## Settled architecture

| Layer | Choice | Notes |
|---|---|---|
| Fetcher | Go binary, `cmd/fetcher` | Concurrent HTTP from ATS APIs, cron-scheduled |
| Database | Postgres in Docker | Option to point at Supabase later |
| DB client | `sqlc` + `database/sql` + `pgx/v5/stdlib` | Write SQL, get generated type-safe Go. No ORM. Standard `database/sql` target (no `sql_package` override); generated models use `sql.NullString`/`sql.NullTime`, translated to `*string`/`*time.Time` at the DB boundary. |
| Vector search | pgvector extension | Enabled from day one. Similarity queries in raw SQL. |
| Storage model | Append-only snapshots | Every fetch writes timestamped rows. Never upsert. Load-bearing for trend analysis. |
| App layer | Next.js (Server Actions / API routes → Postgres direct) | No separate Go API server. `pg` or `postgres.js`. |
| Future | `init` CLI | Scaffolds new users' setups. Not in scope yet. |

## ATS targets

Greenhouse first (cleanest API). Lever and Ashby follow. Each ATS is a separate file in `internal/ats`; all adapters return `domain.Posting` from `internal/domain`.

## The database as AI agent knowledge store

The Postgres DB is also intended to serve as a knowledge store for an AI agent layer. pgvector is load-bearing for this. Semantic search queries are raw SQL — vector ops don't map to ORM query builders.

## Non-goals (current scope)

- Scheduler (deferred)
- Lever and Ashby adapters (deferred)
- Next.js app UI (deferred)
- Enrichment workflow (deferred)
- Agent UI (deferred)
