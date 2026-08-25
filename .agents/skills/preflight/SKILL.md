---
name: preflight
description: Run the applicable quality gates for changed Go and web surfaces, fixing only mechanical issues. Use before committing, pushing, or opening a pull request.
---

# Preflight

Detect changed code surfaces from the working tree. Run every applicable gate. Docs-only changes have no code gate; report that and stop.

## Tools gate

Run from `apps/tools/`:

1. `gofmt -l .`
2. `go build ./... && go vet ./...`
3. `staticcheck ./...` — use `~/go/bin/staticcheck` if needed.
4. `go test ./...`
5. `sqlc diff` when SQL sources or generated database output changed.

## Web gate

Run from `apps/web/`, in this order:

1. `pnpm typecheck`
2. `pnpm test`
3. `pnpm test:db` — skip when `DATABASE_URL` or `DATABASE_URL_RO` is unset.
4. `pnpm build-storybook`
5. `pnpm build`

No web formatter is configured. Do not introduce one as part of preflight.

## Auto-fix policy

- Fix formatting, unused imports, obvious moved imports, and sqlc drift. Re-run the affected gate.
- Do not auto-fix test failures, behavior changes, component-contract changes, or design choices.
- For Tailwind classes silently discarded by `cn()`, rename a custom utility out of `text-`, `bg-`, `border-`, and `fill-` namespaces.

## Report

Report pass, fail, or skipped status for every selected check, grouped by surface. Include relevant failure output and every automatic change.
