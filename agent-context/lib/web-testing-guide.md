# Web Testing Guide

> **Read this when:** writing or changing tests, routes, interactive components, or stories under `apps/web/`.
> **Key invariant:** the DB-free suite is the default gate. Database tests prove a read model only when both roles run them.
> **Related:** [Web App Guide](./web-guide.md) · [Testing Guide](./testing-guide.md) · [Project Overview](./project.md)

---

This guide is separate because web tests own route states, interactive accessibility, Storybook, and two-role Postgres verification. The Go guide supplies shared principles: test behavior over implementation, exercise real seams, and avoid framework internals.

## Test Layers

| Layer | Owner | Purpose | Command |
|---|---|---|---|
| Unit and composition | Vitest | Pure formatting, parsing, vocabulary, and result-shaping behavior | `pnpm test` |
| Component interaction | Vitest | Rendered behavior, control state, and events that can regress without a browser route | `pnpm test` |
| Route state | Route owner | Loading, empty, partial-data, error, and retry behavior in the rendered app | Review in `pnpm dev`; add focused tests when logic has a stable seam |
| Storybook and a11y | Component owner | Reusable visual and interactive states; theme variants; addon-detected accessibility issues | `pnpm build-storybook`; inspect with `pnpm storybook` |
| Read-model integration | Vitest + Postgres | Views and query contracts, executed through the real application role | `pnpm test:db` |

`*.test.ts` and `*.test.tsx` are DB-free. `*.db.test.ts` runs only in DB mode. Keep them separate: the default suite must never create a connection or need credentials.

## Route and Interactive Coverage

Each data-backed route owns its visible states. Cover a successful result, an empty result, loading when data can suspend, and an error with a useful retry path. Partial, stale, unmapped, or unavailable data must be shown honestly; do not render an apparent zero or complete answer when the data does not support it.

Each interactive control needs keyboard and focus review in the rendered app. Use native semantics and labels first. A Storybook a11y result is a useful screen, not proof: verify focus order, visible focus, keyboard operation, and the action that follows a retry or state change.

Stories live beside reusable components under `components/**`. Add stories for meaningful variants and interactive states, including relevant light and dark themes. `pnpm build-storybook` is the compilation gate; use the a11y addon during interactive review.

## Database Tests

Database tests run against a dedicated database, `market_scout_test`, and require both `DATABASE_URL_TEST` and `DATABASE_URL_TEST_RO`. Provisioning steps live in [Developer Guide](./developer-guide.md) §2.

- `DATABASE_URL_TEST` is the owner role. It creates uniquely marked fixture data and cleans it up, or rolls it back in a transaction.
- `DATABASE_URL_TEST_RO` is the application role. It executes the query under test. Do not substitute the owner DSN; the grant boundary is part of the contract.
- Pass a SQL client into the query function. The route wrapper gets the application's client, while the test supplies the read-only client.
- Isolate fixture rows with a unique marker. The test database persists across runs and suites, so assertions must not depend on absolute totals or another test's rows.

Grant coverage here runs one way. Every assertion fails on a privilege the application role is missing — a table or view it cannot select, or the one granted function a suite calls — and none fails on a privilege it should not have. A grant that got wider leaves every result identical. Read a green run as "nothing the read model needs was dropped," not as proof the boundary is tight; the boundary lives in `readonly_role.sql`, and the parity check in [Developer Guide](./developer-guide.md) §2 is what reads it.

Both DSNs resolve through `lib/db/test-dsn.ts`. **There is no fallback to `DATABASE_URL` or `DATABASE_URL_RO`. Never add one.** The status and postings suites date their fixtures from `now()`, so against the development database they land inside the live `now() - interval` windows the read model queries, and a fixture fetch run poisons the `max(started_at)` the status read returns. Teardown is best-effort and has failed more than once. Every fixture company that survives becomes a permanent fetcher target: the fetch list filters on `ats IS NOT NULL`, and migration `000002` made `companies.ats` NOT NULL, so that predicate excludes no company at all. A separate database contains all of that no matter how teardown goes; a fallback DSN reopens it.

The resolver enforces that rather than trusting it. A test DSN must name a database whose name ends in `_test`; one that names anything else — the development database, most likely, since the two variables differ by one word — throws immediately, naming the database it found. The suffix is the contract, so a per-worktree or CI test database needs no code change. Without the guard the invariant rests on a string typed into `.env.local`, and every assertion is marker-scoped or delta-based, so a wrong DSN runs green.

Run from `apps/web/`:

```bash
pnpm test:db
```

With either test DSN unset, individual DB tests call `context.skip()` — they never fall back to the development database. Vitest may exit successfully, but those tests are **skipped**, not passed and not evidence that the view or grants work. Report that distinction explicitly.

## Running Tests

Run from `apps/web/`:

```bash
pnpm test                 # DB-free suite
pnpm typecheck            # Type gate; Vitest does not typecheck
pnpm build-storybook      # Compiles all stories
pnpm build                # Production route build
pnpm preflight            # Canonical DB-free gate: all four commands above
pnpm test:db              # Optional two-role view integration suite
```

Run `pnpm preflight` before handoff. Add `pnpm test:db` after changing a view query, its mapped data contract, or a read-only grant, when both test DSNs are set. A pass means nothing the read model needs broke — not that the grant is narrow.

## Non-Goals

- Browser automation for every route. Use it when a behavior cannot be verified at a smaller seam.
- Snapshotting implementation markup or Tailwind class lists.
- Treating skipped DB tests as a passing integration suite.
