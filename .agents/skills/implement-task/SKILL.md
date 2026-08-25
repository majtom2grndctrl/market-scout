---
name: implement-task
description: Implement one focused plan task or ad-hoc change with source-grounded verification. Use for a specific task from an in-progress plan or a bounded implementation request.
---

# Implement Task

Implement one task. Do not commit unless the user explicitly asks.

## Load context

Read `agent-context/lib/index.md`, then load only the guides for the affected surface.

| Surface | Guides |
|---|---|
| `apps/tools/` | `developer-guide.md`, `testing-guide.md` |
| `apps/web/` | `web-guide.md` and its named developer-guide sections |
| Both | Both sets |

For a plan task, read its Goal, your task section, and the plan-level acceptance criteria. For an ad-hoc task, treat the request as the specification. Ask only when the acceptance criteria are materially ambiguous.

## Implement

1. Open code and tests at every changed seam. Confirm earlier-phase output exists before consuming it.
2. Deliver the stated impact. Handle in-scope errors and edge cases without expanding scope.
3. Derive every identifier claim from source opened this session.
4. Never hand-edit sqlc output. Change SQL or migrations, then run `sqlc generate` from `apps/tools/`.
5. Run only free verification commands. Do not run fetcher, batch-enrich, destructive migrations, or other live surfaces as a test.

## Verify

Run the narrow check while iterating. Before reporting, run the full checks for every touched surface.

| Surface | Narrow loop | Before reporting |
|---|---|---|
| Tools | `go build ./...`, focused `go test` | `gofmt -l .`, `go test ./...` |
| Web | `pnpm typecheck`, focused `pnpm test` | `pnpm typecheck`, `pnpm test`, `pnpm build-storybook`, `pnpm build` |

Run commands from the app directory. Report database view tests as skipped, not passed, when their DSNs are unavailable.

## Report

- Status for each acceptance criterion: met, not met, or deviated.
- Verification commands and their actual results.
- Deviations, assumptions, and unverified risks.
- Files changed.
- For web work, routes and stories a reviewer should inspect.
