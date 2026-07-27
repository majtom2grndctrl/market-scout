---
name: preflight
description: >
  Runs pre-commit quality checks: gofmt, build + vet, staticcheck, tests, and
  sqlc freshness. Reports pass/fail per check and auto-fixes mechanical
  issues. Use before committing or pushing changes, or before opening a pull
  request.
disable-model-invocation: true
---

# Preflight

Run quality checks and report results. Fix mechanical issues automatically; escalate design decisions.

## Checks

From `apps/tools/`, run in parallel:

1. **Format:** `gofmt -l .` — empty output is a pass
2. **Build + vet:** `go build ./... && go vet ./...`
3. **Lint:** `staticcheck ./...` — falls back to `~/go/bin/staticcheck ./...` when not on PATH
4. **Test:** `go test ./...`
5. **Generated code:** `sqlc diff` — committed sqlc output matches the SQL sources

## Reporting

Report each check as ✓ pass or ✗ fail. For failures, include the relevant output.

```
Preflight results:
  ✓ gofmt
  ✓ go build + go vet
  ✗ staticcheck — 1 issue (see below)
  ✓ go test
  ✓ sqlc diff
```

## Auto-fix policy

- **Format:** run `gofmt -w` on the listed files; report what changed.
- **Vet / staticcheck:** fix if mechanical (unused import, redundant conversion, unchecked error with an obvious handling site). If the fix involves a design choice or changes behavior, report and let the user decide.
- **sqlc drift:** run `sqlc generate`; the regenerated diff is the fix. Report it for commit alongside the SQL change (developer-guide §5.8).
- **Test failures:** never auto-fix. Report the failure with enough context to diagnose.

After auto-fixing, re-run the fixed checks to confirm they pass.
