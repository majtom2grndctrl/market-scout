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
- **Colour tokens — which family to reach for, why trend is not status, series slot order** → `agent-context/lib/web-guide.md` §Colour
- **Changing a colour, adding a token, `shadcn add` output that renders unstyled** → `agent-context/lib/web-guide.md` §Colour *(`app/theme.css` is generated; edit `scripts/theme/tokens.mjs`)*
- **Storybook stories — where they live, why the font decorator exists** → `agent-context/lib/web-guide.md` §Storybook
- **Build, typecheck, and dev commands** → `agent-context/lib/web-guide.md` §Commands
- **Web tests, route states, Storybook a11y, or DB view tests** → `agent-context/lib/web-testing-guide.md`
- **Sketching a page or chart against live data before it earns a spec** → `agent-context/lib/web-guide.md` §Prototypes *(`app/prototypes/` — a low-attention pocket; read it only when directed there)*
- **Querying Postgres from the web app; what "currently open posting" means** → `agent-context/lib/project.md` §Settled architecture *(derived in SQL views, not per consumer)*
- **Posting count vs requisition count; why a measure omits a number instead of returning 0** → `agent-context/lib/project.md` §Settled architecture *(both counts are honest; absent is not zero)*
- **Charts — which library, why every mark is hand-rendered** → `agent-context/lib/project.md` §Settled architecture *(d3 supplies scales and geometry; no chart component library)*
- **Geography / `market` dimension, why location is curated not raw** → `agent-context/lib/project.md` §Settled architecture *(curated market dictionary; `unmapped` is an explicit value; no coverage denominator)*
- **Profile, résumé onboarding, pinned roles, title heads as labels** → `agent-context/lib/project.md` §Settled architecture *(pins are canonical roles; extraction is reviewed before save)*
- **Dashboard widgets — how they are chosen, where models may enter** → `agent-context/lib/project.md` §Settled architecture *(detectors propose, ranker chooses; models narrate, never compute)*
- **Chat surface, streaming responses, when a route handler is allowed** → `agent-context/lib/project.md` §Settled architecture *(streaming is the one exception; request-response stays in Server Components)*

**Backend (`apps/tools/`)**

- **Dev setup, Go conventions, logging, comments** → `agent-context/lib/developer-guide.md`
- **Working directory for Go commands, `.env.local` symlink** → `agent-context/lib/developer-guide.md` §2 Development Setup
- **Test strategy, patterns, running tests** → `agent-context/lib/testing-guide.md` *(Go only)*
- **Go fetcher (structure, conventions)** → `agent-context/lib/project.md` §Settled architecture
- **ATS adapter (adding or modifying)** → `agent-context/lib/project.md` §ATS targets
- **Which platforms distinguish a job from a job post; requisition identifiers** → `agent-context/lib/project.md` §ATS targets *(four of six; Gem boards are Greenhouse-shaped)*
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
- **Generated files — never hand-edit** → `agent-context/lib/developer-guide.md` §5.8 *(sqlc)* · `agent-context/lib/web-guide.md` §Colour *(`apps/web/app/theme.css`)*
- **When to skip the plan pipeline; which route — build session, one-pager, brief, or spec** → `agent-context/lib/developer-guide.md` §1.5 *(the light lane; routes)*
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
| App layer | Next.js Server Components → Postgres direct | No separate Go API server. `postgres` on the read-only DSN. Route handlers reserved for streaming. |
| Charts | Hand-rendered SVG on d3 modules | d3 supplies scales and path geometry. No chart component library; marks are app-owned so charts inherit design tokens. |
| Colour | Generated semantic tokens, `apps/web/app/theme.css` | Nine families over one calibrated OKLCH palette. Trend (blue/orange) is deliberately separate from status (green/amber/red): a falling posting count is not a failure. Accent is achromatic so hue stays reserved for data. |
| Primary surface | Profile → pinned roles → adaptive dashboard | Pins are canonical roles; title heads label them. Chat comes later, for unanticipated questions. |
| Read model | SQL views in numbered migrations | "Open posting" and other derived state defined once in SQL, read by both the web app and the agent. |
| ATS adapters | Interface in `apps/tools/cmd/fetcher`; implementations in `apps/tools/internal/ats/` | All adapters implement the same `FetchPostings` contract. |
