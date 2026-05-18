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
| Classifier | Go binary, `cmd/batch-enrich` | Cron-schedulable. Dispatches Haiku subprocesses via `claude -p` for per-posting classification. No Anthropic SDK or direct REST. Writeback through the sqlc query layer. |
| Database | Postgres in Docker | Option to point at Supabase later |
| DB client | `sqlc` + `database/sql` + `pgx/v5/stdlib` | Write SQL, get generated type-safe Go. No ORM. Standard `database/sql` target (no `sql_package` override); generated models use `sql.NullString`/`sql.NullTime`, translated to `*string`/`*time.Time` at the DB boundary. |
| Vector search | pgvector extension | Enabled from day one. Similarity queries in raw SQL. |
| Storage model | Append-only snapshots | Every fetch writes timestamped rows. Never upsert. Load-bearing for trend analysis. |
| App layer | Next.js (Server Actions / API routes → Postgres direct) | No separate Go API server. `pg` or `postgres.js`. |
| Future | `init` CLI | Scaffolds new users' setups. Not in scope yet. |

Our own first-observed timestamp on a job-posting record is the load-bearing repost-detection signal. ATS-reported "first published" and "created at" timestamps refresh on repost on at least some boards, so they cannot be trusted as the primary signal. The append-only snapshot model combined with an immutable first-observed timestamp — set once on initial upsert, never overwritten — is what makes repost detection durable. Source-reported timestamps are captured on every snapshot as a secondary change-detection signal, not as the repost anchor.

Every fetch is recorded as a fetch-run row, one per company per invocation, capturing the outcome and timing of that attempt. New snapshots link back to the run that produced them; snapshots written before the fetch-run migration carry a NULL run reference. This separation is load-bearing for trend queries: a posting's absence from a fetch only counts as "removed" when the run for that company succeeded. A failed run, or a company we didn't fetch in a given window, is distinguishable from a posting that genuinely disappeared from the board. Without the run record, a network blip looks identical to a wave of closed roles.

Classification runs on its own cadence. The classifier selects unclassified postings, strips per-company boilerplate from each description, and dispatches Haiku subprocesses in parallel waves. Each subprocess validates and retries on its own failure modes. Dispatch is parallel; writeback is serial — one transaction at a time through the sqlc query layer. Serial writeback is load-bearing: parallel writers would collide on newly-introduced taxonomy rows mid-wave and produce duplicates. Between waves, the classifier reloads the taxonomy from the DB so roles introduced by earlier waves are visible to later ones. An append-only `failures.jsonl` log is the cron observability surface; without it, failure history vanishes between runs.

## ATS targets

Four adapters are live: Greenhouse, Lever, Ashby, and Workday. Each ATS is a separate file in `internal/ats`; all adapters return `domain.Posting` from `internal/domain`.

| Platform | `board_token` format | API type |
|---|---|---|
| Greenhouse | `<slug>` | GET (Job Board API) |
| Lever | `<slug>` | GET (public postings JSON) |
| Ashby | `<slug>` | GET (public board) |
| Workday | `{host}/{site}` (e.g. `nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`) | POST (Workday CXS public `/jobs` endpoint) |

Greenhouse does not expose structured pay data on the public Job Board API for any board in the current watchlist; compensation appears only in description HTML. Lever is the structured compensation source.

Workday adapter v1 returns the listing-level fields only; per-posting description fetch (the CXS `/job/{id}` endpoint) is deferred to a v1 follow-up. Workday tenants gated behind `wday_vps_cookie` session cookies are unsupported in v1 and surface as fetch errors.

## The database as AI agent knowledge store

The Postgres DB is also intended to serve as a knowledge store for an AI agent layer. pgvector is load-bearing for this. Semantic search queries are raw SQL — vector ops don't map to ORM query builders.

## Non-goals (current scope)

- Scheduler (deferred)
- Next.js app UI (deferred)
- Embedding storage for classification summaries (deferred — pgvector columns not yet added; summary is report-only today; classification provenance schema is live in migration 000001)
- `skills[].requirement` persistence (deferred — writeback ignores the field today)
- Agent UI (deferred)
