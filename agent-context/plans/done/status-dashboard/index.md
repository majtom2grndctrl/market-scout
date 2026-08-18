# Status Dashboard

> Brief — decisions and non-goals. No task breakdown, no acceptance criteria.

## Goal

Give Dan a `/status` page showing what the system holds and whether it's healthy — data volume, how much of it is actually understood, and whether the fetcher is still running — so he can see the whole picture while scoping the "real" product pages. It's also the app's first navigation shell, typed-routing convention, and stat-tile/meter/status vocabulary, all of which every later page inherits.

## Target usage

```ts
// apps/web/lib/paths.ts — Route declared once, on the Record constraint passed
// to satisfies; every key is written once, in the object, and keeps its precise
// inferred type (unlike a `: Paths` annotation, which would widen it). Href
// construction only: no title field, ever.
import type { Route } from "next";

export const paths = {
  home: "/",
  status: "/status",
  company: (id: string) => `/company/${id}`,
} satisfies Record<string, Route | ((...args: any[]) => Route)>;

export type Paths = typeof paths;
```

```ts
// apps/web/lib/db/status.ts — one raw aggregate query. No new view or migration.
export async function getFetchHealth(): Promise<FetchHealthRow> {
  const sql = await getSql();
  const [row] = await sql<FetchHealthRow[]>`
    SELECT
      (SELECT count(*) FROM job_postings) AS jobs_harvested,
      (SELECT count(*) FROM companies) AS companies_tracked,
      (SELECT count(DISTINCT skill_id) FROM job_posting_skills) AS skills_identified,
      (SELECT count(DISTINCT job_posting_id) FROM classifications) AS jobs_classified,
      (SELECT count(*) FROM open_postings) AS jobs_open,
      (SELECT count(*) FILTER (WHERE status = 'success') FROM fetch_runs
        WHERE started_at > now() - interval '7 days') AS runs_succeeded_7d,
      (SELECT count(*) FILTER (WHERE status = 'failed') FROM fetch_runs
        WHERE started_at > now() - interval '7 days') AS runs_failed_7d,
      (SELECT max(started_at) FROM fetch_runs) AS last_run_at,
      (SELECT min(started_at) FROM fetch_runs) AS collecting_since
  `;
  return row;
}
```

```tsx
// apps/web/components/app-sidebar.tsx — nav list is separate from paths; paths carries no titles to generate from.
const navItems = [{ title: "Status", href: paths.status }];
```

## Metrics

Three groups, in this order top to bottom: Scale, Coverage, Pipeline health. Each is rendered with the form its data's job calls for (dataviz skill: a single current value is a **stat tile**; a ratio against a limit is a **meter**, not a 2-slice pie) — see Decisions for the form mapping and why Scale leads. Pipeline health carries no severity treatment at all; see Decisions.

**Scale** — stat tiles, one horizontal row.

| Label | Column | Definition |
|---|---|---|
| Jobs harvested | `jobs_harvested` | `count(*)` on `job_postings` — lifetime, one row per posting regardless of re-fetch count |
| Companies tracked | `companies_tracked` | `count(*)` on `companies` — no status/enabled column exists, so every row is on the fetch list |
| Skills identified | `skills_identified` | `count(distinct skill_id)` on `job_posting_skills` — skills actually assigned to a posting, not the full seed taxonomy |

**Coverage** — meters.

| Label | Column | Definition |
|---|---|---|
| Classification coverage | `jobs_classified` of `jobs_harvested` | share of `job_postings` with at least one row in `classifications` — how much of what's scraped has actually been understood |
| Currently open | `jobs_open` of `jobs_harvested` | share of `job_postings` present in the existing `open_postings` view right now — a churn signal, reusing a view rather than deriving a new one |

**Pipeline health** — plain figures and metadata, no severity.

| Label | Column | Definition |
|---|---|---|
| Run frequency | `runs_started_7d` | `runs_started_7d / 7` — every `fetch_runs` row started in the trailing 7 days, **including `in_progress`**. A run that started is a run. |
| Error rate (7d) | `runs_failed_7d` of `runs_succeeded_7d + runs_failed_7d` | share of *concluded* runs that failed. `in_progress` is excluded from the denominator so an in-flight run is never counted as a silent success. |
| Last run | `last_run_at` | `max(started_at)` on `fetch_runs` |
| Collecting since | `collecting_since` | `min(started_at)` on `fetch_runs` |

The two denominators differ deliberately: frequency asks how often the fetcher fires, error rate asks how often it finished badly. Each supporting line uses its own total.

## Decisions

- `companies_tracked` is `count(*)` on `companies` with no filter. The table has no enabled/status column (`000001_initial_schema.up.sql`), and `watchlist.md` confirms the project tracks no separate candidates pipeline — every row in `companies` is a company the fetcher runs against.
- `skills_identified` is `count(distinct skill_id)` on `job_posting_skills`, not `count(*)` on `skills`. `skills` is a closed, seed-only taxonomy (its migration comment: agents may not extend it at runtime); its size answers "how big is the taxonomy," not "what's shown up in real postings."
- Coverage and pipeline-health metrics were added deliberately, not just scale. Raw volume alone can't say whether the data is useful or the fetcher still works; classification coverage and run success answer those two questions respectively, using data the schema already has (`classifications`, `open_postings`, `fetch_runs.status`).
- Group order is Scale → Coverage → Pipeline health, and Scale is not a preamble to "the real status" — it's part of it. Different adopters of this tool will have different fetch lists and different skill coverage; the scale numbers describe what makes a given dataset distinctive, not just how big it is. Scale, Coverage, and Pipeline health are three facets of one status, ordered by how directly each answers "what do we have," not by importance.
- Stat tiles render as one horizontal row, not a stacked column — true on desktop by default. Try to hold that on mobile too (a tight three-up grid, or a horizontal scroll if the labels don't fit at that width) before falling back to a vertical stack. Which one works is a look-at-it call, not a brief-level decision — verify at build time on an actual narrow viewport.
- Raw query in `lib/db/status.ts`, no new SQL view or migration. The project's read-model views exist to keep one decision consistent across two consumers — the web app and the agent's MCP query gateway (`open_postings_display`, see `project.md` §Settled architecture). This page has one consumer, and its metric definitions are expected to move once the real product pages clarify what matters. A migration is the wrong weight for a number that might be redefined next week.
- Left nav via shadcn's `Sidebar` (`npx shadcn@latest add sidebar`), added to `apps/web/components/ui/`. Desktop rail plus mobile off-canvas sheet ships with the component — `globals.css` already defines the `--sidebar-*` color slots it expects, so no theme work comes first.
- `paths.ts` is a plain object typed via `satisfies`, not an array and not a separately declared type: string literals for static routes, functions for dynamic ones. No `title` field. Nav items are a separate hand-authored list pairing a title with a `paths.*` value, because `paths` has nothing to generate a label from.
- `typedRoutes: true` in `next.config.ts`; `Route` (`import type { Route } from "next"`) is written once, in the `satisfies` constraint. Every `paths` entry is checked against it by assignment — a renamed or removed route fails to compile inside `paths.ts` itself, not silently at every call site.
- Visual form follows the data's job, per the dataviz skill: Scale → stat tile (label + semibold auto-compact value, no delta/trend — see Not doing). Coverage → meter (accent-ramp fill over a lighter-step track, direct-labeled with the percentage) — explicitly not a 2-slice pie, which the skill calls out as the wrong form for a single ratio. Pipeline health → plain label/value rows, each figure with its exact counts beneath it.
- Pipeline health carries no severity ramp, no verdict word, and no status color. An earlier draft graded run success as Healthy / Degraded / Failing at 95% and 80%, and those two numbers were invented — nothing in the schema or the project defines what an acceptable error rate is. Stating "3%, 8 of 238 runs failed" tells Dan what happened; grading it tells him what an agent guessed. Revisit only when someone can name the threshold and say where it came from.
- Run frequency is runs per day, not per-company refresh cadence. Cadence (median gap between successive runs for one company) is the better freshness answer and needs a window function; runs-per-day is what this page ships. Reconsider when freshness-per-company actually gets asked.
- Full styled build — Tailwind + shadcn, following the `dataviz` skill for every figure. This page sets the visual bar the next agent-built page should clear; `/postings` stays unstyled until its own brief.

## Not doing

- A new SQL view or migration for these metrics — see Decisions. Revisit only if a second consumer (the agent, another page) needs the same numbers.
- Grading any figure on this page — no Healthy/Degraded/Failing, no thresholds, no severity color. See Decisions. This also means no `--status-*` color slot: `--destructive` and the `--chart-*` ramp are the whole palette.
- Stat-tile delta or sparkline. The dataviz contract supports both, but they need a value from a prior period, and nothing here queries snapshots-over-time — that's a historical-rollup feature, not a formatting choice. Add it only if a consumer actually needs trend.
- Distinct ATS platforms represented — already covered in `watchlist.md`; this page would just be repeating documentation as a number.
- Updating `web-guide.md` or writing a frontend-practices doc. Dan wants the page built first and the write-up done once the pattern (raw-query-vs-view split, sidebar, `paths`, the stat-tile/meter/status split) has proven out — this brief is what that write-up will point back to.
- A `routes` object alongside `paths`. That concept predates App Router; file-based routing already owns "which component serves which path."
- Generating the nav list from `paths` — `paths` carries no titles to generate from.
- Restyling `/postings` — out of scope; unchanged until it gets its own brief.
- A materialized view or refresh job for these counts. `open_postings_display` benchmarked sub-50ms on 61k rows in `web-data-layer`; a live query is cheap enough here too, and a materialized view would show a stale fetch count on the one page whose job is proving freshness.

## Build order

1. `npx shadcn@latest add sidebar` from `apps/web/`; review the vendored output in `components/ui/sidebar.tsx`.
2. Add `apps/web/lib/paths.ts`; set `typedRoutes: true` in `next.config.ts`.
3. ~~Status color tokens.~~ Dropped — Pipeline health carries no severity, so there is nothing to color. No `--status-*` slot exists.
4. Add `apps/web/lib/db/status.ts` with `getFetchHealth()`, plus a `selectFetchHealth(sql)` seam so the db test can run the shipped query directly — `getSql()` calls `next/server`'s `connection()`, which throws outside a request scope.
5. Add `apps/web/components/app-sidebar.tsx` (nav list); wire `SidebarProvider` / `AppSidebar` / `SidebarInset` / `SidebarTrigger` into `app/layout.tsx`.
6. Add `apps/web/app/status/page.tsx` — Server Component calling `getFetchHealth()`, rendered as three stat tiles, two meters, and four plain pipeline rows.
7. Add `apps/web/lib/db/status.db.test.ts` asserting the non-obvious definitions against seeded fixtures: a twice-fetched posting counts once in `jobs_harvested`; a skill assigned via `job_posting_skills` counts once regardless of how many classifications carry it; a posting with two classifications still counts once in `jobs_classified`; `runs_succeeded_7d`/`runs_failed_7d` split correctly by `fetch_runs.status`; and `runs_started_7d` exceeds their sum whenever a run is `in_progress` — the property that breaks if someone folds the frequency subquery into a sum of the other two. Fixtures run inside a rolled-back `repeatable read` transaction rather than the marker-and-delete pattern in `read-model-views.db.test.ts`: these are global aggregates, and Vitest runs test files in parallel, so a sibling suite's teardown lands mid-measurement and produces negative deltas.

## Done when

- `pnpm typecheck`, `pnpm build`, and `pnpm test` pass in `apps/web`.
- `pnpm test:db` passes with both database DSNs set.
- `/status` shows Scale as one horizontal row of three stat tiles, then Coverage's two meters, then Pipeline health's four plain rows — all with real numbers, none of them graded. The sidebar sits pinned on desktop and collapses to an off-canvas drawer under the mobile breakpoint. The stat-tile row stays legibly horizontal at mobile width, or falls back to a vertical stack if it can't.
