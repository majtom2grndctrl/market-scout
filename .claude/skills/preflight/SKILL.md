---
name: preflight
description: >
  Runs pre-commit quality checks for every surface a change touches — Go
  (`apps/tools/`) and web (`apps/web/`). Reports pass/fail per check and
  auto-fixes mechanical issues. Use before committing or pushing changes, or
  before opening a pull request.
disable-model-invocation: true
---

# Preflight

Run quality checks and report results. Fix mechanical issues automatically; escalate design decisions.

## Surfaces

A change's surface is the app directory it touched. Detect from the working tree, then run every gate that applies — both, when a change spans both.

!`git status --porcelain 2>/dev/null | head -40`

| Touched | Surface | Gate |
|---|---|---|
| `apps/tools/**` | `tools` | §Tools gate |
| `apps/web/**` | `web` | §Web gate |
| Neither | none | Report "no code surface touched" and stop |

Docs-only changes have no gate. Say so rather than running one.

## Tools gate

From `apps/tools/`, run in parallel:

1. **Format:** `gofmt -l .` — empty output is a pass
2. **Build + vet:** `go build ./... && go vet ./...`
3. **Lint:** `staticcheck ./...` — falls back to `~/go/bin/staticcheck ./...` when not on PATH
4. **Test:** `go test ./...`
5. **Generated code:** `sqlc diff` — committed sqlc output matches the SQL sources

## Web gate

From `apps/web/`, run in order. Fast checks first, so a type error fails before a two-minute build:

1. **Typecheck:** `pnpm typecheck` — `tsc --noEmit`, covers `.storybook/` too
2. **Test:** `pnpm test` — DB-free Vitest suite
3. **View tests:** `pnpm test:db` — requires `DATABASE_URL` and `DATABASE_URL_RO`. Report as skipped, never as passed, when they are unset
4. **Stories:** `pnpm build-storybook` — the only check that every story compiles
5. **Build:** `pnpm build` — `next build`

No formatter runs on `apps/web`. None is configured, and preflight does not introduce one.

Accessibility is not gated here. `@storybook/addon-a11y` reports inside the Storybook UI only; headless assertions need `@storybook/addon-vitest`, which needs the `@storybook/nextjs-vite` migration that `web-data-layer` placed out of scope. Until that lands, a11y is a review-gate concern.

## Reporting

Report each check as ✓ pass or ✗ fail, grouped by surface. For failures, include the relevant output.

```
Preflight results — surfaces: tools, web

  tools
    ✓ gofmt
    ✓ go build + go vet
    ✗ staticcheck — 1 issue (see below)
    ✓ go test
    ✓ sqlc diff

  web
    ✓ pnpm typecheck
    ✓ pnpm test
    – pnpm test:db — skipped, DATABASE_URL_RO unset
    ✓ pnpm build-storybook
    ✓ pnpm build
```

## Auto-fix policy

**Both surfaces**

- **Test failures:** never auto-fix. Report the failure with enough context to diagnose.
- **Anything requiring a design choice or a behavior change:** report and let the user decide.

**Tools**

- **Format:** run `gofmt -w` on the listed files; report what changed.
- **Vet / staticcheck:** fix if mechanical (unused import, redundant conversion, unchecked error with an obvious handling site).
- **sqlc drift:** run `sqlc generate`; the regenerated diff is the fix. Report it for commit alongside the SQL change (developer-guide §5.8).

**Web**

- **Type errors:** fix if mechanical (a missing or moved import, an obvious narrowing). Escalate anything that changes a component's contract.
- **Storybook build failure:** usually a story importing a path that moved. Mechanical — fix and report.
- **Silently dropped Tailwind class:** a custom utility inside a color namespace is removed by `cn()` with no error. The fix is renaming it out of `text-`, `bg-`, `border-`, and `fill-` (web-guide §Color namespaces).

After auto-fixing, re-run the fixed checks to confirm they pass.
