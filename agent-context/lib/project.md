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

1. Developing credible backend skills as a generalist.
2. Actionable market intel from day one, compounding into trend analysis over weeks and months.

Not a product to sell. A personal tool that doubles as a portfolio piece and learning vehicle.

## Repo layout

`apps/` houses deployable units. The Go module (binaries and shared packages) lives at `apps/tools/`; the module path is `github.com/majtom2grndctrl/market-scout/apps/tools`. The Next.js app lives at `apps/web/` as a sibling. Shared infra (`docker-compose.yml`, root `.env.local`) stays at repo root and serves both.

`research/` sits at repo root as a deliberate low-attention pocket — quasi-documentation that agents read only when explicitly directed. It is not under `agent-context/` (read by default) and not under `apps/` (deployable code). Tools reference its contents via CLI arg, never a hardcoded path.

## Settled architecture

| Layer | Choice | Notes |
|---|---|---|
| Fetcher | Go binary, `apps/tools/cmd/fetcher` | Concurrent HTTP from ATS APIs, cron-scheduled |
| Classifier | Go binary, `apps/tools/cmd/batch-enrich` | Cron-schedulable. Default runner calls subscription-authenticated `codex exec`; `claude -p` remains an explicit fallback. Go owns selection, batching, validation, serial sqlc writeback, provenance, and reports. No model SDK or direct REST. |
| Database | Postgres in Docker | Option to point at Supabase later |
| DB client | `sqlc` + `database/sql` + `pgx/v5/stdlib` | Write SQL, get generated type-safe Go. No ORM. Standard `database/sql` target (no `sql_package` override); generated models use `sql.NullString`/`sql.NullTime`, translated to `*string`/`*time.Time` at the DB boundary. |
| Vector search | pgvector extension | Enabled from day one. Similarity queries in raw SQL. |
| Storage model | Append-only snapshots | Every fetch writes timestamped rows. Never upsert. Load-bearing for trend analysis. |
| App layer | Next.js Server Components → Postgres direct | No separate Go API server and no route handlers. The `postgres` npm package on the read-only DSN. Server Actions arrive with the first write path. |
| Read model | SQL views in numbered migrations | Derived "current" state — open postings, latest snapshot, latest classification — defined once in SQL rather than per consumer. |
| UI stack | App Router, TypeScript, Tailwind v4, shadcn/ui on Base UI | Components are copied into the repo, not imported. `shadcn add` serves the Base UI variant, not Radix — set in `components.json` (`style: base-nova`, `tsx: true`). Geist font, Lucide icons, neutral base color. |
| Layout and type | Tailwind's own scales | Type and spacing are Tailwind's defaults; shadcn owns color and radius. The one `@theme` block we own adds container widths. See [`web-guide.md`](./web-guide.md). |
| Future | `init` CLI | Scaffolds new users' setups. Not in scope yet. |

Our own first-observed timestamp on a job-posting record is the load-bearing repost-detection signal. ATS-reported "first published" and "created at" timestamps refresh on repost on at least some boards, so they cannot be trusted as the primary signal. The append-only snapshot model combined with an immutable first-observed timestamp — set once on initial upsert, never overwritten — is what makes repost detection durable. Source-reported timestamps are captured on every snapshot as a secondary change-detection signal, not as the repost anchor.

Every fetch is recorded as a fetch-run row, one per company per invocation, capturing the outcome and timing of that attempt. New snapshots link back to the run that produced them; snapshots written before the fetch-run migration carry a NULL run reference. This separation is load-bearing for trend queries: a posting's absence from a fetch only counts as "removed" when the run for that company succeeded. A failed run, or a company we didn't fetch in a given window, is distinguishable from a posting that genuinely disappeared from the board. Without the run record, a network blip looks identical to a wave of closed roles.

Nothing in the schema records whether a posting is currently open. It is derived: a posting is open when it appeared in the most recent **successful** fetch run for its company. Absence from a failed or in-progress run means nothing, which is what makes the fetch-run record above load-bearing rather than incidental. That derivation lives in SQL views, not in each consumer, because both the Next.js app and the agent's MCP `query` tool ask the same question and a second copy would drift. Latest snapshot and latest classification are derived the same way, per posting, from append-only history.

The web app reads Postgres directly from Server Components. Next.js's server layer is the boundary; a route handler would add a second serialization hop inside the same process with no consumer on the other side. The agent does not read through the web app — it has the MCP server, whose read-only and action roles enforce a tighter privilege boundary than an HTTP API could.

When the first web-app write lands (user preferences, saved searches), it goes through Server Actions and plain table grants on a dedicated role. Not `SECURITY DEFINER` functions — the `mcp` schema uses those because the agent writes to core tables and the write *shape* needed constraining, whereas app-owned tables have one writer and no provenance requirement. Persisted query criteria are structured filter terms, never SQL text: stored SQL is an arbitrary-execution path and it rots silently when the views beneath it change.

Design tokens are hand-written CSS in a Tailwind `@theme` block, not a generated artifact. Tailwind already delivers tokens into utility classes, so a TypeScript-source-plus-codegen pipeline would only exist to keep prop types in sync with CSS — a problem the app does not have. Styling runs in two layers. Tailwind's scales are primitive tokens naming a size; custom utilities in `globals.css` are semantic tokens naming a purpose, composed from those primitives. A semantic name earns its place even when it duplicates a primitive's value, because intent survives a value change and a size name does not. Naming is constrained, though: shadcn's `cn()` runs tailwind-merge at every call site, and tailwind-merge drops a name it misreads. See [`web-guide.md`](./web-guide.md).

Classification runs on its own cadence. The classifier selects unclassified postings, strips per-company boilerplate from each description, and dispatches model CLI subprocesses in parallel waves. Each subprocess returns a structured candidate; Go validates and retries failures. Codex runs in an isolated read-only working directory with structured output. Dispatch is parallel; writeback is serial — one transaction at a time through the sqlc query layer. Serial writeback is load-bearing: parallel writers would collide on newly-introduced taxonomy rows mid-wave and produce duplicates. Between waves, the classifier reloads the taxonomy from the DB so roles introduced by earlier waves are visible to later ones. An append-only `failures.jsonl` log is the cron observability surface; without it, failure history vanishes between runs.

## ATS targets

Five adapters are live: Greenhouse, Lever, Ashby, Workday, and Workable. Each ATS is a separate file in `apps/tools/internal/ats`; all adapters return `domain.Posting` from `apps/tools/internal/domain`.

| Platform | `board_token` format | API type |
|---|---|---|
| Greenhouse | `<slug>` | GET (Job Board API) |
| Lever | `<slug>` | GET (public postings JSON) |
| Ashby | `<slug>` | GET (public board) |
| Workday | `{host}/{site}` (e.g. `nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite`) | POST (Workday CXS public `/jobs` endpoint) |
| Workable | `<slug>` (lowercase slug from `apply.workable.com/<slug>`) | GET (public widget endpoint) |

Greenhouse does not expose structured pay data on the public Job Board API for any board in the current watchlist; compensation appears only in description HTML. Lever is the structured compensation source.

Workday adapter v1 returns the listing-level fields only; per-posting description fetch (the CXS `/job/{id}` endpoint) is deferred to a v1 follow-up. Workday tenants gated behind `wday_vps_cookie` session cookies are unsupported in v1 and surface as fetch errors.

Workable adapter v1 returns the listing-level fields only; the widget endpoint is summary-only and per-posting description fetch (via the `/spi/v3/jobs/{shortcode}` endpoint or HTML scrape of the apply page) is deferred. Workable postings carry NULL `description_text` and will not classify until that follow-up ships.

## The database as AI agent knowledge store

The Postgres DB is also intended to serve as a knowledge store for an AI agent layer. pgvector is load-bearing for this. Semantic search queries are raw SQL — vector ops don't map to ORM query builders.

## Evidence trust tiers

Agent-written records carry signals of three trust tiers. Auditing any agent-written row starts with the tier of each signal.

| Tier | Meaning | Example |
|---|---|---|
| Tool-attested | A tool observed it directly. Ground truth. | `add_company` probe result |
| Deterministic | Computed from agent-supplied evidence. Reproducible — but no more trustworthy than its inputs. | `dedup_candidates`, `detect_ats` verdicts |
| Agent-asserted | The agent says so. | Browser-observed URLs, notes |

Records that mix tiers keep them distinguishable — never collapse a probe result and an agent claim into one field. Unattended runs are audited by walking each signal down this gradient.

## Non-goals (current scope)

- Scheduler (deferred)
- Next.js product screens (deferred — the scaffold, shadcn/ui, and Storybook are in place; no product UI built yet. Conventions: [`web-guide.md`](./web-guide.md))
- Embedding storage for classification summaries (deferred — pgvector columns not yet added; summary is report-only today; classification provenance schema is live in migration 000001)
- `skills[].requirement` persistence (deferred — writeback ignores the field today)
- Agent UI (deferred)
