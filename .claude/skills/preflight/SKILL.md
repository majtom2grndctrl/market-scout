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
| `.claude/skills/batch-enrich/SKILL.md` or `.agents/skills/batch-enrich/SKILL.md` | `batch-enrich` | §Batch-enrich pin gate |
| Neither | none | Report "no code surface touched" and stop |

`batch-enrich` is a surface on its own: touching either file still runs its gate, and touching one alongside `apps/tools/**` or `apps/web/**` runs all gates that apply.

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
3. **View tests:** `pnpm test:db` — requires `DATABASE_URL_TEST` and `DATABASE_URL_TEST_RO`. Report as skipped, never as passed, when they are unset
4. **Stories:** `pnpm build-storybook` — the only check that every story compiles
5. **Build:** `pnpm build` — `next build`

No formatter runs on `apps/web`. None is configured, and preflight does not introduce one.

Accessibility is not gated here. `@storybook/addon-a11y` reports inside the Storybook UI only; headless assertions need `@storybook/addon-vitest`, which needs the `@storybook/nextjs-vite` migration that `web-data-layer` placed out of scope. Until that lands, a11y is a review-gate concern.

## Batch-enrich pin gate

Both `.claude/skills/batch-enrich/SKILL.md` and
`.agents/skills/batch-enrich/SKILL.md` pin `PROMPT_VERSION` in a
`classification-pins` block. That pin is the only thing that lets a later
query separate classification cohorts by prompt — it sat unchanged for
four months in 2026 while five materially different prompts shipped under
it, and every `provenance.prompt_version` written in that window is now
unrecoverable as separate cohorts. Preflight fails loudly here so an eighth
silent drift doesn't happen the same way.

Run each check against whichever of the two files the change touched.

1. **Pin moved with the change:**
   ```bash
   for f in .claude/skills/batch-enrich/SKILL.md .agents/skills/batch-enrich/SKILL.md; do
     git diff HEAD --quiet -- "$f" && continue
     git diff HEAD --unified=0 -- "$f" \
       | grep -q '^[+-]PROMPT_VERSION=' && echo "PIN_CHANGED $f" || echo "PIN_UNCHANGED $f"
   done
   ```
   Any change to a file with `PIN_UNCHANGED` is a fail — the check has
   no way to tell a classification-affecting edit from a purely cosmetic
   one, so it treats every unbumped change as suspect and leaves the
   judgment call to whoever committed it.

2. **Pins agree across the two skills:**
   ```bash
   grep -h '^PROMPT_VERSION=' .claude/skills/batch-enrich/SKILL.md \
     .agents/skills/batch-enrich/SKILL.md | sort -u | wc -l
   ```
   Anything but `1` is a fail. The two skills run one classifier contract
   under different harnesses, so they carry one pin value and bump together.

3. **No restated version literal:**
   ```bash
   grep -n 'batch-enrich-v[0-9]' .claude/skills/batch-enrich/SKILL.md \
     .agents/skills/batch-enrich/SKILL.md | grep -v '^[^:]*:[0-9]*:PROMPT_VERSION='
   ```
   Hits outside the pin line are suspect. A version restated in an example,
   a SQL snippet, or a rule is how the Codex skill's workers wrote
   `batch-enrich-v6` under a `batch-enrich-v7` pin. Prose that names a past
   version as history is fine; anything a worker would copy into a
   `save_enrichment` call is not. Report the hits and let the committer judge.

## Reporting

Report each check as ✓ pass or ✗ fail, grouped by surface. For failures, include the relevant output.

```
Preflight results — surfaces: tools, web, batch-enrich

  tools
    ✓ gofmt
    ✓ go build + go vet
    ✗ staticcheck — 1 issue (see below)
    ✓ go test
    ✓ sqlc diff

  web
    ✓ pnpm typecheck
    ✓ pnpm test
    – pnpm test:db — skipped, DATABASE_URL_TEST_RO unset
    ✓ pnpm build-storybook
    ✓ pnpm build

  batch-enrich
    ✗ PROMPT_VERSION pin — .agents SKILL.md changed, pin unchanged
    ✓ pins agree across skills
    ✓ no restated version literal
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

**Batch-enrich**

- **PROMPT_VERSION pin, pin agreement, restated literals:** never auto-fix. Bumping a pin is a judgment call about whether the change is classification-affecting — report and let the user decide the next version string.

After auto-fixing, re-run the fixed checks to confirm they pass.
