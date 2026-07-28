# Market Scout

Local-first market intelligence for job postings. Tracks hiring across a list of
companies you choose, and turns the history into role trends, hiring patterns,
and market dynamics over time.

Think "personal Crunchbase for hiring data, scoped to a niche you care about."

> **AI agents:** start at [`agent-context/lib/index.md`](agent-context/lib/index.md).
> It routes you to the minimal set of docs for your task. Don't read this README
> for orientation — it's written for humans browsing GitHub.

---

## Why it exists

Public job boards tell you what a company is hiring for *today*. They don't tell
you what changed. Postings appear, get reposted, quietly disappear — and none of
that is visible unless someone is watching over time.

Market Scout watches. It fetches job boards on a schedule, records every fetch as
a timestamped snapshot, and never overwrites what it saw before. The history is
the product.

It's a personal tool, not a service. You run it on your own machine, against your
own company list.

## Status

Working today:

- **Fetcher** — pulls postings from five ATS platforms, concurrent, cron-friendly
- **Classifier** — enriches postings into canonical roles, specializations, and skills
- **MCP server** — read-only database access for AI agents, plus onboarding tools
- **111 seeded companies** across the five supported platforms

Not built yet:

- **Web UI** — the Next.js app is scaffolded with the design system in place, but
  no product screens exist
- **Scheduler** — the binaries are cron-ready; there's no scheduler bundled

## How it works

```
ATS APIs  →  fetcher  →  Postgres (append-only snapshots)  →  classifier
                                    ↓
                            Next.js app / MCP agents
```

A few decisions shape everything else:

**Snapshots are append-only.** Every fetch writes new timestamped rows. Nothing is
ever updated in place. A posting that vanishes isn't deleted — it just stops
getting new rows, and the gap between last-seen and now is exactly what trend
queries read as "closed."

**Absence needs context.** Every fetch is recorded as a run, one per company per
invocation. A posting missing from a fetch only counts as removed if that
company's run actually succeeded. Without this, a network blip looks identical to
a wave of closed roles.

**First-observed beats source timestamps.** Some boards refresh their "first
published" date on repost, so it can't anchor repost detection. Market Scout's own
first-observed timestamp is set once and never overwritten.

**No ORM.** SQL lives in `.sql` files; [sqlc](https://sqlc.dev) generates type-safe
Go from it. Vector search runs as raw SQL against pgvector.

**No API server.** The Next.js app talks to Postgres directly.

## Supported job boards

| Platform | `board_token` format | Notes |
|---|---|---|
| Greenhouse | `<slug>` | Compensation only in description HTML |
| Lever | `<slug>` | Structured compensation. **Tokens are case-sensitive** |
| Ashby | `<slug>` | |
| Workday | `{host}/{site}` | Listing fields only; public boards only |
| Workable | `<slug>` | Listing fields only; descriptions not yet fetched |

Workday and Workable adapters return listing-level fields in v1. Workable postings
carry no description text, so they don't classify yet.

## Quickstart

Requires Go 1.26+ (per `go.mod`) and Docker. The web app additionally needs Node
and pnpm; the repo doesn't pin a Node version.

```bash
git clone <your-fork-url> market-scout
cd market-scout

cp .env.example .env.local          # then fill in real credentials
docker compose up -d                # Postgres + pgvector

ln -s ../../.env.local apps/tools/.env.local

cd apps/tools
go run ./cmd/migrate up             # apply schema migrations
go run ./cmd/fetcher                # one-shot fetch across seeded companies
```

Go commands run from `apps/tools/`, not the repo root — the module root and a
couple of path literals assume it. `docker compose` runs from the repo root.

Full setup, including the read-only role the MCP server requires, is in
[`agent-context/lib/developer-guide.md`](agent-context/lib/developer-guide.md).

## Repo layout

```
apps/tools/         Go module — fetcher, classifier, MCP server, migrations
  cmd/              binaries: fetcher, batch-enrich, mcp, migrate, onboard,
                    strip-boilerplate
  internal/ats/     one adapter per ATS platform (HTTP only, no DB access)
  internal/db/      sqlc output + queries + migrations  (generated — don't hand-edit)
apps/web/           Next.js app (App Router, Tailwind v4, shadcn/ui on Base UI)
agent-context/      durable architecture docs and plans — the agent entry point
research/           low-attention research notes, read only when pointed at
```

## Making it yours

The seeded company list reflects one person's niche. To track your own market,
edit `apps/tools/internal/db/seeds/companies.sql` — each row is a display name,
an ATS platform, that company's board token, and an industry bucket.

The `onboard` binary and the MCP server's `add_company` tool both verify a board
token against the live ATS before writing it, so a typo fails loudly instead of
producing a company that silently never fetches.

## License

Licensed under the **GNU Affero General Public License v3.0**. See [LICENSE](LICENSE).

AGPL rather than GPL because this is a hosted application. Running software as a
network service isn't "distribution," so GPLv3 would let a hosted fork stay closed.
AGPL §13 closes that gap: if you run a modified version and let others use it over
a network, you have to offer them the source.

You're free to use, modify, and share this — commercially included. You just can't
make your changes proprietary.

Copyright (C) 2026 Dan Hiester
