---
name: review-implementability
description: Review a plan through an implementer's task-by-task lens. Use after structural draft review and before promoting a plan to ready, or to check a ready plan before orchestration.
---

# Review Implementability

Check whether a task agent, given only its task, the Goal, plan-level acceptance criteria, relevant context slices, and source access, can build the right thing.

## Process

1. Find the plan in `drafts/` or `ready/`. Read it in full.
2. Delegate one read-only review pass. Inline the full plan, locked decisions, relevant context slices, and key source files.
3. Require two exhaustive checks:
   - Per task: required files, call sites, phase outputs, seams, and load-bearing details are visible to a task-only reader.
   - Per acceptance criterion: it is achievable in the current codebase, observable, and neither over- nor under-specified.
4. Require source checks for dependency seams, sqlc limitations, integration build tags, NULL-versus-absent boundaries, and negative claims that need a review gate rather than a test.
5. Apply determinate corrections. Surface choices that change scope or a locked decision.

## Report

For each task: `sets up success` or `needs tightening`, with findings. For each acceptance criterion: `achievable and sound` or `problem`. Finish with blocker count, disposition, and a recommendation: orchestrate, tighten, or escalate.

## Rules

- Run this only after `review-draft-spec` resolves structural issues.
- The reviewer reports; the coordinator owns edits.
- Keep findings at `{location, problem, fix, severity}`.
- Do not pad or praise.
