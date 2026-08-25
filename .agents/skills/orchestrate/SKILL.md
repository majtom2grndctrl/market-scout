---
name: orchestrate
description: Execute a reviewed ready plan in phases by delegating its tasks, integrating evidence, and running the project review gates. Use when a plan in `agent-context/plans/ready/` is ready to implement.
---

# Orchestrate

Coordinate a plan. Keep implementation inside the task delegates.

## Load

1. Read `agent-context/plans/ready/<plan-name>/index.md`. If missing, list ready plans and ask which to run.
2. Identify the Goal, tasks, acceptance criteria, sequencing, and surface.
3. Read `agent-context/lib/index.md`. Load only the guides for the surface: tools, web, or both.
4. Move the plan to `in-progress/` with `git mv`. Commit the move.

## Execute

Run phases in order. Delegate independent tasks concurrently; wait for prerequisite tasks before starting dependents.

Each task packet contains:

1. Goal.
2. Task description and plan-level acceptance criteria.
3. Surface and relevant context slices, inlined when they carry task-specific constraints.
4. Source-grounding rule: confirm every identifier claim from source opened this session.
5. `implement-task` sections covering dependency inspection, implementation, verification, and reporting.

Do not send unrelated tasks or the full plan. Do not give a delegate permission to expand scope.

## Integrate

After each phase, read the delegates' acceptance-criterion status, verification output, deviations, and changed files. Confirm the next phase's prerequisites. Surface contract or scope deviations to the user.

When all phases finish:

1. Run `preflight`.
2. Run `review-panel` on the session changes.
3. Discuss findings. Run `fix-findings` only for findings the user accepts.
4. Explain the important implementation choices the feature made.

When the user asks to land the plan, rerun preflight after post-gate edits, move the plan to `done/`, clean up worktrees, then commit and push as authorized.

## Rules

- Keep each task packet minimal and sufficient.
- A failed task is a user-visible event. Present retry, skip, or abort options.
- Resolve only mechanical merge conflicts. Surface architectural conflicts.
