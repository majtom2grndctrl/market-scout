---
name: preflight
description: Run the applicable quality gates for changed Go and web surfaces, fixing only mechanical issues. Use before committing, pushing, or opening a pull request.
---

# Preflight

Detect changed code surfaces from the working tree. Run every applicable gate. A batch-enrich `SKILL.md` is a surface on its own. Docs-only changes have no code gate; report that and stop.

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
3. `pnpm test:db` — skip when `DATABASE_URL_TEST` or `DATABASE_URL_TEST_RO` is unset.
4. `pnpm build-storybook`
5. `pnpm build`

No web formatter is configured. Do not introduce one as part of preflight.

## Batch-enrich pin gate

Runs when the change touches `.agents/skills/batch-enrich/SKILL.md` or `.claude/skills/batch-enrich/SKILL.md`. The `PROMPT_VERSION` pin in each file's `classification-pins` block is the only thing that lets a later query separate classification cohorts. It sat unchanged for four months in 2026 while five materially different prompts shipped under it; those cohorts are unrecoverable.

1. **Pin moved with the change.** Diff each touched file; fail when the diff contains no `PROMPT_VERSION=` line. The check cannot tell a classification-affecting edit from a cosmetic one, so every unbumped change is suspect.
2. **Pins agree.** Both files must carry the same `PROMPT_VERSION` value. One classifier contract, two harnesses, one pin value.
3. **No restated literal.** `grep 'batch-enrich-v[0-9]'` in either file outside its `PROMPT_VERSION=` line. A version restated in an example or a SQL snippet is how workers wrote `batch-enrich-v6` under a `batch-enrich-v7` pin. Prose naming a past version as history is fine; anything a worker would copy into a `save_enrichment` call is not.

Never auto-fix any of the three. Report and let the committer decide the next version string.

## Auto-fix policy

- Fix formatting, unused imports, obvious moved imports, and sqlc drift. Re-run the affected gate.
- Do not auto-fix test failures, behavior changes, component-contract changes, or design choices.
- For Tailwind classes silently discarded by `cn()`, rename a custom utility out of `text-`, `bg-`, `border-`, and `fill-` namespaces.

## Report

Report pass, fail, or skipped status for every selected check, grouped by surface. Include relevant failure output and every automatic change.
