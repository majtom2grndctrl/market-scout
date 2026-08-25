---
name: one-pager
description: Write a lightweight frontend build brief for direct, human-steered work in `apps/web/`. Use for Next.js, React, Tailwind, or Storybook changes that do not need parallel plan execution.
---

# One Pager

Write a frontend brief at `agent-context/plans/in-progress/<feature-name>/index.md`.

Use this light lane only when the work touches no schema, agent-facing contract, or data-write path. Use `draft-plan` when it needs agent coordination or full acceptance criteria.

## Process

1. Read `agent-context/lib/style-guide.md`, `agent-context/lib/index.md`, and the affected web files.
2. Resolve only questions whose answers would produce different work.
3. Verify library or framework behavior from the locked version, local source, or current primary documentation. Do not use memory for version-sensitive APIs.
4. Write the brief, show it to the user, and incorporate corrections before implementation.
5. Commit the brief only when asked. Capture durable architecture decisions in `agent-context/lib/` in the same change.

```markdown
# <Feature Name>

> Brief — decisions and non-goals. No task breakdown or acceptance criteria.

## Goal

## Target usage

## Decisions

## Not doing

## Build order

## Done when

## Open questions
```

Omit empty optional sections. Keep the marker line.

## Rules

- Target usage shows distinct API properties. Annotate the claim each example makes.
- Decisions and non-goals use one line each, with the reason attached.
- Put research that needs a paragraph in sibling `research.md`.
- Put what the user can see by looking at the screen on the screen, not in acceptance criteria.
- Move the brief directly from `in-progress/` to `done/` when work ships. Do not maintain it after that.
